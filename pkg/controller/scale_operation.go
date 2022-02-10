package controller

import (
	"time"

	"github.com/practo/klog/v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type ScaleOperation int

const (
	ScaleUp ScaleOperation = iota
	ScaleDown
	ScaleNoop
)

func GetScaleOperation(
	q string,
	desiredWorkers int32,
	currentWorkers int32,
	lastScaleTime *metav1.Time,
	scaleDownDelay time.Duration) ScaleOperation {

	if desiredWorkers == currentWorkers {
		return ScaleNoop
	}

	if desiredWorkers > currentWorkers {
		return ScaleUp
	}

	if canScaleDown(
		q, desiredWorkers, currentWorkers, lastScaleTime, scaleDownDelay) {
		return ScaleDown
	}

	return ScaleNoop
}

// canScaleDown checks the scaleDownDelay and the lastScaleTime to decide
// if scaling is required. Checks coolOff!
func canScaleDown(
	q string,
	desiredWorkers int32,
	currentWorkers int32,
	lastScaleTime *metav1.Time, scaleDownDelay time.Duration) bool {

	if lastScaleTime == nil {
		klog.V(2).Infof("%s scaleDownDelay ignored, lastScaleTime is nil", q)
		return true
	}

	nextScaleDownTime := metav1.NewTime(
		lastScaleTime.Time.Add(scaleDownDelay),
	)
	now := metav1.Now()

	if nextScaleDownTime.Before(&now) {
		klog.V(2).Infof("%s scaleDown is allowed, cooloff passed", q)
		return true
	}

	klog.V(2).Infof(
		"%s scaleDown forbidden, nextScaleDownTime: %v",
		q,
		nextScaleDownTime,
	)

	return false
}

func scaleOpString(op ScaleOperation) string {
	switch op {
	case ScaleUp:
		return "scale-up"
	case ScaleDown:
		return "scale-down"
	case ScaleNoop:
		return "no scaling operation"
	default:
		return ""
	}
}
