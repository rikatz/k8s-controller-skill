package main

import (
	"flag"
	"os"
	"time"

	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/config"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	demov1 "example.com/demo/api/v1"
	widgetcontroller "example.com/demo/internal/controller"
)

var scheme = runtime.NewScheme()

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(demov1.AddToScheme(scheme))
}

func main() {
	var (
		probeAddr         string
		leaderElect       bool
		leaderNamespace   string
		qps               float64
		burst             int
		widgetConcurrency int
		cacheSyncTimeout  time.Duration
		shutdownTimeout   time.Duration
	)
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "Probe endpoint address")
	flag.BoolVar(&leaderElect, "leader-elect", true, "Enable leader election")
	flag.StringVar(&leaderNamespace, "leader-election-namespace", "", "Lease namespace; required outside a cluster")
	flag.Float64Var(&qps, "kube-api-qps", -1, "Client-side QPS limit; negative disables it")
	flag.IntVar(&burst, "kube-api-burst", 100, "Client-side burst, used when QPS is positive")
	flag.IntVar(&widgetConcurrency, "widget-concurrency", 1, "Concurrent Widget reconciles")
	flag.DurationVar(&cacheSyncTimeout, "cache-sync-timeout", 2*time.Minute, "Informer sync timeout")
	flag.DurationVar(&shutdownTimeout, "graceful-shutdown-timeout", 30*time.Second, "Runnable drain time on shutdown")
	zapOpts := zap.Options{}
	zapOpts.BindFlags(flag.CommandLine)
	flag.Parse()

	// Without SetLogger every controller-runtime log line is dropped.
	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&zapOpts)))
	setupLog := ctrl.Log.WithName("setup")

	cfg := ctrl.GetConfigOrDie()
	cfg.QPS, cfg.Burst = float32(qps), burst

	mgr, err := ctrl.NewManager(cfg, ctrl.Options{
		Scheme: scheme,
		// "0" disables the metrics server; configure it per metrics.md.
		Metrics:                 metricsserver.Options{BindAddress: "0"},
		HealthProbeBindAddress:  probeAddr,
		LeaderElection:          leaderElect,
		LeaderElectionID:        "widget-controller.demo.example.com", // required when LeaderElection is true
		LeaderElectionNamespace: leaderNamespace,
		GracefulShutdownTimeout: &shutdownTimeout,
		Controller:              config.Controller{CacheSyncTimeout: cacheSyncTimeout},
	})
	if err != nil {
		setupLog.Error(err, "Unable to create manager")
		os.Exit(1)
	}

	r := &widgetcontroller.WidgetReconciler{Client: mgr.GetClient()}
	if err := r.SetupWithManager(mgr, controller.Options{MaxConcurrentReconciles: widgetConcurrency}); err != nil {
		setupLog.Error(err, "Unable to create controller", "controller", "widget")
		os.Exit(1)
	}
	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "Unable to add health check")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLog.Error(err, "Unable to add ready check")
		os.Exit(1)
	}

	setupLog.Info("Starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "Problem running manager")
		os.Exit(1)
	}
}
