// Package misspell reproduces the silent-marker-drop hazard: a misspelled marker
// must not fail generation, and the constraint it names must not appear in the CRD.
// +kubebuilder:object:generate=true
// +groupName=misspell.example.com
package misspell

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// Zap is the Schema for the zaps API.
// +kubebuilder:object:root=true
type Zap struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is the standard object metadata.
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// spec is the desired state of the Zap.
	// +required
	Spec ZapSpec `json:"spec"`
}

// ZapSpec is the desired state of a Zap.
type ZapSpec struct {
	// name carries a deliberately misspelled marker (MaxLenth, not MaxLength).
	// +kubebuilder:validation:MaxLenth=15
	// +optional
	Name string `json:"name,omitempty"`
}
