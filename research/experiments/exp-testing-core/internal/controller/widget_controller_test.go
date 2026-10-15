package controller_test

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	demov1 "example.com/demo/api/v1"
	"example.com/demo/internal/controller"
)

var _ = Describe("Widget", func() {
	var ns string
	BeforeEach(func() { ns = newNamespace() })

	newWidget := func(name string) *demov1.Widget {
		return &demov1.Widget{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
			Spec:       demov1.WidgetSpec{Size: "Small"},
		}
	}

	It("adds a finalizer, owns a ConfigMap and reports observedGeneration", func() {
		w := newWidget("w")
		createAndWait(w)

		Eventually(k.Object(w)).Should(SatisfyAll(
			HaveField("Finalizers", ContainElement(controller.Finalizer)),
			HaveField("Status.ObservedGeneration", w.Generation),
		))
		cm := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "w", Namespace: ns}}
		Eventually(k.Get(cm)).Should(Succeed())
		// Assert the ownerReference, not garbage collection: envtest has no GC.
		Expect(metav1.IsControlledBy(cm, w)).To(BeTrue())
	})

	It("does nothing while paused, then reconciles once unpaused", func() {
		w := newWidget("paused")
		w.Annotations = map[string]string{controller.PausedAnnotation: "true"}
		createAndWait(w)

		cm := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "paused", Namespace: ns}}
		Consistently(k.Get(cm), 2*time.Second).Should(Satisfy(apierrors.IsNotFound))

		// Prove the controller was alive: Consistently alone also passes if it never ran.
		Eventually(k.Update(w, func() { delete(w.Annotations, controller.PausedAnnotation) })).Should(Succeed())
		Eventually(k.Get(cm)).Should(Succeed())
	})

	It("releases its finalizer on delete", func() {
		w := newWidget("gone")
		createAndWait(w)
		Eventually(k.Object(w)).Should(HaveField("Finalizers", ContainElement(controller.Finalizer)))

		Expect(k8sClient.Delete(ctx, w)).To(Succeed())
		Eventually(k.Get(w)).Should(Satisfy(apierrors.IsNotFound))
	})
})
