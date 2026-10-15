// Package webhook holds admission handlers; only the generator marker is shown here.
package webhook

// +kubebuilder:webhook:path=/validate-demo-example-com-v1-widget,mutating=false,failurePolicy=fail,sideEffects=None,groups=demo.example.com,resources=widgets,verbs=create;update,versions=v1,name=vwidget.demo.example.com,admissionReviewVersions=v1
