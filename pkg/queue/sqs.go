package queue

import (
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/sqs"
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
	s.updatePollingCh <- map[string]bool{key: true}
}

func (s *SQSPoller) disablePoll(key string) {
	s.updatePollingCh <- map[string]bool{key: false}
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

func (s *SQSPoller) poll(key string, queueSpec *QueueSpec) {
	if s.isPolling(key) {
		return
	}
	s.disablePoll(key)

	if queueSpec.consumers == 0 {
		// do a long poll and on receiving the message increment the messages by 1
		// this should trigger a single scale up eventually
		// and return with delay
	}

	// get the queue length

	// if message > 0 then set the message length and return with delay

	// if message == 0 and NumberOfEmptyReceive > some value then set idleConsumers true return with delay

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
		time.Sleep(time.Second * 2)
	}
}
