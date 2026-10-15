# Cache and memory

Read this when you add, change or review anything that reads objects (Get/List), watches a type, adds an index or a
predicate, or when the manager's memory or API load is a concern. Snippets compile against controller-runtime v0.25.2
(imports omitted). Tags are defined in `sources.md`; `run` = reproduced in `research/experiments`.

## 1. How the cache works

- Reads (`Get`/`List`) of the manager client come from an in-memory informer cache. Writes (`Create`, `Update`, `Patch`,
  `Apply`, `Delete`, `DeleteAllOf`) always go to the API server. [CR, BLOG]
- There is one informer per GVK AND per representation: typed, `unstructured.Unstructured`, and
  `PartialObjectMetadata`. Reading `*corev1.Secret` and a `PartialObjectMetadata` of Secret creates two list+watch streams
  and two stores. Multi-namespace caches add one informer per namespace. [CR; the BLOG claim "one per GVK" is imprecise]
- Unstructured reads bypass the cache unless `client.Options.Cache.Unstructured` is true; then a third family of full
  objects is cached. [CR]
- The store is updated from watch events asynchronously. A read right after your own write can return the old object or
  NotFound: the write response comes back before the watch event does. Reconcile must be idempotent and must not assume
  it sees what it just wrote. Live reads do not fix this. [CR FAQ, BLOG, TALK]
- A controller starts workers only after all of its sources have synced. A `Get` of a type nothing registered lazily
  creates a full informer and blocks until it syncs (unless `ReaderFailOnMissingInformer`). [CR, BLOG]
- Objects returned by `Get`/`List` are deep copies. Objects passed to predicates and event handlers are the shared store
  objects: `DeepCopy` before mutating. [CR, BLOG]
- A selector or namespace restriction is applied by the API server on list and watch, so excluded objects never enter
  the cache and look nonexistent (`Get` returns NotFound). [CR]

## 2. cache.Options you will actually use

```go
func managerOptions() (ctrl.Options, error) {
	managed, err := labels.NewRequirement("example.com/managed", selection.Exists, nil)
	if err != nil {
		return ctrl.Options{}, err
	}
	return ctrl.Options{
		Cache: cache.Options{
			DefaultTransform:            cache.TransformStripManagedFields(),
			ReaderFailOnMissingInformer: true,
			ByObject: map[client.Object]cache.ByObject{
				&corev1.Secret{}: {
					Label:     labels.NewSelector().Add(*managed),
					Transform: stripAndPrune, // replaces DefaultTransform for Secrets, see section 3
				},
			},
		},
	}, nil
}
```

| Option | Use | Tags |
|---|---|---|
| `DefaultTransform` | Applies to every type that has no own `Transform`. Set it to `cache.TransformStripManagedFields()` (its doc recommends this when you do not read managedFields). | [CR] |
| `ByObject[obj]` | Per-type `Namespaces`, `Label`, `Field`, `Transform`, `UnsafeDisableDeepCopy`. Keyed by GVK, so it applies to the typed AND metadata informer of that GVK. `Namespaces` must be nil for cluster-scoped types. | [CR] |
| `DefaultNamespaces`, `DefaultLabelSelector`, `DefaultFieldSelector` | Global filters; a `ByObject` value wins. Nil `ByObject.Namespaces` inherits `DefaultNamespaces`; an empty non-nil map means all namespaces. | [CR] |
| `ReaderFailOnMissingInformer` | A read of a type with no registered informer returns `ErrResourceNotCached` instead of silently caching the whole type. Register informers (For/Owns/Watches/`GetInformer`) first. Prefer it on. | [CR, run] |
| `SyncPeriod` | Default 10h, jittered. Resync re-emits `OnUpdate(obj, obj)` for every cached object; no API traffic. Old/new-comparing predicates drop it. CAPI lowers it to 10m for its own scale trade-off. | [CR, CAPI] |
| `UnsafeDisableDeepCopy` (default, per type or per call) | Removes the copy on reads and also leaves TypeMeta empty on typed results. Read-only use only; mutating the result corrupts the shared store. Avoid unless a profile shows copies dominate. | [CR] |

Multi-namespace caches cost N namespaces x M types informers; with large N prefer one cluster-wide informer plus a
label selector. [CR]

## 3. Reducing footprint: strip, filter, prune

