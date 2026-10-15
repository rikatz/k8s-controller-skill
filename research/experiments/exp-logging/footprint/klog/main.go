package main

import (
	"flag"

	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
)

func main() {
	klog.InitFlags(nil)
	flag.Parse()
	ctrl.SetLogger(klog.Background())
	klog.Background().Info("Started")
}
