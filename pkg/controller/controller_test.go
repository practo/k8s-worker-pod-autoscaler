package controller_test

import (
	"testing"
	"time"

	"github.com/practo/k8s-worker-pod-autoscaler/pkg/controller"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// TestScaleDownWhenQueueMessagesLessThanTarget tests scale down
//  when unprocessed messages is less than targetMessagesPerWorker #89
func TestScaleDownWhenQueueMessagesLessThanTarget(t *testing.T) {
	queueName := "q"
	queueMessages := int32(10)
	messagesSentPerMinute := float64(10)
	secondsToProcessOneJob := float64(0.3)
	targetMessagesPerWorker := int32(200)
	currentWorkers := int32(20)
	idleWorkers := int32(0)
	minWorkers := int32(0)
	maxWorkers := int32(20)
	maxDisruption := "10%"
	readinessDelaySeconds := int32(0)
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
		readinessDelaySeconds,
		nil,
	)

	if desiredWorkers != expectedDesired {
		t.Errorf("expected-desired=%v, got-desired=%v\n", expectedDesired,
			desiredWorkers)
	}
}

// TestScaleUpWhenCalculatedMinIsGreaterThanMax
// when calculated min is greater than max
func TestScaleUpWhenCalculatedMinIsGreaterThanMax(t *testing.T) {
	queueName := "q"
	queueMessages := int32(1)
	messagesSentPerMinute := float64(2136.6)
	secondsToProcessOneJob := float64(10)
	targetMessagesPerWorker := int32(2500)
	currentWorkers := int32(10)
	idleWorkers := int32(0)
	minWorkers := int32(2)
	maxWorkers := int32(20)
	maxDisruption := "0%"
	readinessDelaySeconds := int32(0)
	expectedDesired := int32(20)

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
		readinessDelaySeconds,
		nil,
	)

	if desiredWorkers != expectedDesired {
		t.Errorf("expected-desired=%v, got-desired=%v\n", expectedDesired,
			desiredWorkers)
	}
}

// TestScaleUpPreventionWhenReadinessDelayHasNotExpired
// scale up should be prevented if readiness delay has not expired
func TestScaleUpPreventionWhenReadinessDelayHasNotExpired(t *testing.T) {
	queueName := "q"
	queueMessages := int32(1)
	messagesSentPerMinute := float64(2136)
	secondsToProcessOneJob := float64(10)
	targetMessagesPerWorker := int32(2500)
	currentWorkers := int32(10)
	idleWorkers := int32(0)
	minWorkers := int32(2)
	maxWorkers := int32(20)
	maxDisruption := "0%"
	readinessDelaySeconds := int32(600)
	lastScaleUpTime := &metav1.Time{
		Time: time.Now().Add(time.Second * time.Duration(-300)),
	}
	expectedDesired := int32(10)

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
		readinessDelaySeconds,
		lastScaleUpTime,
	)

	if desiredWorkers != expectedDesired {
		t.Errorf("expected-desired=%v, got-desired=%v\n", expectedDesired,
			desiredWorkers)
	}
}

// TestScaleUpWhenReadinessDelayHasExpired
// scale up should be prevented if readiness delay has expired
func TestScaleUpWhenReadinessDelayHasExpired(t *testing.T) {
	queueName := "q"
	queueMessages := int32(1)
	messagesSentPerMinute := float64(2136)
	secondsToProcessOneJob := float64(10)
	targetMessagesPerWorker := int32(2500)
	currentWorkers := int32(10)
	idleWorkers := int32(0)
	minWorkers := int32(2)
	maxWorkers := int32(20)
	maxDisruption := "0%"
	readinessDelaySeconds := int32(250)
	lastScaleUpTime := &metav1.Time{
		Time: time.Now().Add(time.Second * time.Duration(-300)),
	}
	expectedDesired := int32(20)

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
		readinessDelaySeconds,
		lastScaleUpTime,
	)

	if desiredWorkers != expectedDesired {
		t.Errorf("expected-desired=%v, got-desired=%v\n", expectedDesired,
			desiredWorkers)
	}
}
