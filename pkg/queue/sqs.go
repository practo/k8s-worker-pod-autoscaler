package queue

import (
	"fmt"
	"path"
	"strconv"
	"time"

	"k8s.io/klog"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/cloudwatch"
	"github.com/aws/aws-sdk-go/service/sqs"
)

const (
	LongPollInterval  = 20
	ShortPollInterval = 500 * time.Millisecond
)

type SQSPoller struct {
	sqsClient       *sqs.SQS
	cwClient        *cloudwatch.CloudWatch
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
		sqsClient:       sqs.New(sess),
		cwClient:        cloudwatch.New(sess),
		queues:          queues,
		polling:         make(map[string]bool),
		listPollingCh:   make(chan map[string]bool),
		updatePollingCh: make(chan map[string]bool),
	}, nil
}

func (s *SQSPoller) setPolling(key string) {
	s.updatePollingCh <- map[string]bool{
		key: true,
	}
}

func (s *SQSPoller) unsetPolling(key string) {
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
				s.polling[key] = value
			}
		}
	}
}

func (s *SQSPoller) longPollReceiveMessage(queueURI string) (int32, error) {
	result, err := s.sqsClient.ReceiveMessage(&sqs.ReceiveMessageInput{
		QueueUrl: aws.String(queueURI),
		AttributeNames: aws.StringSlice([]string{
			"SentTimestamp",
		}),
		VisibilityTimeout:   aws.Int64(0),
		MaxNumberOfMessages: aws.Int64(10),
		MessageAttributeNames: aws.StringSlice([]string{
			"All",
		}),
		WaitTimeSeconds: aws.Int64(LongPollInterval),
	})

	if err != nil {
		return 0, err
	}

	return int32(len(result.Messages)), nil
}

func (s *SQSPoller) getApproxMessagesVisibleInQueue(queueURI string) (int32, error) {
	result, err := s.sqsClient.GetQueueAttributes(&sqs.GetQueueAttributesInput{
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

func (s *SQSPoller) numberOfEmptyReceives(queueURI string) (float64, error) {
	period := int64(60)
	endTime := time.Now()
	duration, err := time.ParseDuration("-5m")
	if err != nil {
		return 0.0, err
	}
	startTime := endTime.Add(duration)

	query := &cloudwatch.MetricDataQuery{
		Id: aws.String("id1"),
		MetricStat: &cloudwatch.MetricStat{
			Metric: &cloudwatch.Metric{
				Namespace:  aws.String("AWS/SQS"),
				MetricName: aws.String("NumberOfEmptyReceives"),
				Dimensions: []*cloudwatch.Dimension{
					&cloudwatch.Dimension{
						Name:  aws.String("QueueName"),
						Value: aws.String(path.Base(queueURI)),
					},
				},
			},
			Period: &period,
			Stat:   aws.String("Average"),
		},
	}

	result, err := s.cwClient.GetMetricData(&cloudwatch.GetMetricDataInput{
		EndTime:           &endTime,
		StartTime:         &startTime,
		MetricDataQueries: []*cloudwatch.MetricDataQuery{query},
	})
	if err != nil {
		return 0.0, err
	}

	if len(result.MetricDataResults) > 1 {
		return 0.0, fmt.Errorf("Expecting cloudwatch metric to return single data point")
	}

	klog.Infof("%#v", result.MetricDataResults[0])
	if result.MetricDataResults[0].Values != nil && len(result.MetricDataResults[0].Values) > 0 {
		return *result.MetricDataResults[0].Values[0], nil
	}
	klog.Errorf("NumberOfEmptyReceives API returned empty result for uri: %q", queueURI)
	return 0.0, nil
}

func (s *SQSPoller) poll(key string, queueSpec *QueueSpec) {
	if s.isPolling(key) {
		return
	}
	s.setPolling(key)
	if s.isPolling(key) {
		return
	}
	defer s.unsetPolling(key)

	if queueSpec.workers == 0 {
		// If there are no workers running we do a long poll to find a job(s)
		// in the queue. On finding job(s) we increment the queue message
		// by no of messages received to trigger scale up.
		// Long polling is done to keep SQS api calls to minimum.
		messagesReceived, err := s.longPollReceiveMessage(queueSpec.uri)
		if err != nil {
			klog.Errorf("Unable to receive message from queue %q, %v.",
				queueSpec.name, err)
			return
		}

		if messagesReceived > 0 {
			s.queues.updateMessage(key, messagesReceived)
		}
		return
	}

	// If the number of workers are not zero then we should make the target scaling
	// happen based on what exactly the queue length is. The desired number of
	// consmers should be calculated based on how high the value of queue length is.
	// For this we will need ApproximateMessageVisible
	approxMessagesVisible, err := s.getApproxMessagesVisibleInQueue(queueSpec.uri)
	if err != nil {
		klog.Errorf("Unable to get approximate messages visible in queue %q, %v.",
			queueSpec.name, err)
		return
	}

	s.queues.updateMessage(key, approxMessagesVisible)
	if approxMessagesVisible != 0 {
		time.Sleep(ShortPollInterval)
		return
	}

	// TODO: make this api call to execute only after 5minutes for every queue
	// as it gets updated after only 5minutes (keep a cache)
	emptyReceives, err := s.numberOfEmptyReceives(queueSpec.uri)
	klog.Infof("numberOfEmptyReceives", emptyReceives)
	if err != nil {
		klog.Errorf("Unable to fetch empty revieve metric for queue %q, %v.",
			queueSpec.name, err)
		return
	}

	idleWorkers := int32(emptyReceives * float64(queueSpec.workers))
	s.queues.updateIdleWorkers(key, idleWorkers)

	time.Sleep(ShortPollInterval)
	return
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