Apply in this order for each cached type, cheapest and safest first.

1. Strip managedFields: `DefaultTransform: cache.TransformStripManagedFields()`. It uses `meta.Accessor`, so it is safe
   for typed, unstructured and metadata informers. [CR]
2. Filter server-side with `ByObject` `Label`/`Field`/`Namespaces` (or the `Default*` forms). Objects outside the
   selector never enter memory. Field selectors only work for fields the API server supports for that type. [CR]
3. Prune fields you never read, inside a `Transform`.

```go
// pruneSecret keeps only the Secret keys the controller reads. In place, idempotent, never nil.
func pruneSecret(in any) (any, error) {
	s, ok := in.(*corev1.Secret) // metadata informers pass *PartialObjectMetadata: leave it alone
	if !ok {
		return in, nil
	}
	crt, found := s.Data["tls.crt"]
	s.Data, s.StringData = nil, nil
	if found {
		s.Data = map[string][]byte{"tls.crt": crt}
	}
	return in, nil
}

// A per-type Transform REPLACES DefaultTransform, so compose it.
func stripAndPrune(in any) (any, error) {
	in, err := cache.TransformStripManagedFields()(in)
	if err != nil {
		return nil, err
	}
	return pruneSecret(in)
}
```

Transform rules:

- Mutating in place is safe: since client-go v1.27 the transform sees the object before any other actor. It must be
  idempotent and fast: it runs on the queue insert path. [CR, client-go doc]
- A per-type `ByObject.Transform` REPLACES `DefaultTransform`; there is no chaining. Call the strip function inside it, as
  above. Verified: a Secret with its own prune-only Transform kept its managedFields while a ConfigMap (default only) had
  none; the composed transform stripped both. [CR, run]
- The same transform is installed on the metadata and unstructured informers of that GVK. Type-switch (or use
  `meta.Accessor`) and return `in` for types you do not handle. [CR]
- Real delete events are transformed, tombstones (`DeletedFinalStateUnknown`) are not. Always return the object. [CR]
- NEVER return `nil, nil` to "drop" an object. With client-go 0.37 default settings the informer ends with an EMPTY store
  (including the objects you wanted) and `HasSynced()` still reports true; the error is swallowed. Use selectors.
  [run: `experiments/exp-nil-transform`, re-run for this file: synced=true, store=[]]
- Pruning changes what every reader sees: reconcilers, predicates, handlers and map functions all get the pruned object.
  Adding a new read (another Secret key, another field) requires updating the transform in the same change.
- Update events still fire for changes to pruned-away data, with a new resourceVersion and identical retained bytes. If
  those wake-ups are unwanted, compare the retained fields in a predicate. If you do care about a key, retain it.
  [run: changing `tls.key` fired an update oldRV 219 -> newRV 220; a predicate comparing `tls.crt` bytes suppressed it]

### Writing objects read from a pruned cache

- `Update` with an object built from the pruned copy sends the pruned object as the full replacement: the other Data
  keys are deleted on the server. [run: after Update, server keys were `[tls.crt]` instead of tls.crt, tls.key, ca.crt]
- `Patch` with `client.MergeFrom(base)` where `base` is the pruned copy only sends the diff you made. Changing a label
  left all three keys intact. [run]
- `mgr.GetAPIReader()` returns the full object (all three keys). [run]
- Prefer: for pruned types use `MergeFrom` patches, or read through the API reader before an `Update`. Never `Update`
  an object that came from a pruned cache.

## 4. Heavy types: reduce, then metadata, then bypass

Order of preference for a type that is large or cluster-wide (Secrets, ConfigMaps, Pods, Nodes, Events are the usual
suspects; which types are heavy is a judgement, measure it):

1. Selectors and pruning as in section 3. Keeps typed reads and events working.
2. Metadata-only. When you need to react to the object or read name/labels/annotations/ownerRefs but not its data:
   `For/Owns/Watches(..., builder.OnlyMetadata)` or `WatchesMetadata`; reads use `PartialObjectMetadata` /
   `PartialObjectMetadataList` with the GVK set (the GVK is mandatory, the list kind ends in `List`).
3. Bypass the cache: `DisableFor` plus `mgr.GetAPIReader()`. Last resort: every read becomes an API call.

