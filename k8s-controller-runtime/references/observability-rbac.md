# Observability and RBAC

Read this when you set up logging, events or probes, write RBAC markers, give the controller leader election, or expose
pprof. Metrics (endpoint, custom metrics, scrape RBAC) are in `metrics.md`: do not duplicate them here. Snippets compile
against controller-runtime v0.25.2 (imports omitted). Tags are defined in `sources.md`; `run` = reproduced in
`research/experiments`.

## 1. Logging

See `logging.md` (backend, `SetLogger` wiring, conventions, levels).

## 2. Events

v0.23.0 added the events.k8s.io recorder `GetEventRecorder` (v0.25.0 only widened its return type); `GetEventRecorderFor` (core/v1 events) is deprecated. [CR, run]

```go
type WidgetReconciler struct {
	client.Client
	Recorder events.EventRecorder // k8s.io/client-go/tools/events
}

// in main: Recorder: mgr.GetEventRecorder("widget-controller")
r.Recorder.Eventf(&w, nil, corev1.EventTypeNormal, ReasonProvisioned, "Provision", "Created Deployment %s", w.Name)
```

`Eventf(regarding, related runtime.Object, eventtype, reason, action, note string, args ...any)`. The recorder name
becomes `reportingController`; client-go appends `-<hostname>` and the result must stay within 128 characters. [CR, run]

- Use `Normal` for state changes worth auditing and `Warning` for failures a user must act on. Define reasons as constants
  (PascalCase, like condition reasons). Do not emit an event on every reconcile: events are not a status mechanism, and
  conditions (`status-conditions.md`) are. [K8S, API]
- Register the object's type in the scheme used by the manager so the event can reference it [unverified here; the run used a ConfigMap].
- RBAC: the new recorder needs `create` and `patch` on `events` in apiGroup `events.k8s.io`. In the run with only that rule,
  the event was stored (`reason=Synced`, reporter = the recorder name). [CR, run]
- Leader election still records its `LeaderElection` event through the legacy recorder (`GetEventRecorderFor` inside
  `leaderelection.NewResourceLock`), which writes core `""` events. Without `create` on core `events` the election still
  succeeds and the log shows `Server rejected event (will not retry!)`; with the rule the event is stored. [CR, run]

## 3. Health and readiness probes

```go
mgr, err := ctrl.NewManager(cfg, ctrl.Options{HealthProbeBindAddress: ":8081"}) // "" or "0" disables
_ = mgr.AddHealthzCheck("ping", healthz.Ping) // GET /healthz
_ = mgr.AddReadyzCheck("ping", healthz.Ping)  // GET /readyz
```

- Handlers exist only for checks you add; paths default to `/healthz` and `/readyz`. Both returned 200 `ok` in the run
  (checks `ping`). Always check the errors in real code. [CR, run]
- With webhooks, add `mgr.GetWebhookServer().StartedChecker()` as a readiness check (CAPI also uses it for liveness; a slow
  webhook start can then fail liveness [unverified as a problem]). See `webhooks.md`. [CAPI]
- Probe port is separate from metrics and serves plain HTTP, so keep it free of sensitive data (checks only). [CR]

## 4. Graceful shutdown (pointers)

- `ctrl.SetupSignalHandler()` returns a context cancelled on SIGTERM/SIGINT; call it once per process (it panics on a
  second call) and pass the context to `mgr.Start`. [CR]
- `GracefulShutdownTimeout` defaults to 30 s; it is skipped when the leader lease is lost. The metrics server shuts down with
  its own 1-minute bound. Details and `LeaderElectionReleaseOnCancel` are in `manager-setup.md`. [CR]

## 5. RBAC from markers

`controller-gen rbac:roleName=manager-role paths=./... output:rbac:dir=config/rbac` turns `+kubebuilder:rbac` markers into
a ClusterRole, and into a Role per `namespace=`. Markers can sit in any comment of a scanned package: above the reconciler,
above `package`, or in `main.go`. Run on a package comment, they produced the YAML below. [CT, run]

```go
// +kubebuilder:rbac:groups=demo.example.com,resources=widgets,verbs=get;list;watch;patch
// +kubebuilder:rbac:groups=demo.example.com,resources=widgets/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=demo.example.com,resources=widgets/finalizers,verbs=update
// +kubebuilder:rbac:groups=events.k8s.io,resources=events,verbs=create;patch
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get,resourceNames=widget-credentials
```

```yaml
- {apiGroups: [""], resourceNames: [widget-credentials], resources: [secrets], verbs: [get]}
- {apiGroups: [demo.example.com], resources: [widgets], verbs: [get, list, patch, watch]}
- {apiGroups: [demo.example.com], resources: [widgets/finalizers], verbs: [update]}
- {apiGroups: [demo.example.com], resources: [widgets/status], verbs: [get, patch, update]}
- {apiGroups: [events.k8s.io], resources: [events], verbs: [create, patch]}
```

Prefer:
- Name every group, resource and verb; no `*` (wildcards also cover resources added later). Grant only the verbs the code
  calls: if you only `Patch` the main object, do not grant `update`; list/watch only for types an informer reads. [K8S]
- Treat `widgets`, `widgets/status` and `widgets/finalizers` as three RBAC resources. Writing status needs the `/status`
  line, and writing finalizers or owner references with `blockOwnerDeletion` needs `/finalizers` `update`
  (kubebuilder scaffold convention) [KB; the admission reason is unverified here].
