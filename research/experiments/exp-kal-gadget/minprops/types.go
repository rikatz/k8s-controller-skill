// Package minprops reproduces the field-level MinProperties rejection: the marker
// belongs on the struct type, not on a field of that type.
// +kubebuilder:object:generate=true
// +groupName=minprops.example.com
package minprops

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
	// tls carries MinProperties on the FIELD, which controller-gen rejects.
	// +kubebuilder:validation:MinProperties=1
	// +optional
	TLS TLSConfig `json:"tls,omitempty"`
}

// TLSConfig configures TLS; the MinProperties marker belongs here, on the type.
type TLSConfig struct {
	// minVersion is the lowest accepted TLS version.
	// +optional
	MinVersion string `json:"minVersion,omitempty"`
}