```go
func metadataOnly(mgr ctrl.Manager, r reconcile.Reconciler) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1.ConfigMap{}, builder.OnlyMetadata).
		Complete(r)
}

func metadataGet(ctx context.Context, c client.Reader, key client.ObjectKey) (*metav1.PartialObjectMetadata, error) {
	pom := &metav1.PartialObjectMetadata{}
	pom.SetGroupVersionKind(corev1.SchemeGroupVersion.WithKind("Secret")) // GVK is mandatory
	return pom, c.Get(ctx, key, pom)
}

func metadataList(ctx context.Context, c client.Reader) (*metav1.PartialObjectMetadataList, error) {
	l := &metav1.PartialObjectMetadataList{}
	l.SetGroupVersionKind(corev1.SchemeGroupVersion.WithKind("SecretList"))
	return l, c.List(ctx, l, client.InNamespace("default"))
}

func disableFor() ctrl.Options {
	return ctrl.Options{
		Client: client.Options{Cache: &client.CacheOptions{DisableFor: []client.Object{&corev1.Secret{}}}},
	}
}
```

Facts behind the order:

- `OnlyMetadata` in `For`/`Owns`/`Watches` makes handlers and predicates receive `*metav1.PartialObjectMetadata`. If code
  anywhere also does a typed `Get` of that type, you now hold both informers. [CR]
- Metadata informers use protobuf; typed informers use protobuf for built-in types and JSON for CRDs, so a metadata watch
  is also cheaper on the wire for CRDs. [CR]
- Metadata cannot be written: `Create`/`Update` of `PartialObjectMetadata` fail; `Patch` and `DeleteAllOf` work. [CR]
- `DisableFor` is keyed by GVK only. With `DisableFor: Secret`, a `PartialObjectMetadata` Get of a Secret through the
  manager client also goes live to the API server, not to a metadata cache. Verified: with `ReaderFailOnMissingInformer`
  on, that Get succeeded while a `PartialObjectMetadata` Get of an enabled, unregistered type (ConfigMap) failed with
  `... is not cached`. [CR, run] Events still come from other sources (a For/Owns/Watches or `WatchesRawSource`).
- `DisableFor` does not stop an informer that a For/Owns/Watches already created. [CR]
- Indexes are per representation: an index registered with a typed object is not visible to a `PartialObjectMetadataList`
  query. Register one with a `*PartialObjectMetadata` and an extractor taking that type. [CR by code structure; not run]

### Example: Secrets in the Cluster API style

CAPI caches only labelled Secrets, prunes their data, strips managedFields, keeps `mgr.GetClient()` off the filtered
cache, reads through a small second client, and uses a separate metadata-only cache to watch all Secrets. [CAPI]

```go
// 1. Filtered, pruned Secret cache: ByObject in managerOptions above (Label + Transform: stripAndPrune).
// 2. mgr.GetClient() never serves Secrets from it: disableFor() above.
// 3. A small client that does read the filtered cache (a Get of an unlabelled Secret is NotFound there).
func secretClients(mgr ctrl.Manager) (secretCachingClient client.Client, err error) {
	return client.New(mgr.GetConfig(), client.Options{
		HTTPClient: mgr.GetHTTPClient(),
		Scheme:     mgr.GetScheme(),
		Mapper:     mgr.GetRESTMapper(),
		Cache:      &client.CacheOptions{Reader: mgr.GetCache()},
	})
}

// 4. A separate metadata-only cache to trigger on ANY Secret, added to the manager so it starts and syncs first.
func allSecretsMetadataCache(mgr ctrl.Manager) (cache.Cache, error) {
	c, err := cache.New(mgr.GetConfig(), cache.Options{
		HTTPClient:       mgr.GetHTTPClient(),
		Scheme:           mgr.GetScheme(),
		Mapper:           mgr.GetRESTMapper(),
		DefaultTransform: cache.TransformStripManagedFields(),
	})
	if err != nil {
		return nil, err
	}
	return c, mgr.Add(c)
}

func watchAllSecrets(mgr ctrl.Manager, c cache.Cache, r reconcile.Reconciler) error {
	pom := &metav1.PartialObjectMetadata{}
	pom.SetGroupVersionKind(corev1.SchemeGroupVersion.WithKind("Secret"))
	return ctrl.NewControllerManagedBy(mgr).
		Named("secret-consumer").
		WatchesRawSource(source.Kind(c, pom,
			handler.TypedEnqueueRequestsFromMapFunc(func(_ context.Context, o *metav1.PartialObjectMetadata) []reconcile.Request {
				return []reconcile.Request{{NamespacedName: client.ObjectKeyFromObject(o)}}
			}),
			predicate.TypedFuncs[*metav1.PartialObjectMetadata]{
				UpdateFunc: func(e event.TypedUpdateEvent[*metav1.PartialObjectMetadata]) bool {
					return e.ObjectOld.ResourceVersion != e.ObjectNew.ResourceVersion // drops resync
				},
			},
		)).
		Complete(r)
}
```

