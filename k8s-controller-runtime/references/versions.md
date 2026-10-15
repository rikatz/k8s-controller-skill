# Versions and API changes

Read this when you pick or bump dependency versions, or when generated or remembered code uses an API that may have changed. This is the single place for
version-specific facts; other references state behaviour of the pinned versions without repeating history.

## Pinned versions

| Component | Version | Facts |
|---|---|---|
| controller-runtime | v0.25.2, released 2026-10-01 | `go 1.26.0`; `k8s.io/{api,apimachinery,client-go,apiserver,apiextensions-apiserver}` v0.37.0 [CR go.mod, `gh api` release] |
| controller-tools (controller-gen) | v0.23.0, released 2026-10-15 | `go 1.26.0`; `k8s.io/{api,apiextensions-apiserver,apimachinery,apiserver}` v0.37.0 [CT go.mod, `gh api` release] |
| kube-api-linter | no tags or releases; use a pseudo-version, e.g. `v0.0.0-20261012124120-79b557aa8c8d` | `go 1.26.0`, so `tools/go.mod` needs `go 1.26` or newer [KAL go.mod, 10-kube-api-linter notes] |
| Go | 1.26 or newer | v0.25.2 needs 1.26.0 (go.mod); `new(expr)` (e.g. `new(true)`) needs Go 1.26, and controller-runtime's own docs use it since v0.24 [CR release notes v0.24.0, run] |
| envtest binaries | default `DownloadBinaryAssetsVersion` = latest stable in the index; the newest entry was 1.37.0 | Pin a full version (`"1.37.0"`) so CI does not move when the index does [CR binaries.go, run] |
| Ginkgo / Gomega | tests here compiled with v2.33.0 / v1.44.0 | controller-runtime's own go.mod has v2.27.4 / v1.39.0 for its tests; controller-tools v0.23.0 requires v2.33.0 / v1.44.0, and `go mod tidy` picked those [CR go.mod, CT go.mod, run] |

## Compatibility and policy

- The README compatibility table stops at v0.24 (`CR v0.24` = k8s.io v0.36, Go 1.26) and has no v0.25 row. v0.25 = k8s.io v0.37 and Go 1.26.0 comes from go.mod and the
  release note "Bump to k8s.io/* v1.37"; the README does not state it. [CR README, CR go.mod, release v0.25.0]
