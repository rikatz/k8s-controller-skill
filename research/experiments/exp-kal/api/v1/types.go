package v1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

var _ = metav1.Condition{}

// Foo is a test type.
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
type Foo struct {
	// spec is the spec.
	// +required
	Spec FooSpec `json:"spec,omitzero"`
	// status is the status.
	// +kubebuilder:validation:MinProperties=1
	// +optional
	Status FooStatus `json:"status,omitzero"`
}

// FooSpec is the spec.
type FooSpec struct {
	// mode is the mode.
	// +optional
	// +kubebuilder:default=Fast
	// +kubebuilder:validation:Enum=Fast;Slow
	// +kubebuilder:validation:MaxLength=16
	Mode string `json:"mode,omitempty"`

	// replicas is the replica count.
	// +optional
	// +kubebuilder:validation:Minimum=0
	Replicas int32 `json:"replicas,omitempty"`

	// name has a non-zero min length.
	// +optional
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=63
	Name string `json:"name,omitempty"`
}

// FooStatus is the status.
type FooStatus struct {
	// conditions are conditions.
	// +optional
	// +listType=map
	// +listMapKey=type
	// +kubebuilder:validation:MaxItems=32
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}
