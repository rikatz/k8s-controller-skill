// What controller-runtime itself logs for each Reconcile outcome, at V(5).
// Usage: KUBEBUILDER_ASSETS=<dir> go run ./errorlog
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"time"

	uzap "go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type reconciler struct {
	client.Client
	mu    sync.Mutex
	calls map[string]int
}

func (r *reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	r.mu.Lock()
	r.calls[req.Name]++
	n := r.calls[req.Name]
	r.mu.Unlock()
	log := ctrl.LoggerFrom(ctx)
	cause := errors.New("simulated failure")
	switch req.Name {
	case "demo-a-error-once":
		if n == 1 {
			return ctrl.Result{}, cause
		}
	case "demo-b-terminal":
		return ctrl.Result{}, reconcile.TerminalError(cause)
	case "demo-c-error-and-requeueafter":
		if n == 1 {
			return ctrl.Result{RequeueAfter: time.Minute}, cause
		}
	case "demo-d-requeueafter":
		if n == 1 {
			return ctrl.Result{RequeueAfter: 200 * time.Millisecond}, nil
		}
	case "demo-e-log-and-return":
		if n == 1 {
			log.Error(cause, "Could not sync ConfigMap") // logged here ...
			return ctrl.Result{}, cause                  // ... and again by controller-runtime
		}
	}
	return ctrl.Result{}, nil
}

func main() {
	ctrl.SetLogger(zap.New(zap.WriteTo(os.Stdout), zap.Level(uzap.NewAtomicLevelAt(zapcore.Level(-5)))))
	env := &envtest.Environment{}
	cfg, err := env.Start()
	if err != nil {
		panic(err)
	}
	defer env.Stop()
	mgr, err := ctrl.NewManager(cfg, ctrl.Options{Metrics: metricsserver.Options{BindAddress: "0"}})
	if err != nil {
		panic(err)
	}
	only := predicate.NewPredicateFuncs(func(o client.Object) bool { return len(o.GetName()) > 5 && o.GetName()[:5] == "demo-" })
	r := &reconciler{Client: mgr.GetClient(), calls: map[string]int{}}
	if err := ctrl.NewControllerManagedBy(mgr).For(&corev1.ConfigMap{}, builder.WithPredicates(only)).Named("configmap").Complete(r); err != nil {
		panic(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	go func() {
		c, _ := client.New(env.Config, client.Options{})
		time.Sleep(2 * time.Second)
		for _, n := range []string{"demo-a-error-once", "demo-b-terminal", "demo-c-error-and-requeueafter", "demo-d-requeueafter", "demo-e-log-and-return"} {
			_ = c.Create(ctx, &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: n, Namespace: "default"}})
		}
	}()
	_ = mgr.Start(ctx)
	fmt.Fprintln(os.Stderr, "calls:", r.calls)
}
