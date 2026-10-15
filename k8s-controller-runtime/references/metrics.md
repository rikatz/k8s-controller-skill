# Metrics

Read this when you bootstrap the manager's metrics endpoint, add a custom metric, or review one. Logging, events, probes
and the rest of RBAC are in `observability-rbac.md`. Snippets compile against controller-runtime v0.25.2 and
client_golang v1.24.0 (imports omitted). Tags are defined in `sources.md`; `run` = reproduced in `research/experiments`.

## 1. Bootstrap: authenticated and TLS-only

Always serve metrics over TLS with the authn/authz filter. The manager default (`:8080`, plain HTTP, no filter) is not a
setup to ship.

```go
mgr, err := ctrl.NewManager(cfg, ctrl.Options{
	Metrics: metricsserver.Options{
		BindAddress:    ":8443",
		SecureServing:  true,
		FilterProvider: filters.WithAuthenticationAndAuthorization, // sigs.k8s.io/controller-runtime/pkg/metrics/filters
		// CertDir / TLSOpts: provide a real serving cert (section 1.3)
	},
})
```

Verified in an envtest manager running as a ServiceAccount, scraped over HTTPS with `net/http` [CR, run]:

| Caller | Result |
|---|---|
| no token | 401 `Unauthorized` |
| valid SA token, no `get` on `/metrics` | 403 `Authorization denied for user ...` |
| SA token bound to the scraper ClusterRole | 200, includes your custom series |
| scraper OK but the controller SA lacks the review RBAC | 500 `Authentication failed` (fails closed) |

### 1.1 What the filter does
- Every request becomes a TokenReview, then a SubjectAccessReview with `Verb = lowercase(HTTP method)` and
  `Path = req.URL.Path`. So the scraper needs `get` on the nonResourceURL of the exact path. Anonymous access is disabled;
  caches: authn 1 min, authz allow 5 min / deny 30 s (hard-coded). [CR: filters.go:62-117]
- The filter wraps `/metrics` and every `ExtraHandlers` path. It is independent of `SecureServing`. [CR: server.go Start]
- Using it adds a `k8s.io/apiserver` dependency to your module (the doc comment says so). [CR]
- Metrics depend on API server reachability for every scrape. [CR]

### 1.2 RBAC (hand-bind; markers generate the roles)
Controller ServiceAccount (delegated authn/authz) and scraper (Prometheus) are different subjects. Verified by running
`go tool controller-gen rbac:roleName=manager-role paths=./... output:rbac:stdout`: the real marker for a non-resource URL
is `urls=`, and `roleName=` on a marker splits its rule into a separate (Cluster)Role. [CT, run]

```go
// +kubebuilder:rbac:groups=authentication.k8s.io,resources=tokenreviews,verbs=create
// +kubebuilder:rbac:groups=authorization.k8s.io,resources=subjectaccessreviews,verbs=create
// +kubebuilder:rbac:urls=/metrics,verbs=get,roleName=metrics-reader
```

Generated output (the first two join `manager-role`; the third becomes its own ClusterRole): [CT, run]

```yaml
- apiGroups: [authentication.k8s.io]
  resources: [tokenreviews]
  verbs: [create]
- apiGroups: [authorization.k8s.io]
  resources: [subjectaccessreviews]
  verbs: [create]
---
kind: ClusterRole
metadata: {name: metrics-reader}
rules:
- nonResourceURLs: [/metrics]
  verbs: [get]
```

Bind `manager-role` to the controller's ServiceAccount and `metrics-reader` to the scraper's ServiceAccount by hand
(controller-gen emits no bindings). If you expose another path through `ExtraHandlers`, the scraper needs that URL too.
[CR, CT, run]

### 1.3 Certificates and scraping
Without a cert in `CertDir` (defaults `tls.crt`/`tls.key` under `<tmp>/k8s-metrics-server/serving-certs`) or
`TLSOpts` `GetCertificate`, the server generates a self-signed cert: fine for tests (the scrape above skipped
verification), not for production. A cert in `CertDir` is hot-reloaded. [CR]
Prometheus Operator `ServiceMonitor`: `scheme: https`, `bearerTokenFile: /var/run/secrets/kubernetes.io/serviceaccount/token`,
a `tlsConfig` with the CA rather than `insecureSkipVerify` in production. [KB]
`BindAddress: "0"` disables the server (use it in envtest to avoid port clashes). [CR, CAPI]

