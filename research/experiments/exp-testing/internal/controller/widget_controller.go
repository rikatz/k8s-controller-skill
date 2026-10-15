package controller

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	demov1 "example.com/demo/api/v1"
)

const (
	Finalizer        = "demo.example.com/cleanup"
	PausedAnnotation = "demo.example.com/paused"
)

type WidgetReconciler struct {
	client.Client
}

// +kubebuilder:rbac:groups=demo.example.com,resources=widgets,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=demo.example.com,resources=widgets/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=demo.example.com,resources=widgets/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch;create;update;patch;delete

func (r *WidgetReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := logf.FromContext(ctx)
	var w demov1.Widget
	if err := r.Get(ctx, req.NamespacedName, &w); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	if w.Annotations[PausedAnnotation] == "true" {
		return ctrl.Result{}, nil
	}
	if !w.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(&w, Finalizer) {
			orig := w.DeepCopy()
			controllerutil.RemoveFinalizer(&w, Finalizer)
			if err := r.Patch(ctx, &w, client.MergeFrom(orig)); err != nil {
				return ctrl.Result{}, client.IgnoreNotFound(err)
			}
		}
		return ctrl.Result{}, nil
	}
	if !controllerutil.ContainsFinalizer(&w, Finalizer) {
		orig := w.DeepCopy()
		controllerutil.AddFinalizer(&w, Finalizer)
		if err := r.Patch(ctx, &w, client.MergeFrom(orig)); err != nil {
			return ctrl.Result{}, err
		}
	}

	cm := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: w.Name, Namespace: w.Namespace}}
	if _, err := controllerutil.CreateOrUpdate(ctx, r.Client, cm, func() error {
		cm.Data = map[string]string{"size": string(w.Spec.Size)}
		return controllerutil.SetControllerReference(&w, cm, r.Scheme())
	}); err != nil {
		return ctrl.Result{}, err
	}

	orig := w.DeepCopy()
	w.Status.ObservedGeneration = w.Generation
	meta.SetStatusCondition(&w.Status.Conditions, metav1.Condition{
		Type: "Ready", Status: metav1.ConditionTrue, Reason: "Reconciled",
		ObservedGeneration: w.Generation,
	})
	if err := r.Status().Patch(ctx, &w, client.MergeFrom(orig)); err != nil && !apierrors.IsNotFound(err) {
		return ctrl.Result{}, err
	}
	logger.Info("Reconciled Widget", "name", w.Name)
	return ctrl.Result{}, nil
}

func (r *WidgetReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&demov1.Widget{}).
		Owns(&corev1.ConfigMap{}).
		Named("widget").
		Complete(r)
}
