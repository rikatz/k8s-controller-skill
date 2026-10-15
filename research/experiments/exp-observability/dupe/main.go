package main

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/tools/events"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	demov1 "example.com/demo/api/v1"
)

const ReasonProvisioned = "Provisioned"

type WidgetReconciler struct {
	client.Client
	Recorder events.EventRecorder
}

func (r *WidgetReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var w demov1.Widget
	if err := r.Get(ctx, req.NamespacedName, &w); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	r.Recorder.Eventf(&w, nil, corev1.EventTypeNormal, ReasonProvisioned, "Provision", "Created Deployment %s", w.Name)
	return ctrl.Result{}, nil
}

func setup(mgr ctrl.Manager) *WidgetReconciler {
	return &WidgetReconciler{Client: mgr.GetClient(), Recorder: mgr.GetEventRecorder("widget-controller")}
}

func main() {}
