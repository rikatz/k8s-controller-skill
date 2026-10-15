package obsrbac_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-logr/logr/funcr"
	coordinationv1 "k8s.io/api/coordination/v1"
	corev1 "k8s.io/api/core/v1"
	eventsv1 "k8s.io/api/events/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
)

type logBuf struct {
	mu sync.Mutex
	b  strings.Builder
}

func (l *logBuf) Write(p, s string) { l.mu.Lock(); l.b.WriteString(p + s + "\n"); l.mu.Unlock() }
func (l *logBuf) String() string { l.mu.Lock(); defer l.mu.Unlock(); return l.b.String() }

func run(t *testing.T, env *envtest.Environment, admin client.Client, name string, rules []rbacv1.PolicyRule) {
	ctx := context.Background()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(admin.Create(ctx, &rbacv1.ClusterRole{ObjectMeta: metav1.ObjectMeta{Name: name}, Rules: rules}))
	must(admin.Create(ctx, &rbacv1.ClusterRoleBinding{ObjectMeta: metav1.ObjectMeta{Name: name},
		RoleRef:  rbacv1.RoleRef{APIGroup: "rbac.authorization.k8s.io", Kind: "ClusterRole", Name: name},
		Subjects: []rbacv1.Subject{{Kind: "User", Name: name, APIGroup: "rbac.authorization.k8s.io"}}}))
	cm := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"}}
	must(admin.Create(ctx, cm))

	logs := &logBuf{}
	ctrl.SetLogger(funcr.New(logs.Write, funcr.Options{Verbosity: 1}))
	cfg := rest.CopyConfig(env.Config)
	cfg.Impersonate = rest.ImpersonationConfig{UserName: name}
	mgr, err := ctrl.NewManager(cfg, ctrl.Options{
		Metrics:                 metricsserver.Options{BindAddress: "0"},
		HealthProbeBindAddress:  "127.0.0.1:18081",
		LeaderElection:          true,
		LeaderElectionID:        "obs-test",
		LeaderElectionNamespace: "default",
	})
	must(err)
	must(mgr.AddHealthzCheck("ping", healthz.Ping))
	must(mgr.AddReadyzCheck("ping", healthz.Ping))
	mctx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() { _ = mgr.Start(mctx); close(done) }()
	select {
	case <-mgr.Elected():
	case <-time.After(30 * time.Second):
		t.Fatalf("%s: never elected\n%s", name, logs.String())
	}
	for _, p := range []string{"/healthz", "/readyz"} {
		resp, err := http.Get("http://127.0.0.1:18081" + p)
		must(err)
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		fmt.Printf("%s %s -> %d %q\n", name, p, resp.StatusCode, b)
	}
	mgr.GetEventRecorder("obs-test").Eventf(cm, nil, corev1.EventTypeNormal, "Synced", "Sync", "Synced %s", "ok")
	time.Sleep(4 * time.Second)
	cancel()
	<-done

	var evs eventsv1.EventList
	must(admin.List(ctx, &evs))
	var corevs corev1.EventList
	must(admin.List(ctx, &corevs))
	fmt.Printf("%s: events.k8s.io events=%d; core-listed=%d\n", name, len(evs.Items), len(corevs.Items))
	for _, e := range evs.Items {
		fmt.Printf("  %s reason=%s regarding=%s/%s reporter=%s\n", name, e.Reason, e.Regarding.Kind, e.Regarding.Name, e.ReportingController)
	}
	for _, l := range strings.Split(logs.String(), "\n") {
		if strings.Contains(l, "forbidden") || strings.Contains(l, "Forbidden") {
			fmt.Printf("  %s LOG: %.300s\n", name, l)
		}
	}
	_ = admin.DeleteAllOf(ctx, &eventsv1.Event{}, client.InNamespace("default"))
	_ = admin.Delete(ctx, &coordinationv1.Lease{ObjectMeta: metav1.ObjectMeta{Name: "obs-test", Namespace: "default"}})
}

func TestRBAC(t *testing.T) {
	env := &envtest.Environment{DownloadBinaryAssets: true}
	cfg, err := env.Start()
	if err != nil {
		t.Fatal(err)
	}
	defer env.Stop()
	admin, _ := client.New(cfg, client.Options{})
	lease := []rbacv1.PolicyRule{{APIGroups: []string{"coordination.k8s.io"}, Resources: []string{"leases"}, Verbs: []string{"get", "create", "update"}}}
	evNew := rbacv1.PolicyRule{APIGroups: []string{"events.k8s.io"}, Resources: []string{"events"}, Verbs: []string{"create", "patch"}}
	evCore := rbacv1.PolicyRule{APIGroups: []string{""}, Resources: []string{"events"}, Verbs: []string{"create", "patch"}}
	run(t, env, admin, "lease-and-newevents", append(append([]rbacv1.PolicyRule{}, lease...), evNew))
	run(t, env, admin, "lease-newevents-coreevents", append(append([]rbacv1.PolicyRule{}, lease...), evNew, evCore))
}
