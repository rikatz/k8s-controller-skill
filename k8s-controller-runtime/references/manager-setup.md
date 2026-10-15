# Manager setup and project layout

Read this when you create the repo, write `cmd/main.go`, or wire a new controller into the manager.
Cache tuning is in `cache-and-memory.md`; the metrics server is in `metrics.md`; webhook server and certs in `webhooks.md`.

Everything below was compiled against controller-runtime v0.25.2 / k8s.io v0.37 and run against an envtest API server [run].

## Layout (no kubebuilder)

```
go.mod                      tool sigs.k8s.io/controller-tools/cmd/controller-gen   (see codegen-workflow.md)
doc.go                      only //go:generate lines
Dockerfile
cmd/main.go                 flags, scheme, manager, wiring
api/v1/                     groupversion_info.go, *_types.go, zz_generated.deepcopy.go (generated)
internal/controller/        one reconciler per Kind
internal/webhook/v1/        admission handlers (optional)
config/crd/                 GENERATED  controller-gen crd
config/rbac/role.yaml       GENERATED  controller-gen rbac
config/webhook/manifests.yaml GENERATED controller-gen webhook (partial)
config/rbac/*.yaml          HAND-WRITTEN: ServiceAccount, leader-election Role, bindings
config/manager/manager.yaml HAND-WRITTEN: Deployment
```

controller-gen does NOT create: ServiceAccount, (Cluster)RoleBindings, the leader-election Role, the manager Deployment, the webhook
Service, serving certs or `caBundle` injection [CT, run: its webhook output has a placeholder `webhook-service`/`system` service ref and no `caBundle`].

## Scheme registration (`api/v1/groupversion_info.go`)

Prefer the apimachinery `runtime.NewSchemeBuilder`; controller-runtime's `scheme.Builder` is Deprecated because API packages should stay
dependency-light [CR `pkg/scheme/scheme.go`]. `SchemeGroupVersion` is only there for generated applyconfigurations (see `codegen-workflow.md`).

```go
// Package v1 contains API Schema definitions for the demo v1 API group.
// +kubebuilder:object:generate=true
// +kubebuilder:ac:generate=true
// +groupName=demo.example.com
package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var (
	GroupVersion = schema.GroupVersion{Group: "demo.example.com", Version: "v1"}
	// SchemeGroupVersion is referenced by the generated applyconfiguration package.
	SchemeGroupVersion = GroupVersion
	SchemeBuilder      = runtime.NewSchemeBuilder(addKnownTypes)
	AddToScheme        = SchemeBuilder.AddToScheme
)

func addKnownTypes(s *runtime.Scheme) error {
	s.AddKnownTypes(GroupVersion, &Widget{}, &WidgetList{})
	metav1.AddToGroupVersion(s, GroupVersion)
	return nil
}
```

## `cmd/main.go`

```go
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
```

The reconciler side of the per-controller options:

```go
func (r *WidgetReconciler) SetupWithManager(mgr ctrl.Manager, opts controller.Options) error {
	return ctrl.NewControllerManagedBy(mgr).For(&demov1.Widget{}).Named("widget").WithOptions(opts).Complete(r)
}
```

## Rules

