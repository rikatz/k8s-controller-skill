package setup

import (
	"context"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	demov1 "example.com/demo/api/v1"
)

const WidgetSizeField = ".spec.size"

// WidgetSize is exported so fake-client unit tests register the same extractor.
func WidgetSize(o client.Object) []string {
	w, ok := o.(*demov1.Widget)
	if !ok {
		return nil
	}
	return []string{string(w.Spec.Size)}
}

// RegisterIndexes runs once, before the controllers are built and before mgr.Start.
func RegisterIndexes(ctx context.Context, mgr ctrl.Manager) error {
	return mgr.GetFieldIndexer().IndexField(ctx, &demov1.Widget{}, WidgetSizeField, WidgetSize)
}
