# 07 - cluster-api (CAPI) as a controller-runtime reference

Source: https://github.com/kubernetes-sigs/cluster-api, shallow clone at /tmp/k8s-controller-skill-research/src/cluster-api
Commit read: c8f8936061361390e2388c7db97261096713e7a2 (main, 2026-10-15, "docs: clarify provider resource naming"). No tag (shallow, `describe` = c8f8936).
Deps at that commit (go.mod:58,51,3): controller-runtime v0.25.2, client-go v0.37.1, go 1.26.0.
NOTE: repo layout moved vs older CAPI. main is `core/main.go` (not main.go); reconcilers are `core/reconcilers/<kind>/` (not internal/controllers); manager options helpers are `core/setup/setup.go`. All paths below are relative to repo root.
Tags: [V] = read in code/docs at that commit. [U] = unverified/inference. "Interpretation" lines are mine.

---------------------------------------------------------------------
## 1. Manager setup (core/main.go, core/setup/setup.go, util/flags/manager.go)

1.1 [V] Flags then `ctrl.GetConfigOrDie()` then QPS/Burst/UserAgent/WarningHandler on the rest.Config.
core/main.go:332-336
```go
restConfig := ctrl.GetConfigOrDie()
restConfig.QPS = restConfigQPS
restConfig.Burst = restConfigBurst
restConfig.UserAgent = remote.DefaultClusterAPIUserAgent(controllerName)
restConfig.WarningHandler = apiwarnings.DefaultHandler(klog.Background().WithName("API Server Warning"))
```
Defaults: core/main.go:251-255 `--kube-api-qps` 100, `--kube-api-burst` 200 (CR default is 20/30). Separate pair for workload-cluster clients: `--clustercache-client-qps` 20 / `--clustercache-client-burst` 30 (main.go:257-261).

1.2 [V] Full ctrl.Options (core/main.go:371-399): Controller{UsePriorityQueue, CacheSyncTimeout: 5m}, LeaderElection with Lease lock + tunable durations, HealthProbeBindAddress, PprofBindAddress, Metrics, Cache, Client, WebhookServer.
```go
Controller: config.Controller{
    UsePriorityQueue: ptr.To[bool](feature.Gates.Enabled(feature.PriorityQueue)),
    // Give the manager more time to sync the caches during startup. This is required
    // in high scale environments when they are more objects in the system (default is 3m).
    CacheSyncTimeout: 5 * time.Minute,
},
LeaderElectionID:           "controller-leader-election-capi",
LeaderElectionResourceLock: resourcelock.LeasesResourceLock,
```
Leader-election flags with 15s/10s/2s defaults at main.go:176-183. `--leader-elect` default false (main.go:173).

1.3 [V] Order in main(): NewManager -> SetupSignalHandler -> setupChecks -> setupIndexes -> setupReconcilers -> setupWebhooks -> mgr.Start (core/main.go:401-419). Indexes are registered BEFORE any controller is built.

1.4 [V] Health/ready: webhook StartedChecker for both (core/main.go:422-432).
```go
mgr.AddReadyzCheck("webhook", mgr.GetWebhookServer().StartedChecker())
mgr.AddHealthzCheck("webhook", mgr.GetWebhookServer().StartedChecker())
```

1.5 [V] Metrics = "diagnostics" endpoint. Secure by default, authn/authz filter, pprof + runtime log-level PUT only when secure. util/flags/manager.go:127-153 (flag defs at :84-92)
```go
metricsOptions = &metricsserver.Options{
    BindAddress:    options.DiagnosticsAddress,   // default ":8443"
    SecureServing:  true,
    FilterProvider: filters.WithAuthenticationAndAuthorization,
    ExtraHandlers: map[string]http.Handler{
        "/debug/flags/v": routes.StringFlagPutHandler(logs.GlogSetter),
        "/debug/pprof/": http.HandlerFunc(pprof.Index), ...
```
`--insecure-diagnostics` flips to http without authn. TLS min version / cipher suites / curves flags shared by webhook and metrics (manager.go:63-80). Required RBAC markers in core/main.go:295-297: `tokenreviews create`, `subjectaccessreviews create`.

1.6 [V] Concurrency: one `--<kind>-concurrency` flag per controller, passed as `controller.Options{MaxConcurrentReconciles: c}` (core/main.go:205-236, 853-855). Defaults: cluster 50, machine 100, machineset 50, clusterclass 10, clustercache 100. Singleton-ish controllers hard-coded with a comment: CRDMigrator `concurrency(1)` "to ensure we don't overwhelm the apiserver" (main.go:497-499); topology MD/MS reconcilers `concurrency(25)` "not adding a command-line arg as this Reconciler will be merged" (main.go:589-591).

1.7 [V] Feature gates via k8s.io/component-base/featuregate: `feature.MutableGates.AddFlag(fs)` (main.go:292), declared in feature/feature.go. Gates graduate: PriorityQueue and ReconcilerRateLimiting are GA/default-true and marked `Deprecated: ... flag will be removed in the v1.17 release` (feature/feature.go:~60-105). Interpretation: gate lifecycle alpha->beta->GA->deprecate->remove is the documented pattern.

1.8 [V] Fail-fast flag validation before NewManager: relational checks on grace periods with `os.Exit(1)` (main.go:343-354); Kubernetes min version check `version.CheckKubernetesVersion` (main.go:356).

1.9 [V] Default log verbosity set programmatically to 2 (main.go:310-314), `ctrl.SetLogger(klog.Background())` (main.go:327), flag dump at V(1) (main.go:322-324).

1.10 [V] Watch scoping: `--namespace` -> `Cache.DefaultNamespaces: map[string]Config{ns: {}}` (setup.go:48-52); `--watch-filter` -> label value predicate (see section 3). `--sync-period` default 10m (main.go:248) vs CR 10h; docs call this out as a scale trade-off (docs/book/src/developer/core/tuning.md:81).

