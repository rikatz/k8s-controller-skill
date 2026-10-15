# 09 - Verification of controller-runtime / controller-gen claims

Environment: Go 1.27.2, controller-runtime v0.25.2, controller-tools v0.23.0, apimachinery/client-go v0.37.1,
sigs.k8s.io/json v0.0.0-20250730193827. Work dir: `/tmp/k8s-controller-skill-research/exp-verify`
(copy of `/tmp/cg-verify`; original untouched). Raw envtest log: `/tmp/envtest-run.log`.
Note: `rtk go test` hides `t.Log` output, so tests were run with `rtk proxy go test ...`.

Network: WORKS. `curl -I https://raw.githubusercontent.com/kubernetes-sigs/controller-tools/HEAD/envtest-releases.yaml` -> HTTP/2 200;
envtest `DownloadBinaryAssets: true` downloaded and started the control plane in ~4s (after first download).

---------------------------------------------------------------------------------------------------

## 1. `omitzero` on optional non-pointer fields

Files: `api/oz/types.go` (CRD gen), `cmd/ozrt/main.go` (serialization).
Command: `go tool controller-gen object crd paths=./api/oz output:crd:dir=/tmp/oz-crd`

### 1a. controller-gen: VERIFIED-BY-RUN + VERIFIED-BY-READING
`controller-tools@v0.23.0/pkg/crd/schema.go:560` `case "omitempty", "omitzero": omitEmpty = true`;
`schema.go:595-603` "everything that's not inline / omitempty / omitzero is required".
So `omitzero` counts exactly like `omitempty`. Explicit markers win over tags (schema.go:566-592):
`+optional`/`+kubebuilder:validation:Optional` => not required; `+required` => required even with omitzero.

Generated `required:` list for spec (observed):
```
required:
- noTag        # json:"noTag"            (no omit option, no marker)      -> required
- ptrNoOmit    # *string json:"ptrNoOmit"                                  -> required (pointer does NOT make it optional)
- strReq       # json:"strReq,omitzero" + // +required                    -> required
```
NOT in required (all optional): str, int, nest, slice, map, dur, time (all `,omitzero`, no marker), strOE/intOE/nestOE/sliceOE/mapOE/durOE/timeOE (`,omitempty`),
strOpt, nestOpt (`omitzero`+`+optional`), noTagOpt (no omit + `+optional`), ptr (`*string,omitzero`).
Schema types: metav1.Duration -> `type: string`; metav1.Time -> `type: string, format: date-time`; nested struct -> object; no change vs omitempty.
Conclusion: `omitzero` alone => optional in CRD, `+optional` not needed for that; a field with NO omit option needs `+optional` to be optional.

### 1b. Which JSON lib: VERIFIED-BY-READING
- Encoding: `k8s.io/apimachinery@v0.37.1/pkg/runtime/serializer/json/json.go:20` imports std `encoding/json`; `doEncode` uses `json.NewEncoder(w).Encode(obj)` (json.go:258) and `json.Marshal` (229, YAML path).
- Decoding: same file imports `kjson "sigs.k8s.io/json"` (json.go:24) - only the decoder (`UnmarshalCaseSensitivePreserveInts`). omitzero is encode-side only, so irrelevant to decode.
- controller-runtime `pkg/client/patch.go:24` imports `k8s.io/apimachinery/pkg/util/json`, whose Marshal is std encoding/json (util/json/json.go:20).
- => encoding is std `encoding/json`, which honors `omitzero` since Go 1.24. Go 1.27.2 here: honored (see 1c). Requires go.mod `go >= 1.24` for omitzero to be recognized by the toolchain/vet.
- fxamacker/cbor v2.9.1 (client-go CBOR serializer, not default) also parses `omitzero` (`structfields.go:234`). CBOR path not exercised (UNVERIFIED by run).

