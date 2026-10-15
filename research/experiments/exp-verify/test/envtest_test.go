package test

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"sync/atomic"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"

	demov1 "example.com/demo/api/v1"
	acv1 "example.com/demo/api/applyconfiguration/api/v1"
)

type counter struct {
	rt   http.RoundTripper
	reqs atomic.Int64
	last atomic.Value
}

func (c *counter) RoundTrip(r *http.Request) (*http.Response, error) {
	c.reqs.Add(1)
	c.last.Store(r.Method + " " + r.URL.Path)
	return c.rt.RoundTrip(r)
}

func TestEnv(t *testing.T) {
	ctx := context.Background()
	dir, _ := os.MkdirTemp("", "envtest-bin-")
	env := &envtest.Environment{
		DownloadBinaryAssets:  true,
		BinaryAssetsDirectory: dir,
		CRDDirectoryPaths:     []string{"../config/crd/bases"},
		ErrorIfCRDPathMissing: true,
	}
	t0 := time.Now()
	cfg, err := env.Start()
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("env started in %v, bin dir=%s", time.Since(t0), dir)
	defer env.Stop()

	sch := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(sch)
	_ = demov1.AddToScheme(sch)
	cnt := &counter{}
	cfg2 := rest.CopyConfig(cfg)
	cfg2.Wrap(func(rt http.RoundTripper) http.RoundTripper { cnt.rt = rt; return cnt })
	c, err := client.New(cfg2, client.Options{Scheme: sch})
	if err != nil {
		t.Fatal(err)
	}

	t.Run("i-namespace-delete", func(t *testing.T) {
		ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "del-me"}}
		must(t, c.Create(ctx, ns))
		start := time.Now()
		must(t, c.Delete(ctx, ns))
		for time.Since(start) < 20*time.Second {
			var got corev1.Namespace
			err := c.Get(ctx, client.ObjectKeyFromObject(ns), &got)
			if apierrors.IsNotFound(err) {
				t.Logf("RESULT ns gone after %v", time.Since(start))
				return
			}
			time.Sleep(2 * time.Second)
			t.Logf("t+%v ns phase=%s deletionTimestamp=%v finalizers=%v spec.finalizers=%v conditions=%d", time.Since(start).Round(time.Second), got.Status.Phase, got.DeletionTimestamp != nil, got.Finalizers, got.Spec.Finalizers, len(got.Status.Conditions))
		}
		t.Logf("RESULT ns still present after 20s")
	})

	t.Run("ii-gc", func(t *testing.T) {
		owner := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "owner", Namespace: "default"}}
		must(t, c.Create(ctx, owner))
		ref := metav1.OwnerReference{APIVersion: "v1", Kind: "ConfigMap", Name: owner.Name, UID: owner.UID}
		dep := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "dep-cm", Namespace: "default", OwnerReferences: []metav1.OwnerReference{ref}}}
		must(t, c.Create(ctx, dep))
		dp := tinyDeploy("dep-deploy", "default", 1)
		dp.OwnerReferences = []metav1.OwnerReference{ref}
		must(t, c.Create(ctx, dp))
		start := time.Now()
		must(t, c.Delete(ctx, owner))
		for time.Since(start) < 30*time.Second {
			time.Sleep(3 * time.Second)
			e1 := c.Get(ctx, client.ObjectKeyFromObject(dep), &corev1.ConfigMap{})
			e2 := c.Get(ctx, client.ObjectKeyFromObject(dp), &appsv1.Deployment{})
			e0 := c.Get(ctx, client.ObjectKeyFromObject(owner), &corev1.ConfigMap{})
			t.Logf("t+%v owner gone=%v dep-cm gone=%v dep-deploy gone=%v", time.Since(start).Round(time.Second), apierrors.IsNotFound(e0), apierrors.IsNotFound(e1), apierrors.IsNotFound(e2))
		}
	})

	t.Run("iii-deployment-children", func(t *testing.T) {
		dp := tinyDeploy("real-deploy", "default", 2)
		must(t, c.Create(ctx, dp))
		time.Sleep(15 * time.Second)
		var rs appsv1.ReplicaSetList
		var pods corev1.PodList
		must(t, c.List(ctx, &rs, client.InNamespace("default")))
		must(t, c.List(ctx, &pods, client.InNamespace("default")))
		var got appsv1.Deployment
		must(t, c.Get(ctx, client.ObjectKeyFromObject(dp), &got))
		t.Logf("RESULT after 15s: replicasets=%d pods=%d deployment.status=%+v", len(rs.Items), len(pods.Items), got.Status)
	})

	t.Run("iv-pod-default-sa", func(t *testing.T) {
		p := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "p1", Namespace: "default"},
			Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "c", Image: "busybox"}}}}
		must(t, c.Create(ctx, p))
		var got corev1.Pod
		must(t, c.Get(ctx, client.ObjectKeyFromObject(p), &got))
		var sas corev1.ServiceAccountList
		must(t, c.List(ctx, &sas, client.InNamespace("default")))
		names := []string{}
		for _, s := range sas.Items {
			names = append(names, s.Name)
		}
		t.Logf("RESULT pod.spec.serviceAccountName=%q serviceAccount=%q volumes=%d phase=%q nodeName=%q; SAs in default=%v", got.Spec.ServiceAccountName, got.Spec.DeprecatedServiceAccount, len(got.Spec.Volumes), got.Status.Phase, got.Spec.NodeName, names)
	})

	t.Run("3-apply-config", func(t *testing.T) {
		ac := acv1.Widget("w-apply", "default").WithSpec(acv1.WidgetSpec().WithSize("Small"))
		err := c.Apply(ctx, ac, client.FieldOwner("verify"), client.ForceOwnership)
		t.Logf("Apply spec err=%v", err)
		var w demov1.Widget
		must(t, c.Get(ctx, client.ObjectKey{Namespace: "default", Name: "w-apply"}, &w))
		t.Logf("applied: size=%s replicas=%v managedFields=%d first manager=%s", w.Spec.Size, w.Spec.Replicas, len(w.ManagedFields), w.ManagedFields[0].Manager)
		sac := acv1.Widget("w-apply", "default").WithStatus(acv1.WidgetStatus().WithObservedGeneration(w.Generation))
		err = c.Status().Apply(ctx, sac, client.FieldOwner("verify"), client.ForceOwnership)
		t.Logf("Status().Apply err=%v", err)
		must(t, c.Get(ctx, client.ObjectKey{Namespace: "default", Name: "w-apply"}, &w))
		t.Logf("status.observedGeneration=%d (generation %d)", w.Status.ObservedGeneration, w.Generation)
		ex, err := acv1.ExtractWidget(&w, "verify")
		t.Logf("ExtractWidget err=%v spec.size=%v", err, *ex.Spec.Size)
	})

	t.Run("6-empty-patch", func(t *testing.T) {
		var w demov1.Widget
		must(t, c.Get(ctx, client.ObjectKey{Namespace: "default", Name: "w-apply"}, &w))
		for _, tc := range []struct {
			name string
			f    func() error
		}{
			{"Patch(MergeFrom) no change", func() error { orig := w.DeepCopy(); return c.Patch(ctx, &w, client.MergeFrom(orig)) }},
			{"Patch(MergeFromWithOptimisticLock) no change", func() error {
				orig := w.DeepCopy()
				return c.Patch(ctx, &w, client.MergeFromWithOptions(orig, client.MergeFromWithOptimisticLock{}))
			}},
			{"Status().Patch(MergeFrom) no change", func() error { orig := w.DeepCopy(); return c.Status().Patch(ctx, &w, client.MergeFrom(orig)) }},
			{"Patch(MergeFrom) with change", func() error {
				orig := w.DeepCopy()
				w.Labels = map[string]string{"a": "b"}
				return c.Patch(ctx, &w, client.MergeFrom(orig))
			}},
			{"Update no change", func() error { return c.Update(ctx, &w) }},
		} {
			before := cnt.reqs.Load()
			err := tc.f()
			t.Logf("RESULT %-48s err=%v requests=%d last=%v", tc.name, err, cnt.reqs.Load()-before, cnt.last.Load())
		}
		orig := w.DeepCopy()
		d, _ := client.MergeFrom(orig).Data(&w)
		t.Logf("empty MergeFrom Data() = %s", d)
	})
	fmt.Println("done")
}

func tinyDeploy(name, ns string, replicas int32) *appsv1.Deployment {
	l := map[string]string{"app": name}
	return &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
		Spec: appsv1.DeploymentSpec{Replicas: &replicas, Selector: &metav1.LabelSelector{MatchLabels: l},
			Template: corev1.PodTemplateSpec{ObjectMeta: metav1.ObjectMeta{Labels: l},
				Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "c", Image: "busybox"}}}}}}
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
