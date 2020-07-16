package controller_test

import (
	"testing"

	"github.com/practo/k8s-worker-pod-autoscaler/pkg/controller"
)

type desiredWorkerTester struct {
	queueName               string
	queueMessages           int32
	messagesSentPerMinute   float64
	secondsToProcessOneJob  float64
	targetMessagesPerWorker int32
	currentWorkers          int32
	idleWorkers             int32
	minWorkers              int32
	maxWorkers              int32
	maxDisruption           string
}

func (c *desiredWorkerTester) getDesired() int32 {
	return controller.GetDesiredWorkers(
		c.queueName,
		c.queueMessages,
		c.messagesSentPerMinute,
		c.secondsToProcessOneJob,
		c.targetMessagesPerWorker,
		c.currentWorkers,
		c.idleWorkers,
		c.minWorkers,
		c.maxWorkers,
		&c.maxDisruption,
	)
}

func (c *desiredWorkerTester) test(t *testing.T, expected int32) {
	desired := c.getDesired()
	if desired != expected {
		t.Errorf("desired=%v, expected=%v\n", desired, expected)
	}
}

// TestScaleDownWhenQueueMessagesLessThanTarget tests scale down
//  when unprocessed messages is less than targetMessagesPerWorker #89
func TestScaleDownWhenQueueMessagesLessThanTarget(t *testing.T) {
	c := desiredWorkerTester{
		queueName:               "q",
		queueMessages:           10,
		messagesSentPerMinute:   float64(10),
		secondsToProcessOneJob:  float64(0.3),
		targetMessagesPerWorker: 200,
		currentWorkers:          20,
		idleWorkers:             0,
		minWorkers:              0,
		maxWorkers:              20,
		maxDisruption:           "10%",
	}

	c.test(t, 18)
}

// TestScaleUpWhenCalculatedMinIsGreaterThanMax
// when calculated min is greater than max
func TestScaleUpWhenCalculatedMinIsGreaterThanMax(t *testing.T) {
	c := desiredWorkerTester{
		queueName:               "q",
		queueMessages:           1,
		messagesSentPerMinute:   float64(2136),
		secondsToProcessOneJob:  float64(10),
		targetMessagesPerWorker: 2500,
		currentWorkers:          10,
		idleWorkers:             0,
		minWorkers:              2,
		maxWorkers:              20,
		maxDisruption:           "0%",
	}

	c.test(t, 20)
}
