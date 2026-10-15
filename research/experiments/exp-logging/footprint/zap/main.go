package main

import (
	"flag"

	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

func main() {
	opts := zap.Options{}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()
	l := zap.New(zap.UseFlagOptions(&opts))
	ctrl.SetLogger(l)
	klog.SetLoggerWithOptions(l, klog.ContextualLogger(true))
	l.Info("Started")
}
