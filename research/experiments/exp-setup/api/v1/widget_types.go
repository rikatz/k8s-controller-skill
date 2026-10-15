package v1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// Size is the widget size.
// +kubebuilder:validation:Enum=Small;Large
type Size string

// WidgetSpec is the desired state of a Widget.
type WidgetSpec struct {
	// Deviation: pointer kept because 0 and unset mean different things here.

	// replicas is the desired count.
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:default=1
	// +optional
	Replicas *int32 `json:"replicas,omitempty"`

	// size is the widget size.
	// +required
	Size Size `json:"size,omitempty"`
}

// WidgetStatus is the observed state of a Widget.
// +kubebuilder:validation:MinProperties=1
type WidgetStatus struct {
	// conditions are the latest observations.
	// +listType=map
	// +listMapKey=type
	// +kubebuilder:validation:MaxItems=32
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// observedGeneration is the last reconciled metadata.generation.
	// +kubebuilder:validation:Minimum=1
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

// Widget is the Schema for the widgets API.
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
type Widget struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is the standard object metadata.
	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// spec is the desired state.
	// +required
	Spec WidgetSpec `json:"spec,omitzero"`

	// status is the observed state.
	// +optional
	Status WidgetStatus `json:"status,omitzero"`
}

// WidgetList contains a list of Widget.
// +kubebuilder:object:root=true
type WidgetList struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is the standard list metadata.
	// +optional
	metav1.ListMeta `json:"metadata,omitzero"`

	// items is the list of Widgets.
	// +kubebuilder:validation:MaxItems=1000
	// +listType=atomic
	// +required
	Items []Widget `json:"items"`
}