---------------------------------------------------------------------
## 2. Cache and client configuration (core/setup/setup.go)

2.1 [V] ByObject + Label selector + Transform for Secrets (setup.go:63-86). Only secrets with `cluster.x-k8s.io/cluster-name` label are cached; data is stripped unless name ends `-kubeconfig`; managedFields dropped.
```go
ByObject: map[client.Object]ctrlcache.ByObject{
    &corev1.Secret{}: {
        Label: clusterSecretCacheSelector,        // labels.NewRequirement(ClusterNameLabel, selection.Exists, nil)
        Transform: func(in any) (any, error) {
            if s, ok := in.(*corev1.Secret); ok {
                s.SetManagedFields(nil)
                if !strings.HasSuffix(s.Name, fmt.Sprintf("-%s", secret.Kubeconfig)) { s.Data = nil }
```

2.2 [V] mgr.GetClient() NEVER reads Secrets/ConfigMaps from cache; Unstructured reads ARE cached (setup.go:89-101).
```go
Cache: &client.CacheOptions{
    DisableFor:   []client.Object{&corev1.ConfigMap{}, &corev1.Secret{}},
    Unstructured: true,
},
```
Companion: a *second* client that reads the filtered Secret cache, built with `client.New(cfg, client.Options{HTTPClient: mgr.GetHTTPClient(), Cache: &client.CacheOptions{Reader: mgr.GetCache()}})` (setup.go:136-143). So: "default client = uncached for Secret; named secretCachingClient for the labelled subset". Interpretation: the filtered cache cannot serve arbitrary secret Gets, so the default client bypasses it.

2.3 [V] Custom informer constructor for naming/metrics: `NewInformer: capicontrollerutil.NewInformerFunc(scheme, informerName)` (setup.go:84; util/controller/informer.go:35-55) using `toolscache.NewInformerName` and exposing `informer_queued_items`, `informer_processing_latency_seconds`, `informer_store_resource_version` metrics.

2.4 [V] Separate metadata-only Secret cache for watches of ALL secrets (ClusterResourceSet, ExtensionConfig): own `ctrlcache.New` with a DefaultTransform that panics on unexpected types and strips managedFields; added to manager via `mgr.Add` (core/main.go:528-562).
```go
DefaultTransform: func(in interface{}) (interface{}, error) {
    obj, ok := in.(*metav1.PartialObjectMetadata)
    if !ok { panic(...) }
    ...
    return ctrlcache.TransformStripManagedFields()(obj)
```
Consumed via `WatchesRawSource(source.Kind(partialSecretCache, &metav1.PartialObjectMetadata{TypeMeta{Kind:"Secret",APIVersion:"v1"}}, handler.TypedEnqueueRequestsFromMapFunc(...), predicates...))` (core/reconcilers/clusterresourceset/clusterresourceset_controller.go:~94-110). ConfigMaps use builder `WatchesMetadata` (same file :88).

2.5 [V] Workload-cluster caches/clients (ClusterCache): `DefaultTransform: ctrlcache.TransformStripManagedFields()` + a Node providerID index; `DisableFor` Secrets, ConfigMaps, Pods, DaemonSets, PVs, VolumeAttachments "we get/list them e.g. during drain" (setup.go:103-131). Separate `createUncachedClient` (controllers/clustercache/cluster_accessor_client.go:217-225) plus cachedClient with `Unstructured: true` and DisableFor (cluster_accessor_client.go:278-285).

2.6 [V] Docs stance (tuning.md:101-111): "As a rule of thumbs it is always better to deal with stale reads/memory consumption than disabling caching." "Whenever possible, you should avoid uncached list calls, or make sure they happen only once in a reconcile loop." "Also, please be aware that some API server read operations are not cached by default, e.g. reads for unstructured objects, but you can enable caching ... when creating the controller runtime client."

2.7 [V] Typed "contract" structs for external (provider) objects instead of Unstructured, served by a DynamicCache keyed by contract version (setup.go:145-186, `TypesByContract{"v1beta1":..., "v1beta2":...}`; `IsUnstructured: new(false)`). Interpretation: they avoid big unstructured copies for provider kinds that are unknown at compile time.

---------------------------------------------------------------------
## 3. Controller builder wrapper, predicates, rate limiting (util/controller, util/predicates)

3.1 [V] CAPI wraps `builder.Builder` (util/controller/builder.go:55-64, `NewControllerManagedBy(m, predicateLog)` :73). Every Owns()/Watches()/WatchesMetadata() gets `ResourceIsChanged` PREPENDED to drop resync events (builder.go:91-120).
```go
// Note: Prepend a ResourceIsChanged predicate to all "secondary" watches, this will filter out resync events,
//       because resync from the primary object is enough.
// Note: Prepending it to avoid calling (potentially) more resource intensive predicates for resyncs.
predicates = append([]predicate.Predicate{predicatesutil.ResourceIsChanged(blder.mgr.GetScheme(), blder.predicateLog)}, predicates...)
```
ResourceIsChanged compares ResourceVersion, returns true for create/delete/generic (util/predicates/generic_predicates.go:312-337).
Interpretation/Pitfall handled: `For()` is deliberately NOT given it (resync of primary is wanted as a periodic re-reconcile).

3.2 [V] Wrapper defaults: ReconciliationTimeout 1m (builder.go:48-52, 209-213); custom LogConstructor that adds `controller`, `controllerGroup`, `controllerKind`, and `<Kind>: klog.KRef(ns,name)` instead of CR's redundant name/namespace (builder.go:215-242).

