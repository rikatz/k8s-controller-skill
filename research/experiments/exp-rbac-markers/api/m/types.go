// +kubebuilder:object:generate=true
// +groupName=m.example.com
package m

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

type Spec struct {
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="name is immutable"
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=63
	// +kubebuilder:validation:Pattern=`^[a-z]+$`
	// +required
	Name string `json:"name,omitempty"`
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=8
	// +listType=set
	// +optional
	Tags []string `json:"tags,omitempty"`
}

// +kubebuilder:validation:XValidation:rule="self.name.size() > 0",message="name required"
type Wrap struct {
	Spec Spec `json:"spec"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:path=ms,shortName=mm,scope=Cluster,categories=all
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Name",type=string,JSONPath=`.spec.name`
// +kubebuilder:selectablefield:JSONPath=`.spec.name`
type M struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitzero"`
	Spec              Spec `json:"spec"`
}

// +kubebuilder:object:root=true
type MList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []M `json:"items"`
}
