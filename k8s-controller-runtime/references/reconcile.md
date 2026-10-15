# Reconcile loop, writes, finalizers, controller options

Read this when you write or review a `Reconcile` function, the writes it makes (status, finalizers, owned objects), the
builder wiring, or controller options. Tags are explained in `sources.md`; `[run]` = reproduced in a scratch envtest project.

## Model
- Level-triggered: look at the state now, act on the difference. Never branch on event type; events can be missed or merged. [CR FAQ, API]
- Reconcile the top-level (parent) object. Map children to it with `Owns` / `EnqueueRequestsFromMapFunc`; reconciling a leaf
  object gave last-writer-wins config loss and double work. [CR FAQ, TALK]
- A `Request` is only a key. Re-read the object, treat `NotFound` as success (`client.IgnoreNotFound`). [CR, CAPI]
- Rebuild the whole desired state on every call from cached reads; keep no important state in memory (write it to an object). [TALK, CAPI]
- Structure the body as flat, independent, idempotent phases (each ensures its own resource, none nested inside "if A was just created"),
  run them all, aggregate the errors. A crash between nested steps is never repaired. [TALK, CAPI]

## Result semantics [run, CR]
| Return | Effect |
|---|---|
| `Result{}, nil` | Success; failure count forgotten; no requeue until the next event |
| `_, err` | `Result` ignored; requeue with per-item exponential backoff (5ms doubling to 1000s); logged as "Reconciler error" |
| `_, reconcile.TerminalError(err)` | No requeue; still logged and counted (`errors.Is(err, reconcile.TerminalError(nil))`) |
| `Result{RequeueAfter: d}, nil` | Requeue after `d`; failure count is forgotten (next failure restarts at 5ms) |
| `Result{Requeue: true}, nil` | Deprecated; rate-limited requeue, failure count is NOT forgotten (gaps kept doubling: 321ms, 641ms, 1.28s) |
| `Result{RequeueAfter: d}, err` | `RequeueAfter` ignored and a "Warning: Reconciler returned both ..." line is logged |
- `Result.Priority *int` is honoured by the priority queue (default) and kept when the item is re-enqueued, also on error. [CR]
- Return an error only when a retry can help. For "waiting on the user" (bad reference, invalid input) set a condition and return nil. [CR, API]
- Do not also log an error you return; the controller logs it. [CR TMP-LOGGING]

## Prefer / Avoid
- Prefer watches over `RequeueAfter` polling; use `RequeueAfter` only for what no event announces (external system, expiry). [CAPI, TALK]
- Avoid `time.Sleep` in `Reconcile`: it holds a worker, and the default is one worker. Return `RequeueAfter`. [CR]
- Prefer passing `ctx` to every blocking call; set `ReconciliationTimeout` so a stuck call cannot hold a worker forever. [CR]
- Prefer typed reads (reads of `unstructured` objects bypass the cache by default); read each object once at the top and pass it down. [CR, CAPI, TALK]
- Avoid comparing the whole desired object with the live one (defaulting, webhooks and other actors change it). Compare the fields you
  own, or store a hash of what you applied in an annotation. [TALK]

