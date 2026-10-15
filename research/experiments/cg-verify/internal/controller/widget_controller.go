package controller

import (
	"context"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	demov1 "example.com/demo/api/v1"
)

type WidgetReconciler struct {
	client.Client
}

// +kubebuilder:rbac:groups=demo.example.com,resources=widgets,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=demo.example.com,resources=widgets/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=demo.example.com,resources=widgets/finalizers,verbs=update
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete

func (r *WidgetReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := logf.FromContext(ctx)
	var w demov1.Widget
	if err := r.Get(ctx, req.NamespacedName, &w); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}
	logger.Info("Reconciling Widget", "name", w.Name)
	return ctrl.Result{}, nil
}

func (r *WidgetReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).For(&demov1.Widget{}).Named("widget").Complete(r)
}
