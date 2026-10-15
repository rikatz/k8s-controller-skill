package setup_test

import (
	"context"
	"testing"

	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	demov1 "example.com/demo/api/v1"
	"example.com/demo/internal/setup"
)

func TestWidgetsBySize(t *testing.T) {
	g := NewWithT(t)
	scheme := runtime.NewScheme()
	g.Expect(demov1.AddToScheme(scheme)).To(Succeed())
	w := func(name string, s demov1.Size) *demov1.Widget {
		return &demov1.Widget{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "ns"}, Spec: demov1.WidgetSpec{Size: s}}
	}
	c := fake.NewClientBuilder().WithScheme(scheme).
		WithStatusSubresource(&demov1.Widget{}). // without it Status().Update is not modelled as a subresource
		WithIndex(&demov1.Widget{}, setup.WidgetSizeField, setup.WidgetSize).
		WithObjects(w("a", "Small"), w("b", "Large")).Build()

	var list demov1.WidgetList
	g.Expect(c.List(context.Background(), &list, client.MatchingFields{setup.WidgetSizeField: "Small"})).To(Succeed())
	g.Expect(list.Items).To(HaveLen(1))
	g.Expect(list.Items[0].Name).To(Equal("a"))
}
