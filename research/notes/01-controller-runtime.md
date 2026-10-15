# controller-runtime research (slice 01)

Method: shallow clone of tag `v0.25.2` at `/tmp/k8s-controller-skill-research/cr-src/repo` (commit 5d8467b3065818a3b382d8f6941147a801fd1969), plus `gh api` release notes. All code paths below are relative to that tag. Base URL for source: `https://github.com/kubernetes-sigs/controller-runtime/blob/v0.25.2/<path>`. Release notes: `https://github.com/kubernetes-sigs/controller-runtime/releases/tag/<vX.Y.Z>`.

## 1. Version, compat, versioning policy

- Latest release: **v0.25.2** (2026-10-01); v0.25.0 was 2026-09-03; v0.25.1 and v0.25.2 are patches. Source: `gh api repos/kubernetes-sigs/controller-runtime/releases/latest`, https://github.com/kubernetes-sigs/controller-runtime/releases
- v0.25.x `go.mod`: `go 1.26.0`, `k8s.io/{api,apimachinery,client-go,apiserver,apiextensions-apiserver} v0.37.0`, `logr v1.4.3`, `ginkgo/v2 v2.27.4`, `gomega v1.39.0`, `prometheus/client_golang v1.24.0`. Source: https://github.com/kubernetes-sigs/controller-runtime/blob/v0.25.2/go.mod
- Compat table: README table only goes to **CR v0.24 = k8s v0.36 / Go 1.26** (README is stale for v0.25). Verified v0.25 = k8s v0.37 (go.mod) and release note "Bump to k8s.io/* v1.37". Older rows (README): v0.23=v0.35/Go1.25, v0.22=v0.34/1.24, v0.21=v0.33/1.24, v0.20=v0.32/1.23, v0.19=v0.31/1.22, v0.18=v0.30/1.22. Source: https://github.com/kubernetes-sigs/controller-runtime/blob/v0.25.2/README.md
  - Not verified: the min Go version for v0.25 is inferred from `go 1.26.0` in go.mod; README does not list it.
- Policy: major version 0, new minor per k8s minor, **breaking changes allowed in minors**, patches never break; only one CR version can be depended on at a time; REST API compat guaranteed, client-go/apimachinery cross-version matrix NOT guaranteed; backports generally only to one previous minor. Source: https://github.com/kubernetes-sigs/controller-runtime/blob/v0.25.2/VERSIONING.md
- go.mod uses Go 1.26 `new(expr)` (e.g. `new(true)`): docs in v0.25 write `Client.EnableReadYourWritesConsistency: new(true)` and `Controller: config.Controller{EnableWarmup: new(true)}`. Pre-1.26 code uses `ptr.To(true)`. Sources: v0.25.0 release notes; `designs/warmreplicas.md`; v0.24.0 notes "Adopt new(x) over ptr.To(x)".
- `setup-envtest` is a separate Go submodule `tools/setup-envtest` (own go.mod); v0.24.0 notes say git tags are auto-created for it. Its README still says install via `go install sigs.k8s.io/controller-runtime/tools/setup-envtest@release-0.22` (stale-looking; I did not verify what `@latest` resolves to). Source: https://github.com/kubernetes-sigs/controller-runtime/blob/v0.25.2/tools/setup-envtest/README.md

## 2. Breaking / notable changes by release (all from release notes, verified against code where noted)

- **v0.25.0**: k8s 1.37; **new `EventRecorder` interface** (`GetEventRecorder(name)`; old `GetEventRecorderFor` now marked Deprecated in `pkg/recorder/recorder.go`); new experimental **read-your-writes client** (`client.Options.Cache.EnableReadYourWritesConsistency`); `source.TypedInformer`; webhook server can be disabled with `Port: -1`; opt-in client-go REST client metrics; fake client `WithGlobalResourceVersionCounter`.
- **v0.25.1/.2**: patches (priorityqueue data race; subresource create fix; metrics handler options; read-your-writes wait-duration metric; docs that RYW is wrong with >1 Namespace configured).
- **v0.24.0**: k8s 1.36; removed deprecated webhook custom-path funcs; `scheme.Builder` deprecated (use `runtime.NewSchemeBuilder`, see `pkg/scheme/scheme.go`); fixes `IndexField` blocking until informer synced.
- **v0.23.0** (2026-01-19) breaking:
  - k8s 1.35.
  - **Webhook builder generics**: `builder.WebhookManagedBy(mgr).For(&X{})` -> `builder.WebhookManagedBy(mgr, &X{})`; `ValidateCreate(ctx, obj runtime.Object)` -> `ValidateCreate(ctx, obj *X)`. Verified in `pkg/builder/webhook.go:60` and `alias.go` (`ctrl.NewWebhookManagedBy[T runtime.Object](mgr, obj T)`). `CustomValidator`/`CustomDefaulter`/`WithCustomValidator`/`WithCustomDefaulter` are `Deprecated` aliases of `Validator[runtime.Object]`/`Defaulter[runtime.Object]`.
  - **Events migrated to the new events API**: RBAC must use apiGroup `events.k8s.io`, not core `""`.
  - **Subresource Apply** added to `client` (breaking for custom `client.Client` implementers).
  - **Priority queue enabled by default**; `reconcile.Result.Priority *int` added.
  - New: `client.Options.FieldOwner`, `client.Options.FieldValidation`, `ReconciliationTimeout` + `controller_runtime_reconcile_timeouts_total`, `WebhookBuilder.WithContextFunc`, per-object cache `SyncPeriod`, `CreateOrPatch` alias in `alias.go`.
