package v1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// Gadget is the Schema for the gadgets API.
type Gadget struct {
	metav1.TypeMeta `json:",inline"`
	// metadata is the standard object metadata.
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty"`
	// spec is the desired state of the Gadget.
	// +required
	Spec GadgetSpec `json:"spec"`
}

// GadgetSpec is the desired state of a Gadget.
type GadgetSpec struct {
	// size is the number of workers.
	// +kubebuilder:validation:XValidation:rule="self >= 4 || (oldSelf.hasValue() && oldSelf.value() < 4)",message="size must be >= 4 unless the old value was below 4",optionalOldSelf=true
	// +optional
	Size int32 `json:"size,omitempty"`
}

// +kubebuilder:object:root=true

// GadgetList contains a list of Gadget.
type GadgetList struct {
	metav1.TypeMeta `json:",inline"`
	// metadata is the standard object metadata.
	// +optional
	metav1.ListMeta `json:"metadata,omitempty"`
	// items is the list of Gadgets.
	// +required
	Items []Gadget `json:"items"`
}