## 2. Registry and registration

- Use `sigs.k8s.io/controller-runtime/pkg/metrics`.`Registry`, a private `prometheus.NewRegistry()`. The manager serves
  only that registry, so metrics on `prometheus.DefaultRegisterer` never appear. [CR, run]
- Register in package `init()`, like every built-in and CAPI. `MustRegister` panics on a second registration, and
  `SetupWithManager` can run twice in one test binary. [CR, CAPI, run]
- Do not register the Go/process collectors again; the registry already has them. [CR]

## 3. A complete small example

```go
var (
	Phases  = []string{"fetch", "provision", "status_update"} // closed sets
	Results = []string{"success", "timeout", "rate_limited", "other"}

	PhaseDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: "myoperator", Subsystem: "widget", Name: "reconcile_phase_duration_seconds",
		Help: "Time spent in a reconcile phase.", Buckets: prometheus.DefBuckets,
	}, []string{"phase"})
	ExternalRequests = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "myoperator", Subsystem: "widget", Name: "external_requests_total",
		Help: "External API requests by operation and result.",
	}, []string{"operation", "result"})
	// Per-object: cardinality = number of Widgets, so it needs Forget below.
	Ready = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "myoperator", Subsystem: "widget", Name: "ready",
		Help: "Whether a Widget is ready (1) or not (0).",
	}, []string{"namespace", "name"})
)

func init() {
	metrics.Registry.MustRegister(PhaseDuration, ExternalRequests, Ready)
	for _, r := range Results { // pre-create bounded series so rate()/absent() work from the first scrape
		ExternalRequests.WithLabelValues("create", r)
	}
	for _, p := range Phases {
		PhaseDuration.WithLabelValues(p)
	}
}

// Forget drops every series of a deleted object: call it on NotFound or after removing the finalizer.
func Forget(namespace, name string) {
	Ready.DeletePartialMatch(prometheus.Labels{"namespace": namespace, "name": name})
}
```

Delete path: `DeletePartialMatch` removes all series whose labels contain the given pairs, so extra enum labels need no
enumeration. CAPI instead calls `DeleteLabelValues` once per value of every extra label (`success`, `error`); that needs
the full label tuple, in order. Both work. Call it where `Get` returns NotFound and/or when the finalizer is removed;
a delete missed while the process was down is only stale until restart. [PROM, CAPI, run]

## 4. Naming and labels

- `<app>_<subsystem>_<what>_<unit>`: snake_case, base units (`_seconds`, `_bytes`, ratio 0-1 not percent), counters end
  `_total` after the unit, `_info` for metadata gauges, no label names inside the metric name, same meaning across all
  label values. [PROM]
- Use the label name `controller` (value = controller name) when you label by controller, so dashboards join on the
  built-ins. Label by kinds, phases and enumerated results: map errors to a closed enum (`timeout`, `auth`,
  `rate_limited`, `not_found`, `conflict`, `other`) before using them as a label. CAPI's runtime SDK reports any error
  as `<error>` for this reason. [PROM, CR, CAPI]
- Keep label cardinality low (the guide says "below 10" per metric, investigate at 100). Never use object name, UID,
  error text, request paths or image tags unless the metric is per-object by design and has a delete path. [PROM]
- Prefer enum results (`result="success|error"`) over boolean-looking strings, for consistency with
  `controller_runtime_reconcile_total`. [CR]
- Plain client_golang, not `k8s.io/component-base/metrics`: component-base registers on its own legacy registry, and CAPI,
  Flux and cert-manager use plain client_golang. State stability in the Help text. Renaming a metric or label breaks
  dashboards. [CR, CAPI, OSS]
- Export the Unix timestamp of an event (`..._timestamp_seconds`), not "seconds since" gauges that go stale. [PROM]