- Policy: major version 0, one new minor per Kubernetes minor, breaking changes are allowed in minors, patch releases never break. [CR VERSIONING.md]
- A project can depend on only one controller-runtime version, because the k8s.io libraries break in the same way. Backports normally reach only the previous minor. [CR VERSIONING.md]
- Kubernetes REST API compatibility is guaranteed; a client-go/apimachinery cross-version matrix is not. Take k8s.io/* at the version controller-runtime requires. [CR VERSIONING.md]
- Read the release notes of every minor you skip over: `gh api repos/kubernetes-sigs/controller-runtime/releases/tags/vX.Y.0 --jq .body`. [CR]
- controller-tools tracks the k8s.io minor: v0.21.0 requires v0.36, v0.22.0 and v0.23.0 require v0.37 (read in each go.mod). CAPV builds against controller-runtime v0.25.2 with controller-gen v0.22.0,
  so an exact match with controller-runtime is not required. Whether a mismatched pair breaks generation is not verified [unverified]. [CT go.mod, CAPV Makefile]
- Pin controller-gen with a `tool` directive in go.mod (`go tool controller-gen`) so the version is recorded; regenerate and review the CRD diff on every bump. [CT, run]

## Changelog of API changes a model may get wrong

Verified against code at the stated tags (module cache or the v0.25.2 clone) and release notes unless marked otherwise.

| Version | Change | Use now |
|---|---|---|
| v0.18.0 (2024-04-24) | Generics for handlers, predicates, events: `TypedEventHandler`, `TypedPredicate`, `event.Typed*` | `handler.EnqueueRequestForObject` etc. still work unchanged |
| v0.19.0 (2024-08-15) | `reconcile.TypedReconciler`, `source.TypedSource`, `TypedSyncingSource`, `builder.TypedBuilder` | `reconcile.Reconciler = TypedReconciler[Request]`; custom request types only for multi-cluster style controllers |
| v0.21.0 | `reconcile.Result.Requeue` deprecated (#3107); honoured as a rate-limited requeue | `RequeueAfter`, or return an error |
| v0.22.0 (2025-08-28) | Native server-side apply: `c.Apply(ctx, runtime.ApplyConfiguration, opts...)`; fake client SSA | Generated applyconfigurations with `client.FieldOwner`, `client.ForceOwnership` |
| v0.23.0 (2026-01-19) | Webhook builder is generic: `ctrl.NewWebhookManagedBy(mgr, &T{})`, `admission.Validator[T]`, `admission.Defaulter[T]`; `CustomValidator`, `CustomDefaulter`, `WithCustomValidator`, `WithCustomDefaulter` deprecated | `ValidateCreate(ctx, obj *T)` takes the typed object, not `runtime.Object` |
| v0.23.0 | `Status().Apply` and `SubResource(x).Apply` added; the `client.Apply` patch value is now `Deprecated` | `c.Apply`, `c.Status().Apply`; do not write `Patch(ctx, obj, client.Apply)` |
| v0.23.0 | Priority queue on by default; `reconcile.Result.Priority *int` | Nothing to enable; `Priority` is honoured only with the priority queue |
| v0.23.0 | New events API: `mgr.GetEventRecorder(name)` writes `events.k8s.io` Events; `GetEventRecorderFor` (core `Event`) is `Deprecated` | RBAC for the `events.k8s.io` group, not core `""` |
| v0.23.0 | `client.Options.FieldOwner`, `ReconciliationTimeout` + `controller_runtime_reconcile_timeouts_total`, `CreateOrPatch` alias in `alias.go` | Release notes |
| v0.24.0 (2026-04-30) | k8s.io v0.36; `scheme.Builder` deprecated; `IndexField` no longer blocks until the informer syncs; custom-path webhook func removed (#3465, identifier not traced) [unverified] | `runtime.NewSchemeBuilder(addKnownTypes)` |
| v0.25.0 (2026-09-03) | k8s.io v0.37; `GetEventRecorder` now returns `recorder.EventRecorder` (adds `AnnotatedEventf`), listed under Breaking Changes in the release note (#3509) | Same call, wider return type |
| v0.25.0 | Experimental read-your-writes client; `webhook.Options{Port: -1}` disables the webhook server; `source.TypedInformer`; opt-in client-go REST metrics | See below |
| v0.25.1, v0.25.2 | Patches: priority-queue data race, subresource create under read-your-writes, `SubResourceCreateOptions`, metrics handler options, read-your-writes wait metric | No API break |

- Read-your-writes: the release note writes `Client.EnableReadYourWritesConsistency`, but the field is `ctrl.Options{Client: client.Options{Cache: &client.CacheOptions{EnableReadYourWritesConsistency: new(true)}}}`. It compiled and
  removed stale reads in a test (0 of 200 against 11 to 32 of 200). [CR client.go, release v0.25.0, run] Documented as incorrect with more than one Namespace configured. [release v0.25.2]
- Event recorder history: `GetEventRecorder` exists in v0.23.3 and v0.24.1 returning `events.EventRecorder`, and is absent in v0.22.1. [CR code at each tag]

```go
// Compiled against v0.25.2.
var (
	GroupVersion  = schema.GroupVersion{Group: "demo.example.com", Version: "v1"}
	SchemeBuilder = runtime.NewSchemeBuilder(addKnownTypes) // replaces the deprecated scheme.Builder
	AddToScheme   = SchemeBuilder.AddToScheme
)

func addKnownTypes(s *runtime.Scheme) error {
	s.AddKnownTypes(GroupVersion, &Widget{}, &WidgetList{})
	metav1.AddToGroupVersion(s, GroupVersion)
	return nil
}

// In a reconciler and SetupWithManager:
//   r.Apply(ctx, appsv1ac.Deployment("x", "ns"), client.FieldOwner("demo"), client.ForceOwnership)
//   return ctrl.Result{RequeueAfter: time.Minute, Priority: new(100)}, nil
//   mgr.GetEventRecorder("demo").Eventf(obj, nil, "Normal", "Reason", "Action", "Note %s", "x")
//   ctrl.NewWebhookManagedBy(mgr, &Widget{}).WithValidator(v).Complete()   // v is admission.Validator[*Widget]
```

- Swapping to `runtime.NewSchemeBuilder` left the generated CRD byte-identical and controller-gen `object` output unchanged; the tests passed. [run]

## Unverified

- The Go minimum for v0.25 is inferred from `go 1.26.0` in go.mod only (the README table has no row).
- Which identifier #3465 removed in v0.24.0, and whether a controller-gen and k8s.io minor mismatch breaks generation.
- Read-your-writes behaviour after the v0.25.2 tag (design PR #3473 merged later) and its use with a selector-filtered cache.
- First v0.23.x patch containing `GetEventRecorder` (checked at v0.23.3 only; release notes v0.23.0 list the events migration).

## How to update this file

1. Bump versions in a scratch copy of `research/experiments/cg-verify`, then `go get sigs.k8s.io/controller-runtime@vX.Y.Z` and `go get -tool sigs.k8s.io/controller-tools/cmd/controller-gen@vA.B.C`.
2. Re-read `go.mod` of both modules, the controller-runtime README table, `VERSIONING.md`, and the release notes of every minor since the last pin.
3. Re-run the experiments in `research/experiments`: `exp-verify` (omitzero, comment leakage, applyconfiguration, envtest limits, empty patches), `exp-nil-transform` with `exp-ctl` as control, `exp-kal` and `exp-kal2`.
   Use `go build -buildvcs=false`, and `rtk proxy go test` to see `t.Log` output.
4. Re-run the testing-envtest suite (including the limit specs): a limit that stops holding is a changelog entry, not only a test failure.
5. Compare each changelog row against the code at the new tag; move rows older than two minors into one line, keep `[unverified]` labels honest, and update the tag-to-URL map in `sources.md`.
