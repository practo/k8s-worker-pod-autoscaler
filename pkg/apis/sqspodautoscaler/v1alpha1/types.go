package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// SqsPodAutoScaler is a specification for a SqsPodAutoScaler resource
type SqsPodAutoScaler struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   SqsPodAutoScalerSpec   `json:"spec"`
	Status SqsPodAutoScalerStatus `json:"status"`
}

// SqsPodAutoScalerSpec is the spec for a SqsPodAutoScaler resource
type SqsPodAutoScalerSpec struct {
	DeploymentName string `json:"deploymentName"`
	Replicas       *int32 `json:"replicas"`
}

// SqsPodAutoScalerStatus is the status for a SqsPodAutoScaler resource
type SqsPodAutoScalerStatus struct {
	AvailableReplicas int32 `json:"availableReplicas"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// SqsPodAutoScalerList is a list of SqsPodAutoScaler resources
type SqsPodAutoScalerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []SqsPodAutoScaler `json:"items"`
}