3.3 [V] Event filters. `WithEventFilter` (global, applies to For/Owns/Watches) is used for the watch-filter label:
core/reconcilers/machine/machine_controller.go:156-176
```go
c, err := capicontrollerutil.NewControllerManagedBy(mgr, *r.predicateLog).
    For(&clusterv1.Machine{}).
    WithOptions(options).
    WithEventFilter(predicates.ResourceHasFilterLabel(mgr.GetScheme(), *r.predicateLog, r.WatchFilterValue)).
    Watches(&clusterv1.Cluster{}, handler.EnqueueRequestsFromMapFunc(clusterToMachines),
        predicates.ClusterControlPlaneInitialized(...),
        predicates.ResourceHasFilterLabel(...)).      // repeated on a Watches too
    WatchesRawSource(r.ClusterCache.GetClusterSource("machine", clusterToMachines, clustercache.WatchForProbeFailure(...))).
    Build(ctx, r)
```
[U] The repeated ResourceHasFilterLabel on the Cluster watch is probably redundant since WithEventFilter is global in controller-runtime; but note the label on the *Cluster* is what matters for mapped requests, and the global filter checks the event object. Verify before copying.

3.4 [V] Predicate catalogue (util/predicates/generic_predicates.go, cluster_predicates.go):
- `ResourceHasFilterLabel(scheme, logger, labelValue)` :161-178; empty labelValue => allow all (processIfLabelMatch :229-232).
- `ResourceNotPaused` :180-209 (annotation `cluster.x-k8s.io/paused`).
- `ResourceNotPausedAndHasFilterLabel` :211-215 built from `All(...)`; `All`/`Any`/`TypedAll` :32-160.
- `ResourceIsNotExternallyManaged` :246, `ResourceIsTopologyOwned` :279, `ResourceIsChanged` :312.
- Cluster: `ClusterUnpaused` :178-184 = `Any(ClusterCreateNotPaused, ClusterUpdateUnpaused)`; `ClusterUpdateUnpaused` :135 returns false for Create/Delete/Generic and true only on paused true->false transition; `ClusterPausedTransitions` :186; `ClusterControlPlaneInitialized` :232; `ClusterTopologyVersionChanged` :347.
Doc comment on ClusterUpdateUnpaused (cluster_predicates.go:133-134): "it also returns true if the resource provided is not a Cluster to allow for use with controller-runtime NewControllerManagedBy" (note: the code actually returns false for non-Cluster at :147-150; comment is stale [V]).
Logging convention inside predicates: `logger.WithValues("predicate", name, "eventType", "update")`, V(4) for blocking decisions, V(6) for allow (generic_predicates.go:217-243).
Pitfall handled [V]: predicates "that always work in OR with paused predicates so logs are adjusted to not provide false negatives/verbosity at V<=5" (cluster_predicates.go:~170 comment).

3.5 [V] Pausing moved from predicate to in-reconcile condition. Newer controllers do NOT filter paused objects out in predicates; they call `paused.EnsurePausedCondition` inside Reconcile so the Paused condition is surfaced (util/paused/paused.go:44-92).
```go
if isPaused, requeue, err := paused.EnsurePausedCondition(ctx, r.Client, cluster, m); err != nil || isPaused || requeue {
    return ctrl.Result{}, err
}
```
(machine_controller.go:226). Noise-avoidance: returns early when only ObservedGeneration changed and not paused "to avoid the additional reconcile that we would get by requeueing" (paused.go:70-74). Docs: providers SHOULD implement pause via `spec.paused` on Cluster and `cluster.x-k8s.io/paused` annotation and surface `Status.Conditions[Paused]` (docs/book/src/developer/providers/contracts/infra-machine.md:530-532).

3.6 [V] Rate limiting + requeue wrapper (util/controller/controller.go, builder.go:244-254). A `reconcilerWrapper` sits in front of every Reconcile:
- Per-key `reconcileCache` (TTL 10m) with ReconcileAfter => "won't run Reconcile more than once per second" (controller.go:70-90, 99-103).
- Custom `typedItemExponentialFailureRateLimiter`: `rateLimitInterval + baseDelay*2^failures`, base 5ms, max 1000s; `Forget` is a no-op, `ActualForget` called only by wrapper (builder.go:250-254, 292-352).
```go
queueRateLimiter = newTypedItemExponentialFailureRateLimiter[reconcile.Request](rateLimitInterval, 5*time.Millisecond, 1000*time.Second)
blder.options.RateLimiter = queueRateLimiter
```
- Result normalisation: on error drop RequeueAfter/Requeue "to avoid this warning (while preserving Priority)" (controller.go:99-106); RequeueAfter is clamped to at least the rate-limit interval (controller.go:113-124).
- Requires PriorityQueue gate (builder.go:188-190). `UsePriorityQueue` is set on manager Controller config (main.go:373).
- Metrics `capi_reconcile_total{controller,result}`, `capi_reconcile_time_seconds`, `capi_reconcile_stale_cache_skips_total` (util/controller/metrics.go:57-73).
[U] This is CAPI-specific over-engineering for scale; a typical controller should not copy it. Takeaway: with controller-runtime v0.25 `UsePriorityQueue` is a first-class manager option.

3.7 [V] Defer-next-reconcile API on the controller: `DeferNextReconcile(req, after)`, `DeferNextReconcileForObject`, used to avoid hammering workload cluster APIs (builder.go:164-178; controller.go:142-158).

---------------------------------------------------------------------
## 4. Stale-cache protection ("expectations" replacement): consistency store

4.1 [V] CAPI vendored the upstream Kubernetes consistency util (util/controller/consistency.go:37 "initially copied from kubernetes/pkg/controller/util/consistency"). After a controller WRITES a child object it records the written ResourceVersion; next reconcile of the owner is requeued (100ms) until the informer store has seen RV >= written.
Write side:
```go
r.controller.DeferNextReconcileUntilCacheUpToDate(s.machineSet,
    capicontrollerutil.StructuredObject(clusterv1.GroupVersion, "Machine"), machine.ResourceVersion)
```
(core/reconcilers/machineset/machineset_controller.go:409; also :975,996,1015; machinedeployment_controller.go:401,441; topology/cluster/reconcile_state.go:536,697,815,1010,1069).
Check side in the wrapper (controller.go:79-97): `consistencyStore.EnsureReady(ctx, req.NamespacedName)` -> `ctrl.Result{RequeueAfter: requeueDurationStaleCache}` (const 100ms, controller.go:27) with log at V(2) rate-limited to once/10s (`atMostEvery10Seconds`) and V(5) details.
`Controller` interface: builder.go:164-178 (`DeferNextReconcileUntilCacheUpToDate`, `ClearConsistencyStore`). Consistency store keyed by owner NamespacedName+UID, compares RVs via informer stores (consistency.go:60-153).
Interpretation: this replaces classic "ControllerExpectations" (create/delete counters). Needs `WithDynamicCache` on builder for provider kinds (builder.go:157-162).

