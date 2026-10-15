---
name: k8s-controller-runtime
description: Build, extend and review Kubernetes controllers and operators in Go with pure controller-runtime and controller-gen (no kubebuilder scaffolding): CRD types and markers, reconcilers, status conditions, finalizers, cache and memory tuning, webhooks, authenticated metrics, logging, RBAC, envtest. Use whenever a task mentions controller-runtime, a Kubernetes operator or reconciler, a CRD, controller-gen, envtest, finalizers, informer or cache tuning, or kube-api-linter, even if no skill is named. Not for OLM bundles, Helm charts, multi-cluster controllers, conversion webhooks or multi-version CRDs (one-line pointers only).
---

# Kubernetes controllers with controller-runtime

Pure controller-runtime + controller-gen. No kubebuilder CLI, PROJECT file, `+kubebuilder:scaffold` markers, kustomize overlays or
Makefile scaffolding. The `+kubebuilder:` marker prefix is still correct: controller-gen reads it.
Target: controller-runtime v0.25.2, controller-tools v0.23.0, Go 1.26+, k8s.io/* v0.37 (details and API changes in `references/versions.md`).

## How to use this skill

- Almost everything here is **advice** ("prefer", "avoid"). Following it is the default; breaking it needs a reason written next to the code
  (see "Deviation notes"). The **Hazards** below are different: they are factual bugs, each with its reason, and not matters of taste.
- Rules carry source tags like `[CR]`, `[API, run]`. `run` means the behaviour was reproduced in `research/experiments`; `[unverified]` means it was not
  confirmed. `references/sources.md` maps every tag to its source and version.
- Load only the reference files the task needs (table below). Each one opens with a "Read this when" line; all but `versions.md`, `sources.md`
  and `review-checklist.md` end with their own Hazards section.

## Workflow for a new controller

1. **Design the API.** Spec/status split, enums over booleans, markers, conditions: `api-design.md`, `status-conditions.md`.
   Decide whether you need a webhook at all (CEL and markers first): `webhooks.md`.
2. **Lay out the project and the tooling.** Hand-rolled layout, `cmd/main.go`, hand-written manifests: `manager-setup.md`.
   `controller-gen` as a `go.mod` `tool` directive, kube-api-linter in `tools/go.mod`: `codegen-workflow.md`.
3. **Generate and read the output.** Run the generators, diff the generated CRD (a misspelled marker is silently ignored), run the API linter from the module root.
4. **Write the reconciler.** Idempotent, level-triggered, one deferred write, correct requeue semantics: `reconcile.md`.
5. **Review the cache footprint.** Run the four questions in `cache-and-memory.md` section 8 for every cached type. **Re-run them whenever the
   reconciler starts reading a new type, field, label or Secret key**, because pruning and selectors silently hide whatever you did not keep.
6. **Make it operable.** Authenticated metrics: `metrics.md`. One logging setup: `logging.md`. Events, probes, RBAC, leader election: `observability-rbac.md`.
7. **Test it.** envtest with downloaded binaries, Ginkgo and Gomega: `testing-envtest.md`.
8. **Review it** against `review-checklist.md` before calling it done. To review an existing controller, start there.

## Which reference when

| Task | Read |
|---|---|
| Define or change CRD types, markers, defaults, validation, CEL | `api-design.md` |
| Conditions, `observedGeneration`, status writes | `status-conditions.md` |
| Reconcile logic, requeue, finalizers, ownerReferences, patch vs apply, controller options | `reconcile.md` |
| Memory, informers, selectors, transforms, indexes, predicates, `PartialObjectMetadata` | `cache-and-memory.md` |
| `main.go`, manager options, leader election, project layout, Dockerfile | `manager-setup.md` |
| controller-gen commands, markers, applyconfiguration, kube-api-linter config | `codegen-workflow.md` |
| Defaulting and validating webhooks | `webhooks.md` |
| Tests, envtest, fake client | `testing-envtest.md` |
| Events, probes, RBAC markers, pprof, pod security | `observability-rbac.md` |
| Custom metrics, scrape authentication | `metrics.md` |
| Logging backend, levels, conventions | `logging.md` |
| Which version introduced or changed an API | `versions.md` |
| Reviewing an existing controller | `review-checklist.md` |
| What a tag such as `[CAPI]` points to | `sources.md` |

## Defaults in one screen (all advice; details and reasons are in the references)

- **API.** Upstream Kubernetes API conventions. Think twice before a boolean; prefer an enum. Avoid pointers: optional fields are non-pointer with `+optional`
  and `omitzero`; keep a pointer only where unset and zero mean different things, and for numerics with `Minimum=0` and optional structs, each with a
  deviation note. Defaults come from markers, each justified. Every list gets a `listType`. Run kube-api-linter with the minimum set in `codegen-workflow.md`.
- **Status.** `metav1.Condition`, a `Ready` summary, PascalCase reason constants, `observedGeneration` on every condition and on the status, written through the
  status subresource. A controller writes only the conditions it owns.
- **Writes.** `Patch` with `MergeFrom` by default, skipped when nothing changed. Server-side apply with generated applyconfigurations when several actors own
  different fields. One deferred write per reconcile is a pattern, not a library: do not carry a helper you do not need.
- **Requeue.** Never `Result.Requeue` (deprecated). `RequeueAfter` for an expected wait, a returned error for a failure that should back off,
  `reconcile.TerminalError` for one retrying cannot fix. Prefer a watch to polling.
- **Cache.** Strip `managedFields` by default (`cache.TransformStripManagedFields()` as `DefaultTransform`), prune to the fields you read, filter with
  `ByObject` selectors and namespaces, then metadata-only reads, and `DisableFor` plus `APIReader` only as a last resort. Register indexes once, centrally.
- **Metrics (mandatory).** The metrics endpoint MUST be authenticated and TLS: `metricsserver.Options{SecureServing: true, FilterProvider:
  filters.WithAuthenticationAndAuthorization}`, with the RBAC in `metrics.md`. Custom metrics use plain `client_golang` on `metrics.Registry`.
- **Logging.** Application code logs through `ctrl.LoggerFrom(ctx)` (logr). Default backend is klog (`klog.InitFlags(nil)`, `flag.Parse()`,
  `ctrl.SetLogger(klog.Background())`); `logging.md` says when to deviate and which level to use for what. Capitalised message, no trailing period, key/value pairs.
- **Tests.** `envtest.Environment{DownloadBinaryAssets: true}` (no `setup-envtest`, no Makefile download step), Ginkgo and Gomega, a unique namespace per test,
  fake client only for narrow unit tests. Do not assert garbage collection, namespace deletion or Pods in envtest: assert `ownerReference` and finalizer fields.
- **Tools.** `go get -tool sigs.k8s.io/controller-tools/cmd/controller-gen@v0.23.0`, then `go tool controller-gen`.

## Deviation notes

When code breaks a piece of advice, say why in its own comment group **above** the doc comment, separated by a blank line. Anything inside the adjacent
comment group (including `// nolint:` lines) leaks into the generated CRD description `[CT, run]`.

```go
// Deviation: pointer kept because 0 and unset mean different things here.

// Replicas is the desired count.
// +optional
Replicas *int32 `json:"replicas,omitempty"`
```

## Hazards (cross-cutting; each reference lists the rest)

Cache and reads
- A transform that returns `nil, nil` to drop an object empties the store while `HasSynced()` stays true. Filter with selectors. `[CR, run]`
- `Update` of an object read from a pruned cache deletes the pruned keys on the server. Use `Patch`, or read the full object through `APIReader` first. `[run]`
- A per-type `ByObject.Transform` replaces `DefaultTransform`, so `managedFields` stay unless you compose them. `[CR, run]`
- `DisableFor` is keyed by GVK, so a `PartialObjectMetadata` read of a disabled type goes live to the API server every time. `[CR, run]`
- `GetClient()` before `Start()` returns `ErrCacheNotStarted` but still creates the informer. Reading an unregistered type builds a full informer unless
  `ReaderFailOnMissingInformer` is set. `[CR, run]`
- `GenerationChangedPredicate` drops every update of kinds whose `generation` is always 0 (Secret, ConfigMap). Adding a finalizer does not bump `generation`
  either, so returning after adding one and waiting for the update event never fires. `[run]`
- Mutating an object inside a predicate or handler, or one read with `UnsafeDisableDeepCopy`, corrupts the shared store. `[CR, BLOG]`
- Registering the same field index twice on one informer fails with "indexer conflict"; an unregistered `MatchingFields` errors at read time. `[run]`

Reconcile and writes
- A `Patch` or `Update` of the main resource overwrites the in-memory status with the server response (observedGeneration 5 became 0). `[run]`
- A plain `MergeFrom` status patch replaces the whole `conditions` array, so a stale copy erases another writer's condition. Use an optimistic lock,
  server-side apply with `listType=map`, or a single writer. `[run]`
- `Result.Requeue` never resets the backoff; an error returned together with `RequeueAfter` ignores `RequeueAfter` and logs a warning. `[run]`
- `AddFinalizer` and `RemoveFinalizer` change memory only: you must persist with a patch. `CreateOrUpdate` discards status changes. `[CR]`
- A webhook default on a field that also has a marker default never runs: the marker fires first. A marker `path` that differs from the registered path
  fails every matching write. `[run]`

API and codegen
- A misspelled marker is silently ignored and controller-gen exits 0, so a constraint can vanish: diff the generated CRD. `[run]`
- A field with no omit tag and no marker is required, and a pointer does not make it optional. `omitempty` on a struct still serializes `{}`. `omitzero`
  does not omit a non-nil empty slice or map. `[CT, run]`
- RBAC markers glued to a func or type comment generate nothing and raise no error: leave a blank line after the marker group. `[CT, run]`
- An empty condition `reason`, or one containing a space, fails the whole status write with 422. `meta.SetStatusCondition` does not stamp `observedGeneration`. `[run]`
- `+default=ref(...)` and non-JSON `+default=` values are silently dropped from the CRD. `[CT, run]`

Operations
- Plain-HTTP or unauthenticated metrics (`SecureServing: false`, or no `FilterProvider`), and a filter without TLS (it sends bearer tokens in cleartext). `[CR, run]`
- Without `tokenreviews` and `subjectaccessreviews` `create`, every authenticated scrape returns 500. `[CR, run]`
- Metrics registered on `DefaultRegisterer` are never served; `MustRegister` inside `Reconcile` or `SetupWithManager` panics on the second call. `[CR, run]`
- Missing lease RBAC leaves the manager alive and Ready with no workers and one log line. A `Reconcile` that ignores `ctx` outlives
  `GracefulShutdownTimeout`. `[CR, run]`
- Without `SetLogger`, logs are dropped. Logging a whole object leaks `Data` on most backends. Logging an error and returning it logs it twice. `[CR, run]`

Testing
- `cancel()` followed by `testEnv.Stop()` without waiting for `mgr.Start` to return times out after 20 seconds. `[run]`
- Awaiting namespace deletion or garbage collection in a spec waits forever: envtest has no controller to do either. `[run]`
- controller-runtime recovers panics in reconcilers and webhooks, so specs stay green unless the suite checks the `*_panics_total` metrics. `[CR, run]`
- A fake client without `WithStatusSubresource` returns NotFound from `Status().Update` and lets `Update` write status. `[CR, run]`

## Out of scope (pointers only)

OLM bundles, Helm charts, multi-cluster controllers, conversion webhooks and multi-version CRDs are not covered or verified here. For CRD versioning start
with the Kubernetes CRD versioning documentation (see `K8S` in `references/sources.md`). The controller-gen output under `config/` is the input for any packaging.
