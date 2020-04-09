package queue

// QueuingService is the interface for the message queueing service
// For example: SQS and Beanstalk implements QueuingService interface
// poll functions polls the queue service provider is responsible to update
// the queueSpec with the polled information
// informations it updates are
// 1. updateMessageSent(key, messagesSentPerMinute) i.e messagesSentPerMinute
// 2. updateIdleWorkers(key, -1) i.e tells how many workers are idle
// 3. updateMessage(key, approxMessagesVisible) i.e queuedMessages
type QueuingService interface {
	poll(key string, queueSpec QueueSpec)
}
