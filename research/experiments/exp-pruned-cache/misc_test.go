package prunedcache

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
)

// Checks DisableFor is GVK-keyed (metadata Get goes live) and index error texts.
func TestDisableForAndIndexes(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	env := &envtest.Environment{DownloadBinaryAssets: true, BinaryAssetsDirectory: t.TempDir()}
	cfg, err := env.Start()
	if err != nil {
		t.Fatal(err)
	}
	defer env.Stop()

	// ReaderFailOnMissingInformer: any read served by the cache without a registered informer errors.
	mgr, err := ctrl.NewManager(cfg, ctrl.Options{
		Metrics: metricsserver.Options{BindAddress: "0"},
		Cache:   cache.Options{ReaderFailOnMissingInformer: true},
		Client:  client.Options{Cache: &client.CacheOptions{DisableFor: []client.Object{&corev1.Secret{}}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = mgr.Start(ctx) }()
	if !mgr.GetCache().WaitForCacheSync(ctx) {
		t.Fatal("not synced")
	}
	c := mgr.GetClient()
	if err := c.Create(ctx, &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "s", Namespace: "default"}}); err != nil {
		t.Fatal(err)
	}
	key := types.NamespacedName{Namespace: "default", Name: "s"}
	var typed corev1.Secret
	t.Logf("DisableFor Secret, typed Get: err=%v", c.Get(ctx, key, &typed))
	pom := &metav1.PartialObjectMetadata{TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "Secret"}}
	t.Logf("DisableFor Secret, PartialObjectMetadata Get: err=%v", c.Get(ctx, key, pom))
	pcm := &metav1.PartialObjectMetadata{TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "ConfigMap"}}
	t.Logf("control: ConfigMap PartialObjectMetadata Get (cache, no informer): err=%v", c.Get(ctx, key, pcm))
	var cm corev1.ConfigMap
	t.Logf("control: ConfigMap typed Get (cache, no informer): err=%v", c.Get(ctx, key, &cm))

	// Indexes on a plain cache.
	ca, err := cache.New(cfg, cache.Options{})
	if err != nil {
		t.Fatal(err)
	}
	extract := func(o client.Object) []string { return []string{o.GetLabels()["k"]} }
	t.Logf("IndexField first: err=%v", ca.IndexField(ctx, &corev1.ConfigMap{}, "k", extract))
	t.Logf("IndexField same type+field again: err=%v", ca.IndexField(ctx, &corev1.ConfigMap{}, "k", extract))
	go func() { _ = ca.Start(ctx) }()
	if !ca.WaitForCacheSync(ctx) {
		t.Fatal("ca not synced")
	}
	var l corev1.ConfigMapList
	t.Logf("List MatchingFields registered: err=%v", ca.List(ctx, &l, client.MatchingFields{"k": "v"}))
	t.Logf("List MatchingFields unregistered: err=%v", ca.List(ctx, &l, client.MatchingFields{"nope": "v"}))
}
