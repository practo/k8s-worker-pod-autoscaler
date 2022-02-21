package controller_test

import (
	"testing"
	"time"

	"github.com/practo/k8s-worker-pod-autoscaler/pkg/controller"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func timeBeforeSeconds(before int) *metav1.Time {
	t := metav1.NewTime(time.Now().Add(
		-time.Second * time.Duration(before)),
	)

	return &t
}

type opTestCase struct {
	desired           int32
	current           int32
	scaleDownDelay    time.Duration
	lastScaleTime     *metav1.Time
	expectedOperation controller.ScaleOperation
}

func TestScaleOperation(t *testing.T) {
	var opTestCases = []opTestCase{
		{
			current:           10,
			desired:           5,
			scaleDownDelay:    time.Second * time.Duration(5),
			lastScaleTime:     timeBeforeSeconds(10),
			expectedOperation: controller.ScaleDown,
		},
		{
			current:           10,
			desired:           5,
			scaleDownDelay:    time.Second * time.Duration(5),
			lastScaleTime:     timeBeforeSeconds(1),
			expectedOperation: controller.ScaleNoop,
		},
		{
			current:           10,
			desired:           5,
			scaleDownDelay:    time.Second * time.Duration(5),
			lastScaleTime:     nil,
			expectedOperation: controller.ScaleDown,
		},
		{
			current:           10,
			desired:           15,
			scaleDownDelay:    time.Second * time.Duration(5),
			lastScaleTime:     timeBeforeSeconds(1),
			expectedOperation: controller.ScaleUp,
		},
		{
			current:           10,
			desired:           10,
			scaleDownDelay:    time.Second * time.Duration(5),
			lastScaleTime:     timeBeforeSeconds(1),
			expectedOperation: controller.ScaleNoop,
		},
	}

	for _, optc := range opTestCases {
		tc := optc
		op := controller.GetScaleOperation(
			"q",
			tc.desired,
			tc.current,
			tc.lastScaleTime,
			tc.scaleDownDelay,
		)
		if op != tc.expectedOperation {
			t.Errorf("expected op=%v, got=%v", tc.expectedOperation, op)
		}
	}
}
