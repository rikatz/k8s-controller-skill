// +kubebuilder:object:generate=true
// +groupName=oz.example.com
package oz

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/scheme"
)

var (
	GroupVersion  = schema.GroupVersion{Group: "oz.example.com", Version: "v1"}
	SchemeBuilder = &scheme.Builder{GroupVersion: GroupVersion}
	AddToScheme   = SchemeBuilder.AddToScheme
)

type Nested struct {
	A string `json:"a,omitzero"`
}

type NestedNoOZ struct {
	A string `json:"a,omitempty"`
}

type OZSpec struct {
	// required-by-default no-tag fields
	Str       string            `json:"str,omitzero"`
	Int       int32             `json:"int,omitzero"`
	Nest      Nested            `json:"nest,omitzero"`
	Slice     []string          `json:"slice,omitzero"`
	Map       map[string]string `json:"map,omitzero"`
	Dur       metav1.Duration   `json:"dur,omitzero"`
	Time      metav1.Time       `json:"time,omitzero"`
	// controls
	StrOE   string            `json:"strOE,omitempty"`
	IntOE   int32             `json:"intOE,omitempty"`
	NestOE  Nested            `json:"nestOE,omitempty"`
	SliceOE []string          `json:"sliceOE,omitempty"`
	MapOE   map[string]string `json:"mapOE,omitempty"`
	DurOE   metav1.Duration   `json:"durOE,omitempty"`
	TimeOE  metav1.Time       `json:"timeOE,omitempty"`
	NoTag   string            `json:"noTag"`
	// +optional
	StrOpt string `json:"strOpt,omitzero"`
	// +optional
	NestOpt Nested `json:"nestOpt,omitzero"`
	// +required
	StrReq string `json:"strReq,omitzero"`
	// no omit tag at all but explicit optional
	// +optional
	NoTagOpt string `json:"noTagOpt"`
	// ptr with omitzero
	Ptr *string `json:"ptr,omitzero"`
	// ptr without omit
	PtrNoOmit *string `json:"ptrNoOmit"`
}

// +kubebuilder:object:root=true
type OZ struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitzero"`
	Spec              OZSpec `json:"spec"`
}

// +kubebuilder:object:root=true
type OZList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []OZ `json:"items"`
}

func init() { SchemeBuilder.Register(&OZ{}, &OZList{}) }
