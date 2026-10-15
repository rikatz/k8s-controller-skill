package controller_test

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Documents what envtest cannot do. Each spec asserts the LIMITATION, so it fails
// the day that changes; do not copy these into product tests.
var _ = Describe("envtest limits", func() {
	It("never finishes namespace deletion", func() {
		ns := newNamespace()
		Expect(k8sClient.Delete(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: ns}})).To(Succeed())
		Consistently(k.Object(&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: ns}}), 5*time.Second).
			Should(HaveField("Status.Phase", corev1.NamespaceTerminating))
	})

	It("does not garbage-collect ownerReference dependents", func() {
		ns := newNamespace()
		owner := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "owner", Namespace: ns}}
		createAndWait(owner)
		dep := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "dep", Namespace: ns,
			OwnerReferences: []metav1.OwnerReference{{APIVersion: "v1", Kind: "ConfigMap", Name: "owner", UID: owner.UID}}}}
		createAndWait(dep)
		Expect(k8sClient.Delete(ctx, owner)).To(Succeed())
		Eventually(k.Get(owner)).ShouldNot(Succeed())
		Consistently(k.Get(dep), 5*time.Second).Should(Succeed())
	})

	It("runs no controller-manager: Deployment gets no ReplicaSets or Pods, Pod gets no ServiceAccount", func() {
		ns := newNamespace()
		lbl := map[string]string{"a": "b"}
		d := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "d", Namespace: ns},
			Spec: appsv1.DeploymentSpec{Replicas: new(int32(2)), Selector: &metav1.LabelSelector{MatchLabels: lbl},
				Template: corev1.PodTemplateSpec{ObjectMeta: metav1.ObjectMeta{Labels: lbl},
					Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "c", Image: "x"}}}}}}
		createAndWait(d)
		Consistently(func() (int, error) {
			var rs appsv1.ReplicaSetList
			err := k8sClient.List(ctx, &rs, client.InNamespace(ns))
			return len(rs.Items), err
		}, 5*time.Second).Should(BeZero())
		Expect(k.Object(d)()).To(HaveField("Status.ObservedGeneration", BeZero()))

		p := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "p", Namespace: ns},
			Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "c", Image: "x"}}}}
		createAndWait(p)
		Expect(k.Object(p)()).To(SatisfyAll(
			HaveField("Spec.ServiceAccountName", BeEmpty()),
			HaveField("Status.Phase", corev1.PodPending)))
		var sas corev1.ServiceAccountList
		Expect(k8sClient.List(ctx, &sas, client.InNamespace(ns))).To(Succeed())
		Expect(sas.Items).To(BeEmpty())
	})
})