## Skeleton (compiled, behaviour run in envtest; imports omitted)
```go
const widgetFinalizer = "demo.example.com/cleanup"

type WidgetReconciler struct {
	client.Client
}

// +kubebuilder:rbac:groups=demo.example.com,resources=widgets,verbs=get;list;watch;patch
// +kubebuilder:rbac:groups=demo.example.com,resources=widgets/status,verbs=get;patch
// +kubebuilder:rbac:groups=demo.example.com,resources=widgets/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch;create;patch

func (r *WidgetReconciler) Reconcile(ctx context.Context, req ctrl.Request) (_ ctrl.Result, reterr error) {
	log := ctrl.LoggerFrom(ctx)

	w := &demov1.Widget{}
	if err := r.Get(ctx, req.NamespacedName, w); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err) // Gone is a success
	}

	// Finalizer: its own earlier patch. AddFinalizer/RemoveFinalizer only mutate in memory.
	if !w.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(w, widgetFinalizer) {
			// ... idempotent external cleanup goes here; NotFound counts as done ...
			orig := w.DeepCopy()
			controllerutil.RemoveFinalizer(w, widgetFinalizer)
			return ctrl.Result{}, client.IgnoreNotFound(r.Patch(ctx, w, client.MergeFrom(orig)))
		}
		return ctrl.Result{}, nil
	}
	if !controllerutil.ContainsFinalizer(w, widgetFinalizer) {
		orig := w.DeepCopy()
		controllerutil.AddFinalizer(w, widgetFinalizer)
		if err := r.Patch(ctx, w, client.MergeFrom(orig)); err != nil {
			return ctrl.Result{}, err
		}
	}

	// One deferred status write per reconcile, skipped when nothing changed.
	// Snapshot AFTER the finalizer patch: Patch decodes the server response into w.
	orig := w.DeepCopy()
	defer func() {
		if equality.Semantic.DeepEqual(orig.Status, w.Status) {
			return
		}
		if err := r.Status().Patch(ctx, w, client.MergeFrom(orig)); err != nil {
			reterr = utilerrors.NewAggregate([]error{reterr, err})
		}
	}()

	// Flat, independent, idempotent phases; every phase runs, errors are aggregated.
	var errs []error
	for _, phase := range []func(context.Context, *demov1.Widget) error{r.ensureConfigMap} {
		if err := phase(ctx, w); err != nil {
			errs = append(errs, err)
		}
	}
	if err := utilerrors.NewAggregate(errs); err != nil {
		meta.SetStatusCondition(&w.Status.Conditions, metav1.Condition{
			Type: "Ready", Status: metav1.ConditionFalse, Reason: "ReconcileError",
			Message: "See controller logs", ObservedGeneration: w.Generation,
		})
		return ctrl.Result{}, err
	}
	meta.SetStatusCondition(&w.Status.Conditions, metav1.Condition{
		Type: "Ready", Status: metav1.ConditionTrue, Reason: "Reconciled", ObservedGeneration: w.Generation,
	})
	w.Status.ObservedGeneration = w.Generation
	log.V(1).Info("Reconciled Widget", "generation", w.Generation)
	return ctrl.Result{}, nil
}

// ensureConfigMap uses a deterministic name, so a stale cache yields AlreadyExists, not a duplicate.
func (r *WidgetReconciler) ensureConfigMap(ctx context.Context, w *demov1.Widget) error {
	cm := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: w.Name + "-cfg", Namespace: w.Namespace}}
	err := r.Get(ctx, client.ObjectKeyFromObject(cm), cm)
	if apierrors.IsNotFound(err) {
		cm.Data = map[string]string{"size": string(w.Spec.Size)}
		if err := controllerutil.SetControllerReference(w, cm, r.Scheme()); err != nil {
			return err
		}
		return client.IgnoreAlreadyExists(r.Create(ctx, cm))
	}
	if err != nil {
		return err
	}
	if cm.Data["size"] == string(w.Spec.Size) {
		return nil
	}
	orig := cm.DeepCopy()
	cm.Data["size"] = string(w.Spec.Size)
	return r.Patch(ctx, cm, client.MergeFrom(orig))
}

func (r *WidgetReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&demov1.Widget{}).
		Owns(&corev1.ConfigMap{}).
		Named("widget").
		Complete(r)
}
```
The finalizer patch does not bump `metadata.generation` (generation stayed 1 [run]), so a `For` predicate such as
`GenerationChangedPredicate` will not re-trigger after it: continue in the same reconcile as above instead of returning.

## Write patterns
- Prefer `Patch` with `client.MergeFrom(orig)`: it sends only your diff. `Update` sends the whole object, fails on conflict and can drop
  fields a newer API version added. Lists (finalizers, ownerReferences, conditions) are replaced wholesale in a merge patch. [CR]
- Skip the call when nothing changed. An empty merge patch still sends `PATCH {}`; the API server treats it as a no-op (resourceVersion
  unchanged) but you pay the round trip. [run]
  ```go
  patch := client.MergeFrom(orig)
  if data, err := patch.Data(w); err == nil && string(data) == "{}" { return nil } // nothing to send
  ```
- Optimistic lock when the new value depends on what you read: `client.MergeFromWithOptions(orig, client.MergeFromWithOptimisticLock{})`
  puts `resourceVersion` in the patch; a stale copy gets a Conflict (`apierrors.IsConflict`), a plain stale patch succeeds [run]. On conflict, re-read and
  retry (return the error: the queue backs off).
