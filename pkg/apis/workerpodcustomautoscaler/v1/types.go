package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// WorkerPodCustomAutoScaler is a specification for a WorkerPodCustomAutoScaler resource
type WorkerPodCustomAutoScaler struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   WorkerPodCustomAutoScalerSpec   `json:"spec"`
	Status WorkerPodCustomAutoScalerStatus `json:"status"`
}

// WorkerPodCustomAutoScalerSpec is the spec for a WorkerPodCustomAutoScaler resource
type WorkerPodCustomAutoScalerSpec struct {
	MinReplicas    *int32  `json:"minReplicas"`
	MaxReplicas    *int32  `json:"maxReplicas"`
	MaxDisruption  *string `json:"maxDisruption,omitempty"`
	DeploymentName string  `json:"deploymentName,omitempty"`
	ReplicaSetName string  `json:"replicaSetName,omitempty"`
	Queues         []Queue `json:"queues,omitempty"`
}

type Queue struct {
	URI                     string  `json:"uri"`
	TargetMessagesPerWorker int32   `json:"targetMessagesPerWorker"`
	SecondsToProcessOneJob  float64 `json:"secondsToProcessOneJob,omitempty"`
}

// WorkerPodCustomAutoScalerStatus is the status for a WorkerPodCustomAutoScaler resource
type WorkerPodCustomAutoScalerStatus struct {
	CurrentMessages   int32 `json:"CurrentMessages"`
	CurrentReplicas   int32 `json:"CurrentReplicas"`
	AvailableReplicas int32 `json:"AvailableReplicas"`
	DesiredReplicas   int32 `json:"DesiredReplicas"`

	// LastScaleTime is the last time the WorkerPodAutoscaler scaled the workers
	// It is used by the autoscaler to control
	// how often the number of pods is changed.
	// +optional
	LastScaleTime *metav1.Time `json:"LastScaleTime,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// WorkerPodCustomAutoScalerList is a list of WorkerPodCustomAutoScaler resources
type WorkerPodCustomAutoScalerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []WorkerPodCustomAutoScaler `json:"items"`
}