4.2 [V] APIReader (uncached) used where stale reads cause correctness bugs, with an explicit comment on the field:
core/reconcilers/topology/cluster/cluster_controller.go:82-84
```go
// APIReader is used to list MachineSets directly via the API server to avoid
// race conditions caused by an outdated cache.
APIReader client.Reader
```
Same wording at topology/machinedeployment/machinedeployment_controller.go:56-58, topology/machineset/machineset_controller.go:59-61. Reconcilers validate dependencies in SetupWithManager: `if r.Client == nil || r.APIReader == nil || r.ClusterCache == nil { return pkgerrors.New("Client, APIReader and ClusterCache must not be nil") }` (machine_controller.go:127-133; machineset_controller.go:133-135).

4.3 [V] SSA caching proxy to avoid redundant Apply calls: `ssa.Patch(..., ssa.WithCachingProxy{...})` caches result keyed by hash(original,modified)+RV so repeated reconciles skip SSA (internal/util/ssa/patch.go:46-162, cache.go). Also `PrepareModified` filters intent to "only paths we have an opinion on" "to avoid continuous reconciles" (patch.go:85-87, 164-190). Error messages intentionally omit random object names "If these errors are written to conditions this would lead to an infinite reconcile" (patch.go:132-134).

4.4 [V] Hook/external-object TTL caches (`util/cache/cache.go`: `Cache[E Entry]` generic TTL cache, DefaultTTL 10m, HookCacheDefaultTTL 24h) held on reconcilers; created in SetupWithManager (machine_controller.go:181-182).

---------------------------------------------------------------------
## 5. Field indexes (util/index)

5.1 [V] Central registration function, gated by feature gates, called once from main before controllers (util/index/index.go:AddDefaultIndexes; core/main.go:434-439).
```go
func AddDefaultIndexes(ctx context.Context, mgr ctrl.Manager) error {
    if err := ByMachineNode(ctx, mgr); err != nil { return err }
    if err := ByMachineProviderID(ctx, mgr); err != nil { return err }
    if feature.Gates.Enabled(feature.ClusterTopology) { ByClusterClassRef ... }
```
5.2 [V] Pattern per index (util/index/machine.go:28-62): exported field-name const, `mgr.GetCache().IndexField(ctx, &T{}, Field, ExtractFn)`, exported extractor func (so fake client can reuse it), extractor panics on wrong type, returns nil for "no value".
```go
MachineNodeNameField = "status.nodeRef.name"
func MachineByNodeName(o client.Object) []string {
    machine, ok := o.(*clusterv1.Machine)
    if !ok { panic(fmt.Sprintf("Expected a Machine but got a %T", o)) }
    if machine.Status.NodeRef.IsDefined() { return []string{machine.Status.NodeRef.Name} }
    return nil
}
```
5.3 [V] Consumption: `r.Client.List(ctx, list, client.MatchingFields{index.MachineNodeNameField: node.Name})` (machine_controller.go:1159, 1177; machinehealthcheck_controller.go:709).
5.4 [V] Tests: envtest suite passes `SetupIndexes` to `envtest.Run` (core/reconcilers/machine/suite_test.go:~44-48,120); fake client uses `.WithIndex(&clusterv1.Cluster{}, index.ClusterClassRefPath, index.ClusterByClusterClassRef)` (core/webhooks/admission/clusterclass_test.go:1962).
5.5 [V] Indexes for the remote-cluster caches are declared as data: `clustercache.CacheOptionsIndex{Object:&corev1.Node{}, Field: index.NodeProviderIDField, ExtractValue: index.NodeByProviderID}` (controllers/clustercache/index.go; wired in setup.go:103-109).

---------------------------------------------------------------------
## 6. Patch helper (util/patch/patch.go, options.go)

