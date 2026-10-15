# Review checklist

Read this when reviewing an existing controller or checking your own before calling it done. Each item points to the reference that has the reason and
the evidence. An item that fails is advice not followed: ask whether a deviation note explains it. Items marked **hazard** are factual bugs.

## API and generated output (`api-design.md`, `codegen-workflow.md`)
- [ ] Spec and status are separate; the status subresource marker is present.
- [ ] No unjustified boolean (think twice); states are enums.
- [ ] Optional fields are non-pointer with `+optional` and `omitzero`. Every pointer has a deviation note (numerics with `Minimum=0`, optional structs).
- [ ] No field relies on "no omit tag and no marker" (that makes it required). **hazard**
- [ ] Defaults come from markers and are justified; no webhook default on a marker-defaulted field. **hazard**
- [ ] Every list has `listType` (and `listMapKey` for maps); strings and lists have `maxLength` and `maxItems`; CEL rules are bounded.
- [ ] Deviation notes sit in their own comment group above the doc comment (nothing leaks into the CRD description). **hazard**
- [ ] The generated CRD was diffed (a misspelled marker is silently ignored). **hazard**
- [ ] kube-api-linter runs clean from the module root with the minimum set.
- [ ] `controller-gen` is a `go.mod` `tool` directive; generated files are regenerated, not hand-edited.

## Status and conditions (`status-conditions.md`)
- [ ] `metav1.Condition` with a `Ready` summary; reasons are non-empty PascalCase constants without spaces. **hazard**
- [ ] `observedGeneration` is set on every condition and on the status, and only after a successful reconcile.
- [ ] Each controller writes only its own conditions; status goes through the status subresource with an optimistic lock, server-side apply, or a single writer. **hazard**

## Reconcile (`reconcile.md`)
- [ ] Level-triggered and idempotent: a second reconcile of unchanged state makes zero writes.
- [ ] NotFound returns without an error; no `time.Sleep`; no `Result.Requeue`; waits use `RequeueAfter`; failures return the error; unrecoverable ones use `TerminalError`. **hazard**
- [ ] One deferred write per reconcile, skipped when nothing changed; the main resource is written before status is set. **hazard**
- [ ] A finalizer exists only for external cleanup; it is added and removed with a patch; the reconcile continues after adding it. **hazard**
- [ ] Owned objects have a controller reference and an `Owns` watch; the controller reconciles the top-level parent.
- [ ] There is no "read the cache, fall back to the API" logic; stale reads are handled by idempotency.

## Cache footprint (`cache-and-memory.md`), re-run whenever reads change
For each cached type:
- [ ] Filtered by selector or namespace? (Never by returning `nil, nil` from a transform. **hazard**)
- [ ] `managedFields` stripped, including inside any per-type transform? **hazard**
- [ ] Pruned to the fields actually read, and still valid for what the code reads now?
- [ ] Every write from a pruned type is a `Patch` (never an `Update`)? **hazard**
- [ ] Heavy types follow selectors, then metadata-only, then `DisableFor` plus `APIReader`; no metadata read of a `DisableFor` type. **hazard**
- [ ] Indexes are registered once, centrally, before the controllers; `MatchingFields` queries have a registered index. **hazard**
- [ ] Predicates on `For()` are repeated on `Owns`/`Watches`; no `GenerationChangedPredicate` on kinds with generation 0. **hazard**
- [ ] Nothing mutates objects inside predicates or handlers. **hazard**

## Manager, operations and RBAC (`manager-setup.md`, `observability-rbac.md`, `metrics.md`, `logging.md`)
- [ ] `LeaderElectionID` is set; lease RBAC is present (and core `events` create/patch); `Reconcile` respects `ctx`; the shutdown timeout is honoured. **hazard**
- [ ] Probes are wired; the signal handler is installed once.
- [ ] Metrics are authenticated and TLS (`SecureServing` with `WithAuthenticationAndAuthorization`) and the RBAC for it exists. **hazard**
- [ ] Custom metrics: plain `client_golang` on `metrics.Registry`, registered in `init()`, bounded labels, a delete path for per-object series, no duplicate of built-in metrics. **hazard**
- [ ] pprof and extra handlers sit behind the same authenticated filter. **hazard**
- [ ] `SetLogger` is called; application code logs through `ctrl.LoggerFrom(ctx)`; no whole objects or Secret data in logs; no log-and-return of one error. **hazard**
- [ ] RBAC comes from markers with a blank line after the group; no wildcards; `/status` and `/finalizers` subresources are granted; Secret `list`/`watch` is scoped.
- [ ] The event recorder matches its RBAC (`GetEventRecorder` writes `events.k8s.io`).

## Webhooks (`webhooks.md`)
- [ ] A webhook exists only where CEL and markers cannot do the job; it uses the generic builder; the marker `path` matches the registered path; `failurePolicy` was chosen on purpose. **hazard**

## Tests (`testing-envtest.md`)
- [ ] envtest starts with `DownloadBinaryAssets: true` and a fixed `BinaryAssetsDirectory` (no `setup-envtest`, no `KUBEBUILDER_ASSETS`). **hazard**
- [ ] A unique namespace per spec; no spec waits for namespace deletion, garbage collection, Pods or Deployment controllers; ownership is asserted on `ownerReference` and finalizer fields. **hazard**
- [ ] `AfterSuite` waits for `mgr.Start` to return before stopping envtest and fails on `*_panics_total` above 0. **hazard**
- [ ] Reads right after writes in tests tolerate a stale cache (poll, or read-your-writes).
- [ ] The fake client appears only in narrow unit tests, with `WithStatusSubresource` where status is written. **hazard**