- Typed controllers/handlers/predicates/sources (`TypedReconciler[request]`, `TypedBuilder`, `TypedEventHandler`, `TypedPredicate`, `TypedSource`) are present in v0.25.2 (see sections below); they were introduced before v0.23, I did NOT trace the exact release (not verified).
- `reconcile.Result.Requeue` is **Deprecated** ("Use RequeueAfter instead"); still honoured by the controller (rate-limited requeue). Source: `pkg/reconcile/reconcile.go:30-41`, `pkg/internal/controller/controller.go:507`.
- `client.Apply` (the `Patch` value) is currently marked **Deprecated: use client.Client.Apply() and SubResource().Apply()** (`pkg/client/patch.go:28-32`). Flag: v0.23.0 release notes say "Revert deprecation of client.Apply (#3307)" but the v0.25.2 code carries the Deprecated comment again; I did not find which later PR re-added it. Treat the new `c.Apply(ctx, applyConfig, opts...)` API as the canonical one.

## 3. Reconcile semantics (`pkg/reconcile`, `pkg/internal/controller/controller.go`)

Source: https://github.com/kubernetes-sigs/controller-runtime/blob/v0.25.2/pkg/reconcile/reconcile.go and `pkg/internal/controller/controller.go:464-518`.

- `Reconciler = TypedReconciler[Request]`; `Reconcile(context.Context, Request) (Result, error)`. `Request` carries only `NamespacedName`.
- `Result{Requeue bool (deprecated), RequeueAfter time.Duration, Priority *int}`.
- Doc contract (verbatim): "If the returned error is non-nil, the Result is ignored and the request will be requeued using exponential backoff. The only exception is if the error is a TerminalError in which case no requeuing happens. If the error is nil and RequeueAfter != 0 the request is requeued after that duration."
- Verified handler behaviour:
  ```go
  case err != nil:
      if errors.Is(err, reconcile.TerminalError(nil)) { TerminalReconcileErrors.Inc() }
      else { c.Queue.AddWithOpts(AddOpts{RateLimited: true, Priority: ...}, req) }
      ... log.Error(err, "Reconciler error")
  case result.RequeueAfter > 0: c.Queue.Forget(req); AddWithOpts(After: result.RequeueAfter)
  case result.Requeue: rate-limited re-add
  default: c.Queue.Forget(req)
  ```
  A `Warning` is logged if both a non-nil error and RequeueAfter/Requeue are returned (the latter is ignored). Terminal errors are still logged and counted in `controller_runtime_reconcile_errors_total` + `..._terminal_reconcile_errors_total`.
