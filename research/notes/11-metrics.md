# 11 - Custom metrics in pure controller-runtime controllers

Legend: [V] = read/verified (source file:line in local clones, or fetched URL); [U] = unverified / my judgement.
CR = controller-runtime v0.25.2 at `/tmp/k8s-controller-skill-research/cr-src/repo` (commit 5d8467b, tag v0.25.2; go.mod pins `prometheus/client_golang v1.24.0`).
CAPI = `/tmp/k8s-controller-skill-research/src/cluster-api`; CAPV = `.../src/cluster-api-provider-vsphere`.

---------------------------------------------------------------------------
## 1. controller-runtime mechanics

### 1.1 The registry [V]
`pkg/metrics/registry.go:25-31`
```go
type RegistererGatherer interface { prometheus.Registerer; prometheus.Gatherer }
// Registry is a prometheus registry for storing metrics within the controller-runtime.
var Registry RegistererGatherer = prometheus.NewRegistry()
```
- It is a *private* `prometheus.NewRegistry()`, NOT `prometheus.DefaultRegisterer`. Metrics registered on the default registry are NOT served by the manager. Always use `sigs.k8s.io/controller-runtime/pkg/metrics`.Registry.
- The manager serves exactly this registry: `pkg/metrics/server/server.go:225` `handler := promhttp.HandlerFor(metrics.Registry, s.handlerOpts())` mounted at `/metrics` (`defaultMetricsEndpoint`).
- `/metrics` cannot be overridden (`server.go` NewServer: "overriding builtin /metrics endpoint is not allowed"; same in `AddExtraHandler`). If you truly need a second registry, serve it on another path via `Options.ExtraHandlers` / `mgr.AddMetricsServerExtraHandler(path, h)` (`pkg/manager/manager.go:69-76`) [V mechanism; the idea itself is my suggestion, U]. Note ExtraHandlers also go through the FilterProvider (authn/authz) (`server.go` Start loop).
- Registry init also installs Go collector + process collector: `pkg/internal/controller/metrics/metrics.go:95-105` (`collectors.NewProcessCollector`, `collectors.NewGoCollector(WithGoCollectorRuntimeMetrics(MetricsAll))`) [V]. Do not register them again (duplicate -> MustRegister panic).

### 1.2 Registering custom collectors: init() vs SetupWithManager [V + U]
- Every CR built-in registers in package `init()` with `metrics.Registry.MustRegister(...)` (internal/controller/metrics/metrics.go:94; workqueue.go:103; webhook/internal/metrics/metrics.go:68; client/consistency.go:59; certwatcher/metrics/metrics.go:41). CAPI does the same for all its own metrics (`controllers/clustercache/metrics.go:25-29`, `internal/util/ssa/metrics.go:24-28`, `util/controller/metrics.go:72-74`) [V].
- Kubebuilder book says the same ("declare as package-level variables, register in init()") [V] https://raw.githubusercontent.com/kubernetes-sigs/kubebuilder/master/docs/book/src/reference/metrics.md
- Why init(): `MustRegister` panics on duplicate registration. `SetupWithManager` can be called twice in one test binary (envtest suites, multiple managers), and Flux documents exactly this hazard: `MustMakeRecorder` "panics upon the first registration that causes an error. Which usually happens if you try to initialise a Metrics value twice" [V] https://raw.githubusercontent.com/fluxcd/pkg/main/runtime/metrics/recorder.go
- If metrics must be constructed with runtime dependencies (e.g. a collector that needs `mgr.GetCache()`), register inside `SetupWithManager` guarded by `sync.Once`, or use `Registry.Register` and tolerate `prometheus.AlreadyRegisteredError`. CAPI's informer metrics use `registerOnce sync.Once` (`util/controller/informer.go:94-104`) [V]. cert-manager registers collectors lazily in `NewServer` because listers are only available then [V metrics.go:194-240] https://raw.githubusercontent.com/cert-manager/cert-manager/master/pkg/metrics/metrics.go

Minimal pattern (shape matches CAPI/CR; not compiled by me, [U] as a snippet):
```go
var reconcilePhaseDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
    Namespace: "myoperator", Subsystem: "widget", Name: "phase_duration_seconds",
    Help: "Time spent in a reconcile phase.", Buckets: prometheus.DefBuckets,
    NativeHistogramBucketFactor: 1.1, NativeHistogramMaxBucketNumber: 100, NativeHistogramMinResetDuration: time.Hour,
}, []string{"controller", "phase"})
func init() { metrics.Registry.MustRegister(reconcilePhaseDuration) }
```

