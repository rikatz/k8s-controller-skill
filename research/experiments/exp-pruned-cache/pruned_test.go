package prunedcache

import (
	"bytes"
	"context"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	toolscache "k8s.io/client-go/tools/cache"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/event"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

const ns = "default"

type upd struct {
	oldRV, newRV string
	suppressed   bool
}

// pruneTLSCrt keeps only Data["tls.crt"]. Idempotent, in place, never returns nil.
func pruneTLSCrt(in any) (any, error) {
	if s, ok := in.(*corev1.Secret); ok {
		crt, found := s.Data["tls.crt"]
		s.Data, s.StringData = nil, nil
		if found {
			s.Data = map[string][]byte{"tls.crt": crt}
		}
	}
	return in, nil
}

// composed runs the default strip first, then the prune (the per-type Transform REPLACES DefaultTransform).
func composed(in any) (any, error) {
	out, err := cache.TransformStripManagedFields()(in)
	if err != nil {
		return nil, err
	}
	return pruneTLSCrt(out)
}

func keys(m map[string][]byte) []string {
	var k []string
	for key := range m {
		k = append(k, key)
	}
	return k
}

func TestPrunedCache(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	env := &envtest.Environment{DownloadBinaryAssets: true, BinaryAssetsDirectory: t.TempDir()}
	cfg, err := env.Start()
	if err != nil {
		t.Fatal(err)
	}
	defer env.Stop()

	// Manager A: DefaultTransform strips managedFields; Secret has its OWN Transform (prune only, NOT composed).
	mgr, err := ctrl.NewManager(cfg, ctrl.Options{
		Metrics: metricsserver.Options{BindAddress: "0"},
		Cache: cache.Options{
			DefaultTransform: cache.TransformStripManagedFields(),
			ByObject:         map[client.Object]cache.ByObject{&corev1.Secret{}: {Transform: pruneTLSCrt}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	// Informer + handler for (d); registered before Start.
	inf, err := mgr.GetCache().GetInformer(ctx, &corev1.Secret{})
	if err != nil {
		t.Fatal(err)
	}
	updates := make(chan upd, 16)
	var pred predicate.Predicate = predicate.Funcs{UpdateFunc: func(e event.UpdateEvent) bool {
		o, n := e.ObjectOld.(*corev1.Secret), e.ObjectNew.(*corev1.Secret)
		return !bytes.Equal(o.Data["tls.crt"], n.Data["tls.crt"])
	}}
	_, _ = inf.AddEventHandler(toolscache.ResourceEventHandlerFuncs{UpdateFunc: func(o, n any) {
		os, ns := o.(*corev1.Secret), n.(*corev1.Secret)
		passes := pred.Update(event.UpdateEvent{ObjectOld: os, ObjectNew: ns})
		updates <- upd{os.ResourceVersion, ns.ResourceVersion, !passes}
	}})
	go func() { _ = mgr.Start(ctx) }()
	if !mgr.GetCache().WaitForCacheSync(ctx) {
		t.Fatal("cache not synced")
	}
	c, reader := mgr.GetClient(), mgr.GetAPIReader()

	mk := func(name string) *corev1.Secret {
		s := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
			Data:       map[string][]byte{"tls.crt": []byte("CRT"), "tls.key": []byte("KEY"), "ca.crt": []byte("CA")},
		}
		if err := c.Create(ctx, s); err != nil {
			t.Fatal(err)
		}
		return s
	}
	cachedGet := func(name string) *corev1.Secret {
		var got corev1.Secret
		deadline := time.Now().Add(10 * time.Second)
		for {
			err := c.Get(ctx, types.NamespacedName{Namespace: ns, Name: name}, &got)
			if err == nil {
				return &got
			}
			if !apierrors.IsNotFound(err) || time.Now().After(deadline) {
				t.Fatal(err)
			}
			time.Sleep(50 * time.Millisecond)
		}
	}
	live := func(name string) *corev1.Secret {
		var got corev1.Secret
		if err := reader.Get(ctx, types.NamespacedName{Namespace: ns, Name: name}, &got); err != nil {
			t.Fatal(err)
		}
		return &got
	}

	// (a) cached Get returns only tls.crt; (e) APIReader returns everything.
	mk("a")
	got := cachedGet("a")
	t.Logf("(a) cached keys=%v  managedFields=%d generation=%d", keys(got.Data), len(got.ManagedFields), got.Generation)
	t.Logf("(e) APIReader keys=%v", keys(live("a").Data))

	// (b) Update built from the pruned cached object.
	mk("b")
	obj := cachedGet("b")
	obj.Labels = map[string]string{"touched": "yes"}
	if err := c.Update(ctx, obj); err != nil {
		t.Fatal(err)
	}
	t.Logf("(b) after Update from pruned copy, server keys=%v", keys(live("b").Data))

	// (c) MergeFrom patch from the pruned copy, label only.
	mk("c")
	obj = cachedGet("c")
	base := obj.DeepCopy()
	obj.Labels = map[string]string{"touched": "yes"}
	if err := c.Patch(ctx, obj, client.MergeFrom(base)); err != nil {
		t.Fatal(err)
	}
	l := live("c")
	t.Logf("(c) after MergeFrom label patch, server keys=%v labels=%v", keys(l.Data), l.Labels)

	// (d) change a NON-retained key on the server and watch the informer.
	mk("d")
	cachedGet("d")
	drain(updates)
	full := live("d")
	orig := full.DeepCopy()
	full.Data["tls.key"] = []byte("KEY-ROTATED")
	if err := c.Patch(ctx, full, client.MergeFrom(orig)); err != nil {
		t.Fatal(err)
	}
	select {
	case u := <-updates:
		t.Logf("(d) update fired: oldRV=%s newRV=%s rvChanged=%v predicateSuppressed=%v", u.oldRV, u.newRV, u.oldRV != u.newRV, u.suppressed)
	case <-time.After(10 * time.Second):
		t.Log("(d) NO update event within 10s")
	}

	// (f) per-type Transform replaced DefaultTransform: Secret managedFields kept, ConfigMap (default only) stripped.
	cm := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "cm", Namespace: ns}, Data: map[string]string{"k": "v"}}
	if err := c.Create(ctx, cm); err != nil {
		t.Fatal(err)
	}
	var gotCM corev1.ConfigMap
	for i := 0; i < 100; i++ {
		if c.Get(ctx, client.ObjectKeyFromObject(cm), &gotCM) == nil {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Logf("(f) ConfigMap managedFields=%d (DefaultTransform only); Secret managedFields=%d (per-type Transform, not composed)", len(gotCM.ManagedFields), len(got.ManagedFields))
	// Composed variant, via a plain cache.Cache on the same config.
	ca, err := cache.New(cfg, cache.Options{
		DefaultTransform: cache.TransformStripManagedFields(),
		ByObject:         map[client.Object]cache.ByObject{&corev1.Secret{}: {Transform: composed}},
	})
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = ca.Start(ctx) }()
	if !ca.WaitForCacheSync(ctx) {
		t.Fatal("ca not synced")
	}
	var gc corev1.Secret
	if err := ca.Get(ctx, types.NamespacedName{Namespace: ns, Name: "a"}, &gc); err != nil {
		t.Fatal(err)
	}
	t.Logf("(f) composed Transform: managedFields=%d keys=%v", len(gc.ManagedFields), keys(gc.Data))
}

func drain(ch chan upd) {
	for {
		select {
		case <-ch:
		case <-time.After(500 * time.Millisecond):
			return
		}
	}
}
