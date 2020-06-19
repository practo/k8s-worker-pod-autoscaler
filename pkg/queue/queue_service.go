package queue

import (
	"regexp"
)

// QueuingService is the interface for the message queueing service
// For example: SQS and Beanstalk implements QueuingService interface

const (
	SqsQueueService       = "sqs"
	BeanstalkQueueService = "beanstalkd"
)

type QueuingService interface {
	// GetName returns the name of the queing service
	GetName() string
	Sync(stopCh <-chan struct{})
	// poll functions polls the queue service provider is responsible to update
	// the queueSpec with the polled information
	// informations it updates are
	//1. updateMessageSent(key, messagesSentPerMinute) i.e messagesSentPerMinute
	//2. updateIdleWorkers(key, -1) i.e tells how many workers are idle
	//3. updateMessage(key, approxMessagesVisible) i.e queuedMessages
	poll(key string, queueSpec QueueSpec)
}

// getQueueService returns the provider name
// TODO: add validation for the queue service in the wpa custom resource
func getQueueServiceName(host, protocol string) (bool, string, error) {
	matched, err := regexp.MatchString(
		"^sqs.[a-z][a-z]-[a-z]*-[0-9]{1}.amazonaws.com", host)
	if err != nil {
		return false, "", nil
	}

	if matched {
		return true, SqsQueueService, nil
	}

	if protocol == BenanstalkProtocol {
		return true, BeanstalkQueueService, nil
	}

	return false, "", nil
}