## 5. What to measure, and what not to duplicate

Already provided by controller-runtime (do not re-implement): `controller_runtime_reconcile_total{controller,result}`,
`_reconcile_errors_total`, `_terminal_reconcile_errors_total`, `_reconcile_panics_total`, `_reconcile_time_seconds`,
`_active_workers`, `_max_concurrent_reconciles`, `workqueue_*`, webhook metrics, `leader_election_master_status`.
[CR]
- Only `rest_client_requests_total` is registered by default among the client-go REST metrics. The latency, rate-limiter,
  size and retry ones are opt-in through `metrics.RegisterRESTClientMetrics(metrics.MetricRequestLatency,
  metrics.MetricRateLimiterLatency, ...)` (v0.25). Call it in `main` before building the manager; each metric is
  enabled once, so repeat calls are harmless. [CR, run]
- `leader_election_slowpath_total` is defined but never registered (only the gauge is): do not alert on it. [CR]
- `reconcile_total` counts rate-limited retries and intentional `requeue_after` polling too. CAPI added its own
  `capi_reconcile_total` for that reason: add one only with a documented semantic difference. [CR, CAPI]

Worth adding in-process (controller behaviour, not object state):

| Signal | Shape |
|---|---|
| Phase duration | histogram `..._phase_duration_seconds{phase}`, closed phase enum |
| External API calls | counter `..._requests_total{operation,result}` + histogram `..._request_duration_seconds{operation}` |
| Retries / rate limiting | `..._retries_total`; HTTP 429 as `result="rate_limited"` |
| Time-to-ready | histogram observed once on the NotReady to Ready transition (guard the transition; reconcile re-runs), wide buckets |
| Stale-cache waits, internal caches | counters as CAPI's `capi_reconcile_stale_cache_skips_total`; CR's own wait metric is only filled when `EnableReadYourWritesConsistency` is on |

The first three rows are [CAPI, PROM]; time-to-ready is a design suggestion without an upstream example [unverified].

## 6. Object state: kube-state-metrics or a scrape-time collector

| Need | Use |
|---|---|
| State derivable from spec/status/metadata (Ready condition, phase, paused, timestamps) | kube-state-metrics custom-resource-state config: no code, no stale series, survives controller crashes |
| Behaviour of the controller (above) | in-process client_golang metrics |
| Aggregates needing data outside the CR | in-process scrape-time `Collector` |

- kube-state-metrics' `custom-resource-state` is feature-frozen in favour of `resource-state-metrics`; it is still what
  CAPI and Flux use. No controller-gen generator exists for its config: CAPI's `generate-metrics-config` target is a stub
  and its README calls the YAML "a temporary workaround". Hand-write the config and a ClusterRole for it. [KSM, CAPI]
- Flux deliberately does not emit per-object info labels from its controllers: cardinality, and KSM can add custom labels.
  [OSS]
- A scrape-time collector reads the cache at scrape and emits `MustNewConstMetric`, so the series always equal the current
  objects and nothing is ever deleted. Prefer aggregates over per-object series. Every replica serves metrics (the
  server does not need leader election), so aggregate with `max by()` or accept duplicates. [CR, OSS]

```go
// A prometheus.Collector needs both methods; MustRegister(collector) will not compile without Describe.
func (c *WidgetCollector) Describe(ch chan<- *prometheus.Desc) { ch <- c.desc }

func (c *WidgetCollector) Collect(ch chan<- prometheus.Metric) {
	var list demov1.WidgetList
	if err := c.Reader.List(context.Background(), &list); err != nil { // c.Reader: mgr.GetCache()
		ch <- prometheus.NewInvalidMetric(c.desc, err)
		return
	}
	counts := map[[2]string]float64{} // zero-fill the closed sets (size x ready) first, then count
	for i := range list.Items {
		w := &list.Items[i]
		ready := "Unknown"
		if cond := meta.FindStatusCondition(w.Status.Conditions, "Ready"); cond != nil {
			ready = string(cond.Status)
		}
		counts[[2]string{string(w.Spec.Size), ready}]++
	}
	for k, v := range counts {
		ch <- prometheus.MustNewConstMetric(c.desc, prometheus.GaugeValue, v, k[0], k[1])
	}
}
```