- Secrets: a cached read needs `list` and `watch` on all Secrets in scope, which exposes every Secret value there.
  `resourceNames` restricts `get`, `update`, `patch`, `delete` but cannot restrict `create`, and a list/watch informer
  cannot be limited by name. So: read named Secrets with a live `Get` (APIReader or `DisableFor`) and `resourceNames`, or
  use a namespaced Role and a label-filtered cache (`cache-and-memory.md`). [K8S, CT]
- Add `namespace=<ns>` to emit a Role instead of a ClusterRole when the controller watches one namespace; pair it with
  `cache.Options.DefaultNamespaces`. [CT, CR]
- Split subjects with `roleName=` on the marker (separate Role/ClusterRole per marker group, see `metrics.md`). [CT, run]
- A dedicated ServiceAccount, not `default`; never grant `escalate`, `bind`, `impersonate`, `serviceaccounts/token`
  `create` or `nodes/proxy` unless the controller needs it. [K8S]
- Keep markers equal to what the code does. Test it: in envtest, bind the generated Role to a user and run the manager with
  `rest.Config.Impersonate` set to that user; a missing verb then fails the test (the leader-election and metrics runs
  above used this). [run]

Hand-written, controller-gen does not emit them: ServiceAccount, ClusterRoleBinding/RoleBinding for every generated role,
the manager Deployment. [CT]

## 6. Leader election RBAC

Set `LeaderElection: true`, a unique `LeaderElectionID` (required) and, outside a pod, `LeaderElectionNamespace`
(otherwise it reads the in-cluster namespace file or errors). The default lock is a `coordination.k8s.io` Lease. [CR]

Verified minimal rule: a manager running as a user with only `leases` `get`, `create`, `update` in the namespace became
leader and kept running (client-go calls only Get, Create and Update on the Lease). `list`, `watch` and `patch` are not
needed. [CR, run]

```go
// +kubebuilder:rbac:groups=coordination.k8s.io,namespace=widget-system,roleName=leader-election-role,resources=leases,verbs=get;create;update
// +kubebuilder:rbac:groups="",namespace=widget-system,roleName=leader-election-role,resources=events,verbs=create;patch
```

This generated a Role `leader-election-role` in `widget-system` with exactly those two rules. [CT, run] Bind it by hand to
the controller ServiceAccount. If you set `LeaderElectionResourceLock` to something other than leases, the rules change;
that was not run [unverified].

## 7. pprof only behind the authenticated filter

Expose pprof through the metrics server, so it gets the same TLS and authn/authz as `/metrics` (CAPI does this, and only
when secure serving is on). [CAPI, CR]

```go
Metrics: metricsserver.Options{
	BindAddress:    ":8443",
	SecureServing:  true,
	FilterProvider: filters.WithAuthenticationAndAuthorization,
	ExtraHandlers:  map[string]http.Handler{"/debug/pprof/": http.HandlerFunc(pprof.Index)},
},
```

In the run: no token gave 401; a token with only `/metrics` got 403. A role with the exact URL `/debug/pprof/` could load the
index (200) but got 403 on `/debug/pprof/heap`, because the authorized path is the full request path; a rule with the
suffix wildcard `nonResourceURLs: [/debug/pprof/*]` allowed it. controller-gen accepts that value:
`+kubebuilder:rbac:urls=/debug/pprof/*,verbs=get,roleName=pprof-reader`. [CR, CT, run]
`pprof.Index` serves named profiles (heap, goroutine, ...); `profile`, `trace`, `cmdline`, `symbol` need their own
`ExtraHandlers` entries. [run: `/debug/pprof/cmdline` through Index returned 404]
Grant the pprof role only to humans or tools that need it, not to the Prometheus ServiceAccount.

## 8. Pod security (Deployment you write by hand)

For the Restricted Pod Security Standard the controller container sets `runAsNonRoot: true`,
`allowPrivilegeEscalation: false`, `capabilities.drop: [ALL]` and `seccompProfile.type: RuntimeDefault` (or Localhost).
[K8S] This is from the standard's documentation and was not run against a cluster. Resource requests and limits are not
covered here: no source I verified gives controller-specific numbers; size them from your own metrics (`metrics.md`).

## 9. Hazards (each is a factual bug, not taste)

- Missing `SetLogger`: all logs are dropped; the notice only prints on a log call made after 30 s, so earlier lines are lost silently. See `logging.md`. [CR, run]
- Core `events` RBAC only (the old rule) while code uses `GetEventRecorder`: the new recorder writes events.k8s.io, so it
  needs that group [CR; this exact case not run]. The reverse loses only the `LeaderElection` event, with a log line. [CR, run]
- `list`/`watch` on Secrets to read a few of them: exposes every Secret in scope; the informer also caches them all. [K8S]
- Wildcard (`*`) groups, resources or verbs in generated RBAC: grants types added later. [K8S]
- `PprofBindAddress` or pprof via `ExtraHandlers` on an unfiltered server: plain HTTP, no authentication, and pprof is
  sensitive. [CR]
- Calling `ctrl.SetupSignalHandler()` twice: panics. [CR]
- Logging or eventing a referenced Secret's value: it ends up in logs and in objects any reader of events can see. [API]
- Markers that drift from the code: a verb added in code without a marker passes unit tests and fails in cluster. Cover it with
  the impersonating envtest run in section 5. [run]
