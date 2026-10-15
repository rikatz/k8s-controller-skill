// Compile check for the snippets in logging.md (main wiring options and reconcile logging patterns).
package main

import (
	"context"
	"flag"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	corev1 "k8s.io/api/core/v1"
)

// Default: klog is the backend. One flag set (-v), one output format for controller-runtime, client-go and app code.
func wireKlog() {
	klog.InitFlags(nil) // registers -v, -vmodule, -logtostderr ... on flag.CommandLine
	flag.Parse()
	ctrl.SetLogger(klog.Background())
}

// Alternative: controller-runtime's zap, with klog (client-go) routed into the same logger.
func wireZap() {
	opts := zap.Options{}
	opts.BindFlags(flag.CommandLine) // --zap-devel, --zap-encoder, --zap-log-level, ...
	flag.Parse()
	logger := zap.New(zap.UseFlagOptions(&opts))
	ctrl.SetLogger(logger)
	klog.SetLoggerWithOptions(logger, klog.ContextualLogger(true))
}

type reconciler struct{ client.Client }

func (r *reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	cm := &corev1.ConfigMap{}
	if err := r.Get(ctx, req.NamespacedName, cm); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err) // expected: no log
	}
	// Once per reconcile; helpers that receive ctx inherit the pair.
	log := ctrl.LoggerFrom(ctx).WithValues("generation", cm.Generation)
	ctx = ctrl.LoggerInto(ctx, log)

	sec := &corev1.Secret{}
	if err := r.Get(ctx, client.ObjectKey{Namespace: cm.Namespace, Name: "creds"}, sec); err != nil {
		if apierrors.IsNotFound(err) {
			log.V(1).Info("Waiting for Secret", "secret", klog.KRef(cm.Namespace, "creds"))
			return ctrl.Result{RequeueAfter: 30e9}, nil // expected: debug line, no error
		}
		return ctrl.Result{}, fmt.Errorf("getting Secret: %w", err) // controller-runtime logs "Reconciler error"
	}
	log.V(1).Info("Using Secret", "secret", klog.KObj(sec)) // never "secret", sec

	log.Info("Created Deployment", "deployment", klog.KRef(cm.Namespace, cm.Name)) // state transition
	return ctrl.Result{}, nil
}

func background(ctx context.Context, err error) {
	// Cannot return the error from here, so this is the one place to log it.
	ctrl.LoggerFrom(ctx).Error(err, "Could not refresh inventory", "interval", "5m")
	ctrl.LoggerFrom(ctx).Error(nil, "Unsupported state, no error value", "state", "x") // nil error is allowed
}

func main() {
	if false {
		wireKlog()
		wireZap()
		background(context.Background(), nil)
		_ = &reconciler{}
	}
}
