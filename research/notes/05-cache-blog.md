# Source: "How the controller-runtime Cache Actually Works, and Why Your Controller Does Not Crash the API Server"
URL: https://kubernetes.io/blog/2026/07/29/controller-runtime-cache-explained/
Authors: Andrei Kvapil, Timofei Larkin (Ænix). Published 2026-07-29. Page says it was REVISED to fix "significant technical inaccuracies" in the original.
Coverage: article opening (first ~1 paragraph) NOT read (fetch truncated). Everything below is from the rest of the body, extracted by a summarizer, not verified against controller-runtime source. Verify each claim against code/FAQ before it goes in the skill.

## Mechanics
- Get/List read the local cache; writes (Create/Update/Patch/Apply/Delete/DeleteAllOf) go to the API server.
- Pipeline: Reflector -> RealFIFO delta queue -> Indexer/ThreadSafeStore -> SharedIndexInformer -> handlers -> workqueue -> Reconcile. One informer per GVK per manager, shared by all controllers/webhooks.
- Store updates before handlers are notified; handler delivery async. Objects in handlers may be ahead of the event.
- Dedup only in controller workqueue (namespace/name key).
- Reconcile runs only after all of a controller's sources have synced. Get on a type nothing registered lazily starts an informer and blocks until synced.
- Single RWMutex over store + indexes; big List can delay informer writes (kubernetes/kubernetes#130767).

## Pitfalls
1. Read-after-write returns stale data (FAQ: https://github.com/kubernetes-sigs/controller-runtime/blob/main/FAQ.md). Reconcile must be idempotent; live reads don't fix it.
2. Objects from client Get/List are deep-copied (opt out: UnsafeDisableDeepCopy); objects in Predicates/EventHandlers are shared store objects -> DeepCopy before mutating.
3. cache.Options.SyncPeriod (default 10h) = resync, emits OnUpdate(old, old), no API traffic; old/new-comparing predicates (GenerationChangedPredicate) drop it.
4. No time.Sleep; return ctrl.Result{RequeueAfter: d}.
5. Don't use mgr.GetClient() before mgr.Start() (ErrCacheNotStarted); use mgr.GetAPIReader().
6. Don't write "cache then fall back to API" logic.

## Indexes
- mgr.GetFieldIndexer().IndexField(ctx, &corev1.Pod{}, "spec.nodeName", func(obj client.Object) []string {...}); query with client.MatchingFields{...}. Unregistered index -> error. Equality only. Get never uses field index. MatchingLabels is not indexed.

## Limiting cache
- ctrl.Options{Cache: cache.Options{ByObject: map[client.Object]cache.ByObject{&corev1.Secret{}: {Namespaces: map[string]cache.Config{"ns": {}}, Label: labels.SelectorFromSet(...)}, &corev1.Pod{}: {Transform: func(obj any)(any, error){... ManagedFields=nil ...}}}}}
- Label/Field selectors pushed to the watch; Transform runs pre-store; DefaultLabelSelector/DefaultNamespaces global. Objects outside selector look nonexistent.
- Heavy types: Secrets, Nodes, Events, Pods.

## Metadata-only
- metav1.PartialObjectMetadataList with GVK set -> separate metadata-only informer; large memory savings (esp. Secrets); no spec/status/data.

## APIReader / DisableFor
- mgr.GetAPIReader() for uncached types, pre-Start reads, pagination (client.Continue unsupported by cache client).
- ctrl.Options{Client: client.Options{Cache: &client.CacheOptions{DisableFor: []client.Object{&corev1.Secret{}}}}} -> no informer, no events; pair with metadata-only watch for triggers.
- WatchesRawSource / source.Channel for event feeds without a watch.

## Article checklist
Constrain cache scope; Transform; IndexField per MatchingFields; DeepCopy before mutating in handlers/predicates; idempotent Reconcile; no read-after-write assumptions; APIReader for init/uncached; PartialObjectMetadata where possible; RequeueAfter for deferral.
