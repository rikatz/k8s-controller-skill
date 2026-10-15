package v1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// Mode selects how fast a Gadget processes work.
// +kubebuilder:validation:Enum=Fast;Slow
type Mode string

// +kubebuilder:validation:MinProperties=1
type GadgetSpec struct {
	// mode selects the processing speed.
	// When omitted, Fast is used.
	// +kubebuilder:default=Fast
	// +optional
	Mode Mode `json:"mode,omitempty"`

	// storageClass names the class used for data volumes. It cannot change once set.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=253
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="storageClass is immutable"
	// +optional
	StorageClass string `json:"storageClass,omitempty"`

	// Deviation: pointer kept because 0 (paused) and unset (use the default) differ here.

	// replicas is the desired number of workers. 0 pauses the Gadget.
	// +kubebuilder:validation:Minimum=0
	// +optional
	Replicas *int32 `json:"replicas,omitempty"`

	// tls configures TLS for the Gadget endpoint.
	// +optional
	TLS TLSConfig `json:"tls,omitzero"`

	// ports lists the exposed ports.
	// +listType=map
	// +listMapKey=name
	// +kubebuilder:validation:MaxItems=16
	// +optional
	Ports []Port `json:"ports,omitempty"`
}

// TLSConfig configures TLS.
// +kubebuilder:validation:MinProperties=1
type TLSConfig struct {
	// minVersion is the lowest accepted TLS version.
	// +kubebuilder:validation:Enum=TLS12;TLS13
	// +optional
	MinVersion string `json:"minVersion,omitempty"`
}

// Port is an exposed port.
type Port struct {
	// name identifies the port within the Gadget.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=15
	// +required
	Name string `json:"name,omitempty"`

	// protocol is TCP or UDP. It cannot change once set.
	// +kubebuilder:validation:Enum=TCP;UDP
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="protocol is immutable"
	// +required
	Protocol string `json:"protocol,omitempty"`
}

// GadgetStatus is the observed state of a Gadget.
// +kubebuilder:validation:MinProperties=1
type GadgetStatus struct {
	// conditions represent the latest observations of the Gadget state.
	// +listType=map
	// +listMapKey=type
	// +kubebuilder:validation:MaxItems=32
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// observedGeneration is the metadata.generation last reconciled successfully.
	// +kubebuilder:validation:Minimum=1
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`

// Gadget is the Schema for the gadgets API.
type Gadget struct {
	metav1.TypeMeta `json:",inline"`
	// metadata is the standard object metadata.
	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`
	// spec is the desired state of the Gadget.
	// +required
	Spec GadgetSpec `json:"spec,omitempty,omitzero"`
	// status is the observed state of the Gadget.
	// +optional
	Status GadgetStatus `json:"status,omitempty,omitzero"`
}

// GadgetList contains a list of Gadget.
// +kubebuilder:object:root=true
type GadgetList struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is the standard list metadata.
	// +optional
	metav1.ListMeta `json:"metadata,omitzero"`

	// items is the list of Gadgets.
	// +kubebuilder:validation:MaxItems=1000
	// +listType=atomic
	// +required
	Items []Gadget `json:"items"`
}