- `reconcile.TerminalError(err error) error` wraps; `errors.Is(err, reconcile.TerminalError(nil))` detects it (`terminalError.Is` uses `errors.As`). Unwrap works.
- `reconcile.AsReconciler[T client.Object](c client.Client, rec ObjectReconciler[T]) Reconciler`: fetches the object, **returns `client.IgnoreNotFound(err)` on Get error**, then calls `Reconcile(ctx, obj)`. Optional alternative to hand-written Get.
- Panic handling: `RecoverPanic` defaults to true; recovered panic returns `fmt.Errorf("panic: %v [recovered]")` (-> backoff retry) and increments `controller_runtime_reconcile_panics_total` (`internal/controller/controller.go:196-213`).
- Reconcile ctx: logger with `controller`, `object`, `namespace`, `name`, `reconcileID` is put into ctx via `logf.IntoContext`; `controller.ReconcileIDFromContext(ctx)` exists (`pkg/controller/controller.go:330`).
- `ReconciliationTimeout` (controller option, also manager-level `config.Controller`): per-Reconcile `context.WithTimeoutCause`; no default timeout. Reconciler must honour ctx for it to matter.
- FAQ guidance (https://github.com/kubernetes-sigs/controller-runtime/blob/v0.25.2/FAQ.md): one reconciled type per controller; map others to the root with `EnqueueRequestForOwner` / `EnqueueRequestsFromMapFunc` (+ indices); do not branch on event type, reconcile level-based; stale cache: use deterministic names, or requeue and assume eventual consistency; **prefer envtest over fake client**; assert on resulting state not API calls; "no Kind registered" = incomplete Scheme (own Scheme with only needed types).

## 4. Builder (`pkg/builder`)

Source: https://github.com/kubernetes-sigs/controller-runtime/blob/v0.25.2/pkg/builder/controller.go

- `ctrl.NewControllerManagedBy(mgr)` == `builder.ControllerManagedBy(mgr)`; generic form `builder.TypedControllerManagedBy[request](mgr)`.
- Chain: `.For(obj, opts...)` (once only; second call errors "For(...) should only be called once"), `.Owns(obj, opts...)` (default: only the controller-type ownerRef; `builder.MatchEveryOwner` for all owners), `.Watches(obj, handler, opts...)`, `.WatchesMetadata(...)`, `.WatchesRawSource(src)`, `.WithEventFilter(pred)`, `.WithOptions(controller.Options{...})`, `.WithLogConstructor(...)`, `.Named(name)`, `.Complete(r)` / `.Build(r)`.
- Options: `builder.WithPredicates(...)`, `builder.OnlyMetadata` (For/Owns/Watches; caches only PartialObjectMetadata. Doc warns: read the same object with `*metav1.PartialObjectMetadata` + GVK, not the concrete type, otherwise a second full cache is created), `builder.MatchEveryOwner`.
- **`WatchesRawSource` does NOT respect `WithEventFilter` predicates** (doc comment). Global predicates apply to every watched type, so they must handle all types.
- `Named(name)`: used in metrics and logs; "underscores and alphanumeric"; must be unique; default is lowercased kind.
- `Complete(nil reconciler)` errors "must provide a non-nil Reconciler".
- Webhook builder (`pkg/builder/webhook.go`): `ctrl.NewWebhookManagedBy(mgr, &T{}).WithValidator(v).WithDefaulter(d).WithConverter(...).Complete()`; also `RecoverPanic(bool)` (default true), `WithLogConstructor`, `WithContextFunc`, `WithValidatorCustomPath`, `WithDefaulterCustomPath`. Generated default paths:
  ```go
  "/mutate-"   + strings.ReplaceAll(gvk.Group, ".", "-") + "-" + gvk.Version + "-" + strings.ToLower(gvk.Kind)
  "/validate-" + ... same shape
  ```
  These paths must match the controller-gen `+kubebuilder:webhook:path=...` markers (core group yields `/mutate--v1-pod`, i.e. empty group gives a double dash).

## 5. Controller options (`pkg/controller/controller.go`, `pkg/config/controller.go`)

Source: https://github.com/kubernetes-sigs/controller-runtime/blob/v0.25.2/pkg/controller/controller.go

`controller.Options` = `TypedOptions[reconcile.Request]`. Fields (and defaults as verified in `NewTypedUnmanaged`):
- `MaxConcurrentReconciles` int: default **1**; same key is never processed concurrently by workers (workqueue guarantee). Manager-level defaults: `config.Controller.MaxConcurrentReconciles` and `GroupKindConcurrency map[string]int` (key form `Kind.group`, e.g. `ReplicaSet.apps`); per-controller wins.
- `CacheSyncTimeout`: default 2 min.
- `RecoverPanic *bool`: default true.
- `NeedLeaderElection *bool`: default true (nil -> leader-elected). Set false for controllers that must run on all replicas.
- `SkipNameValidation *bool`: default false; unique controller names are enforced via a global registry, which breaks tests that build several managers/controllers with the same name. Set `Controller: config.Controller{SkipNameValidation: new(true)}` on the manager or per controller.
- `RateLimiter workqueue.TypedRateLimiter[request]`: default is **per-item exponential 5ms..1000s** when the priority queue is used, else `workqueue.DefaultTypedControllerRateLimiter` (exponential + token bucket).
- `NewQueue`: low-level; default is `priorityqueue.New` unless `UsePriorityQueue` false.
- `UsePriorityQueue *bool`: **default on** ("enabled by default"); `handler.WithLowPriorityWhenUnchanged(h)` demotes initial-list/resync events to `LowPriority = -100` (design: `designs/priorityqueue.md`; `PriorityQueue.AddWithOpts`, `GetWithPriority`; Result.Priority only honoured with the priority queue).
- `EnableWarmup *bool`: default false; beta; starts sources (informers) on non-leader replicas to shorten failover; no-op when `NeedLeaderElection` is false. Design: https://github.com/kubernetes-sigs/controller-runtime/blob/v0.25.2/designs/warmreplicas.md (tracking issue #3220).
- `ReconciliationTimeout`, `LogConstructor func(*request) logr.Logger`, `Logger`.
- Low-level: `controller.New(name, mgr, opts)` registers with manager; `controller.NewUnmanaged` does not. `Options.Reconciler` is required ("must specify Reconciler"), name required.

## 6. Handlers, predicates, sources

- `handler.EnqueueRequestForObject`, `handler.EnqueueRequestForOwner(scheme, mapper, &Owner{}, handler.OnlyControllerOwner())`, `handler.EnqueueRequestsFromMapFunc(func(ctx, obj client.Object) []reconcile.Request)` (for updates the map func runs on both old and new objects and enqueues both), `handler.Funcs`, `handler.WithLowPriorityWhenUnchanged`. Typed variants: `TypedEnqueueRequestForObject`, `TypedEnqueueRequestsFromMapFunc[object, request]` ("TypedMapFunc is experimental"). `MapFunc` and `Handler` use `workqueue.TypedRateLimitingInterface[request]` in signatures. Source: `pkg/handler/*.go`.
- Handler event methods signature: `Create(ctx, event.TypedCreateEvent[T], q workqueue.TypedRateLimitingInterface[reconcile.Request])` (ctx-first; no `source.Source` injection).
- `source.Kind(cache, obj, handler, predicates...)` (generic over object), `source.TypedKind`, `source.Channel`, `source.Func`, `source.TypedInformer` (new in v0.25). Source: `pkg/source/source.go:76-100`.
- Predicates (`pkg/predicate/predicate.go`): `Predicate = TypedPredicate[client.Object]`; `Funcs`, `NewPredicateFuncs(filter)`, `GenerationChangedPredicate`, `ResourceVersionChangedPredicate`, `AnnotationChangedPredicate`, `LabelChangedPredicate`, `And/Or/Not`, `LabelSelectorPredicate`. Predicates "do not affect what the cache stores" (use cache options for that).
- `GenerationChangedPredicate` caveats (verbatim in docs): (1) generation semantics differ per API (Deployment also bumps generation on annotation change); (2) it skips status-only updates, so a wiped/overwritten status is not self-healed; (3) a cache `SyncPeriod` resync produces Update events with identical old/new objects and thus is filtered out by it. For CRs, generation only bumps on spec change if status is a subresource.

## 7. controllerutil (`pkg/controller/controllerutil/controllerutil.go`)

Source: https://github.com/kubernetes-sigs/controller-runtime/blob/v0.25.2/pkg/controller/controllerutil/controllerutil.go

- `SetControllerReference(owner, controlled metav1.Object, scheme *runtime.Scheme, opts ...OwnerReferenceOption) error`: sets controller ownerRef (errors with `*AlreadyOwnedError` if another controller owner exists). `SetOwnerReference`, `RemoveOwnerReference`, `RemoveControllerReference`, `HasControllerReference`, `HasOwnerReference`; option `WithBlockOwnerDeletion(bool)`. Owner and controlled must be same namespace or owner cluster-scoped (validateOwner, not re-verified in detail).
- `CreateOrUpdate(ctx, c, obj, MutateFn) (OperationResult, error)`: Get -> mutate -> Create/Update/no-op via `equality.Semantic.DeepEqual`. Doc warnings: (a) if MutateFn resets a defaulted field (e.g. Deployment replicas nil) it will update every time; (b) obj must have only Name/Namespace set before the call; (c) **changes to subresources (status) in MutateFn are discarded**.
- `CreateOrPatch(ctx, c, obj, MutateFn)`: same but uses `client.MergeFrom` for object and for status; status changes only applied if object already exists (on create, requeue to set status).
- `OperationResult`: `unchanged | created | updated | updatedStatus | updatedStatusOnly`.
- Finalizers: `AddFinalizer(o, name) bool`, `RemoveFinalizer(o, name) bool`, `ContainsFinalizer(o, name) bool` - these only mutate in memory and return whether anything changed; caller must Update/Patch. There is also `pkg/finalizer` (`finalizer.NewFinalizers()`, `Register(key, Finalizer)`, `Finalize(ctx, obj) (Result{Updated, StatusUpdated}, error)`) which wraps add/run/remove; caller still persists.
- Aliases in `alias.go`: `ctrl.CreateOrUpdate`, `ctrl.CreateOrPatch`, `ctrl.SetControllerReference`, `ctrl.SetupSignalHandler`, `ctrl.Log`, `ctrl.LoggerFrom` (= `log.FromContext`), `ctrl.LoggerInto`, `ctrl.SetLogger`, `ctrl.NewManager`, `ctrl.GetConfigOrDie`, `ctrl.Options`, `ctrl.Manager`, `ctrl.Result`, `ctrl.SchemeBuilder` (deprecated alias).

## 8. Client (`pkg/client`)

Source: `interfaces.go`, `patch.go`, `options.go`, `client.go`, `fieldowner.go`, `applyconfigurations.go` at v0.25.2.

- `client.Client` = `Reader` (Get/List) + `Writer` (**Apply**, Create, Delete, Update, Patch, DeleteAllOf) + `StatusClient` (`Status() SubResourceWriter`) + `SubResource(name)`; plus `Scheme()`, `RESTMapper()`, `GroupVersionKindFor`, `IsObjectNamespaced`.
- **Server-side apply API (new)**: `Apply(ctx, obj runtime.ApplyConfiguration, opts ...ApplyOption) error` and `c.Status().Apply(...)` / `SubResource(x).Apply(...)`. Takes client-gen applyconfigurations (e.g. `appsv1ac.Deployment(...)`) or `client.ApplyConfigurationFromUnstructured(u)` (doc: do NOT use Unstructured generated from typed objects; zero values indistinguishable). `ApplyOptions{DryRun, Force *bool, FieldManager (required, <=128 chars), DisableReadYourWritesConsistency}`. Use `client.FieldOwner("name")` and `client.ForceOwnership`.
- Field owner: `client.FieldOwner(name)` per call; `client.WithFieldOwner(c, name)` wrapper (wraps Create/Update/Patch/Apply, not Delete); **`client.Options.FieldOwner`** default (since v0.23). `client.Options.FieldValidation`: apply requests are always strict regardless.
- Patch helpers: `client.MergeFrom(orig)`, `client.MergeFromWithOptions(orig, client.MergeFromWithOptimisticLock{})`, `client.StrategicMergeFrom(orig, ...)`, `client.RawPatch(type, data)`, `client.Merge` (raw object as merge patch), `client.Apply` (deprecated Patch value, see section 2). Typical:
  ```go
  patch := client.MergeFrom(obj.DeepCopy())
  obj.Status.X = ...
  err := c.Status().Patch(ctx, obj, patch)
  ```
  (pattern is standard; the signatures `Status() SubResourceWriter` with `Patch(ctx, obj, patch, ...)` are verified in `interfaces.go`).
- Status subresource: `c.Status().Update/Patch/Apply`; `c.Status().Update(ctx, obj)` only writes status. `StatusWriter` kept as alias of `SubResourceWriter`. Other subresources via `c.SubResource("scale"|"eviction"|"binding"|"token"|"approval")`.
- Helpers: `client.IgnoreNotFound(err)`, `client.IgnoreAlreadyExists(err)`, `client.ObjectKeyFromObject`, list options `InNamespace`, `MatchingLabels`, `MatchingFields`, `MatchingFieldsSelector`, `Limit`, `Continue` etc.
- **Cache-backed reads**: manager default client "reads from cache and writes directly" (`manager.Options.Client` doc; `cluster.go:195-215` sets `Cache.Unstructured: false`, so **reads of `unstructured.Unstructured` objects always hit the API server unless `Cache.Unstructured: true`**; typed and PartialObjectMetadata reads use the cache). `client.CacheOptions{Reader, DisableFor []Object, Unstructured bool, EnableReadYourWritesConsistency *bool}`. Use `DisableFor: []client.Object{&corev1.Secret{}, &corev1.ConfigMap{}}` to never cache those. `mgr.GetAPIReader()` = uncached reader, "should be used sparingly".
- **Read-your-writes (experimental, v0.25)**: `Client: client.Options{Cache: &client.CacheOptions{EnableReadYourWritesConsistency: new(true)}}`; reads block until cache observed prior writes; per-request opt-out `DisableReadYourWritesConsistency`; scoped by representation (typed/unstructured/metadata); **does not work correctly with >1 Namespace configured**; "details may change". Tracking issue #3320. Sources: client.go:97-115, v0.25.0/.2 notes. Note: in this tag the option lives on `client.CacheOptions`, whereas the v0.25.0 release note says `Client.EnableReadYourWritesConsistency` - minor inconsistency; I did not run it to settle the exact manager field path (flag).
- Fake client (`pkg/client/fake`): `fake.NewClientBuilder().WithScheme().WithObjects().WithStatusSubresource(&T{}).WithIndex(obj, field, fn).WithInterceptorFuncs(interceptor.Funcs{...}).Build()`; **status subresource behaviour requires `WithStatusSubresource`**; FAQ still recommends envtest. Interceptors in `pkg/client/interceptor`.

## 9. Cache (`pkg/cache/cache.go`, `cacheapi`)

Source: https://github.com/kubernetes-sigs/controller-runtime/blob/v0.25.2/pkg/cache/cache.go

- `cache.Options` fields: `Scheme, Mapper, HTTPClient, SyncPeriod (default 10h, 10% jitter), ReaderFailOnMissingInformer, DefaultNamespaces map[string]Config, DefaultLabelSelector, DefaultFieldSelector, DefaultTransform, DefaultWatchErrorHandler, DefaultUnsafeDisableDeepCopy, DefaultEnableWatchBookmarks (default true), ByObject map[client.Object]ByObject, NewInformer`.
- `ByObject{Namespaces map[string]Config, Label, Field, Transform, UnsafeDisableDeepCopy, EnableWatchBookmarks, SyncPeriod}`. Precedence: ByObject > DefaultNamespaces[ns] > Default*. `Namespaces` must be unset for cluster-scoped objects. `cache.AllNamespaces` key for the rest.
- `cache.TransformStripManagedFields()` as `DefaultTransform` cuts memory unless you read managedFields.
- **SyncPeriod doc**: do not tune it to cover missed events; return `RequeueAfter: t` instead. It fires local synthetic Update events with identical old/new (filtered by GenerationChangedPredicate).
- **Filtering the cache narrows what `Get/List` can ever return** (design `use-selectors-at-cache.md`): objects not matching the selector are invisible, so only use when sure.
- Field indexes: `mgr.GetFieldIndexer().IndexField(ctx, &Pod{}, ".spec.nodeName", func(o client.Object) []string {...})`; then `c.List(ctx, &list, client.MatchingFields{".spec.nodeName": name})`. Doc: automatically handles namespaced + all-namespace variants; "for k8s apiserver compat return a single key"; equality means at least one key matches. Register indices **before `mgr.Start`**; v0.24.0 fixed `IndexField` blocking until informer sync (#3445).
- `ReaderFailOnMissingInformer: true` returns `*cache.ErrResourceNotCached` instead of lazily starting an informer on first Get/List (avoids accidental informer creation, e.g. for Secrets).
- `UnsafeDisableDeepCopy`: must DeepCopy before mutating or you corrupt the cache.

## 10. Manager (`pkg/manager/manager.go`, `internal.go`)

Source: https://github.com/kubernetes-sigs/controller-runtime/blob/v0.25.2/pkg/manager/manager.go

- `ctrl.NewManager(cfg, ctrl.Options{...})`. Verified fields: `Scheme`, `Cache cache.Options`, `Client client.Options`, `Logger`, `LeaderElection bool`, `LeaderElectionID` (**required if LeaderElection true**: `leaderelection` returns "LeaderElectionID must be configured"), `LeaderElectionNamespace`, `LeaderElectionResourceLock` (default `leases`), `LeaderElectionReleaseOnCancel` (only safe if the process exits immediately after the manager stops), `LeaderElectionConfig`, `LeaderElectionLabels`, `LeaderElectionResourceLockInterface`, `LeaseDuration` (15s), `RenewDeadline` (10s), `RetryPeriod` (2s), `Metrics metricsserver.Options`, `HealthProbeBindAddress` ("0"/"" disables), `ReadinessEndpointName` (`/readyz`), `LivenessEndpointName` (`/healthz`), `PprofBindAddress`, `WebhookServer webhook.Server`, `BaseContext`, `GracefulShutdownTimeout *time.Duration` (default **30s**; 0 disables; negative = wait forever; skipped if leader lease lost), `Controller config.Controller`, `EventBroadcaster` (Deprecated - may leak goroutines).
- Metrics (`pkg/metrics/server/server.go`): `metricsserver.Options{BindAddress (default ":8080"; "0" disables), SecureServing bool (default http), FilterProvider, CertDir/CertName/KeyName, TLSOpts, ExtraHandlers, HandlerOpts (new in v0.25.2), ListenConfig}`. Authn/z filter: `FilterProvider: filters.WithAuthenticationAndAuthorization` from `pkg/metrics/filters` (requires RBAC for TokenReview/SubjectAccessReview; I did not verify the exact RBAC verbs). Metric names verified in `pkg/internal/controller/metrics/metrics.go`: `controller_runtime_reconcile_total{controller,result}`, `..._reconcile_errors_total`, `..._terminal_reconcile_errors_total`, `..._reconcile_panics_total`, `..._reconcile_time_seconds`, `..._max_concurrent_reconciles`, `..._active_workers`, `..._reconcile_timeouts_total`.
- Health: `mgr.AddHealthzCheck("healthz", healthz.Ping)`, `mgr.AddReadyzCheck("readyz", healthz.Ping)`; webhook server offers `StartedChecker()` for readyz. Package `pkg/healthz`.
- Runnables are bucketed (HTTP servers, webhooks, caches, others, warmup, leader-election). Leader-elected runnables only start after the lock is won; a `Runnable` opts out by implementing `NeedLeaderElection() bool { return false }` (`manager.LeaderElectionRunnable`). Webhook server never needs leader election (`webhook.Server.NeedLeaderElection`).
- Signals: `ctrl.SetupSignalHandler()` returns ctx cancelled on SIGTERM/SIGINT; second signal exits 1; may be called only once per process (`onlyOneSignalHandler` channel panics on a second call - inferred from code, not run).
- Multi-cluster: `pkg/cluster.New(cfg, opts)` returns a `cluster.Cluster` (client, cache, indexer, APIReader, recorder) that can be `mgr.Add`ed; design `designs/move-cluster-specific-code-out-of-manager.md`. Any ecosystem "multicluster-runtime" project: NOT verified here.

## 11. Webhooks (`pkg/webhook`)

- Server: `webhook.NewServer(webhook.Options{Host, Port (default 9443; **-1 disables, since v0.25**), CertDir, CertName, KeyName, ClientCAName, TLSOpts, WebhookMux})`; pass via `ctrl.Options{WebhookServer: ...}`. Default cert dir/names: not re-verified in this pass. Source: `pkg/webhook/server.go:38-148`.
- `admission.Validator[T runtime.Object]`: `ValidateCreate(ctx, obj T) (Warnings, error)`, `ValidateUpdate(ctx, oldObj, newObj T)`, `ValidateDelete(ctx, obj T)`. `admission.Defaulter[T]`: `Default(ctx, obj T) error`. Option `admission.DefaulterRemoveUnknownOrOmitableFields`. Source: `pkg/webhook/admission/validator_custom.go`, `defaulter_custom.go` (file names still say "custom").
- Conversion: `pkg/conversion` (`Hub`, `Convertible`), `WithConverter(...)` in webhook builder; v0.23 allows implementing conversion outside API packages. envtest installs conversion webhooks only for scheme types that implement Hub/Spoke.

## 12. envtest (`pkg/envtest`)

Source: https://github.com/kubernetes-sigs/controller-runtime/blob/v0.25.2/pkg/envtest/server.go

- `envtest.Environment{CRDDirectoryPaths, CRDs, ErrorIfCRDPathMissing, CRDInstallOptions, WebhookInstallOptions, Scheme, UseExistingCluster *bool, BinaryAssetsDirectory, DownloadBinaryAssets bool, DownloadBinaryAssetsVersion, DownloadBinaryAssetsIndexURL, ControlPlaneStartTimeout (20s), ControlPlaneStopTimeout, AttachControlPlaneOutput, Config, ControlPlane}`; `cfg, err := env.Start()`; `env.Stop()`.
- Binaries (etcd, kube-apiserver, kubectl): `KUBEBUILDER_ASSETS` env var overrides `BinaryAssetsDirectory`, default `/usr/local/kubebuilder/bin`. **New: `DownloadBinaryAssets: true`** downloads binaries in-process (index default `https://raw.githubusercontent.com/kubernetes-sigs/controller-tools/HEAD/envtest-releases.yaml`; version default = latest stable). Alternative: `setup-envtest use <k8s-version> -p path`.
- Other env vars: `USE_EXISTING_CLUSTER`, `KUBEBUILDER_CONTROLPLANE_START_TIMEOUT`/`STOP_TIMEOUT`, `KUBEBUILDER_ATTACH_CONTROL_PLANE_OUTPUT`.
- Set `ErrorIfCRDPathMissing: true` to avoid silent "CRDs not found" (controller-gen output dir is the usual `CRDDirectoryPaths`).
- envtest has no controller-manager/garbage collector/kubelet: ownerRef GC and finalizer-based deletion by controllers other than yours do not occur; pods never run. (Well-known behaviour; not documented in the files I read - flagged unverified.)
- Test stack in repo uses Ginkgo v2 + Gomega.

## 13. Events (recorder) and logging

- New API (v0.25): `mgr.GetEventRecorder(name) recorder.EventRecorder` (`Eventf(regarding, related runtime.Object, eventtype, reason, action, note string, args...)` and `AnnotatedEventf(...)`); name becomes `reportingController` (valid qualified name; `len(name)+1+len(hostname) <= 128`). Old `mgr.GetEventRecorderFor(name) record.EventRecorder` is **Deprecated, will be removed**. Source: `pkg/recorder/recorder.go:28-55`, `pkg/cluster/internal.go:87-92`. RBAC: `events.k8s.io` group `create;patch` (per v0.23.0 notes; verbs not verified).
- Logging: logr only. `log.SetLogger(zap.New(zap.UseDevMode(...)))` must be called at start or logs are silently dropped (after 30s, it prints "log.SetLogger(...) was never called; logs will not be displayed" to stderr and installs a null sink) - `pkg/log/log.go:49-78`. `ctrl.LoggerFrom(ctx, kv...)` = `log.FromContext` (falls back to root `log.Log`); `ctrl.LoggerInto(ctx, l)`. `zap.Options.BindFlags(flag.CommandLine)` + `zap.UseFlagOptions(&opts)`.
- TMP-LOGGING.md guidelines (https://github.com/kubernetes-sigs/controller-runtime/blob/v0.25.2/TMP-LOGGING.md): constant messages + KV pairs, never `fmt.Sprintf` in message; always `log.Error(err, msg, kv...)` for errors (nil err allowed); `V(1)` for debug, avoid higher levels; keys lower-case space separated (`api version`, `object`); log k8s objects directly (zap encoder reduces to name/namespace/kind/apiVersion in non-dev mode); log GroupKind not bare Kind; pluralise keys for multiple things; request key = `request`/the object key. Note: it does NOT mandate capitalised messages with no trailing period; that is the user's global standard, while controller-runtime's own messages (e.g. "Reconciler error", "Reconciling") are capitalised.
- Returning an error from Reconcile already logs `"Reconciler error"` with stack-trace-capable `log.Error`; an extra log in Reconcile is optional (TMP-LOGGING.md).

## 14. Designs directory (listing at v0.25.2)

`README.md, cache_options.md, component-config.md, move-cluster-specific-code-out-of-manager.md, priorityqueue.md, template.md, use-selectors-at-cache.md, warmreplicas.md, images/`. There is **no `docs/` directory** in the repo (404 on contents API; top-level has only `designs/`). designs/README.md says GoDoc is the canonical, up-to-date reference and designs may be out of date. There is no design file for builder, predicates, leader election, webhooks, metrics or read-your-own-writes in this tag (the RYW feature has only the tracking issue #3320 and code docs). `component-config.md` (2020) is historical.
- Relevant: `priorityqueue.md` (AddOpts{After, RateLimited, Priority}), `warmreplicas.md` (Warmup phase after caches/others, before leader election; opt-in via `EnableWarmup`), `use-selectors-at-cache.md`, `cache_options.md`, `move-cluster-specific-code-out-of-manager.md` (multi-cluster).

## 15. Unverified / flagged

- Exact release in which Typed* generics (controllers, handlers, predicates) first shipped: not traced.
- Re-deprecation commit of `client.Apply` after v0.23.0's revert: not found.
- Exact manager-level field path for read-your-writes in release note vs code (`client.CacheOptions.EnableReadYourWritesConsistency` verified in code; manager example in notes not executed).
- Default webhook cert dir/file names and metrics-filter RBAC verbs: not re-checked.
- README compatibility table lacks a v0.25 row; Go min for v0.25 inferred from go.mod only.
- Behaviours of envtest lacking GC / controller-manager: general knowledge, not found in the files read.
- `SetupSignalHandler` second-call panic: inferred from code structure, not executed.