### 1c. Round-trip: VERIFIED-BY-RUN (`go run ./cmd/ozrt`)
Zero-value OZ (Spec has zero everything), via `encoding/json.Marshal` AND `serializer.NewCodecFactory(scheme).LegacyCodec(gv)` (identical output):
```
{"kind":"OZ","apiVersion":"oz.example.com/v1","metadata":{"name":"x"},
 "spec":{"nestOE":{},"durOE":"0s","timeOE":null,"noTag":"","noTagOpt":"","ptrNoOmit":null}}
```
- OMITTED (omitzero, zero): str, int, nest, slice(nil), map(nil), dur, time, ptr. 
- EMITTED despite omitempty: `nestOE:{}` (struct), `durOE:"0s"`, `timeOE:null` -> omitempty NEVER omits structs / metav1.Duration / metav1.Time. omitzero does. (This is the main reason to use omitzero on struct/Time fields.)
- EMITTED (no omit option): noTag, noTagOpt, ptrNoOmit:null.
- sigs.k8s.io/json decode of `{"spec":{"time":null,"dur":"0s","nest":{}}}` then std re-marshal: identical zero-omitting output (dur "0s" is zero Duration -> omitted).
- `runtime.DefaultUnstructuredConverter.ToUnstructured` ALSO honors omitzero: `map[metadata:map[name:x] spec:map[durOE:0s nestOE:map[] noTag: noTagOpt: ptrNoOmit:<nil> timeOE:<nil>]]`.
- GOTCHA (VERIFIED-BY-RUN): non-nil EMPTY slice/map with `omitzero` IS emitted (`"slice":[],"map":{}`); with `omitempty` it would be omitted (len 0). omitzero only omits nil slices/maps. Matters for patch diffs/SSA when code sets `[]T{}`.
- Non-zero check: `Nest{A:"q"}`, `Duration(1)`, `metav1.Now()` -> `"nest":{"a":"q"},"dur":"1ns","time":"2026-10-15T18:16:03Z"` emitted.
- Also seen in the Widget envtest: the Create body for a Widget with omitzero status was `{"kind":"Widget","apiVersion":"demo.example.com/v1","metadata":{"name":"rv","namespace":"default"},"spec":{"size":"Small"}}` (no `status`, no `replicas`).

---------------------------------------------------------------------------------------------------

## 2. Doc-comment leakage into CRD description

Files: `api/cm/types.go`; `go tool controller-gen object crd paths=./api/cm output:crd:dir=/tmp/cm-crd`. All VERIFIED-BY-RUN.
Rule (from observed behavior): description = the doc comment group immediately adjacent to the field/type (Go `Doc`), with `+marker` lines stripped; a detached group above (blank line between) is dropped; trailing comments dropped.