6.1 [V] Pattern: `NewHelper(obj, client)` BEFORE mutation (deep-copies `beforeObject`, patch.go:70-81), mutate, `Patch(ctx, obj, opts...)` in a `defer`. Documented as the write-minimisation recipe: "use a defer call to patch the object with the patch helper to make a single write at the end of the reconcile loop (and only if there are actual changes)" (tuning.md:113).
Real usage (machine_controller.go:229-262):
```go
patchHelper, err := patch.NewHelper(m, r.Client)
...
defer func() {
    updateRes := r.updateStatus(ctx, s)
    retres = util.LowestNonZeroResult(retres, updateRes)
    // Always attempt to patch the object and status after each reconciliation.
    // Patch ObservedGeneration only if the reconciliation completed successfully
    patchOpts := []patch.Option{}
    if reterr == nil { patchOpts = append(patchOpts, patch.WithStatusObservedGeneration{}) }
    if err := patchMachine(ctx, patchHelper, m, patchOpts...); err != nil {
        reterr = kerrors.NewAggregate([]error{reterr, err})
    }
}()
```
6.2 [V] No-op avoidance, three layers: (a) `reflect.DeepEqual(obj, h.beforeObject)` early return (patch.go:89); (b) `h.calculateChanges` computes which top-level sections (metadata/spec/status) changed and `shouldPatch` skips others (patch.go:~156-160, 197-200); (c) empty merge-patch check `bytes.Equal(data, []byte("{}"))` before sending, plus special cases for first-time conditions writes `{"status":{"v1beta2":{}}}` (patch.go:207-243).
```go
// Check for empty patches as a safeguard to avoid continuously sending empty patches to the apiserver.
data, err := client.MergeFrom(beforeObject).Data(afterObject)
if bytes.Equal(data, []byte("{}")) { return nil }
return h.client.Patch(ctx, afterObject, client.RawPatch(types.MergePatchType, data))
```
Spec/metadata go through `Patch`, status through `Status().Patch`, as separate calls (patch.go:197-250).
6.3 [V] Order and conflict handling (patch.go:168-195): conditions FIRST, then spec/metadata, then status; errors aggregated. Comment: "Given that we pass in metadata.resourceVersion to perform a 3-way-merge conflict resolution, patching conditions first avoids an extra loop if spec or status patch succeeds first given that causes the resourceVersion to mutate."
Tolerates NotFound on status patch when object is being deleted and has no finalizers (patch.go:185-188).
6.4 [V] Conditions patch = retry loop with GET latest + 3-way condition merge + optimistic lock (patch.go:255-380):
```go
backoff := wait.Backoff{Steps: 5, Duration: 100 * time.Millisecond, Jitter: 1.0}
return wait.ExponentialBackoff(backoff, func() (bool, error) {
    latest := h.beforeObject.DeepCopyObject().(client.Object)
    h.client.Get(ctx, key, latest)
    conditionsPatch := client.MergeFromWithOptions(latest.DeepCopyObject().(client.Object), client.MergeFromWithOptimisticLock{})
    ... diff.Apply(latestSetter, conditions.ForceOverwrite(forceOverwrite), conditions.OwnedConditionTypes(ownedV1Beta2Conditions))
    err := h.client.Status().Patch(ctx, latest, conditionsPatch)
    case apierrors.IsConflict(err): return false, nil   // retry
```
(patch.go:351-380.) Note this GET goes through `h.client`, i.e. the (cached) manager client, unless the caller passed an uncached one [U].
6.5 [V] `ownedConditions` semantics (util/conditions/patch.go:130-228): `conditions.NewPatch(before, after)` yields Add/Change/Remove ops; `Apply(latest, opts)`: for an owned condition type or ForceOverwrite -> always take `after`; for a non-owned condition -> if `latest` diverged from `Before` and `HasSameState(latest, after)` is false => return conflict error; if same state keep latest's LastTransitionTime. So multiple controllers can write different conditions on one object without clobbering.
Options (util/patch/options.go:27-118): `WithOwnedConditions{Conditions []string}` (v1beta2/metav1), `WithOwnedV1Beta1Conditions`, `WithForceOverwriteConditions` ("should only ever be set in controller managing the object being patched"), `WithStatusObservedGeneration` ("only if there is a change"), `Metav1ConditionsFieldPath` overrides.
Usage with explicit list (machine_controller.go:341-358): owned = Paused, Available, Ready, BootstrapConfigReady, InfrastructureReady, NodeReady, NodeHealthy, Deleting, Updating; others (UpToDate, HealthCheckSucceeded, OwnerRemediated set by other controllers) are intentionally NOT owned.
6.6 [V] Status.observedGeneration: set by helper from `metadata.generation` ONLY when requested and "only if there is a change"; callers request it only when `reterr == nil` (machine_controller.go:256-258; patch.go:150-163 uses `unstructured.SetNestedField(h.after.Object, h.after.GetGeneration(), "status", "observedGeneration")`).
6.7 [V] Finalizer is added with a SEPARATE immediate patch BEFORE the patch helper/pause handling, then return so the update re-triggers reconcile (util/finalizers/finalizers.go:27-48):
```go
// Add finalizer first if not set to avoid the race condition between init and delete.
if finalizerAdded, err := finalizers.EnsureFinalizer(ctx, r.Client, m, clusterv1.MachineFinalizer); err != nil || finalizerAdded {
    return ctrl.Result{}, err
}
```
(machine_controller.go:217-220). Implementation: no-op if deletionTimestamp set or already contains; else `c.Patch(ctx, o, client.MergeFrom(original))` using `controllerutil.AddFinalizer`.

---------------------------------------------------------------------
## 7. Conditions (util/conditions, metav1.Condition-based v1beta2)

7.1 [V] CAPI v1beta2 uses plain `[]metav1.Condition` with helper package `util/conditions` (doc.go: "implements utils for metav1.Conditions that will be used starting with the v1beta2 API"). Legacy clusterv1.Condition kept under util/conditions/deprecated/v1beta1 (patch helper handles both).
Interfaces: `Getter{GetConditions() []metav1.Condition}` (getter.go:~30), `Setter` adds SetConditions.
7.2 [V] `conditions.Set` stamps ObservedGeneration from `metadata.generation`, truncates LastTransitionTime to seconds ("prevents inconsistencies from what we have in objects in memory and what Marshal/Unmarshal will do"), keeps LTT unless status changes, delegates to `meta.SetStatusCondition`, sorts (Available, Ready first; Deleting, Paused last; rest alphabetical) (util/conditions/setter.go:72-111; sort.go:33-37).
```go
if objMeta, ok := targetObj.(metav1.Object); ok { condition.ObservedGeneration = objMeta.GetGeneration() }
...
condition.LastTransitionTime.Time = condition.LastTransitionTime.Truncate(1 * time.Second)
```
7.3 [V] Helpers: `HasSameState` (Type,Status,Reason,ObservedGeneration,Message; excludes LTT) and `HasSameStateExceptObservedGeneration` (conditions/patch.go:238-252); `NewSummaryCondition/SetSummaryCondition` (summary.go:58,156) with options `ForConditionTypes`, `IgnoreTypesIfMissing`, `NegativePolarityConditionTypes`, `CustomMergeStrategy`, `TargetConditionType`; `NewAggregateCondition/SetAggregateCondition` over children (aggregate.go:60,131); `NewMirrorCondition/SetMirrorCondition(+FromUnstructured)` to mirror a child's Ready (mirror.go:62-150).
7.4 [V] Status computation is its own pass at end of reconcile (`updateStatus`), separate from the phase functions, so conditions are ALWAYS set even on early-error paths (machine_controller_status.go:41-74; comment: "this func should ensure that the conditions managed by this controller are always set in order to comply with the recommendation in the Kubernetes API guidelines"). Phases only write "reason/message" hints into the `scope` (e.g. `s.deletingReason`, `s.deletingMessage`, machine_controller.go:~290-300) which updateStatus turns into conditions.
7.5 [V] Ready/Available semantics (machine_controller_status.go:735-780): Ready = summary over a default set of "readiness gates" (Deleting, BootstrapConfigReady, InfrastructureReady, NodeHealthy, HealthCheckSucceeded[ignored if missing]) plus user-defined `spec.readinessGates` with per-gate Polarity; Deleting/Updating are NEGATIVE polarity conditions (`conditions.NegativePolarityConditionTypes`); custom merge strategy to improve reasons/reduce noisy messages. Available is a separate condition (e.g. returns requeue result: `return setAvailableCondition(...)`, line 74).
7.6 [V] Contract docs for providers: "SHOULD implement status.conditions ... CAPI will only consider type, status, reason, message, lastTransitionTime, observedGeneration"; if a `Ready` condition exists it is mirrored to Machine's InfrastructureReady (docs/book/src/developer/providers/contracts/infra-machine.md:340-352).

