package main

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func main() {
	cfg, _ := clientcmd.BuildConfigFromFlags("", "/tmp/draft-kc")
	c, _ := client.New(cfg, client.Options{Scheme: runtime.NewScheme()})
	fmt.Println(c.Get(context.Background(), client.ObjectKey{Name: "x", Namespace: "default"}, &corev1.ConfigMap{}))
}