| # | Placement (field) | In CRD description? | Observed description |
|---|---|---|---|
| a | doc comment directly above | YES | `A doc comment directly above.` |
| b | second paragraph after blank `//` in same group | YES (blank line preserved) | `Second paragraph test first paragraph.\n\nSecond paragraph after blank slash line.` |
| c | separate group ABOVE the adjacent doc group, blank line between ("Hidden: ...") | NO - dropped; only adjacent group kept | `Visible: group directly above the field.` |
| c' | detached group above a field that has only markers (fm) | NO | no description at all |
| d | trailing end-of-line comment (`FD string ... // trailing`) | NO | no description |
| e | `// +notamarker this looks like a marker` line (no error from controller-gen) | line DROPPED, following text kept | `Text after unknown marker.` |
| f | `// Deviation: ...` as LAST line of doc comment | YES (leaks) | `Normal first line.\nDeviation: last line of the doc comment.` |
| g | `// Deviation:` after `// +optional` marker line | YES (leaks) | `...\nDeviation: after the marker line.` |
| h | `// Deviation:` after marker + blank `//` | YES (leaks, blank line kept) | `Description for FH.\n\nDeviation: after marker and blank //.` |
| i | doc, blank `//`, then `// +optional` (marker last) | marker stripped only | `Description for FI.` |
| j | `/* block comment */` after markers | YES leaks | `Description for FJ.\nblock comment Deviation` |
| k | `// TODO(alice): fix later` line in doc | NO - dropped (TODO(user) lines are filtered) | `Description FL.` |
| l | `//go:generate ...`, `//lint:ignore ...` (no space after //) | NO (Go directives stripped) | `Description FN.\nnolint:revive // lint-style directive` - the `nolint` line LEAKED (it has a space after `//`), the two directives were stripped |

Type-level (field with no own doc inherits the type's doc; field doc overrides type doc):
| Type | Observed description in CRD |
|---|---|
| TypeA: doc para, blank `//`, `Deviation: ...` same group | `TypeA has a doc comment.\n\nDeviation: type-level note after blank line, same group.` LEAKS |
| TypeB: `Deviation:` as last line of type doc | LEAKS (`TypeB has a doc.\nDeviation: last line of type doc.`) |
| TypeC: `// TypeC has a doc.` blank line `// Deviation: ...` then type | ONLY the adjacent group kept -> description = `Deviation: separate group above type (blank line).` (i.e. detached FIRST group dropped, adjacent group kept - the WRONG one for hiding) |
| TypeD: `// TypeD has a doc. // trailing` | trailing text is part of the same line: LEAKS |
| TypeE: `// Deviation: HIDDEN ...` blank line, `// TypeE real doc.` then type | NO leak: `TypeE real doc.` |
| Root CM: `// Deviation: HIDDEN ...` blank line, `// CM is the root doc.` + `// +kubebuilder:object:root=true` | NO leak: top-level `description: CM is the root doc.` |

CONCLUSION: put the `// Deviation: ...` note in its OWN comment group ABOVE the doc comment, separated from it by a blank line (the
doc comment must stay the group directly adjacent to the field/type). Alternatively, put it as a trailing end-of-line comment (d) - also dropped,
but gofmt/lint-unfriendly for long text. Never put it inside the doc group (even after `+optional` markers or after a blank `//`).
Warning: if a field has only markers and no doc group, a detached group above is still dropped, but a type-level doc will then be inherited as the description.

---------------------------------------------------------------------------------------------------

## 3. controller-gen `applyconfiguration` generator

- EXISTS in v0.23.0: VERIFIED-BY-RUN. `go tool controller-gen -h` lists `+applyconfiguration[:externalApplyConfigurations=<[]string>][,headerFile=<string>][,omitToolVersion=<bool>]`, example "controller-gen applyconfiguration paths=./apis/...".
- Markers (`go tool controller-gen -www applyconfiguration`; source `pkg/applyconfiguration/gen.go:50-53`):
  - `+kubebuilder:ac:generate=<bool>` (package) - REQUIRED on the package (doc.go / groupversion_info.go); without it the generator emits nothing (gen.go:113-123, `generateForPackage` returns early at ~223). VERIFIED-BY-RUN: first run with no marker produced no files.
  - `+kubebuilder:ac:generate=<bool>` (type) OR `+kubebuilder:resource` on the type enables a type (gen.go:125-130,148). With the package marker but no type marker: only an empty `utils.go` + `internal/internal.go` generated (empty `ForKind` switch). VERIFIED-BY-RUN. Both `+kubebuilder:ac:generate=true` and `+kubebuilder:resource:scope=Namespaced` were tested and each produced widget.go/widgetspec.go/widgetstatus.go (nested spec/status types come along automatically).
  - `+kubebuilder:ac:output:package=<string>` (package) - default `applyconfiguration`, relative to the API package dir (gen.go:132-146, 258-260 `OutputDir = Join(root.Dir, outpkg)`).
- Command: `go tool controller-gen applyconfiguration paths=./api/v1` (no output:dir needed; the generator writes next to the API package).
  Default output: `api/v1/applyconfiguration/{utils.go, internal/internal.go, api/v1/widget.go,widgetspec.go,widgetstatus.go}`  (note the odd nested `api/v1`).
  With `+kubebuilder:ac:output:package=../applyconfiguration`: `api/applyconfiguration/...` (same inner layout).
- Compiles? Out of the box NO. VERIFIED-BY-RUN: `go build` -> `api/v1/applyconfiguration/utils.go:19:10: undefined: v1.SchemeGroupVersion` (x3).
  Generated `utils.go` references `v1.SchemeGroupVersion`; kubebuilder scaffolds only `GroupVersion`. Fix: add `SchemeGroupVersion = GroupVersion` in the API package's groupversion_info.go.
  After that `go build ./...` passes.
- Works with `c.Apply`? YES, VERIFIED-BY-RUN in envtest (v0.25.2 `Client.Apply(ctx, runtime.ApplyConfiguration, ...ApplyOption)`, interfaces.go:67; `SubResourceWriter.Apply` at interfaces.go:162):
  ```go
  ac := acv1.Widget("w-apply","default").WithSpec(acv1.WidgetSpec().WithSize("Small"))
  c.Apply(ctx, ac, client.FieldOwner("verify"), client.ForceOwnership)   // err=nil
  c.Status().Apply(ctx, acv1.Widget(...).WithStatus(acv1.WidgetStatus().WithObservedGeneration(g)), client.FieldOwner("verify"), client.ForceOwnership) // err=nil, status.observedGeneration=1
  acv1.ExtractWidget(&w, "verify")  // err=nil, spec.size=Small
  ```
  Result: `size=Small`, managedFields has 1 entry manager=`verify`; CRD default `replicas` applied by server (pointer set). Conditions with `+listType=map` fine.
- Generated structs use `omitempty` and pointers for all fields (per client-gen convention), independent of the `omitzero` tags on the API types.

---------------------------------------------------------------------------------------------------

## 4. envtest consequences (VERIFIED-BY-RUN, `test/envtest_test.go`, env: `envtest.Environment{DownloadBinaryAssets:true, BinaryAssetsDirectory: tmp}`)

Start: ~4s (binaries already downloaded in tmp dir; first start includes download). Default arg `--disable-admission-plugins=ServiceAccount` (cr-src `pkg/internal/testing/controlplane/apiserver.go:337,452`).
- (i) Namespace create+delete: stays **Terminating forever**. Polled every 2s for 20s: `phase=Terminating deletionTimestamp=true metadata.finalizers=[] spec.finalizers=[kubernetes]`. Never disappears (no namespace controller to empty & finalize it). Tests must use unique namespace names and never wait for ns deletion.
- (ii) GC: owner ConfigMap deleted with default propagation (client.Delete with no options -> background). Owner gone immediately; a dependent ConfigMap and a dependent Deployment (both with ownerReference to owner UID) were **still present at t+30s** (checked every 3s: `owner gone=true dep-cm gone=false dep-deploy gone=false` for all 10 samples). No garbage collector. Tests must delete dependents explicitly (or assert ownerReferences instead of GC).
- (iii) Deployment, replicas=2, observed after 15s: `replicasets=0 pods=0`, status all zero, `observedGeneration=0`. No controller-manager => no ReplicaSets, no Pods, no status.
- (iv) Pod created in `default`: `spec.serviceAccountName=""`, `serviceAccount=""`, `volumes=0`, `phase=Pending`, `nodeName=""`; ServiceAccounts in `default` = `[]` (no `default` SA is created either - no serviceaccount controller). No defaulting of SA, no token volume. Pod stays Pending forever.
- Also seen: CRDs installed from `config/crd/bases` work; status subresource works; SSA works (check 3).

---------------------------------------------------------------------------------------------------

## 5. Default rate limiter, concurrency, CacheSyncTimeout (v0.25.2) - VERIFIED-BY-READING

Repo: `/tmp/k8s-controller-skill-research/cr-src/repo` (tag v0.25.2).
- Priority queue is ON by default: `pkg/controller/controller.go:272,281` `ptr.Deref(options.UsePriorityQueue, true)`; doc at controller.go:110-115 "This flag is enabled by default"; manager-level `config.Controller.UsePriorityQueue` is copied if the per-controller value is nil (controller.go:156-157).
- Default RateLimiter (priority queue ON): `pkg/controller/controller.go:273`
  `workqueue.NewTypedItemExponentialFailureRateLimiter[request](5*time.Millisecond, 1000*time.Second)` -> per-item exponential only: base 5ms, max 1000s (5ms * 2^n; first failure 5ms, cap reached at the 18th failure: 5ms*2^17=655s, 2^18 capped to 1000s). **No bucket limiter.**
  Same default repeated in `pkg/controller/priorityqueue/priorityqueue.go:86` (and doc at priorityqueue.go:63-64).
- If `UsePriorityQueue=false`: controller.go:275 `workqueue.DefaultTypedControllerRateLimiter[request]()` =
  `NewTypedMaxOfRateLimiter(ItemExponentialFailure(5ms,1000s), TypedBucketRateLimiter{rate.NewLimiter(10 qps, burst 100)})`
  (`client-go@v0.37.1/util/workqueue/default_rate_limiters.go:50-56`). NOTE: the `Options.RateLimiter` doc (controller.go:86-88) still says "MaxOfRateLimiter ... overall token bucket" - STALE for the default PQ path.
- `MaxConcurrentReconciles` default = **1**: `controller.go:263-265` (`<= 0 -> 1`); first filled from manager `config.Controller.MaxConcurrentReconciles` (controller.go:148-149), and by builder from `GroupKindConcurrency[gk]` if per-controller value is 0 (`pkg/builder/controller.go:414-419`).
- `CacheSyncTimeout` default = **2 minutes**: `controller.go:267-269` (`== 0 -> 2*time.Minute`; doc controller.go:67); manager-level `config.Controller.CacheSyncTimeout` and builder global opts override (controller.go:152-153, builder/controller.go:423-425).
- Effect of Result:
  - `RequeueAfter > 0` (err == nil): `pkg/internal/controller/controller.go:497-506` `c.Queue.Forget(req)` (resets per-item failure count -> exponential backoff restarts at 5ms) then `AddWithOpts{After: RequeueAfter}` (not rate-limited). So YES it Forgets.
  - `Result{}` success: `c.Queue.Forget(req)` (controller.go:515). Forget -> `priorityqueue.go:480-482` `rateLimiter.Forget(item)`.
  - err != nil: `AddWithOpts{RateLimited:true}` (controller.go:~490) -> priorityqueue.go:223-227 `rlAfter := rateLimiter.When(key)` (increments failure count); `RequeueAfter`/`Requeue` are ignored with a log warning if err != nil (controller.go:495-497). Terminal error (`reconcile.TerminalError`) -> no requeue.
  - deprecated `Requeue: true` -> `AddWithOpts{RateLimited:true}` (controller.go:507-510), NOT Forgotten (backoff grows). `reconcile.go:41` marks it Deprecated.

---------------------------------------------------------------------------------------------------

## 6. Empty-diff Patch - VERIFIED-BY-RUN + VERIFIED-BY-READING

Code: `pkg/client/typed_client.go:127-149`: `data := patch.Data(obj)` then ALWAYS `o.Patch(...).Body(data).Do(ctx)` - no check for empty data. Same shape for `PatchSubResource` (typed_client.go:304+). `mergeFromPatch.Data` (`pkg/client/patch.go:102+`) just marshals both and diffs.
Run (client wrapped with counting `http.RoundTripper`, envtest apiserver, Widget `w-apply`):
```
Patch(MergeFrom) no change                    requests=1  PATCH /apis/demo.example.com/v1/namespaces/default/widgets/w-apply
Patch(MergeFromWithOptimisticLock) no change  requests=1  PATCH ...widgets/w-apply
Status().Patch(MergeFrom) no change           requests=1  PATCH ...widgets/w-apply/status
Patch(MergeFrom) with change                  requests=1  PATCH ...
Update no change                              requests=1  PUT ...
empty MergeFrom Data() = {}
```
Body capture (`test/rv_test.go`):
```
PATCH .../widgets/rv         ct=application/merge-patch+json body={}
PATCH .../widgets/rv/status  ct=application/merge-patch+json body={}
PATCH .../widgets/rv         body={"metadata":{"resourceVersion":"218"}}   # MergeFromWithOptimisticLock, no other change
rv before=218, after empty Patch=218, after empty Status().Patch=218, after empty optimistic-lock Patch=218
```
So: an empty `MergeFrom` patch STILL sends an HTTP PATCH with body `{}`; the apiserver treats it as a no-op (resourceVersion and generation unchanged, no watch event). The optimistic-lock variant never has an empty body (it always carries resourceVersion; also a mismatching RV => 409). The client does NOT skip empty patches; callers must compare (`equality.Semantic.DeepEqual` on status before/after) to avoid the round trip.

---------------------------------------------------------------------------------------------------
## Not run / caveats
- Namespace workaround via `/finalize` subresource (to force-delete Terminating namespaces) not tested.
- CBOR serializer path for omitzero (read only).
- `git init` was run inside exp-verify only to quiet tooling; `/tmp/cg-verify` was not modified.
