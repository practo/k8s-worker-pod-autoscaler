package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// WorkerPodAutoScaler is a specification for a WorkerPodAutoScaler resource
type WorkerPodAutoScaler struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   WorkerPodAutoScalerSpec   `json:"spec"`
	Status WorkerPodAutoScalerStatus `json:"status"`
}

// WorkerPodAutoScalerSpec is the spec for a WorkerPodAutoScaler resource
type WorkerPodAutoScalerSpec struct {
	MinReplicas             *int32   `json:"minReplicas"`
	MaxReplicas             *int32   `json:"maxReplicas"`
	MaxDisruption           *string  `json:"maxDisruption"`
	QueueURI                string   `json:"queueURI"`
	DeploymentName          string   `json:"deploymentName"`
	TargetMessagesPerWorker *int32   `json:"targetMessagesPerWorker"`
	SecondsToProcessOneJob  *float64 `json:"secondsToProcessOneJob"`
}

// WorkerPodAutoScalerStatus is the status for a WorkerPodAutoScaler resource
type WorkerPodAutoScalerStatus struct {
	CurrentMessages int32 `json:"CurrentMessages"`
	CurrentReplicas int32 `json:"CurrentReplicas"`
	DesiredReplicas int32 `json:"DesiredReplicas"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// WorkerPodAutoScalerList is a list of WorkerPodAutoScaler resources
type WorkerPodAutoScalerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []WorkerPodAutoScaler `json:"items"`
}