CAPI's own docs state the rule of thumb: dealing with stale reads and memory use is better than disabling caching, and
uncached lists should happen at most once per reconcile. [CAPI]

## 5. Indexes

```go
const NodeNameField = "spec.nodeName"

// NodeName is exported so the fake client can reuse it.
func NodeName(o client.Object) []string {
	p, ok := o.(*corev1.Pod)
	if !ok || p.Spec.NodeName == "" {
		return nil
	}
	return []string{p.Spec.NodeName}
}

func setupIndexes(ctx context.Context, mgr ctrl.Manager) error {
	return mgr.GetFieldIndexer().IndexField(ctx, &corev1.Pod{}, NodeNameField, NodeName)
}

func fakeClient() client.Client {
	return fake.NewClientBuilder().WithIndex(&corev1.Pod{}, NodeNameField, NodeName).Build()
}

func podsOnNode(ctx context.Context, c client.Reader, node string) (*corev1.PodList, error) {
	l := &corev1.PodList{}
	return l, c.List(ctx, l, client.MatchingFields{NodeNameField: node})
}
```

- Register every index once, in one central place, before building controllers and before `Start`, so the first `List`
  never meets a missing index. Export the extractors so unit tests can pass them to the fake client. [CR]
- `MatchingFields` needs a registered index for every field named. An unregistered field fails with `Index with name
  field:<name> does not exist`. [CR, run]
- Registering the same type and field twice fails with `indexer conflict`. [CR, run]
- Equality only (`=`); `!=`/`in` are rejected. `Get` never uses an index. `MatchingLabels` is not indexed (linear scan).
  [CR, BLOG]
- Cache has no built-in index for `metadata.name`, `spec.nodeName` etc.; you register them. [CR]
- Each value is stored twice per namespaced object (`ns/value` and an all-namespaces key), and extractors run under the
  store write lock on every add/update/delete: index only what you query, keep extractors cheap, pure and nil-safe. [CR]
- `RemoveInformer` drops the informer with its indexes; a later read recreates a bare informer. Re-register if you call
  it. [CR]
- `List` deep-copies every returned item. A broad list of a big type costs a copy per object; narrow with an index,
  or use `PartialObjectMetadataList`. [CR]

## 6. Predicates and handlers

- Predicates passed to `For()` do not apply to `Watches`/`Owns`. Repeat the relevance logic for each secondary watch (in
  its predicate or map function) or you enqueue and then discard reconciles. [CR, TALK]
- `WithEventFilter` is prepended to every `For`/`Owns`/`Watches` source, but `WatchesRawSource` ignores it. [CR]
- `GenerationChangedPredicate` drops label, annotation, status and resync updates. It does NOT starve deletion on types
  with generation > 0: the API server bumps generation when `deletionTimestamp` is first set. [CR, K8S]
- For types that have no generation (Secret and ConfigMap report generation 0) it drops EVERY update. Use
  `ResourceVersionChangedPredicate` or a field comparison there. [run, CR]
- CAPI prepends a resourceVersion-changed predicate to secondary watches to drop resync events (keeping resync on the
  primary as a periodic re-reconcile). [CAPI]
- Prefer typed predicates and handlers over `any`/`client.Object` assertions. In the builder, predicates are
  `predicate.Predicate` (client.Object), so use a checked assertion; with `source.Kind` the typed form works (see the
  example in section 4).

```go
func checkedSecretUpdate() predicate.Predicate {
	return predicate.Funcs{UpdateFunc: func(e event.UpdateEvent) bool {
		o, ok1 := e.ObjectOld.(*corev1.Secret)
		n, ok2 := e.ObjectNew.(*corev1.Secret)
		return ok1 && ok2 && string(o.Data["tls.crt"]) != string(n.Data["tls.crt"])
	}}
}
```

