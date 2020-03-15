package queue

// QueuingService is the interface for the message queueing service
// For example: SQS and Beanstalk implements QueuingService interface

const (
	SqsQueueService       = "sqs"
	BeanstalkQueueService = "beanstalkd"
)

type QueuingService interface {
	Sync(stopCh <-chan struct{})
	poll(key string, queueSpec QueueSpec)
}
