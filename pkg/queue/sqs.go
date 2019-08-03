package queue

import (
	"fmt"
	"strconv"
	"time"

	"k8s.io/klog"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/sqs"
)

const (
	ShortPollSleepSeconds = 30 * time.Second
)

type SQSPoller struct {
	client          *sqs.SQS
	queues          *Queues
	polling         map[string]bool
	listPollingCh   chan map[string]bool
	updatePollingCh chan map[string]bool
}

func NewSQSPoller(awsRegion string, queues *Queues) (Poller, error) {
	sess, err := session.NewSession(&aws.Config{
		Region: aws.String(awsRegion)},
	)
	if err != nil {
		return nil, err
	}
	return &SQSPoller{
		client:          sqs.New(sess),
		queues:          queues,
		polling:         make(map[string]bool),
		listPollingCh:   make(chan map[string]bool),
		updatePollingCh: make(chan map[string]bool),
	}, nil
}

func (s *SQSPoller) enablePoll(key string) {
	s.updatePollingCh <- map[string]bool{
		key: true,
	}
}

func (s *SQSPoller) disablePoll(key string) {
	s.updatePollingCh <- map[string]bool{
		key: false,
	}
}

func (s *SQSPoller) isPolling(key string) bool {
	polling := <-s.listPollingCh
	if _, ok := polling[key]; !ok {
		return false
	}
	return polling[key]
}

func (s *SQSPoller) lister() {
	for {
		s.listPollingCh <- s.polling
	}
}

func (s *SQSPoller) updater() {
	for {
		select {
		case status := <-s.updatePollingCh:
			for key, value := range status {
				if _, ok := s.polling[key]; !ok {
					continue
				}
				s.polling[key] = value
			}
		}
	}
}

func (s *SQSPoller) longPollReceiveMessage(queueURI string) (int32, error) {
	result, err := s.client.ReceiveMessage(&sqs.ReceiveMessageInput{
		QueueUrl: aws.String(queueURI),
		AttributeNames: aws.StringSlice([]string{
			"SentTimestamp",
		}),
		VisibilityTimeout:   aws.Int64(0),
		MaxNumberOfMessages: aws.Int64(10),
		MessageAttributeNames: aws.StringSlice([]string{
			"All",
		}),
		WaitTimeSeconds: aws.Int64(20),
	})

	if err != nil {
		return 0, err
	}

	return int32(len(result.Messages)), nil
}

func (s *SQSPoller) getApproxMessagesVisibleInQueue(queueURI string) (int32, error) {
	result, err := s.client.GetQueueAttributes(&sqs.GetQueueAttributesInput{
		QueueUrl:       &queueURI,
		AttributeNames: []*string{aws.String("ApproximateNumberOfMessages")},
	})
	if err != nil {
		return 0, err
	}
	messages, ok := result.Attributes["ApproximateNumberOfMessages"]
	if !ok {
		return 0, fmt.Errorf("ApproximateNumberOfMessages not found: %+v",
			result.Attributes)
	}
	i64, err := strconv.ParseInt(*messages, 10, 32)
	if err != nil {
		return 0, err
	}
	return int32(i64), nil
}

func (s *SQSPoller) poll(key string, queueSpec *QueueSpec) {
	if s.isPolling(key) {
		return
	}
	s.disablePoll(key)

	if queueSpec.consumers == 0 {
		// If there are no consumers running we do a long poll to find a job(s)
		// in the queue. On finding job(s) we increment the queue message
		// by no of messages received to trigger scale up.
		// Long polling is done to keep SQS api calls to minimum.
		messagesReceived, err := s.longPollReceiveMessage(queueSpec.uri)
		if err != nil {
			klog.Errorf("Unable to receive message from queue %q, %v.",
				queueSpec.name, err)
			s.enablePoll(key)
			return
		}

		if messagesReceived > 0 {
			s.queues.updateMessage(key, messagesReceived)
		}

		s.enablePoll(key)
		return
	}

	// If the number of consumers are not zero then we should make the target scaling
	// happen based on what exactly the queue length is.
	// For this we will need ApproximateMessageVisible
	approxMessagesVisible, err := s.getApproxMessagesVisibleInQueue(queueSpec.uri)
	if err != nil {
		klog.Errorf("Unable to get approximate messages visible in queue %q, %v.",
			queueSpec.name, err)
		s.enablePoll(key)
		return
	}

	if approxMessagesVisible == 0 {
		// TODO: add NumberOfEmptyReceive
		time.Sleep(ShortPollSleepSeconds)
		s.enablePoll(key)
		return
	}

	s.queues.updateMessage(key, approxMessagesVisible)
	time.Sleep(ShortPollSleepSeconds)
	s.enablePoll(key)
}

func (s *SQSPoller) Run() {
	go s.lister()
	go s.updater()

	for {
		queues := s.queues.List()
		for key, queueSpec := range queues {
			go s.poll(key, queueSpec)
		}
		// TODO: use tickers
		time.Sleep(time.Second * 2)
	}
}
