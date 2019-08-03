package queue

type Poller interface {
	Run()
	lister()
	updater()
	enablePoll(key string)
	disablePoll(key string)
	isPolling(key string) bool
	poll(key string, queueSpec *QueueSpec)
}
