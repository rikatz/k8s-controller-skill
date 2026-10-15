package main

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"

	demov1 "example.com/demo/api/v1"
	acv1 "example.com/demo/api/v1/applyconfiguration/api/v1"
)

func main() {
	cfg, _ := clientcmd.BuildConfigFromFlags("", "/tmp/draft-kc")
	s := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(s)
	_ = demov1.AddToScheme(s)
	c, err := client.New(cfg, client.Options{Scheme: s})
	if err != nil {
		panic(err)
	}
	ctx := context.Background()
	ac := acv1.Widget("w1", "default").WithSpec(acv1.WidgetSpec().WithSize(demov1.Size("Small")))
	fmt.Println("apply:", c.Apply(ctx, ac, client.FieldOwner("probe"), client.ForceOwnership))
	var w demov1.Widget
	_ = c.Get(ctx, client.ObjectKey{Namespace: "default", Name: "w1"}, &w)
	fmt.Println("size:", w.Spec.Size, "replicas:", *w.Spec.Replicas)
	st := acv1.Widget("w1", "default").WithStatus(acv1.WidgetStatus().WithObservedGeneration(w.Generation))
	fmt.Println("status apply:", c.Status().Apply(ctx, st, client.FieldOwner("probe"), client.ForceOwnership))
	_ = c.Get(ctx, client.ObjectKey{Namespace: "default", Name: "w1"}, &w)
	fmt.Println("observedGeneration:", w.Status.ObservedGeneration)
}
