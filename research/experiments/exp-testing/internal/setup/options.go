// Package setup holds manager options shared by main() and the envtest suite.
package setup

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Options returns the cache and client settings used in production. Tests start
// from the same value and override only the listeners.
func Options(scheme *runtime.Scheme) ctrl.Options {
	return ctrl.Options{
		Scheme: scheme,
		Cache:  cache.Options{DefaultTransform: cache.TransformStripManagedFields()},
		Client: client.Options{Cache: &client.CacheOptions{DisableFor: []client.Object{&corev1.Secret{}}}},
	}
}