- `DeepCopy` before mutating the objects in a predicate or handler (shared store objects). [CR, BLOG]
- Do not sleep or poll for the cache; return `ctrl.Result{RequeueAfter: d}`. [CR]

## 7. Client rate limits and read-your-writes

```go
func clientConfig() (*rest.Config, ctrl.Options) {
	cfg := ctrl.GetConfigOrDie() // QPS -1 (client-side limiter off); a hand-built rest.Config with QPS 0 gets 5/10
	cfg.QPS, cfg.Burst = 100, 200
	cfg.UserAgent = "my-operator/v1"
	return cfg, ctrl.Options{
		Client: client.Options{Cache: &client.CacheOptions{EnableReadYourWritesConsistency: new(true)}},
		// Reads wait only until their ctx ends, so bound it.
		Controller: config.Controller{ReconciliationTimeout: time.Minute},
	}
}
```

- `ctrl.GetConfig` sets `QPS = -1` when QPS is 0 (relying on API priority and fairness); Burst is untouched. A
  `rest.Config` you build yourself (for example `rest.InClusterConfig()`) with QPS 0 gets client-go's default 5 QPS / 10
  burst, which throttles a busy controller. Set QPS/Burst and a `UserAgent` deliberately. CAPI defaults to 100/200 by
  flags. [CR, CAPI]
- Read-your-writes is opt-in and experimental: `client.Options.Cache.EnableReadYourWritesConsistency` is a `*bool`
  (the field is on `client.CacheOptions`, not on `client.Options`; release-note shorthand omits `Cache`). Cached reads
  then block until the informer has observed this client's latest write for that GVK and representation. [CR]
- Its limits: documented as not working correctly when a resource has more than one namespace configured; the wait is
  bounded only by the read's context (no internal timeout), so also set `ReconciliationTimeout`; per-call opt-out is
  `client.DisableReadYourWritesConsistency`. A write whose object never reaches the cache (selector excluded) could wait
  until the ctx ends [unverified: inferred from design, not run]. [CR]

## 8. Cache footprint review

Re-run this for every cached type whenever a reconciler's reads change (new Get/List, new watch, new field read):

1. Filtered? Is there a selector or namespace restriction, or does it cache the whole cluster? Is the informer registered
   deliberately (`ReaderFailOnMissingInformer`)?
2. managedFields stripped? `DefaultTransform` set, and any per-type `Transform` composes it?
3. Pruned to what is read? Could a heavy type be metadata-only? Does a new read require a field the transform removes
   (update the transform in the same change)?
4. Is pruning still valid? No `Update` of a pruned object, predicates/map funcs only use retained fields, indexes only
   cover queried fields, and the extractors only use retained fields.

## Hazards

- A transform returning `nil, nil` empties the store while `HasSynced()` stays true: nil is queued as an object, key
  computation fails and the swallowed error drops the whole list. [run, CR]
- `Update` of an object read from a pruned cache deletes every key the transform removed: it is a full-object PUT of the
  pruned copy. [run]
- A per-type `ByObject.Transform` replaces `DefaultTransform`: managedFields are silently kept unless composed. [run, CR]
- `DisableFor` is keyed by GVK: a `PartialObjectMetadata` Get of a disabled type goes live to the API server on every
  call. [run, CR]
- `mgr.GetClient()` before `Start()` returns `ErrCacheNotStarted` but has already created the informer; use
  `mgr.GetAPIReader()` before start. [CR]
- Reading a type that no controller registered silently creates a full cluster-wide informer (and blocks to sync) unless
  `ReaderFailOnMissingInformer` is set. [CR, run]
- `GenerationChangedPredicate` on Secrets/ConfigMaps (generation 0) drops every update. [run, CR]
- Mutating objects from `UnsafeDisableDeepCopy` reads, or from predicates/handlers, corrupts the shared store seen by
  every controller. [CR]
- `IndexField` twice for the same type and field errors (`indexer conflict`); unregistered `MatchingFields` errors at
  read time. [run]
- Read-your-writes with more than one namespace configured is documented as incorrect, and its waits have no timeout
  but the ctx. [CR]
