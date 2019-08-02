package queue

type Poller interface {
	GetJobs(queueName string) int32
	GetEmptyReceives(queueName string) int32
	Run()
}