### 1.3 Metrics server options [V] `pkg/metrics/server/server.go:46-110`
```go
var DefaultBindAddress = ":8080"
type Options struct {
    SecureServing  bool                                   // default http
    BindAddress    string                                 // "" -> ":8080"; "0" disables the server entirely
    ExtraHandlers  map[string]http.Handler                // e.g. pprof; also wrapped by FilterProvider
    FilterProvider func(c *rest.Config, httpClient *http.Client) (Filter, error)
    CertDir, CertName, KeyName string                      // default <tmp>/k8s-metrics-server/serving-certs, tls.crt, tls.key
    TLSOpts        []func(*tls.Config)
    HandlerOpts    []func(*promhttp.HandlerOpts)          // override promhttp options (default ErrorHandling = HTTPErrorOnError)
    ListenConfig   net.ListenConfig
}
```
- Set via `ctrl.Options{Metrics: metricsserver.Options{...}}` (`pkg/manager/manager.go:234-235`). `BindAddress: "0"` -> `NewServer` returns `nil, nil` (no server) - CAPI envtest uses this (`internal/test/envtest/environment.go:391-393`).
- Default bind is `:8080` over plain HTTP, unauthenticated. Kubebuilder scaffolds `:8443` + `SecureServing: true` + `FilterProvider: filters.WithAuthenticationAndAuthorization`.
- Secure serving cert logic (`createListener`): if `TLSOpts` sets `GetCertificate` use it; else if `CertDir/CertName` and `KeyName` both exist use a `certwatcher` (hot reload; emits `certwatcher_read_certificate_total/_errors_total`); else generates a SELF-SIGNED cert (`certutil.GenerateSelfSignedCertKeyWithFixtures("localhost", ...)`). Self-signed is fine for dev, scrapers then need `insecureSkipVerify` or a CA - use cert-manager/CertDir for prod (kubebuilder doc [V]).
- `NextProtos: []string{"h2"}` is default; CAPI/kubebuilder harden through `TLSOpts` (min version, cipher suites; CAPI `util/flags/manager.go:95-145`) [V]. Kubebuilder scaffolds disabling HTTP/2 for CVE reasons [U - not re-verified here].
- The server `NeedLeaderElection() == false`: serves metrics on non-leaders too (server.go). So per-replica metrics exist on standbys; gauges about "objects" computed from the cache are emitted by every replica unless you gate them (see 3.5) [V + U].

### 1.4 Secure metrics: authn/authz filter and RBAC [V] `pkg/metrics/filters/filters.go:41-130`
Doc comment (verbatim):
```
// For the authentication and authorization the controller needs a ClusterRole with the following rules:
// * apiGroups: authentication.k8s.io, resources: tokenreviews, verbs: create
// * apiGroups: authorization.k8s.io, resources: subjectaccessreviews, verbs: create
// To scrape metrics e.g. via Prometheus the client needs a ClusterRole with the following rule:
// * nonResourceURLs: "/metrics", verbs: get
// Note: ... this metrics provider will introduce a dependency to "k8s.io/apiserver" to your go module.
```
Behaviour: anonymous disabled; TokenReview via `authenticatorfactory.DelegatingAuthenticatorConfig` (cache 1 min); SubjectAccessReview via `DelegatingAuthorizerConfig` (allow cache 5 min, deny 30 s); `AttributesRecord{User, Verb: lower(req.Method), Path: req.URL.Path}` - so the scraper needs `get` on the **nonResourceURL of the exact path** (`/metrics`, and also each ExtraHandler path such as `/debug/pprof/` if exposed). Responses: 500 on authn error, 401 not authenticated, 403 denied. Hardcoded TTLs/timeouts, no `alwaysAllow`, and metrics depend on apiserver reachability (kubebuilder doc [V]).

