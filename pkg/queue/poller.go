package queue

type Poller interface {
	Run()
	lister()
	updater()
	setPolling(key string)
	unsetPolling(key string)
	isPolling(key string) bool
	poll(key string, queueSpec *QueueSpec)
}
