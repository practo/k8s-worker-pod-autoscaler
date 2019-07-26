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
	DeploymentName string `json:"deploymentName"`
	Replicas       *int32 `json:"replicas"`
}

// WorkerPodAutoScalerStatus is the status for a WorkerPodAutoScaler resource
type WorkerPodAutoScalerStatus struct {
	AvailableReplicas int32 `json:"availableReplicas"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// WorkerPodAutoScalerList is a list of WorkerPodAutoScaler resources
type WorkerPodAutoScalerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []WorkerPodAutoScaler `json:"items"`
}
