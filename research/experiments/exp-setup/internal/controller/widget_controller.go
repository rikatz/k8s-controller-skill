package controller

import (
	"context"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	demov1 "example.com/demo/api/v1"
)

type WidgetReconciler struct {
	client.Client
}

// +kubebuilder:rbac:groups=demo.example.com,resources=widgets,verbs=get;list;watch
// +kubebuilder:rbac:groups=demo.example.com,resources=widgets/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=demo.example.com,resources=widgets/finalizers,verbs=update

func (r *WidgetReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var w demov1.Widget
	if err := r.Get(ctx, req.NamespacedName, &w); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	logf.FromContext(ctx).Info("Reconciling Widget", "name", w.Name)
	return ctrl.Result{}, nil
}

// SetupWithManager registers the controller; opts carries per-controller concurrency.
func (r *WidgetReconciler) SetupWithManager(mgr ctrl.Manager, opts controller.Options) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&demov1.Widget{}).
		Named("widget").
		WithOptions(opts).
		Complete(r)
}
