package queue

import (
	_ "github.com/golang/mock/gomock"
	// "testing"
)

const (
	queueNameOne = "q1"
)

func newTestQueues() *Queues {
	q := NewQueues()

	q.Add(
		"namespace1",
		"queuename1",
		"beanstalk://beanstalkd.namespace1:11300/"+queueNameOne,
		1,
		0.0,
	)
	return q
}
