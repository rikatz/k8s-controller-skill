package main

import (
	"log/slog"
	"os"

	"github.com/go-logr/logr"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
)

func main() {
	l := logr.FromSlogHandler(slog.NewJSONHandler(os.Stderr, nil))
	ctrl.SetLogger(l)
	klog.SetLoggerWithOptions(l, klog.ContextualLogger(true))
	l.Info("Started")
}
