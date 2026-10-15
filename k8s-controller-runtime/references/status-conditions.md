# Status and conditions

Read this when you add status fields or conditions to a CRD, write the status update in a reconciler, or review how a controller reports state.
Type and marker rules for the status struct are in `api-design.md`; write patterns in general are in `reconcile.md`.

Tags: `run` = reproduced against an envtest apiserver (controller-runtime v0.25.2, apimachinery v0.37.1). Others are listed in `sources.md`.

## Reference pattern (compiled; behavior run in envtest)

```go
const (
	ConditionReady       = "Ready"       // summary of every other condition
	ConditionConfigValid = "ConfigValid" // owned by this controller
)

const (
	ReasonReconciled     = "Reconciled"
	ReasonPending        = "Pending"
	ReasonInvalidConfig  = "InvalidConfig"
	ReasonConfigAccepted = "ConfigAccepted"
	ReasonNotReady       = "NotReady"
)

func setCondition(g *gv1.Gadget, t string, s metav1.ConditionStatus, reason, msg string) {
	meta.SetStatusCondition(&g.Status.Conditions, metav1.Condition{
		Type: t, Status: s, Reason: reason, Message: msg,
		ObservedGeneration: g.Generation, // the generation this verdict was computed from
	})
}

// setReady recomputes the Ready summary from every other condition.
func setReady(g *gv1.Gadget) {
	for _, c := range g.Status.Conditions {
		if c.Type != ConditionReady && c.Status != metav1.ConditionTrue {
			setCondition(g, ConditionReady, c.Status, ReasonNotReady, c.Type+" is not True")
			return
		}
	}
	setCondition(g, ConditionReady, metav1.ConditionTrue, ReasonReconciled, "All conditions are True")
}

func (r *GadgetReconciler) Reconcile(ctx context.Context, req ctrl.Request) (_ ctrl.Result, reterr error) {
	g := &gv1.Gadget{}
	if err := r.Get(ctx, req.NamespacedName, g); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	orig := g.DeepCopy()

	// One deferred status write per reconcile; skipped when nothing changed.
	defer func() {
		if reterr == nil {
			g.Status.ObservedGeneration = g.Generation // success only
		}
		setReady(g)
		if equality.Semantic.DeepEqual(orig.Status, g.Status) {
			return
		}
		patch := client.MergeFromWithOptions(orig, client.MergeFromWithOptimisticLock{})
		if err := r.Status().Patch(ctx, g, patch); err != nil {
			reterr = errors.Join(reterr, err)
		}
	}()

	// Set every owned condition on the first visit, even as Unknown.
	if meta.FindStatusCondition(g.Status.Conditions, ConditionConfigValid) == nil {
		setCondition(g, ConditionConfigValid, metav1.ConditionUnknown, ReasonPending, "Validating configuration")
	}
	if g.Spec.StorageClass == "" {
		setCondition(g, ConditionConfigValid, metav1.ConditionFalse, ReasonInvalidConfig, "spec.storageClass is required")
		return ctrl.Result{}, nil // user must edit spec; retrying cannot help
	}
	setCondition(g, ConditionConfigValid, metav1.ConditionTrue, ReasonConfigAccepted, "Configuration is valid")
	return ctrl.Result{}, nil
}
```

Run result: first reconcile wrote `ConfigValid=True`, `Ready=True`, `status.observedGeneration=1` (each condition `observedGeneration=1`); a second reconcile left `resourceVersion` unchanged (no write). With `storageClass` unset it wrote `ConfigValid=False`, `Ready=False` and returned nil. `[run]`

## Prefer

**Shape**
- Use `[]metav1.Condition` as the first status field with `+listType=map`, `+listMapKey=type`, `+optional` and `MaxItems=32`. For CRDs omit `patchStrategy`/`protobuf` tags (KAL `conditions` with `usePatchStrategy`/`useProtobuf: Forbid`). `[API, KAL, CAPI, CAPV]`
- Spell the type name as an adjective or past-tense verb describing state (`Ready`, not `Deploying`); use `Unknown` for in-between. `[API]`
- Prefer positive polarity (True = normal) by default: `Ready`, `Accepted`, `Programmed`. Upstream gives no hard rule, but warns that `Failed=False` causes double-negative confusion. Pick one convention per API and document it. `[API, GW]`
- Provide a `Ready` summary condition, recomputed in a last pass on every reconcile (also on early-error paths) so it is never stale. `[GW, CAPI]`
- Avoid a boolean `ready` or a `phase` in status; conditions replace both. `[API, KAL]`

