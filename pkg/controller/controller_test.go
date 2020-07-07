package controller_test

import (
	"testing"

	"github.com/practo/k8s-worker-pod-autoscaler/pkg/controller"
)

// TestScaleDownWhenQueueMessagesLessThanTarget tests scale down
//  when unprocessed messages is less than targetMessagesPerWorker #89
func TestScaleDownWhenQueueMessagesLessThanTarget(t *testing.T) {
	queueName := "otpsms"
	queueMessages := int32(10)
	messagesSentPerMinute := float64(10)
	secondsToProcessOneJob := float64(0.3)
	targetMessagesPerWorker := int32(200)
	currentWorkers := int32(20)
	idleWorkers := int32(0)
	minWorkers := int32(0)
	maxWorkers := int32(20)
	maxDisruption := "10%"
	expectedDesired := int32(18)

	desiredWorkers := controller.GetDesiredWorkers(
		queueName,
		queueMessages,
		messagesSentPerMinute,
		secondsToProcessOneJob,
		targetMessagesPerWorker,
		currentWorkers,
		idleWorkers,
		minWorkers,
		maxWorkers,
		&maxDisruption,
	)

	if desiredWorkers != expectedDesired {
		t.Errorf("expected-desired=%v, got-desired=%v\n", desiredWorkers,
			expectedDesired)
	}
}