---------------------------------------------------------------------
## 8. Reconcile structure

8.1 [V] Top of Reconcile (core/reconcilers/machine/machine_controller.go:188-262): Get -> NotFound => `ctrl.Result{}, nil`; enrich logger (`ctrl.LoggerInto(ctx, ctrl.LoggerFrom(ctx).WithValues("Cluster", klog.KRef(ns, name)))`, `clog.AddOwners(ctx, r.Client, m)` adds owner kinds as KV) -> get Cluster -> EnsureFinalizer -> NewHelper -> EnsurePausedCondition -> build `scope` struct -> `defer { updateStatus; Patch }` -> deletion vs normal.
```go
type machineReconcileFunc func(context.Context, *scope) (ctrl.Result, error)
alwaysReconcile := []machineReconcileFunc{r.reconcileMachineOwnerAndLabels, r.reconcileBootstrap, r.reconcileInfrastructure, r.reconcileNode, r.reconcileCertificateExpiry}
if !m.DeletionTimestamp.IsZero() { return doReconcile(ctx, append(alwaysReconcile, r.reconcileDelete), s) }
return doReconcile(ctx, append(alwaysReconcile, r.reconcileInPlaceUpdate), s)
```
8.2 [V] Phases are methods `func(ctx, *scope) (ctrl.Result, error)` run in a flat slice; `doReconcile` (machine_controller.go:376-396): every phase runs even after an earlier phase errored (the `continue` only skips merging that phase's Result); errors are aggregated with `kerrors.NewAggregate`; results combined with `util.LowestNonZeroResult`.
```go
for _, phase := range phases {
    phaseResult, err := phase(ctx, s)
    if err != nil { errs = append(errs, err) }
    if len(errs) > 0 { continue }
    res = util.LowestNonZeroResult(res, phaseResult)
}
if len(errs) > 0 { return ctrl.Result{}, kerrors.NewAggregate(errs) }
```
(Interpretation: this lets later phases still populate status/scope for conditions even if an earlier phase failed.)
8.3 [V] `util.LowestNonZeroResult(i, j ctrl.Result)` (util/util.go:715-730): zero loses; `Requeue:true` wins; else smaller RequeueAfter wins. Note `//nolint:staticcheck SA1019: Requeue is deprecated`.
8.4 [V] Deletion flow in-phase (`reconcileDelete`, machine_controller.go:~430+): sets "fallback" deletingReason/Message first, then specific ones; skippable steps are logged with `log.Info("Skipping deletion of Kubernetes Node ...", "Node", klog.KRef("", nodeName), "cause", err.Error())`; waits on hook annotations "Return early without error, will requeue if/when the hook owner removes the annotation" i.e. event-driven rather than polling.
8.5 [V] Owner refs: `util.EnsureOwnerRef(m.GetOwnerReferences(), metav1.OwnerReference{...})` (machine_controller.go:~418-424) in a phase function mutating in-memory object; persisted by the deferred patch. Docs: RBAC for `<owner>/finalizers update` is needed when `blockOwnerDeletion` is set (providers/getting-started/controllers-and-reconciliation.md RBAC section).
8.6 [V] Error handling: wrap with `fmt.Errorf("...: %w", err)` / `pkgerrors.Wrapf` (machine_controller.go:~235-260). Aggregate via `kerrors.NewAggregate` in deferred patch to keep both reconcile and patch errors (machine_controller.go:260).
8.7 [V] Requeue policy: `ctrl.Result{}` + error => rate-limited backoff; RequeueAfter used for polling external things; docs steer AWAY from requeue-polling: "The best optimization that can be done is to avoid any work at all ... instead of re-queuing every few seconds when a controller is waiting for something to happen ... it is always better to watch for events" (tuning.md:93). ClusterCache doc: "If GetClient does return an error, controllers should not requeue, requeue after or return an error and go in exponential backoff. Instead, they should register a source (via GetClusterSource) to receive reconnect events" (controllers/clustercache/doc.go:~13-17).
8.8 [V] ClusterCache is itself a controller-runtime reconciler registered via `clustercache.SetupWithManager(ctx, mgr, opts, controller.Options)` returning an interface; it uses `ErrClusterNotConnected` sentinel wrapped with `pkgerrors.WithMessage` (cluster_accessor.go:383-446), never holds write lock during connect (doc.go "Implementation considerations").
8.9 [V] Logging conventions (docs/book/src/developer/core/logging.md):
- "Logs are for SRE & developers, not for end users! ... opportunity for improvement of other observability in our API, like e.g. conditions and events." (:7-9)
- Contextual logging with `ctrl.LoggerFrom(ctx)` / `LoggerInto` (:63-64); must add hierarchy KVs e.g. Cluster for a MachineDeployment (:71-75).
- "Developers MUST use `klog.KObj` or `klog.KRef`" (:87); kinds UpperCamel keys (`MachineSet`), all others lowerCamel (`oldReplicas`, `resourceVersion`) (:89-93); standard keys `reason`, `diff` (:107-108).
- Messages: start with capital letter (:112), no trailing period (:113), log BEFORE the action using -ing verbs (:114-115), API field names written with full path as in the API (`spec.providerID`) (:119-120), controlled/referenced object: include Kind and ns/name in message AND as KV (:121-123).
- Levels: <=3 "what happened", >=4 "how it happened", never above 5 (:130-136); default V(2) for core controllers (:138).
Example in code: `log.Info("Waiting for pre-drain hooks to succeed", "hooks", strings.Join(hooks, ","))` (machine_controller.go:~455); `log.Info(fmt.Sprintf("Completed triggering in-place update for Machine %s", machine.Name))` (machineset_controller.go:411).
8.10 [V] Event recorder: `r.recorder = mgr.GetEventRecorderFor("machine-controller")` (machine_controller.go:184), `r.recorder.Event(machine, corev1.EventTypeNormal, "SuccessfulStartInPlaceUpdate", "...")` (machineset_controller.go:412).
8.11 [V] Mapper helpers as constructors that can fail at setup: `clusterToMachines, err := util.ClusterToTypedObjectsMapper(mgr.GetClient(), &clusterv1.MachineList{}, mgr.GetScheme())` then used in `handler.EnqueueRequestsFromMapFunc(...)` (machine_controller.go:139-150).
8.12 [V] Owns vs Watches: MachineSet uses `Owns(&Machine{})` PLUS `Watches(&Machine{}, EnqueueRequestsFromMapFunc(r.MachineToMachineSets))` "enqueues MachineSet for corresponding Machine resources, if no managed controller reference (owner) exists" (machineset_controller.go:147-170) - label-selector adoption path. Owns used in 4 reconcilers (machinedeployment:114, machineset:150, kcp:156, machinepool:109).
8.13 [V] Contract docs "typical reconciliation workflow" for providers (infra-machine.md:610-650): exit if no owner Machine; exit if no Cluster; add finalizer; gate on Cluster infra ready ("This check should not be blocking any further delete reconciliation flows"); ...; "Patch the resource to persist changes"; deletion: cleanup infra, "If any errors are encountered, exit the reconciliation", remove finalizer, patch.

---------------------------------------------------------------------
## 9. Testing

9.1 [V] Three layers (docs/book/src/developer/core/testing.md:8-50): unit (go test + gomega + fakeclient), integration (envtest + real controllers), e2e. Quote (:14-16): "using fakeclient is not suitable for all the use cases due to some limitations in how it is implemented. In some cases contributors will be required to use envtest."
"unit test are focused on code structure: func(input) = output ... integration test are user story driven" (:90-92). Test "by layers" to avoid redundant tests (:81-85). Use "generic providers" (GenericInfrastructureCluster etc.) not real providers (:28-36).
9.2 [V] envtest harness: internal/test/envtest/environment.go. `envtest.Run(ctx, RunInput{M, SetupManagerCacheOptions, ManagerClientOptions, SetupEnv, SetupIndexes, SetupReconcilers})` (environment.go:173; used in core/reconcilers/machine/suite_test.go:115-125). The test manager reuses PRODUCTION cache/client options so tests exercise the same caching behaviour:
```go
SetupManagerCacheOptions: func(scheme *runtime.Scheme) ctrlcache.Options {
    return setup.ManagerCacheOptions(scheme, "test-controller-manager", "", 10*time.Minute)
},
ManagerClientOptions: setup.ManagerClientOptions(),
```
Test manager: `Metrics: metricsserver.Options{BindAddress: "0"}`, `GracefulShutdownTimeout: 90s` (environment.go:391-410), real webhooks with CRDs preloaded, `UsePriorityQueue` follows gate.
9.3 [V] Suite polling defaults: `SetDefaultEventuallyPollingInterval(100 * time.Millisecond)`, `SetDefaultEventuallyTimeout(30s)` (suite_test.go:112-113). Test speed tweaks via test-only hooks: `SetConnectionCreationRetryInterval(2 * time.Second)` "otherwise client creation is only retried every 30s. If we get unlucky tests are then failing with timeout" (suite_test.go:~92-95).
9.4 [V] Cache-staleness anti-flake helpers (environment.go:261-266, 596-740): `CreateAndWait`, `CleanupAndWait`, `DeleteAndWait`, `PatchAndWait` (SSA) which poll the CACHED client with `wait.ExponentialBackoff(cacheSyncBackoff, ...)`.
```go
var cacheSyncBackoff = wait.Backoff{Duration: 100 * time.Millisecond, Factor: 1.5, Steps: 8, Jitter: 0.4}
// "NOTE: Waiting for the cache to be updated helps in preventing test flakes due to the cache sync delays."
```
Docs (testing.md:~461-467): "it could happen that after creating or deleting objects the cache takes a few milliseconds to get updated. This can lead to test flakes, and thus it always recommended to use patterns like create and wait or delete and wait".
9.5 [V] Isolation: per-test namespace `ns, err := env.CreateNamespace(ctx, "test-machine-watches")` + deferred cleanup (machine_controller_test.go:64, 215; testing.md:~470-500). "Because there are no controllers monitoring built-in resources, objects do not get deleted, even if an OwnerReference is set up; as a consequence, usually test implements code for cleaning up created objects" (no GC controller in envtest).
9.6 [V] Panic detection: after the suite, `verifyPanicMetrics()` gathers `controller_runtime_reconcile_panics_total`, `controller_runtime_webhook_panics_total`, `controller_runtime_conversion_webhook_panics_total` and FAILS the run if any >0 (environment.go:245, 758-790). Great trick: controller-runtime recovers panics so tests would otherwise pass.
9.7 [V] Assertions with `g.Eventually(func() bool { if err := env.Get(...); err != nil { return false }; return machine.Status.NodeRef.IsDefined() }, timeout).Should(BeTrue())` (machine_controller_test.go:188-195). Shared builders `util/test/builder`, matchers from `sigs.k8s.io/controller-runtime/pkg/envtest/komega` (testing.md:74-77). Env var knobs: `CAPI_DISABLE_TEST_ENV`, `CAPI_TEST_ENV_KUBECONFIG`, `CAPI_TEST_ENV_SKIP_STOP` (testing.md:105-111).
9.8 [V] Fake client: `fake.NewClientBuilder().WithScheme(scheme).WithObjects(...).WithStatusSubresource(&clusterv1.MachineSet{}).Build()` (core/reconcilers/machineset/machineset_controller_test.go:705, 764; util/paused/paused_test.go:107). Needed so `Status().Patch/Update` behave like a real subresource. `WithIndex(&T{}, field, extractor)` when code uses `MatchingFields` (admission/clusterclass_test.go:1962).
9.9 [V] Fakes for CAPI-specific services built-in: `clustercache` has `cluster_cache_fake.go`; `util/controller/controller_fake.go`; runtime client fake `internal/runtime/client/fake`.
9.10 [U] No `Consistently` usage found in the machine tests I grepped (grep returned none); other packages not checked.

---------------------------------------------------------------------
## 10. Explicit "do this, not that" from maintainers (docs)

All [V] unless noted.
- Caching: "As a rule of thumbs it is always better to deal with stale reads/memory consumption than disabling caching." (tuning.md:103) AND still use APIReader where races matter (4.2). Net rule: default to cache; add uncached reads only for specific list calls with a comment.
- "Whenever possible, you should avoid uncached list calls, or make sure they happen only once in a reconcile loop" (tuning.md:111).
- Writes: "use a defer call to patch the object with the patch helper to make a single write at the end of the reconcile loop (and only if there are actual changes)" (tuning.md:113).
- Prefer watches over polling; "when implementing watches, non-relevant changes should be filtered out whenever possible" (tuning.md:93).
- "Only optimize if there is clear evidence of an issue" (tuning.md:6); tune QPS/burst/concurrency/sync-period only with data; raising concurrency raises API call volume (tuning.md:77-83). "Re-using long lived clients instead of continuously re-creating new ones" (tuning.md:117); expensive ops named: key generation, client creation (tuning.md:99).
- Reconcilers must be stateless: "you should not store any important state in memory: if you need it, write it into a Kubernetes object and store it." (providers/getting-started/controllers-and-reconciliation.md "Reconciliation" section). Note CAPI itself keeps TTL caches but only as optimisations.
- Return `ctrl.Result{}, nil` on NotFound; return error otherwise (same doc + machine_controller.go:192-200).
- Add finalizer BEFORE anything else, in its own write (machine_controller.go:217).
- Do not rely on Cluster API internals when implementing contract resources (infra-machine.md:31 "you MUST NOT rely on any Cluster API internal behaviour").
- Provider naming: don't use a derived infra name as the sole lookup key; use provider-owned tags/labels (providers/best-practices.md, bullet 2). Don't pass invalid names to the platform and "rely on it to reject the request during provisioning".
- Logging MUSTs: see 8.9. "Logs are for SRE & developers, not for end users" (use conditions/events for users).
- Pause: providers SHOULD implement pause; surface via Paused condition (infra-machine.md:530-532).
- ClusterCache: do not requeue on "not connected"; register a source (clustercache/doc.go).
- Support `--watch-filter` so multiple instances can run (providers/contracts/infra-machine.md:~518-522; core/support-multiple-instances.md).
- Tests: envtest shared across a package, isolate per namespace, "create and wait"/"delete and wait" patterns (testing.md).

---------------------------------------------------------------------
## 11. Things worth copying vs not copying into a generic pure-controller-runtime skill (my synthesis, [U] judgement)

Copy: (1) `defer patchHelper.Patch` single-write pattern with before-copy diff, empty-patch guard, conditions-first ordering; (2) owned-condition conflict model if multiple writers; (3) ObservedGeneration only on success; (4) ByObject Label+Transform for big/sensitive kinds, DisableFor Secret/ConfigMap on the default client, separate small cache for metadata-only watches; (5) prepend ResourceIsChanged on non-primary watches; (6) register indexes before controllers, export extractor funcs for fake client reuse; (7) per-controller `--x-concurrency` flags + kube-api-qps/burst flags; (8) secure diagnostics address + TokenReview/SAR RBAC markers; (9) envtest harness reusing prod cache/client opts + CreateAndWait helpers + panic-metric check; (10) `WithStatusSubresource` on fake client; (11) LogConstructor with klog.KRef, capitalized messages without trailing period (matches user's own standard); (12) phases as flat `func(ctx, *scope)(ctrl.Result, error)` slice + aggregate errors + LowestNonZeroResult + a final updateStatus pass.
Don't copy (CAPI scale-specific): custom reconcilerWrapper rate limiter, consistency store, SSA caching proxy, DynamicCache/contract typed shims, workload ClusterCache, CRD migrator.
Pitfall to document: CAPI itself notes the stale-cache problem is handled either by APIReader (cheap, rare) or the consistency store (heavy); a generic skill should recommend (a) idempotent create with AlreadyExists tolerance, (b) APIReader for rare critical reads, (c) RV-based wait only if needed.
Deprecated-API notes seen: `ctrl.Result.Requeue` is deprecated (util/util.go:714, controller.go:104-106 nolint staticcheck) - prefer `RequeueAfter`.
