package v1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// +kubebuilder:validation:Enum=Small;Large
type Size string

type WidgetSpec struct {
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:default=1
	// +optional
	Replicas *int32 `json:"replicas,omitempty"`
	// +required
	Size Size `json:"size"`
}

type WidgetStatus struct {
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:ac:generate=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Size",type=string,JSONPath=`.spec.size`
type Widget struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitzero"`
	Spec              WidgetSpec   `json:"spec"`
	Status            WidgetStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true
type WidgetList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []Widget `json:"items"`
}

func init() { SchemeBuilder.Register(&Widget{}, &WidgetList{}) }