- Build your own `runtime.NewScheme()` with `clientgoscheme` plus your API groups; do not rely on a global scheme. A missing registration fails with "no kind is registered for the type v1.ConfigMap in scheme ..." [CR FAQ, run].
- Call `ctrl.SetLogger` first. Otherwise all logs are dropped; the "log.SetLogger(...) was never called" notice prints only on a log call made after 30 s, so earlier lines are lost silently (`logging.md`) [CR `pkg/log/log.go`, run].
- Use ONE manager per process. All controllers then share one cache, one client and one leader lease [CR]. Use `cluster.Cluster` (added to the manager) only for a second API server [CR].
- Leader election: `LeaderElectionID` is mandatory (`NewManager` fails with "LeaderElectionID must be configured") [CR, run]. Outside a cluster also set `LeaderElectionNamespace` ("unable to find leader election namespace") [CR, run]. The default lock is a `leases` Lease [CR].
- Probes: `HealthProbeBindAddress` only serves checks you add; every replica serves `/readyz`, including non-leaders (readyz=200, no workers started) [CR, run].
- Shutdown: `ctrl.SetupSignalHandler()` (a second call panics with "close of closed channel" [run]) cancels the context on SIGTERM/SIGINT. `GracefulShutdownTimeout` defaults to 30s (set explicitly to document it); if workers outlive it, `Start` returns "failed waiting for all runnables to end within grace period of 2s" [CR, run]. Keep `terminationGracePeriodSeconds` larger than it; the Pod default is 30s [K8S `PodSpec` doc].
- `LeaderElectionReleaseOnCancel: true` hands the lease over at once, but only if the process exits right after `Start` returns [CR]. Without it the lease stays held after a clean exit until it expires (15s default `LeaseDuration`) [CR, run].
- Concurrency: `MaxConcurrentReconciles` defaults to 1 and the same key never runs twice at once [CR, run: "worker count":4 with the flag]. Expose one flag per controller like CAPI/CAPV [CAPI, CAPV]; manager-wide defaults live in `config.Controller{MaxConcurrentReconciles, GroupKindConcurrency}` and the per-controller value wins [CR].
- Client rate limit: `ctrl.GetConfigOrDie()` sets `QPS=-1` (client-side limiting off, relies on API priority and fairness) when the kubeconfig leaves it 0; client-go alone would use 5/10 [CR `pkg/client/config/config.go`, client-go]. CAPI/CAPV re-enable it with `--kube-api-qps` 100 / `--kube-api-burst` 200 [CAPI, CAPV]. Prefer leaving it off; add flags so operators can cap it.
- `CacheSyncTimeout` defaults to 2 minutes (`config.Controller` sets it manager-wide); CAPI raises it to 5 minutes for large clusters [CR, CAPI].
- Set `SkipNameValidation: new(true)` in `config.Controller` ONLY in tests that build several managers; production names stay unique, because the name is the metrics/log label [CR, run: "controller with name dup already exists"].
- Before `Start`, `mgr.GetClient()` reads return "the cache is not started" and still register the informer; `mgr.GetAPIReader()` works immediately and is the tool for startup reads [CR, run].
- Webhook server, `Cache` options and `Metrics` are `ctrl.Options` fields too; see the sibling references. Keep `main.go` as wiring only: parse flags, build options, register controllers, start.

## Hand-written manifests

```yaml
# config/rbac/service_account.yaml
apiVersion: v1
kind: ServiceAccount
metadata: {name: widget-controller, namespace: widget-system}
---
# config/rbac/leader_election_role.yaml (a RoleBinding to the ServiceAccount is also needed)
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata: {name: widget-controller-leader-election, namespace: widget-system}
rules:
  - {apiGroups: [coordination.k8s.io], resources: [leases], verbs: [get, list, watch, create, update, patch, delete]}
  - {apiGroups: [""], resources: [events], verbs: [create, patch]}
```

Plus a `ClusterRoleBinding` from `manager-role` (the generated `config/rbac/role.yaml`) to the ServiceAccount.
Applied with the manager impersonating the ServiceAccount, it acquired the lease and started workers [run]. Without lease RBAC the process stays
up and healthy but never starts workers, logging only `"Error retrieving lease lock" ... forbidden` [run]. Without `events create` the leader-election event
is rejected with a klog "Server rejected event (will not retry!)" line (core group `""`) [run].

Deployment essentials (server-side dry run accepted the full file [run]): `serviceAccountName`, `replicas: 2` with leader election,
`terminationGracePeriodSeconds: 60`, `securityContext: {runAsNonRoot: true, seccompProfile: {type: RuntimeDefault}}` on the pod and
`{allowPrivilegeEscalation: false, readOnlyRootFilesystem: true, capabilities: {drop: [ALL]}}` on the container [K8S Pod Security restricted profile],
liveness `/healthz` and readiness `/readyz` on the probe port.

## Dockerfile

Built with podman and run (`--help` printed the flags; `Config.User` = 65532) [run]:

```dockerfile
FROM golang:1.26 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY api/ api/
COPY cmd/ cmd/
COPY internal/ internal/
RUN CGO_ENABLED=0 go build -trimpath -o /out/manager ./cmd

FROM gcr.io/distroless/static:nonroot
COPY --from=build /out/manager /manager
USER 65532:65532
ENTRYPOINT ["/manager"]
```

## Hazards

- Missing lease RBAC does not crash the manager: it stays Ready, never starts controllers, and only logs "Error retrieving lease lock". Check `kubectl get lease` after deploying [run].
- A `Reconcile` that ignores `ctx` outlives `GracefulShutdownTimeout`; `Start` then returns an error and the process exits non-zero with work in flight [CR, run].
- `LeaderElectionReleaseOnCancel` is unsafe unless the binary ends immediately after `Start` returns (the field doc says so) [CR].
- `SkipNameValidation` in production hides two controllers reporting under one metrics label [CR].
- Plain-HTTP metrics (the `:8080` default when `Metrics` is left empty) are unauthenticated; configure them per `metrics.md` or disable with `"0"` [CR].
