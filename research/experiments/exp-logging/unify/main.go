// One logger for controller-runtime, client-go (klog) and application code, proven against envtest.
// Usage: KUBEBUILDER_ASSETS=<dir> go run ./unify -backend zap|klog|slog [-wire=false] [-level 3] [-v N for klog legacy calls]
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/go-logr/logr"
	uzap "go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

type reconciler struct{ client.Client }

func (r *reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)
	cm := &corev1.ConfigMap{}
	if err := r.Get(ctx, req.NamespacedName, cm); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	if cm.Annotations["demo/done"] != "" {
		log.V(1).Info("Skipped ConfigMap, already annotated", "configMap", klog.KObj(cm))
		return ctrl.Result{}, nil
	}
	log.Info("Annotating ConfigMap", "configMap", klog.KObj(cm))
	// Three direct writes in a row; the client is limited to QPS=1 so client-go logs a throttling line.
	for i := 0; i < 3; i++ {
		patch := fmt.Sprintf(`{"metadata":{"annotations":{"demo/step%d":"x"}}}`, i)
		if err := r.Patch(ctx, cm, client.RawPatch(types.MergePatchType, []byte(patch))); err != nil {
			return ctrl.Result{}, err
		}
	}
	if err := r.Patch(ctx, cm, client.RawPatch(types.MergePatchType, []byte(`{"metadata":{"annotations":{"demo/done":"yes"}}}`))); err != nil {
		return ctrl.Result{}, err
	}
	log.Info("Annotated ConfigMap", "configMap", klog.KObj(cm))
	return ctrl.Result{}, nil
}

func main() {
	backend := flag.String("backend", "zap", "zap|klog|slog")
	wire := flag.Bool("wire", true, "route klog (client-go) through the same logger")
	contextual := flag.Bool("contextual", true, "pass klog.ContextualLogger(true); false is plain klog.SetLogger")
	level := flag.Int("level", 3, "verbosity for the logr backend (klog keeps its own -v)")
	klog.InitFlags(nil) // klog's own flags on flag.CommandLine (-v, -logtostderr, ...)
	flag.Parse()

	var logger logr.Logger
	switch *backend {
	case "zap":
		logger = zap.New(zap.WriteTo(os.Stdout), zap.Level(uzap.NewAtomicLevelAt(zapcore.Level(-*level))))
	case "slog":
		logger = logr.FromSlogHandler(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.Level(-*level)}))
	case "klog":
		_ = flag.Set("v", fmt.Sprint(*level))
		_ = flag.Set("logtostderr", "false")
		_ = flag.Set("stderrthreshold", "FATAL")
		klog.SetOutput(os.Stdout)
		logger = klog.NewKlogr()
	}
	ctrl.SetLogger(logger)
	if *wire && *backend != "klog" {
		klog.SetLoggerWithOptions(logger, klog.ContextualLogger(*contextual))
	}

	env := &envtest.Environment{}
	cfg, err := env.Start()
	if err != nil {
		panic(err)
	}
	defer env.Stop()
	cfg.QPS, cfg.Burst = 1, 1

	mgr, err := ctrl.NewManager(cfg, ctrl.Options{
		Metrics:                 metricsserver.Options{BindAddress: "0"},
		LeaderElection:          true,
		LeaderElectionID:        "demo",
		LeaderElectionNamespace: "default",
	})
	if err != nil {
		panic(err)
	}
	only := predicate.NewPredicateFuncs(func(o client.Object) bool { return o.GetName() == "demo" })
	if err := ctrl.NewControllerManagedBy(mgr).For(&corev1.ConfigMap{}, builder.WithPredicates(only)).Named("configmap").
		Complete(&reconciler{mgr.GetClient()}); err != nil {
		panic(err)
	}
	ctx, cancel := context.WithTimeout(ctrl.SetupSignalHandler(), 12*time.Second)
	defer cancel()
	go func() {
		c, _ := client.New(env.Config, client.Options{})
		time.Sleep(3 * time.Second)
		_ = c.Create(ctx, &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "demo", Namespace: "default"}})
	}()
	_ = mgr.Start(ctx)
	klog.Flush()
}