**Reasons and messages**
- Reasons are exported PascalCase constants in the API package, grouped per condition. Set a reason and message on happy states too (`Reconciled`, `AsExpected`). `[API, CAPV, OCP]`
- Start messages with a capital letter. Keep them stable: never put counters, timestamps or random object names in a message that is written every reconcile. `[OCP, CAPI]`

**Generations**
- Set `observedGeneration` on each condition to `metadata.generation` at the time that condition was computed; a lagging value means the condition is stale. `meta.SetStatusCondition` copies the value you pass and never stamps it. `[API, K8S, run]`
- Set `status.observedGeneration` only when the reconcile finished without returning an error (CAPI patch helper: `if reterr == nil`). Handling a user-fixable error by setting a condition and returning nil counts as finished, as the run above shows. `[CAPI, run]`
- Set the conditions you own on the first visit even as `Unknown`; a missing condition is read as `Unknown`. `[API]`

**Writing**
- Write status through the status subresource with `Status().Patch` and `client.MergeFrom` of a pre-mutation `DeepCopy`, once per reconcile (deferred), and skip the call when `equality.Semantic.DeepEqual` says nothing changed. `[CAPI, CR, run]`
- Prefer `MergeFromWithOptimisticLock` when another actor may write other conditions on the same object: a stale write then fails with 409 and is retried by the queue. `[CAPI, run]`
- Each controller writes only the condition types it owns and does not remove or reorder others; if a condition you do not own carries a newer `observedGeneration` than your copy, your copy is stale, so do not touch it. `[GW, CAPI]`
- When several controllers or tools own different conditions, prefer server-side apply with a distinct `FieldOwner` per controller (needs `listType=map`): both `E` and `F` survived and managedFields showed one `Apply` manager each on `status`. `[K8S, run]`

```go
o := &unstructured.Unstructured{Object: map[string]any{
	"apiVersion": "gadget.example.com/v1", "kind": "Gadget",
	"metadata": map[string]any{"name": g.Name, "namespace": g.Namespace},
	"status": map[string]any{"conditions": []any{map[string]any{
		"type": "E", "status": "True", "reason": "Ok", "message": "",
		"lastTransitionTime": metav1.Now().UTC().Format("2006-01-02T15:04:05Z"),
	}}},
}}
err := c.Status().Apply(ctx, client.ApplyConfigurationFromUnstructured(o),
	client.FieldOwner("ctrl-1"), client.ForceOwnership)
```

With generated apply configurations the same call takes the typed builder (`codegen-workflow.md`). Apply needs `patch` RBAC (on the `/status` subresource for status writes). `[K8S, CT]`

**Handling errors**
- Transient error: return it (backoff retry). User-fixable spec error: set the condition `False` with a reason that names the fix and return nil, because retry cannot help until the spec changes. `[KB, API]`

## Documented alternatives (pick per API, do not mix inside one API)

- OpenShift operator style: `Available`, `Progressing`, `Degraded` side by side (negative polarity for `Degraded`). `Available=False` only when an administrator must act now, never during a normal upgrade. `Degraded` only after persistent failure, with no flapping, never during a normal upgrade; it can be true while `Available` is true. `[OCP]`
- Gateway API style: positive summary conditions that must always be present (`Accepted`, `ResolvedRefs`, `Programmed`); `ResolvedRefs=False` does not block `Accepted=True`; use `Unknown` when the controller cannot decide yet, never an empty status. `[GW]`
- CAPI style: some conditions have negative polarity (`Deleting`, `Updating`) and the summary takes the polarity list as an option; `Ready` is a summary over a default set plus user-declared gates. `[CAPI]`

## Hazards

- A plain `MergeFrom` status patch replaces the whole `conditions` array, because JSON merge patch has no list merge. A stale copy that adds `B` erased `A` (result `[Ready B]`). Use optimistic lock, SSA, or write only from one controller. `[run]`
- An empty `reason` (`""`) or one with a space fails the whole status write with 422 (`should be at least 1 chars long`, `should match '^[A-Za-z]([A-Za-z0-9_,:]*[A-Za-z0-9_])?$'`); the CRD schema carries both rules. Always set a constant reason. `[API, CT, run]`
- `meta.SetStatusCondition` leaves `observedGeneration` at whatever you pass (0 if you forget), so the condition looks stale forever. It keeps `lastTransitionTime` unless the status flips. `[K8S, run]`
- An empty `MergeFrom` patch is still sent (`PATCH {}`); the apiserver ignores it, but it costs a round trip. Compare before writing. `[CR, run]`
- Writing status through `Update`/`Patch` on the main resource is ignored once the status subresource is on. `[K8S]`
- A status field or message that changes every reconcile produces a watch event that re-triggers reconcile (hot loop); CAPI strips random object names from errors for this reason. `[CAPI]`