- One deferred write per reconcile, as a pattern (see the skeleton): snapshot, mutate in memory, write once in a `defer`, aggregate the
  reconcile and write errors. Fewer writes means fewer watch events and fewer re-reconciles. CAPI patches in a `defer` on success and error. [CAPI, TALK]
- Status goes through the status subresource (`r.Status().Patch`); a write to the main resource ignores `status` when the subresource is
  enabled. Keep spec/metadata and status as separate calls. [API, K8S]
- Server-side apply when several actors own different fields (your controller, an autoscaler, GitOps). Generated applyconfigurations
  are required (codegen steps and quirks: `codegen-workflow.md`). Two actors kept their own fields and `managedFields` listed both [run]:
  ```go
  ac := acv1.Widget(w.Name, w.Namespace).WithSpec(acv1.WidgetSpec().WithReplicas(4))
  err := c.Apply(ctx, ac, client.FieldOwner("autoscaler"), client.ForceOwnership)
  err = c.Status().Apply(ctx, acv1.Widget(w.Name, w.Namespace).
  	WithStatus(acv1.WidgetStatus().WithObservedGeneration(w.Generation)), client.FieldOwner("widget-controller"), client.ForceOwnership)
  ```
  `FieldOwner` is required; the applied object must contain every field you care about on every apply; never build the
  config from an `Unstructured` converted from a typed object (zero values become indistinguishable). Re-applying an unchanged config leaves `resourceVersion` unchanged [run]. [CR, K8S]
- Avoid `controllerutil.CreateOrUpdate` for objects with status: status changes in the mutate function are discarded, and a mutate
  function that resets a defaulted field updates on every call. [CR]

## Finalizers
- Add one only when deleting the object needs cleanup the garbage collector cannot do (external systems). A finalizer keeps the object in
  `Terminating` until your controller removes it, so an outage of the controller blocks deletion. Children in the cluster need an ownerReference, not a finalizer. [K8S]
- Name it domain-qualified (`example.com/cleanup`); the API server rejects others. [K8S]
- Add it in its own earlier patch, before creating the external state, so a fast delete cannot be missed. Once `deletionTimestamp` is set
  you can remove finalizers but not add. [K8S, TALK, CAPI]
- `controllerutil.AddFinalizer/RemoveFinalizer/ContainsFinalizer` only mutate the in-memory object; you must `Patch`. [CR]
- Remove it with a patch after an idempotent cleanup (NotFound of the external thing counts as done). [CR, K8S]
- RBAC: your own `widgets/finalizers` `update` is needed when your owned objects carry `blockOwnerDeletion`, which `SetControllerReference`
  sets to true. [CR, CAPI] Not exercised in envtest (it runs as admin): [unverified] whether the cluster enforces it for your setup.
  Details of RBAC: `observability-rbac.md`.

## ownerReferences
- `controllerutil.SetControllerReference(owner, child, scheme)` before create, plus `Owns(&Child{})` in the builder. It errors for a
  cross-namespace owner, for a namespaced owner of a cluster-scoped child, and with `*AlreadyOwnedError` if another controller owns the child. [CR]
- `Owns` reacts only to the controller owner by default (`builder.MatchEveryOwner` for all). Changing the owned object triggered a reconcile
  that repaired it [run]. [CR]
- Give children deterministic names. A stale cache then yields `AlreadyExists`, not a duplicate. [TALK, CR FAQ]
- Garbage collection does not exist in envtest: the dependent ConfigMap survived deletion of the owner [run]. Assert the ownerReference field.

## Stale cache, read-after-write
Reads come from the informer cache and can lag your own writes. This is by design. [CR FAQ, TALK]
1. Make the reconcile idempotent first (deterministic names, tolerate `AlreadyExists`, converge on the next event). Usually enough. [CR FAQ, TALK]
2. For a rare read that must be live, use `mgr.GetAPIReader()` with a comment saying why. Avoid uncached lists in the hot path;
   CAPI: "better to deal with stale reads than disabling caching". [CAPI]
3. Advanced, only with evidence: CAPI records the `resourceVersion` it wrote and requeues (100ms) until its informer store has seen it
   (`DeferNextReconcileUntilCacheUpToDate`, `util/controller/consistency.go`). It is a wrapper around the controller, not a controller-runtime feature; the ReplicaSet "expectations" idea is similar. [CAPI, TALK]
