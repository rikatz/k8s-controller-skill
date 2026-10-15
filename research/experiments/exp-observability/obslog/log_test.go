package obslog_test

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-logr/logr/funcr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	demov1 "example.com/demo/api/v1"
)

type rec struct {
	done chan struct{}
	once sync.Once
}

func (r *rec) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)
	log.Info("Reconciling Widget")
	ctx = ctrl.LoggerInto(ctx, log.WithValues("phase", "provision"))
	helper(ctx)
	r.once.Do(func() { close(r.done) })
	return ctrl.Result{}, nil
}

func helper(ctx context.Context) {
	ctrl.LoggerFrom(ctx).V(1).Info("Created Deployment", "deployment", "w-1")
	ctrl.LoggerFrom(ctx).V(2).Info("Hidden at V1")
}

func TestLog(t *testing.T) {
	var mu sync.Mutex
	var out []string
	ctrl.SetLogger(funcr.New(func(p, a string) { mu.Lock(); out = append(out, p+a); mu.Unlock() }, funcr.Options{Verbosity: 1}))
	env := &envtest.Environment{CRDDirectoryPaths: []string{filepath.Join("..", "config", "crd", "bases")}, ErrorIfCRDPathMissing: true, DownloadBinaryAssets: true}
	cfg, err := env.Start()
	if err != nil {
		t.Fatal(err)
	}
	defer env.Stop()
	s := runtime.NewScheme()
	_ = demov1.AddToScheme(s)
	mgr, err := ctrl.NewManager(cfg, ctrl.Options{Scheme: s, Metrics: metricsserver.Options{BindAddress: "0"}})
	if err != nil {
		t.Fatal(err)
	}
	r := &rec{done: make(chan struct{})}
	if err := ctrl.NewControllerManagedBy(mgr).For(&demov1.Widget{}).Named("widget").Complete(r); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go mgr.Start(ctx)
	c, _ := client.New(cfg, client.Options{Scheme: s})
	if err := c.Create(ctx, &demov1.Widget{ObjectMeta: metav1.ObjectMeta{Name: "w", Namespace: "default"}, Spec: demov1.WidgetSpec{Size: "Small"}}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-r.done:
	case <-time.After(20 * time.Second):
		t.Fatal("timeout")
	}
	mu.Lock()
	defer mu.Unlock()
	for _, l := range out {
		if strings.Contains(l, "Reconciling") || strings.Contains(l, "Created Deployment") || strings.Contains(l, "Hidden") {
			fmt.Println(l)
		}
	}
}