RBAC, verbatim from kubebuilder scaffold (https://raw.githubusercontent.com/kubernetes-sigs/kubebuilder/master/docs/book/src/cronjob-tutorial/testdata/project/config/rbac/metrics_auth_role.yaml, metrics_reader_role.yaml) [V]:
```yaml
kind: ClusterRole  # metrics-auth-role -> bind to the controller's ServiceAccount
rules:
- {apiGroups: [authentication.k8s.io], resources: [tokenreviews], verbs: [create]}
- {apiGroups: [authorization.k8s.io], resources: [subjectaccessreviews], verbs: [create]}
---
kind: ClusterRole  # metrics-reader -> bind to the Prometheus ServiceAccount (kubebuilder deliberately ships NO binding)
rules:
- {nonResourceURLs: ["/metrics"], verbs: ["get"]}
```
These rules are NOT produced by `+kubebuilder:rbac` markers for controller-gen unless you write them (`// +kubebuilder:rbac:groups=authentication.k8s.io,resources=tokenreviews,verbs=create` etc.) [U: markers are standard but I did not run controller-gen here].

ServiceMonitor (kubebuilder scaffold, https://raw.githubusercontent.com/kubernetes-sigs/kubebuilder/master/docs/book/src/cronjob-tutorial/testdata/project/config/prometheus/monitor.yaml) [V]:
```yaml
spec:
  endpoints:
  - path: /metrics
    port: https            # must match the Service port NAME
    scheme: https
    bearerTokenFile: /var/run/secrets/kubernetes.io/serviceaccount/token
    tlsConfig: {insecureSkipVerify: true}   # "not recommended for production" per the file's own comment
  selector: {matchLabels: {control-plane: controller-manager, app.kubernetes.io/name: project}}
```
Needs: a Service exposing the metrics port, Prometheus Operator CRDs, and Prometheus SA bound to metrics-reader. Optional NetworkPolicy restricts source namespaces but gives no authn (kubebuilder doc [V]).

CAPI's convention [V `util/flags/manager.go:128-148`]: `--diagnostics-address` default; unless `--insecure-diagnostics`, serve HTTPS + `FilterProvider: filters.WithAuthenticationAndAuthorization`, and ONLY then expose pprof + `/debug/flags/v` ExtraHandlers (because endpoint is protected). Good rule for the skill: never put ExtraHandlers on an unauthenticated metrics server.

### 1.5 Built-in metrics (names, labels, types) [V]
All from CR v0.25.2 source.

| Metric | Type | Labels | Source |
|---|---|---|---|
| `controller_runtime_reconcile_total` | counter | controller, result (`success`,`error`,`requeue`,`requeue_after`) | internal/controller/metrics/metrics.go:35; result consts internal/controller/controller.go:~445 |
| `controller_runtime_reconcile_errors_total` | counter | controller | metrics.go:43 |
| `controller_runtime_terminal_reconcile_errors_total` | counter | controller | metrics.go:49 (incremented when `errors.Is(err, reconcile.TerminalError(nil))`) |
| `controller_runtime_reconcile_panics_total` | counter | controller | metrics.go:55 |
| `controller_runtime_reconcile_timeouts_total` | counter | controller | metrics.go (only when the wrapper `ReconciliationTimeout` fires) |
| `controller_runtime_reconcile_time_seconds` | histogram (classic 5ms..60s buckets + native) | controller | metrics.go:63; observed in `reconcileHandler` defer, controller.go:`updateMetrics` |
| `controller_runtime_max_concurrent_reconciles` | gauge | controller | metrics.go; set in `initMetrics` to `MaxConcurrentReconciles` |
| `controller_runtime_active_workers` | gauge | controller | `Add(1)` / deferred `Add(-1)` around processing |
| `workqueue_depth` | gauge | name, controller, priority | internal/metrics/workqueue.go:46 (priority label is new vs older versions) |
| `workqueue_adds_total`, `workqueue_retries_total` | counter | name, controller | ibid. (retries = adds with non-zero delay, includes RequeueAfter) |
| `workqueue_queue_duration_seconds`, `workqueue_work_duration_seconds` | histogram (10ns..1e5 s exponential) | name, controller | ibid. |
| `workqueue_unfinished_work_seconds`, `workqueue_longest_running_processor_seconds` | gauge | name, controller | ibid. |
| `rest_client_requests_total` | counter | code, method, host | client_go_adapter.go (REGISTERED BY DEFAULT) |
| `rest_client_request_duration_seconds`, `rest_client_dns_resolution_duration_seconds`, `rest_client_rate_limiter_duration_seconds`, `rest_client_request_size_bytes`, `rest_client_response_size_bytes`, `rest_client_request_retries_total` | hist/hist/hist/hist/hist/counter | verb, host (retries: code, verb, host) | client_go_adapter.go:130-230 - **OPT-IN in v0.25.2** via `metrics.RegisterRESTClientMetrics(metrics.MetricRequestLatency, ...)` / `RegisterRESTClientMetricsWithOptions(RESTClientMetricsOptions{DurationBuckets: ...}, ...)` |
| `controller_runtime_webhook_latency_seconds` (hist), `_requests_total` (code), `_requests_in_flight` | | webhook (= path), code | webhook/internal/metrics/metrics.go:30-68 |
| `controller_runtime_webhook_panics_total`, `controller_runtime_conversion_webhook_panics_total` | counter | - | webhook/admission/metrics, webhook/conversion/metrics |
| `controller_runtime_client_consistency_wait_duration_seconds` | histogram | operation (`get`/`list`), result | pkg/client/consistency.go:46-56 - only populated when `Cache.EnableReadYourWritesConsistency` is true (default false, client.go:263) = the stale-cache/read-your-writes wait signal |
| `leader_election_master_status` | gauge | name | pkg/metrics/leaderelection.go:29 |
| `certwatcher_read_certificate_total`, `certwatcher_read_certificate_errors_total` | counter | - | certwatcher/metrics |
| process_* / go_* | | | collectors (1.1) |

Quirks found [V]:
- `leader_election_slowpath_total` is defined and incremented (`leaderSlowpathCounter`, leaderelection.go:34,62) but only `leaderGauge` is passed to `Registry.MustRegister` (leaderelection.go:41) -> that counter is never exposed in v0.25.2. Do not build alerts on it.
- Counters are pre-initialised to 0 per controller in `initMetrics` (controller.go:451-461), so `rate()` / `absent()` work from the first scrape. Do the same for your own labelled series with a bounded known label set (Prometheus "initialize to 0" advice [V]; CAPI copies this: `util/controller/builder.go:277-283`).
- `reconcile_total{result="requeue_after"}` and `workqueue_retries_total` both count intentional `RequeueAfter` polling, so a controller that polls will show large "requeue_after"; and `reconcile_total`/`reconcile_time_seconds` also include rate-limited retries. CAPI added its own `capi_reconcile_total` / `capi_reconcile_time_seconds` explicitly because the CR ones count rate-limiting requeues (comment at `util/controller/metrics.go:30-33`) [V].
- Cleanup: CR never calls `DeleteLabelValues` for `controller`-labelled series (grep over pkg: none) - fine because controller names are a small static set. `Named()` doc: name "shows up in metrics ... should be a prometheus compatible name (underscores and alphanumeric characters only)... must be unique" (`pkg/builder/controller.go:246-253`); uniqueness validated unless `SkipNameValidation` (`pkg/config/controller.go:29`) [V]. Use the same `controller` label name (value = controller name) in your own metrics so dashboards can join.

### 1.6 Cardinality guidance specific to CR [V]
Built-ins only label by controller name / HTTP verb+host+code / webhook path - all small static sets. `rest_client_*` use `host` not full URL (client_go_adapter.go: `h.WithLabelValues(verb, u.Host)`) precisely to bound cardinality. Follow that: label with kinds, phases, enumerated result strings; never name/UID/error text.

---------------------------------------------------------------------------
## 2. Naming / label / type conventions

### 2.1 Prometheus naming [V] https://prometheus.io/docs/practices/naming/
- Name: lowercase snake_case; first word = application/domain prefix (`namespace` in client_golang); single unit per metric; base units (seconds, bytes, meters; ratio 0-1, not percent); name ends with unit plural (`..._seconds`, `..._bytes`); counters end `_total` (`process_cpu_seconds_total`: `_total` AFTER unit); `_info` for metadata pseudo-metrics (value 1); timestamps `..._timestamp_seconds`.
- "Do not put the label names in the metric name."; a metric should mean the same thing across all label values (sum/avg over all dimensions should be meaningful - don't mix capacity and length).
- "Every unique combination of key-value label pairs represents a new time series"; do not use user IDs, emails, unbounded sets.
### 2.2 Prometheus instrumentation practices [V] https://prometheus.io/docs/practices/instrumentation/
- Keep label cardinality "below 10"; 100+ (or potential to grow to 100) needs a different approach; most metrics should have no labels; start unlabeled.
- Failure counters: count failures AND total attempts so the ratio is trivial.
- Export 0 for series that could exist; labelled series must be initialised by you.
- Export the Unix timestamp of an event, not "time since" (`time() - metric` in PromQL) - applies to e.g. `..._last_success_timestamp_seconds` and cert expiry.
- Instrument everything cheap; cache `WithLabelValues`/`With` lookups only in >100k/s hot loops.
### 2.3 Kubernetes instrumentation conventions [V] https://raw.githubusercontent.com/kubernetes/community/master/contributors/devel/sig-instrumentation/metric-instrumentation.md
- `<component_name>_<metric>` prefix + Prometheus practices; kubectl resource spelling (`daemonset`, not `daemon_set`).
- Labels: minimal set; extra metadata goes in an always-1 `_info` metric (e.g. `kube_pod_info`); label named after the resource (`pod`, `deployment`), not `pod_name`; namespace in `namespace`. Object-state metrics: `kube_<kind>_<metric>`, and CRs add API group to avoid collisions.
- "Know a comprehensive list of all possible values for a label at instrumentation time"; avoid user IDs/error messages as labels.
- Stability: ALPHA -> BETA needs clear use-case, naming compliance, bounded cardinality, a behaviour test (not just "Inc() changes value"), clear help, SIG review; STABLE needs >=2 releases at BETA. There is no rule about deleting series for deleted objects in this doc.
### 2.4 component-base vs plain client_golang [V + U]
- `k8s.io/component-base/metrics` `opts.go` defines `StabilityLevel` (`INTERNAL`, `ALPHA` (default when empty), `BETA`, `STABLE`) + `DeprecatedVersion`, `LabelValueAllowLists`; it annotates Help with `[ALPHA]`/`(Deprecated since ...)`. It registers on `legacyregistry`/`KubeRegistry`, not on the CR registry. [V https://raw.githubusercontent.com/kubernetes/component-base/master/metrics/opts.go lines 74-100]
- CR deliberately COPIES the component-base workqueue/leaderelection/clientgo adapters to register on its own registry instead ("copied and adapted ... which registers metrics to the k8s legacy Registry. We require very similar functionality, but must register metrics to a different Registry" `pkg/metrics/leaderelection.go:22-24`, `pkg/internal/metrics/workqueue.go:32-34`) [V]. CR marks component-base only as `// indirect` in go.mod (line 104) [V].
- CAPI (a large pure-CR project) uses plain `prometheus.NewXxxVec` + `ctrlmetrics.Registry` everywhere and has no import of `component-base/metrics` (grep: no hits) [V]. Same for cert-manager, Flux, Argo CD, Kong KIC samples below [V].
- Recommendation for the skill: use plain client_golang + `metrics.Registry`. The stability-level machinery only pays off if you adopt k8s static metric-stability checking, which is tied to kubernetes/kubernetes tooling [U]. Document stability in the Help text/README instead (e.g. "alpha, may change") and keep names+labels stable in practice; renaming a metric or label is a breaking change for dashboards/alerts.
- Don't create `prometheus.NewRegistry()` of your own for the manager; don't use `prometheus.MustRegister` (default registry).

---------------------------------------------------------------------------
## 3. What to measure in a controller

Principle (CAPI proposal, https://... `docs/proposals/20220411-cluster-api-state-metrics.md`): process-health/performance metrics belong in the controller; "state of the object" metrics are better produced outside the controller [V partially, see 3.6].

### 3.1 Reconcile outcome by result
CR already gives success/error/requeue/requeue_after/terminal/panic/timeout per controller (1.5). Custom additions only if you need semantic outcomes the framework cannot see (e.g. `result="waiting_for_dependency"` vs `"noop"`). CAPI wraps the Reconciler to emit `capi_reconcile_total{controller,result}` excluding rate-limit-induced retries, using the same 4 result strings and pre-initialised zeros (`util/controller/controller.go:96-130`, `builder.go:277-283`) [V]. Excerpt:
```go
switch {
case err != nil:                 reconcileTotal.WithLabelValues(r.name, labelError).Inc()
case result.RequeueAfter > 0:    reconcileTotal.WithLabelValues(r.name, labelRequeueAfter).Inc()
case result.Requeue:             reconcileTotal.WithLabelValues(r.name, labelRequeue).Inc()
default:                         reconcileTotal.WithLabelValues(r.name, labelSuccess).Inc()
}
```
Do not duplicate CR's metrics unless you have a documented reason like CAPI's.
### 3.2 Phase durations
Histogram `<app>_<kind>_reconcile_phase_duration_seconds{controller,phase}` with a CLOSED enum of phases (e.g. `fetch`,`finalize`,`provision`,`status_update`) [U design suggestion consistent with the cardinality rules in 2.x]. Use `prometheus.NewTimer(obs)` / `defer`. Copy CR's bucket style incl. `NativeHistogram*` options (CR `internal/controller/metrics/metrics.go:63-72`, CAPI `util/controller/metrics.go:47-55`) [V] so both classic and native histograms work.
### 3.3 External API call latency / errors / rate-limit hits (the highest-value custom metrics)
Model after client-go/CR REST metrics and CAPI runtime SDK: counter by status + histogram by operation, labels = `host`/`operation`/`code` only.
- CAPI `internal/runtime/metrics/metrics.go:44-70` [V]: `capi_runtime_sdk_requests_total{code,host,group,version,hook,status}`, `capi_runtime_sdk_request_duration_seconds{host,group,version,hook}`; note its anti-cardinality comment: "Errors can be arbitrary strings. Unbound label cardinality is not suitable for a metric system so they are reported as `<error>`" and `code := "<error>"` when `err != nil` (metrics.go:~74).
- cert-manager: `certmanager_http_acme_client_request_count{scheme,host,action,method,status}` + `_duration_seconds` summary, `venafi_client_request_duration_seconds{api_call}` [V metrics.go:83-135].
- Kong KIC: `configPushCount{success,protocol,failure_reason,dataplane,instance_id}` with `failure_reason` constrained to enum {conflict,network,other}, `success` as "true"/"false" strings [V https://raw.githubusercontent.com/Kong/kubernetes-ingress-controller/main/internal/metrics/prometheus.go:29-60,105-130].
- Rate limiting: enable CR's opt-in `rest_client_rate_limiter_duration_seconds` (`RegisterRESTClientMetrics(metrics.MetricRateLimiterLatency, metrics.MetricRequestLatency)`) for the Kubernetes API [V]; CAPI's tuning doc notes the absence of a client-go rate-limit metric made throttling invisible (only "client-side throttling" log lines): `docs/book/src/developer/core/tuning.md:26,59` [V]. For third-party APIs, count HTTP 429 as its own `code`/`result="rate_limited"` and add a `*_retries_total`.
- Map errors to a small enum (`timeout`, `auth`, `rate_limited`, `not_found`, `conflict`, `server_error`, `other`) before using as label.
### 3.4 Time-to-ready, finalizer/deletion durations
Histogram with wide buckets: `<app>_<kind>_ready_duration_seconds` observed once on the transition NotReady->Ready (use condition `lastTransitionTime - creationTimestamp`), plus `..._deletion_duration_seconds` observed when the finalizer is removed (`now - deletionTimestamp`). Labels: only bounded dimensions (kind/class/provider), never name. [U - design suggestion, no upstream example read]. Caveats: observe exactly once (guard on the status transition; idempotent reconcilers re-run), and buckets must span the real range (Flux uses `ExponentialBucketsRange(10e-3, 1800, 10)` for reconcile duration: 10ms..30min [V recorder.go]). If the number is derivable from conditions/timestamps, kube-state-metrics can export the timestamps (CAPI exports `creationTimestamp` and condition `lastTransitionTime` via KSM: `core/config/metrics/templates/common_metrics.yaml`) and PromQL computes the delta [V].
### 3.5 Resource-state gauges (objects by phase/condition)
Two valid in-process shapes:
a) Scrape-time custom `prometheus.Collector` reading the informer cache, emitting `MustNewConstMetric` per object/aggregate. The series set always equals the current cache => no stale series, no delete bookkeeping. cert-manager `internal/collectors/certificate_collector.go` [V]: `Collect` lists via lister, emits `certmanager_certificate_ready_status{name,namespace,condition,issuer_name,issuer_kind,issuer_group}` with one series per condition value (1 for current, 0 for others), `..._not_after_timestamp_seconds`, `..._renewal_timestamp_seconds`. Argo CD does the same with `appCollector` (`controller/metrics/metrics.go:429-480`, `argocd_app_info{... sync_status,health_status,phase ...}`) [V]. For CR: pass `mgr.GetCache()`/client into the collector, `Collect` does `List` from cache (cheap, no apiserver call); register after manager creation (`metrics.Registry.MustRegister(collector)` inside setup, guarded by Once). Prefer AGGREGATES (count by phase/condition) over per-object series: `myoperator_widgets{phase="Ready"} 12`. Every replica (incl. non-leaders) serves it (server NeedLeaderElection=false) so aggregate with `max by()` or filter by leader [U].
b) Event-driven `GaugeVec` updated in Reconcile - simple but you must delete series on object deletion/finalization (see 4.3); stale series are the #1 bug here.
### 3.6 In-process vs kube-state-metrics CustomResourceState
kube-state-metrics (KSM) Custom Resource State [V] https://raw.githubusercontent.com/kubernetes/kube-state-metrics/main/docs/metrics/extend/customresourcestate-metrics.md:
- Config-only (no code): `--custom-resource-state-config` / `--custom-resource-state-config-file`, YAML `kind: CustomResourceStateMetrics`, `spec.resources[].groupVersionKind`, `metricNamePrefix`, `labelsFromPath`, `metrics[].each.type` in {`Gauge`,`StateSet`,`Info`}; `valueFrom`; `nilIsZero`; RBAC: KSM needs list/watch on CRDs (`customresourcedefinitions.apiextensions.k8s.io`) and on each resource.
- Gauge converts bool/number/"true"/"false"/"unknown"/RFC3339 timestamp/quantity. StateSet emits one series per list entry (1 for the match, 0 else). Info always 1, data in labels. Reserved labels `customresource_group/_version/_kind` are set by KSM.
- STATUS WARNING [V, top of the page]: "`custom-resource-state` is feature-frozen in favor of resource-state-metrics (kubernetes-sigs/resource-state-metrics). Once resource-state-metrics is stable, custom-resource-state will be deprecated." The skill should mention this; as of this research it is still the practical option (CAPI and Flux use it).
- Example conditions config as used by CAPI (`core/config/metrics/templates/common_metrics.yaml`) [V]: `status_condition` as StateSet over `status.conditions` with `labelsFromPath: {type: [type]}`, `labelName: status`, `list: ['True','False',Unknown]`, `valueFrom: [status]`; `status_condition_last_transition_time` as Gauge on `lastTransitionTime`; `annotation_paused` as Info; `created` Gauge on `metadata.creationTimestamp`. Per-resource common labels there include name, namespace, uid (KSM is the exception to the "no name label" rule since it is object-state by definition).
- CAPI ships `core/config/metrics/crd-metrics-config.yaml` + `crd-clusterrole.yaml` (ClusterRole labelled `kube-state-metrics/aggregate-to-manager: "true"`, get/list/watch on each CR) and observability manifests `hack/observability/kube-state-metrics/*` [V]. Its generator was a controller-gen fork ("Generated by controller-gen version v0.0.0-00010101000000-000000000000", header of crd-metrics-config.yaml); `make generate-metrics-config` is currently a stub that exits with "Use PR state from https://github.com/kubernetes-sigs/cluster-api/pull/9347" (Makefile:616-619) - i.e. there is NO supported controller-gen generator for KSM config; hand-write or template it [V]. README: "Disclaimer: temporary workaround. The long-term goal is to generate metric configuration from API type markers." [V `core/config/metrics/templates/README.md`]
- CAPI's own rationale: proposal `docs/proposals/20220411-cluster-api-state-metrics.md` "Expose metrics by the controllers" alternative rejected because it needs changes in every controller/provider and "potential bugs of the metrics implementation may also have a negative impact on provisioning" [V lines 232-244].
- Flux: docs state the KSM-generated `gotk_resource_info` is "not exported by the Flux controllers themselves"; reasons: controller metrics target Flux admins, adding informational labels in-controller "may increase the cardinality", KSM can add custom labels (team, status values) [V https://fluxcd.io/flux/monitoring/metrics/ and /custom-metrics/]. Flux still emits `gotk_reconcile_duration_seconds`, `gotk_cache_events_total`, token-cache metrics from the controller (performance/internal) [V].

Decision rule for the skill:
| Need | Use |
|---|---|
| Object state derivable from spec/status/metadata (ready condition, phase, paused, generation vs observedGeneration, timestamps, version info) | KSM CustomResourceState (zero code, no stale series, decoupled from controller crashes, per-object labels acceptable there) |
| Behaviour of the controller (reconcile outcome/duration, queue, external API calls, retries, rate limits, cache-staleness waits, internal caches, leader) | In-process client_golang metrics on `metrics.Registry` |
| Derived values that need data not in CR status (external system state, computed aggregates) | In-process scrape-time Collector (3.5a) |
| User wants dashboards without installing KSM | In-process aggregate collector (bounded labels) |
Both are complementary; ship a KSM config snippet + ClusterRole when your CRD has conditions [U recommendation].
### 3.7 Stale-cache / expectation waits / internal caches
- CR: `controller_runtime_client_consistency_wait_duration_seconds{operation,result}` (opt-in read-your-writes) [V].
- CAPI: `capi_reconcile_stale_cache_skips_total{controller,cached_kind}` incremented when reconcile is skipped because the cache lags own writes (`util/controller/controller.go:76`); `capi_ssa_cache_hits_total/_misses_total{kind,controller}` (`internal/util/ssa/metrics.go`); `informer_queued_items`, `informer_processing_latency_seconds`, `informer_store_resource_version{name,group,version,resource}` (`util/controller/informer.go:60-92`, "initially copied from component-base fifo metrics") [V].
- Queue-depth-ish signals: use built-in `workqueue_depth{name}`, `workqueue_queue_duration_seconds`, `workqueue_longest_running_processor_seconds`, and `active_workers/max_concurrent_reconciles` saturation (kubebuilder metrics-reference lists exactly this PromQL idea: `controller_runtime_active_workers / controller_runtime_max_concurrent_reconciles >= 1`) [V https://raw.githubusercontent.com/kubernetes-sigs/kubebuilder/master/docs/book/src/reference/metrics-reference.md]. Custom ones only for domain queues (e.g. outstanding external operations).
- Connection health to remote clusters/systems: CAPI `capi_cluster_cache_connection_up{cluster_name,cluster_namespace}`, `capi_cluster_cache_healthcheck`, `capi_cluster_cache_healthchecks_total{...,status}` (`controllers/clustercache/metrics.go`) [V].

---------------------------------------------------------------------------
## 4. Real examples, anti-patterns, cleanup

### 4.1 Survey [V unless noted]
| Project | Where | Style |
|---|---|---|
| controller-runtime | `pkg/internal/controller/metrics`, `pkg/metrics/*`, `pkg/client/consistency.go` | init() + `metrics.Registry`; labels controller/verb/host/code |
| cluster-api | `util/controller/metrics.go`, `controllers/clustercache/metrics.go`, `internal/util/ssa/metrics.go`, `internal/runtime/metrics/metrics.go`, `util/controller/informer.go`, `test/infrastructure/inmemory/pkg/server/{api,etcd}/metrics.go` | all `ctrlmetrics.Registry.MustRegister` in `init()`, `capi_` prefix; Counter/Gauge/HistogramVec; native histograms; explicit delete for per-cluster series |
| cluster-api-provider-vsphere | `main.go:604-612` | NO custom prometheus metrics (grep `NewCounterVec|NewGaugeVec|NewHistogramVec|MustRegister` over repo excluding vendor: only hit is `hack/tools/cleanup-scale-env`); only wires `capiflags.GetManagerOptions` -> `managerOpts.Metrics`. So a provider can lean 100% on CR built-ins + KSM. |
| cert-manager | `pkg/metrics/metrics.go`, `internal/collectors/*` | Own registry + own HTTP server (NOT CR registry), namespace `certmanager`, scrape-time collectors from listers for object state; `controller_sync_call_count{controller}`/`_error_count` (note non-standard `_count` instead of `_total` - legacy naming, don't copy) |
| Flux (fluxcd/pkg runtime/metrics) | `recorder.go` | `crtlmetrics.Registry`; `gotk_reconcile_condition{kind,name,namespace,type,status}`, `gotk_suspend_status`, `gotk_reconcile_duration_seconds{kind,name,namespace}` with explicit `DeleteCondition/DeleteSuspend/DeleteDuration` (labels incl. name+namespace => per-object cardinality, mitigated by deletes; the docs now point users to KSM `gotk_resource_info` for object state) |
| Argo CD | `controller/metrics/metrics.go` | own registry; scrape-time `appCollector` for `argocd_app_info`; per-app labels on `argocd_app_k8s_request_total{...,server,response_code,verb,resource_kind,resource_namespace,dry_run}` = large cross product (my judgement [U] this is the cardinality hazard; Argo mitigates with label allow-lists/flags [U not verified]) |
| Kong KIC | `internal/metrics/prometheus.go:423` | `metrics.Registry.MustRegister`; domain metrics on config push/translation with bounded enum labels (`success`, `protocol`, `failure_reason`, `dataplane`) behind a `Recorder` interface (mockable) |
| Envoy Gateway | `internal/message/metrics.go` | domain-queue metrics with enum labels (`runner`,`message`,`event_type`); OpenTelemetry-based wrapper [V names only]; per-object labels not used |
Gateway API itself (kubernetes-sigs/gateway-api) ships CRDs only; for Gateway API *implementations* the spec asks you to report in `status.conditions` (+`observedGeneration`) and the common convention is to expose state via KSM/conditions rather than controller metrics [U - not verified; the examples above were the only implementations read].

### 4.2 Registration shape in CAPI (copy-ready) [V `controllers/clustercache/metrics.go`]
```go
func init() {
	// Register the metrics at the controller-runtime metrics registry.
	ctrlmetrics.Registry.MustRegister(healthCheck)
	ctrlmetrics.Registry.MustRegister(connectionUp)
}
var healthCheck = prometheus.NewGaugeVec(prometheus.GaugeOpts{
	Name: "capi_cluster_cache_healthcheck", Help: "Result of the last clustercache healthcheck for a cluster.",
}, []string{"cluster_name", "cluster_namespace"})
```
### 4.3 Cleanup of per-object series [V]
- CAPI: `controllers/clustercache/cluster_cache.go:641-646`
```go
func (cc *clusterCache) cleanupMetricsForCluster(cluster client.ObjectKey) {
	healthCheck.DeleteLabelValues(cluster.Name, cluster.Namespace)
	connectionUp.DeleteLabelValues(cluster.Name, cluster.Namespace)
	healthChecksTotal.DeleteLabelValues(cluster.Name, cluster.Namespace, "success")
	healthChecksTotal.DeleteLabelValues(cluster.Name, cluster.Namespace, "error")
}
```
  called from `cleanupForCluster` (line ~655) when a Cluster is gone. Note it must enumerate every value of the extra `status` label (`success`,`error`) - with `DeleteLabelValues` you need the full label tuple.
- Flux: `Delete*` methods per metric (recorder.go) called when the object is deleted [V mechanism; call site in controllers not re-read].
- client_golang v1.24.0 API [V `prometheus/vec.go:74-130`]: `DeleteLabelValues(lvs...) bool` (exact tuple, order matters), `Delete(Labels) bool`, `DeletePartialMatch(Labels) int` (deletes all series whose variable labels contain the given ones; "curried labels will never be matched if deleting from the curried vector") - use `DeletePartialMatch(prometheus.Labels{"namespace": ns, "name": name})` to drop all series of an object without enumerating label combos; `Reset()` drops everything. CAPI does not use DeletePartialMatch (grep) [V].
- When to delete: in the reconcile path where `apierrors.IsNotFound` on Get (object gone) and/or when the finalizer is removed. Pitfall [U]: if the delete event happens while the controller is down/not leader the series are never deleted for that process - with in-process per-object GaugeVec this is only "stale until restart"; a scrape-time collector (3.5a) avoids this class of bug entirely.
- Rename/recreate with same name: series reused - OK.
### 4.4 Anti-patterns to put in the skill
1. Labels: object `name`/`uid`, error strings, request paths, free-text reasons, image tags, node IPs -> unbounded series (Prometheus naming/instrumentation pages [V]; K8s doc "know all possible values" [V]; CAPI RTSDK maps errors to `<error>` [V]).
2. Per-object labelled metrics with no delete path (4.3). If per-object labels are essential (Flux/CAPI-clustercache style), cap count (e.g. per-cluster not per-machine), delete on finalize, and document the cardinality = number of CRs.
3. Registering on `prometheus.DefaultRegisterer` (not served) or creating own registry for the manager (not served); or `MustRegister` inside `Reconcile`/`SetupWithManager` without Once (duplicate-registration panic).
4. Duplicating CR's reconcile counters/histograms; use `controller` label + built-ins unless semantic difference documented (CAPI rationale [V]).
5. Using `_count`/no unit/percent/ms (cert-manager legacy `controller_sync_call_count` - [V] shows it, don't copy). Seconds, `_total`, `_bytes`, `_ratio`.
6. Gauge for something that should be a counter (cumulative events) or "seconds since" gauge that goes stale: export timestamps instead (Prometheus [V]).
7. Observing time-to-ready on every reconcile (non-idempotent): guard by transition.
8. Booleans-as-labels `success="true"` are used by KIC [V] but project rules for CRD fields (no booleans) apply to API types, not metric labels [U]; prefer `result="success|error"`-style enums for consistency with `controller_runtime_reconcile_total`.
9. Exposing metrics unauthenticated on 0.0.0.0 with pprof ExtraHandlers.
10. Metrics code that can panic in the reconcile path: CAPI's proposal calls this out as a risk [V]; use `WithLabelValues` only with correct label cardinality (it panics on wrong arity - prefer `GetMetricWithLabelValues` in dynamic code [V API exists in client_golang; U on preference]).

---------------------------------------------------------------------------
## 5. Testing metrics

Verified patterns:
- Counter delta with `promtestutil`: CR `pkg/certwatcher/certwatcher_test.go:190-216` [V]
```go
import "github.com/prometheus/client_golang/prometheus/testutil"
before := testutil.ToFloat64(metrics.ReadCertificateTotal)
Expect(watcher.ReadCertificate()).To(Succeed())
Expect(testutil.ToFloat64(metrics.ReadCertificateTotal)).To(Equal(before + 1))
```
  Always compare deltas (global registry shared across specs). `ToFloat64` takes a single-series collector, so for a Vec use `vec.WithLabelValues(...)` (a Counter/Gauge is itself a Collector).
- Registry-level gather in envtest: CAPI `controllers/clustercache/cluster_cache_test.go:839-866` `getCounterMetric` iterates `metrics.Registry.Gather()` and matches family name + `controller` label to read `controller_runtime_reconcile_total` etc. [V]; CAPI envtest `verifyPanicMetrics` (`internal/test/envtest/environment.go:758-790`) fails the whole suite if `controller_runtime_reconcile_panics_total`, `controller_runtime_webhook_panics_total` or conversion-webhook panics are > 0 - a cheap, valuable guard to copy [V].
- Opt-in REST metrics test: `pkg/metrics/client_go_adapter_test.go:40-100` gathers names via `Registry.Gather()` [V].
- Envtest manager: set `Metrics: metricsserver.Options{BindAddress: "0"}` to avoid port clashes (CAPI environment.go:391) [V]; to test the scrape path itself use `BindAddress: "127.0.0.1:0"` + `testutil.ScrapeAndCompare(url, ...)` [U as a recipe; V that pieces exist: client_golang `testutil.ScrapeAndCompare` testutil.go:166, and CR's server keeps the resolved listener address in `defaultServer.bindAddr` exposed by `GetBindAddr()` server.go:348 - but that method is on the unexported type, so from user code you'd need to pick a free port yourself]. CR's own `server_test.go` only tests `handlerOpts()` (`HandlerOpts` can set `EnableOpenMetrics`, `ErrorHandling`) [V].
- Other helpers in client_golang v1.24.0 `prometheus/testutil/testutil.go` [V]: `CollectAndCount(c, names...)`, `GatherAndCount`, `CollectAndCompare(c, expected io.Reader, names...)` (golden text exposition), `GatherAndCompare`, `ScrapeAndCompare`; `testutil/lint.go`: `CollectAndLint`/`GatherAndLint` (promlint: checks `_total` for counters, units, help text) - cheap unit test to enforce naming conventions; K8s doc says tests should be behaviour-based, not "Inc() changes value" [V].
- For unit tests without the global registry, construct metrics in a struct (`NewMetrics(reg prometheus.Registerer)`) and pass `prometheus.NewRegistry()` in tests; production passes `metrics.Registry` (Kong's `Recorder` interface pattern, [V]); avoids duplicate-registration panics in tests [U].
- For scrape-time collectors: `testutil.CollectAndCompare(collector, strings.NewReader(expected))` with a fake client/cache populated [U, standard client_golang API].

---------------------------------------------------------------------------
## 6. Quick checklist for the skill (derived)
1. Metrics package `internal/metrics` (or `pkg/metrics`) with `var x = prometheus.NewXxxVec(...)` + `func init(){ metrics.Registry.MustRegister(...) }`.
2. Name: `<app>_<subsystem>_<what>_<unit>[_total]`; Help always set; seconds/bytes; closed-enum labels; reuse `controller` label name.
3. Don't duplicate CR built-ins; enable opt-in `RegisterRESTClientMetrics(...)` (request latency, rate limiter) in `main`.
4. External calls: counter(`code|result`,`operation`) + histogram(`operation`); error -> enum.
5. Object state: prefer KSM CustomResourceState config (+ ClusterRole) or a scrape-time Collector over event-driven per-object gauges; if per-object gauges, `DeleteLabelValues`/`DeletePartialMatch` on delete/finalize.
6. Secure serving: `SecureServing: true`, `FilterProvider: filters.WithAuthenticationAndAuthorization`, RBAC (tokenreviews+subjectaccessreviews create for the controller SA; nonResourceURLs `/metrics` get for the scraper), real certs via CertDir/cert-manager, ServiceMonitor `scheme: https`; pprof only behind the filter.
7. Tests: delta with `testutil.ToFloat64`, `Registry.Gather()` in envtest, `testutil.CollectAndLint`, envtest `BindAddress:"0"`.