4. Experimental read-your-writes client: `ctrl.Options{Client: client.Options{Cache: &client.CacheOptions{EnableReadYourWritesConsistency: new(true)}}}`
   (compiled). Reads block until the cache has seen your write, bounded only by the read's `ctx` (pair with `ReconciliationTimeout`); wrong with more than one
   Namespace configured; "details may change". [CR]

## Controller options (`controller.Options`, per controller; manager default in `config.Controller`) [CR]
| Field | Default / note |
|---|---|
| `MaxConcurrentReconciles` | 1. The same key is never processed by two workers. Raise only if reconcile is idempotent; more workers moves load to the API server [TALK] |
| `UsePriorityQueue` | on. Default limiter is per-item exponential 5ms to 1000s with no global bucket; with it off, `DefaultTypedControllerRateLimiter` (adds a token bucket) |
| `CacheSyncTimeout` | 2m; bounds source start and sync. A missing CRD for a watched kind fails startup after it |
| `ReconciliationTimeout` | none; only cancels `ctx`, a reconcile that ignores `ctx` is not interrupted |
| `RecoverPanic` | true: a panic becomes an error with backoff and `controller_runtime_reconcile_panics_total` |
| `SkipNameValidation` | false: controller names are globally unique; set true in tests that build several managers |
| `NeedLeaderElection` | true; false runs on every replica |
| `EnableWarmup` | false (beta): starts sources on non-leaders to shorten failover; no-op with `NeedLeaderElection: false` |

## Builder gotchas [CR]
- `For` can be called once (second call errors "For(...) should only be called once"). One reconciled kind per controller.
- Predicates passed to `For` apply only to `For`; repeat the relevance logic on every `Owns` / `Watches` or you enqueue and discard
  reconciles. [CR, TALK] `WithEventFilter` is prepended to `For/Owns/Watches` but `WatchesRawSource` ignores it, so a global predicate must handle every watched type.
  Cache-related predicate advice (resync, `GenerationChangedPredicate`): `cache-and-memory.md`.
- `Named()` must be unique per process; default is the lowercased kind.

## Logging
See `logging.md` (logger from `ctx`, message and key conventions, levels, errors).

## Events
- v0.23.0 added `GetEventRecorder` (v0.25.0 only widened its return type); `rec := mgr.GetEventRecorder("widget-controller")` then
  `rec.Eventf(w, nil, corev1.EventTypeNormal, "Reconciled", "Reconcile", "Reconciled Widget %s", w.Name)` (`regarding, related, type, reason, action, note, args`). Produced an `events.k8s.io/v1` Event
  with `reportingController` set [run]. `GetEventRecorderFor` is deprecated. `len(name)+1+len(hostname)` must be <= 128. [CR]
- RBAC is on `events.k8s.io` (`create;patch`, the verbs the client-go sink uses [CR]); see `observability-rbac.md`.
- Keep events short, CamelCase reason constants, one per meaningful transition. Logs are for developers, events and conditions for users. [CAPI]

## Hazards
- A `Patch`/`Update` of the main resource decodes the server response into your object and overwrites in-memory `status` changes (observedGeneration 5 became 0) [run]. Write status last or from a snapshot taken after the other writes.
- `Result{RequeueAfter: d}` returned with an error is ignored (and a warning logged): the retry follows the backoff, not `d`. [run]
- `Requeue: true` never resets the backoff, so a flapping object keeps slowing down. [run]
- `AddFinalizer`/`RemoveFinalizer` change memory only; forgetting the `Patch` leaves the object stuck or unprotected. [CR]
- A finalizer without a controller (uninstalled, crash-looping) leaves objects `Terminating` forever. [K8S]
- Returning right after adding a finalizer and waiting for the update event never re-reconciles under `GenerationChangedPredicate`. [run, CR]
- Predicates on `For` do not cover `Owns`/`Watches`; `WatchesRawSource` skips `WithEventFilter`. [CR]
- Logs are dropped unless `ctrl.SetLogger(...)` runs at startup; the notice only prints on a log call made after 30 s, so earlier lines are lost silently (`logging.md`). [CR, run]
- `ReconciliationTimeout` does not stop code that ignores `ctx`. [CR]
