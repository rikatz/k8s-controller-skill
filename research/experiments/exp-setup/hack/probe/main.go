package main

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/clientcmd"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/config"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
)

func newMgr(opts ctrl.Options) ctrl.Manager {
	cfg, err := clientcmd.BuildConfigFromFlags("", "/tmp/draft-kc")
	if err != nil {
		panic(err)
	}
	s := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(s)
	opts.Scheme = s
	opts.Metrics = metricsserver.Options{BindAddress: "0"}
	m, err := ctrl.NewManager(cfg, opts)
	if err != nil {
		fmt.Println("NewManager error:", err)
		return nil
	}
	return m
}

func main() {
	// 1. LeaderElection without ID
	newMgr(ctrl.Options{LeaderElection: true, LeaderElectionNamespace: "default"})

	// 2. pre-Start reads
	m := newMgr(ctrl.Options{})
	var ns corev1.Namespace
	err := m.GetClient().Get(context.Background(), client.ObjectKey{Name: "default"}, &ns)
	fmt.Println("GetClient pre-Start:", err)
	err = m.GetAPIReader().Get(context.Background(), client.ObjectKey{Name: "default"}, &ns)
	fmt.Println("GetAPIReader pre-Start:", err, ns.Name)

	// 3. duplicate controller name
	build := func(opts ctrl.Options) error {
		m := newMgr(opts)
		return ctrl.NewControllerManagedBy(m).For(&corev1.ConfigMap{}).Named("dup").
			Complete(reconcile.Func(func(context.Context, reconcile.Request) (reconcile.Result, error) { return reconcile.Result{}, nil }))
	}
	fmt.Println("dup #1:", build(ctrl.Options{}))
	fmt.Println("dup #2:", build(ctrl.Options{}))
	fmt.Println("dup #3 with SkipNameValidation:", build(ctrl.Options{Controller: config.Controller{SkipNameValidation: new(true)}}))

	// 4. grace period: a reconcile that ignores ctx and takes 10s, shutdown timeout 2s
	d := 2 * time.Second
	m4 := newMgr(ctrl.Options{GracefulShutdownTimeout: &d})
	started := make(chan struct{})
	err = ctrl.NewControllerManagedBy(m4).For(&corev1.Namespace{}).Named("slow").Complete(reconcile.Func(func(context.Context, reconcile.Request) (reconcile.Result, error) {
		select {
		case <-started:
		default:
			close(started)
		}
		time.Sleep(10 * time.Second)
		return reconcile.Result{}, nil
	}))
	fmt.Println("slow build:", err)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- m4.Start(ctx) }()
	<-started
	t0 := time.Now()
	cancel()
	fmt.Println("Start returned:", <-done, "after", time.Since(t0).Round(100*time.Millisecond))
}
