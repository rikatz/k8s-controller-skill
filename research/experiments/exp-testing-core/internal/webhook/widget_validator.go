package webhook

import (
	"context"
	"fmt"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	demov1 "example.com/demo/api/v1"
)

// +kubebuilder:webhook:path=/validate-demo-example-com-v1-widget,mutating=false,failurePolicy=fail,sideEffects=None,groups=demo.example.com,resources=widgets,verbs=create;update,versions=v1,name=vwidget.demo.example.com,admissionReviewVersions=v1

type WidgetValidator struct{}

var _ admission.Validator[*demov1.Widget] = &WidgetValidator{}

func (WidgetValidator) ValidateCreate(_ context.Context, w *demov1.Widget) (admission.Warnings, error) {
	return nil, validate(w)
}

func (WidgetValidator) ValidateUpdate(_ context.Context, _, w *demov1.Widget) (admission.Warnings, error) {
	return nil, validate(w)
}

func (WidgetValidator) ValidateDelete(context.Context, *demov1.Widget) (admission.Warnings, error) {
	return nil, nil
}

func validate(w *demov1.Widget) error {
	if w.Spec.Replicas != nil && *w.Spec.Replicas > 10 {
		return fmt.Errorf("spec.replicas must be <= 10")
	}
	return nil
}

func SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr, &demov1.Widget{}).WithValidator(WidgetValidator{}).Complete()
}