`desc` is `prometheus.NewDesc("myoperator_widgets", "...", []string{"size", "ready"}, nil)`. Register it after the manager
exists (`metrics.Registry.MustRegister(collector)` during setup), because it needs the cache, so `init()` is not possible;
guard it with `sync.Once` if setup can run twice (CAPI does this for its informer metrics) [CAPI]. The collector and its
test compile and pass against a fake client; wiring it to `mgr.GetCache()` was not run [unverified].

## 7. Testing metrics

```go
It("counts by delta", func() {
	c := obsmetrics.ExternalRequests.WithLabelValues("create", "timeout")
	before := testutil.ToFloat64(c) // takes one series; globals are shared, so compare deltas
	c.Inc()
	Expect(testutil.ToFloat64(c)).To(Equal(before + 1))
})
It("follows naming rules", func() {
	problems, err := testutil.CollectAndLint(obsmetrics.ExternalRequests)
	Expect(err).NotTo(HaveOccurred())
	Expect(problems).To(BeEmpty())
})
It("forgets deleted objects", func() {
	obsmetrics.Ready.WithLabelValues("ns", "a").Set(1)
	obsmetrics.Forget("ns", "a")
	Expect(testutil.CollectAndCount(obsmetrics.Ready)).To(BeZero())
})
It("serves the collector's aggregates", func() {
	col := obsmetrics.NewWidgetCollector(fake.NewClientBuilder().WithScheme(scheme).WithObjects(&w).Build())
	Expect(testutil.CollectAndCompare(col, strings.NewReader(expectedText))).To(Succeed())
})
```

- Read series through `metrics.Registry.Gather()` in envtest to assert on built-ins such as
  `controller_runtime_reconcile_total{controller=...}`. [CAPI]
- Fail the suite if `controller_runtime_reconcile_panics_total`, `controller_runtime_webhook_panics_total` or the
  conversion-webhook panic counter is above 0 (CAPI's `verifyPanicMetrics`). A recovered panic only shows up there. [CAPI]
- To test the endpoint itself, run the manager as a ServiceAccount, request TokenRequest tokens, and assert the four rows
  of the table in section 1 (`BindAddress` on a free `127.0.0.1` port; skip TLS verification only for the self-signed cert). [run]
- Metric assertions should prove behaviour, not only that `Inc()` changes a value. [K8S]

## 8. Hazards (each is a factual bug, not taste)

- Plain HTTP (`SecureServing` false) or no `FilterProvider`: anyone who reaches the port reads metrics; a filter without
  TLS sends bearer tokens in cleartext. Never ship it. [CR]
- `ExtraHandlers` (pprof, log-level) on an unfiltered server, or `PprofBindAddress`: the manager's pprof listener serves
  plain HTTP with no filter, and the field doc calls pprof sensitive. [CR]
- Registering on `prometheus.DefaultRegisterer` or your own registry: the series are never served. [CR, run]
- `MustRegister` inside `Reconcile` or `SetupWithManager` without a guard: duplicate-registration panic on the second call. [run]
- Per-object labels without a delete path: series leak for the life of the process. [PROM, CAPI]
- Unbounded label values (error strings, UIDs, paths): one series per distinct value. [PROM]
- `WithLabelValues` with the wrong number of values panics; in dynamic code use `GetMetricWithLabelValues`. A panic in
  Reconcile is recovered but counted. [PROM, CR]
- Controller ServiceAccount without create on tokenreviews and subjectaccessreviews: every scrape returns 500. [CR, run]
- Alerting on `leader_election_slowpath_total`: it is never exposed. [CR]
- Self-signed serving cert in production: scrapers must disable verification. [CR]

Advice (break with a reason): do not duplicate `controller_runtime_*`; prefer kube-state-metrics or a scrape-time
collector for object state; keep one metrics package with plain `init()` registration; pass a `prometheus.Registerer`
into a constructor if you need isolated registries in unit tests (Kong KIC style) [OSS, unverified as a recipe here].
