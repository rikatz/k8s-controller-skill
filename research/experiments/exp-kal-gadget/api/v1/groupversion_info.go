// Package v1 contains API Schema definitions for the demo v1 API group.
// +kubebuilder:object:generate=true
// +kubebuilder:ac:generate=true
// +groupName=demo.example.com
package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var (
	GroupVersion = schema.GroupVersion{Group: "demo.example.com", Version: "v1"}
	// SchemeGroupVersion is referenced by the generated applyconfiguration package.
	SchemeGroupVersion = GroupVersion
	SchemeBuilder      = runtime.NewSchemeBuilder(addKnownTypes)
	AddToScheme        = SchemeBuilder.AddToScheme
)

func addKnownTypes(s *runtime.Scheme) error {
	s.AddKnownTypes(GroupVersion, &Widget{}, &WidgetList{}, &Gadget{}, &GadgetList{})
	metav1.AddToGroupVersion(s, GroupVersion)
	return nil
}
