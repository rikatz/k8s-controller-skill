# Best practices for pure controller-runtime + controller-gen controllers (upstream/community slice)

Legend: [V] = read from the cited source this session. [U] = from general knowledge, NOT verified (WebSearch was blocked by org policy; some pages were truncated at ~100k chars). [D] = sources disagree / nuance.

Abbreviations of base URLs:
- API-CONV = https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/api-conventions.md
- API-CHG = https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/api_changes.md
- CTRL = https://github.com/kubernetes/community/blob/master/contributors/devel/sig-api-machinery/controllers.md
- SAMPLE = https://github.com/kubernetes/sample-controller (controller.go)
- CRD-DOC = https://kubernetes.io/docs/tasks/extend-kubernetes/custom-resources/custom-resource-definitions/
- OS-API = https://github.com/openshift/enhancements/blob/master/dev-guide/api-conventions.md
- OS-CO = https://github.com/openshift/enhancements/blob/master/dev-guide/cluster-version-operator/dev/clusteroperator.md

## 1. API design (CRD types)

- Spec = desired state, declarative not imperative; status = observed state. Status written via `/status` subresource; writes to main resource ignore status. [V] API-CONV (Spec and Status); CRD-DOC (Status subresource: `subresources: {status: {}}`).
- `metadata.generation` increments on any change except `.metadata` or `.status` (when status subresource enabled). [V] CRD-DOC.
- Large or fast-changing status data belongs in separate objects, not in status. [V] API-CONV.
- No `phase` enum in new APIs: "The pattern of using phase is deprecated"; use conditions, because adding enum values breaks compatibility. [V] API-CONV; kube-api-linter has a `nophase` linter [V] https://github.com/kubernetes-sigs/kube-api-linter/blob/main/docs/linters.md
- Booleans: upstream says "Think twice about bool fields", prefer a string alias that can grow; if used, optional bools should be pointers + omitempty. [V] API-CONV. OpenShift is stricter: booleans "explicitly forbidden", use PascalCase enums naming the action (e.g. Optional/Required/Disabled). [V] OS-API. [D] upstream = discouraged, OpenShift = forbidden. User's global rule follows OpenShift. kube-api-linter `nobools` (off by default) enforces it.
- Enums: string aliases, CamelCase values with initial capital; no numeric enums. [V] API-CONV. Adding an enum value later is not automatically compatible; document that the set is open-ended and say how unknown values are handled. [V] API-CHG.
- No unsigned ints; use int32/int64 (linter `integers`); avoid floats (`nofloats`); durations as integers with unit in name e.g. `fooSeconds` (`nodurations`); int64 within +-2^53. [V] API-CONV + linters doc.
- Lists of named subobjects, not maps of subobjects; pure maps only for labels/annotations/data. Use `+listType=map` + `+listMapKey=` for SSA-friendly merging; linter `ssatags` checks listType markers. [V] API-CONV; SSA doc https://raw.githubusercontent.com/kubernetes/website/main/content/en/docs/reference/using-api/server-side-apply.md
  - listType: `atomic` (whole list replaced, one owner), `set` (unique scalars), `map` (keyed). CRDs with no annotations: all lists atomic. Changing atomic -> map later yields no conflicts and is not recommended. [V] SSA doc.
  - CEL transition rules (`oldSelf`) are only allowed on "correlatable" paths: every ancestor array must be `listType=map`; set/atomic arrays error at CRD write. [V] CRD-DOC.
- Optional/required: mark every field explicitly `+optional`/`+required` (linter `optionalorrequired`). Optional fields: `omitempty`, pointer only when zero value is meaningful. [V] API-CONV. OpenShift: in CRD APIs avoid pointers unless zero vs unset matters; `omitzero` for structs on Go 1.24+; structs need >=1 required field or MinProperties. [V] OS-API.
- `+nullable` discouraged (breaks merge patch, SSA applyconfigs). [V] API-CONV; linter `nonullable`.
- Defaulting: put defaults in the schema (`+kubebuilder:default` -> `default:`) on optional fields. Defaults apply on request, on read from etcd (not persisted until next write), and after mutating webhooks. Defaults must only fill unset values. [V] API-CONV (Defaulting), CRD-DOC (Defaulting). OpenShift: workload APIs default on the object and it must not change later; config APIs default in the controller and godoc says "subject to change over time". [V] OS-API.
- Object references: same namespace by default ("namespaces are a security boundary"); name fields `fooRef`/`fooName`; controllers must handle missing referents and must not mutate referents; don't copy referent values (esp. secret data) into referrer/events/logs. [V] API-CONV. OpenShift: reference by group+resource not apiVersion+kind; omit "Ref" suffix; use resource-specific reference types, not corev1.ObjectReference. [V] OS-API. [D] upstream names `xRef`, OpenShift omits "Ref" suffix; kube-api-linter `noreferences` prefers Ref/Refs over Reference(s).
- Validation: bound everything. CEL cost estimator assumes worst case for unbounded strings/lists/maps; CRD is rejected if rules exceed budget; set `maxLength`/`maxItems`/`maxProperties` (kubebuilder `MaxLength`, `MaxItems`). Avoid nested lists/maps under rules. [V] CRD-DOC (Cost limits).
- Validation (OpenShift): put limits in both markers and godoc; each concept should have one valid phrasing; discriminated unions need a required string discriminator matching one member. [V] OS-API.
- CEL rules (`x-kubernetes-validations`, kubebuilder `+kubebuilder:validation:XValidation:rule=...,message=...`): stable since v1.29; fields `message`, `messageExpression`, `reason` (FieldValueInvalid default / Forbidden / Required / Duplicate), `fieldPath`, `optionalOldSelf` (stable 1.33). Rules see only the current object (and oldSelf), no cross-object/stateful checks. [V] CRD-DOC.
- Immutability pattern: `self == oldSelf` (message "field is immutable") on the field; "set once": `!has(oldSelf.f) || self.f == oldSelf.f` style; other transition-rule idioms in CRD-DOC table (restrict transitions, append-only set, non-decreasing). Transition rules apply only on update. [V] CRD-DOC.
- Validation ratcheting (stable 1.33; alpha 1.28): update accepted if every failing part is unchanged; NOT ratcheted: transition rules, `oldSelf` rules, allOf/oneOf/anyOf/not, list-type/map-key changes, `required` changes, added/removed/renamed properties. [V] CRD-DOC. Implication: tightening validation is safer on new versions, but still not free.
- Structural schema is mandatory in apiextensions.k8s.io/v1; unknown fields pruned unless `x-kubernetes-preserve-unknown-fields`. [V] CRD-DOC. Schema restrictions: no `$ref`, `additionalProperties:false`, `uniqueItems:true`, etc. [V] CRD-DOC.
- Finalizer names must be domain-qualified (`example.com/name`); API server rejects otherwise. [V] https://kubernetes.io/docs/concepts/overview/working-with-objects/finalizers/
- Printer columns / selectableFields (stable 1.32) / scale subresource: `additionalPrinterColumns` priority 0 = default view; `selectableFields` enables `--field-selector`. [V] CRD-DOC.
- Webhook vs CEL: prefer in-process CEL rules in the CRD; ValidatingAdmissionPolicy (GA 1.30) is the "declarative, in-process alternative" to validating webhooks for rules that need request context/params/other policy scope (policy + binding; `validationActions` Deny/Warn/Audit; `failurePolicy`). [V] https://raw.githubusercontent.com/kubernetes/website/main/content/en/docs/reference/access-authn-authz/validating-admission-policy.md. Webhooks only when you need external calls/mutation/cross-object lookups [U: the VAP page gives no explicit preference guide]. Mutating webhook replacement (MutatingAdmissionPolicy) status: page did not state maturity [unverified].
- Compatible API changes: adding optional fields with sane defaults is compatible; never remove/rename a field in a version (add new alongside old, keep in sync); old field dropped only in a new API version. [V] API-CHG. Abandoned alpha fields leave tombstones. [V] API-CHG.
- Versioning & conversion for CRDs: exactly one `storage: true`; conversion strategy None only changes apiVersion (don't use when schemas differ); Webhook for real conversion; failed conversions can block reads/updates/deletes so never use conversion for validation; `status.storedVersions` blocks removal of old versions until storage migrated (StorageVersionMigration or manual rewrite + patch status). Deprecate with `deprecated: true` + `deprecationWarning`. [V] https://raw.githubusercontent.com/kubernetes/website/main/content/en/docs/tasks/extend-kubernetes/custom-resources/custom-resource-definition-versioning.md
- Operator-SDK doc: define CRDs with structural OpenAPI schema; version CRDs per Kubernetes API versioning; use conversion webhooks for API changes; do NOT self-register CRDs from the operator (global resource, needs risky privileges). [V] https://sdk.operatorframework.io/docs/best-practices/best-practices/
- Lint it: kube-api-linter (kubernetes-sigs) covers conditions, nobools, maxlength, optionalorrequired, ssatags, statussubresource, nophase, integers, etc.; several are off by default. [V] linters.md above. Useful for a "pure controller-gen" repo with no scaffolding.

## 2. Reconcile semantics

- Level-triggered: act on the state observed now, not on transitions; events can be missed/reordered; record what a decision depended on in status (e.g. `observedGeneration`). [V] CTRL; API-CONV ("level-based rather than edge-based").
- Queue holds object keys (namespace/name), not objects; reads the latest from cache at sync time; queue dedups and never gives the same key to two workers. Collapse secondary events (child changed) into "enqueue the owning primary". [V] CTRL, SAMPLE.
- Idempotent sync: compare desired vs actual and act on the diff; other actors can change things at any time; leader election is imperfect so multiple active replicas are possible, hence idempotency is also a safety property. [V] CTRL.
- Never mutate cache objects: DeepCopy before modifying (shallow copy of Annotations map still mutates shared object). [V] CTRL, SAMPLE (`foo.DeepCopy()` before status update).
- Do not treat absence in cache as proof of deletion if cache may not be synced; wait for cache sync before workers (`WaitForCacheSync`). controller-runtime does this in Manager; relevant when using raw informers. [V] CTRL, SAMPLE.
- Adoption with ownerRefs: bypass the cache with a live read if a child's owner ref appears changed, to avoid racing GC; never adopt when parent/child is deleting. [V] CTRL. [U] controller-runtime equivalent is `APIReader`.
- Async loops: on error end the sync and return the error; do not block polling until a condition clears; for long operations return and check again later. [V] Google blog https://cloud.google.com/blog/products/containers-kubernetes/best-practices-for-building-kubernetes-operators-and-stateful-apps
- One controller per CRD, one operator per application; separate controllers for scale/backup/restore sub-concerns. [V] same Google blog; operator-sdk best practices; CNCF whitepaper https://raw.githubusercontent.com/cncf/tag-app-delivery/main/operator-wg/whitepaper/Operator-WhitePaper_v1-0.md
- Do not make the application depend on the operator being up; operator shouldn't install other operators or assume its own namespace. [V] CNCF whitepaper; operator-sdk best practices (watched namespace configurable, empty = all).
- Clean up created objects when no longer needed. [V] operator-sdk best practices.

## 3. Status and conditions

- Use `[]metav1.Condition`: `+listType=map`, `+listMapKey=type`, `+patchStrategy=merge`, `+patchMergeKey=type`. [V] https://raw.githubusercontent.com/kubernetes/apimachinery/master/pkg/apis/meta/v1/types.go
- Condition fields: `type` CamelCase (optionally `domain/CamelCase`, max 316); `status` True/False/Unknown; `observedGeneration` optional (>=0); `lastTransitionTime` required, changes only when status changes; `reason` required, CamelCase, 1..1024 chars, regex `^[A-Za-z]([A-Za-z0-9_,:]*[A-Za-z0-9_])?$`; `message` <=32768, may be empty. [V] types.go.
- Set `observedGeneration` to `metadata.generation` that the condition was computed from; a lagging value means the condition is stale. [V] API-CONV; types.go.
- Set conditions on first visit even if Unknown; absence is interpreted as Unknown. [V] API-CONV. Gateway API: use Unknown when the controller cannot yet decide; controllers must not set empty string. [V] https://raw.githubusercontent.com/kubernetes-sigs/gateway-api/main/geps/gep-1364/index.md
- Naming: adjectives or past-tense verbs describing state, not transitions ("Ready", not "Deploying"); short names. Polarity: upstream gives no hard rule (prefer avoiding double negatives like Failed=False). [V] API-CONV. [D] Gateway API reserves positive polarity for summary conditions (Accepted, ResolvedRefs, Programmed, Ready) and uses negative-polarity conditions that appear only when true for errors. [V] GEP-1364. [D] OpenShift ClusterOperator uses Available(+), Progressing, Degraded(-) side by side. [V] OS-CO. => pick one convention per API and document it.
- Gateway API semantics worth copying: `Accepted` = valid enough to produce configuration (syntax, references mandatory, supported features); `ResolvedRefs` is non-blocking (Accepted=True, ResolvedRefs=False for missing backend); `Programmed` = config sent to data plane. Positive summary conditions must always be present. [V] GEP-1364.
- Gateway API multi-controller status: each controller writes only entries carrying its own `controllerName`, and must not touch others; read-modify-write with freshest copy; do not reorder/remove conditions you do not own; if a condition's `observedGeneration` is greater than what you know, do not update it (your copy is stale, wait for next reconcile); clean up stale entries; `parents` capped at 32, conditions per entry 1..8. [V] https://raw.githubusercontent.com/kubernetes-sigs/gateway-api/main/apis/v1/shared_types.go
- OpenShift library-go style semantics (OS-CO) [V]:
  - Available=False only when admin intervention is needed now; never during a normal upgrade.
  - Progressing=True while rolling out/moving between two steady states; NOT for mere reconciliation of known state or node-scale-up adjustments.
  - Degraded only after sustained failure; do not flap; can be Available and Degraded at once; not during normal upgrade.
  - Always set reason/message on happy states too (e.g. `AsExpected`); messages start with a capital; update all status fields atomically; Upgradeable optional.
- Use `UpdateStatus`/status client for status; don't change spec in the same call; SAMPLE sets FieldManager on writes. [V] SAMPLE.
- SSA caveat: apply on subresources that don't receive the resource type may not track ownership correctly. [V] SSA doc. [U] practical consequence: status via SSA works with the CRD status subresource but verify managed fields in tests.

## 4. Ownership, finalizers, SSA

- Owner refs express ownership/GC; labels/selectors express grouping. Controller refs: `metav1.NewControllerRef`; check `metav1.GetControllerOf` / `IsControlledBy` before taking over an object. [V] SAMPLE; owners-dependents https://raw.githubusercontent.com/kubernetes/website/main/content/en/docs/concepts/overview/working-with-objects/owners-dependents.md
- Cross-namespace owner references are disallowed by design: namespaced dependent may have cluster-scoped owner or same-namespace owner; cluster-scoped dependent may only have cluster-scoped owners (invalid -> unresolvable, never GC'd; event `OwnerRefInvalidNamespace`). [V] owners-dependents; garbage-collection doc https://raw.githubusercontent.com/kubernetes/website/main/content/en/docs/concepts/architecture/garbage-collection.md
- Deletion modes: background (default), foreground (`foregroundDeletion` finalizer; only `blockOwnerDeletion=true` dependents in GC cache block owner), orphan. [V] garbage-collection doc.
- Finalizers: with finalizers set, delete sets `deletionTimestamp` and returns 202; after deletion requested you can remove but not add finalizers; deleting object cannot be resurrected; avoid manually stripping finalizers. [V] finalizers doc. Finalizer only when external (non-owner-ref-GC'd) cleanup is needed [U: opinion, not stated in sources].
- SSA for controllers: applied object must contain every field the controller cares about; no prior read and no `resourceVersion` needed; controllers should "always force conflicts" on objects they own; fields not previously applied cannot be removed by apply (use patch/update); apply needs `patch` (+`create`) RBAC; identical values share ownership. [V] SSA doc.
- Ownership handover pitfalls (e.g. replicas with HPA: removing field before the other manager sets it defaults it to 1). [V] SSA doc.
- Unstructured/no-schema CRDs: all lists atomic under SSA. [V] SSA doc. => schema every list with listType.

## 5. Caching and performance

- Use shared informers from the shared factory (one watch connection and serialization per type) rather than ad-hoc reflectors. [V] CTRL.
- Reads from cache (listers); client for writes only. [V] SAMPLE. [U] controller-runtime's default client does the same, and this includes Secrets/ConfigMaps once you `Get` them, which creates a cluster-wide informer.
- Narrow what you cache: controller-runtime added label/field selectors on the cache because namespace+type filtering "might end up not being efficient enough". Caveat: a filtered-cache client never returns objects outside the filter. [V] https://raw.githubusercontent.com/kubernetes-sigs/controller-runtime/main/designs/use-selectors-at-cache.md. That design doc does NOT discuss memory/Secrets/ConfigMaps explicitly. Specific advice (disable caching for Secrets/ConfigMaps, restrict to namespace, metadata-only watches, `APIReader`) = [U] verify against controller-runtime docs (other slice).
- Watch scope: namespace for operator is configurable; empty = all namespaces. [V] operator-sdk best practices. Watch-all vs scoped RBAC trade-off [U].
- Secrets: `list`/`watch` on Secrets exposes all secret contents; an informer on Secrets requires list+watch. [V] RBAC good practices https://raw.githubusercontent.com/kubernetes/website/main/content/en/docs/concepts/security/rbac-good-practices.md. Practical consequence (caching Secrets means cluster-wide read access unless namespaced) [U inference].
- Skipping on equal ResourceVersion in update handler filters resync noise, but skipping requeue on a failure path can mean the item is never retried. [V] CTRL, SAMPLE. [U] controller-runtime equivalent = predicates (`GenerationChangedPredicate` etc.): beware it hides status/annotation changes and delete/finalizer changes.

## 6. Errors, requeue, backoff

- Return the error to retry with rate-limited backoff; success -> `Forget`; `Done` always. Failure -> `AddRateLimited`. [V] CTRL, SAMPLE.
- Combine per-item exponential backoff with an overall bucket limiter (sample uses `rate.NewLimiter(50, 300)`); controller-runtime's default limiter is comparable [U].
- Do not retry unretryable errors: sample deliberately returns nil for missing object or empty deployment name; later events re-enqueue. Return errors only when retry can help; otherwise report and return nil, which makes retry behavior auditable. [V] SAMPLE, CTRL.
- Don't log an error and also return it (double logging); wrap with `fmt.Errorf("...: %w", err)`, avoid "failed to" repetition; libraries return errors rather than logging. [V] https://raw.githubusercontent.com/kubernetes/community/master/contributors/devel/sig-instrumentation/logging.md
- Hot-loop and status-conflict pitfalls: status writes that change a field every reconcile (timestamps, unstable ordering, message with counters) generate a watch event that re-triggers reconcile; only update status when it differs; on `Conflict` re-read and retry. [U: not from a fetched source; widely repeated community lore. Foundational support: `lastTransitionTime` should change only on status transitions [V] types.go; generation excludes status [V] CRD-DOC]. Conflict handling with backoff = user's global standard (Idempotency rule).
- Deletion must tolerate missing referents/children (NotFound is success). [V] API-CONV (controllers must handle missing referents).

## 7. Testing

- Fake client (controller-runtime) maintainers say "almost always better not to use this package"; use envtest. Caveats: cannot inject specific errors; subresource support partial; no OpenAPI schema validation; Generation/ResourceVersion unreliable. [V] https://raw.githubusercontent.com/kubernetes-sigs/controller-runtime/main/pkg/client/fake/doc.go. Status-subresource handling via `WithStatusSubresource` and absence of SSA fidelity are NOT in that doc [U/unverified].
- envtest: local etcd + kube-apiserver binaries (`KUBEBUILDER_ASSETS`), can also target an existing cluster just to load CRDs/config. [V] https://raw.githubusercontent.com/kubernetes-sigs/controller-runtime/main/pkg/envtest/doc.go. That doc does NOT list limits; "no controller-manager, no kubelet, no GC, namespaces never finish terminating, pods never run" = [U] (well known, verify against the controller-runtime/book slice). Consequences [U]: create ownerRef-GC assertions manually; set owner/status of child objects yourself; do not wait for Pods Ready.
- Test pyramid per SIG Testing: fast unit tests (seconds per package), integration for components needing cluster pieces with test-specific config ("often the case for controllers"), e2e costly/flaky; retry only on explicitly retriable errors; periodic jobs besides presubmit. [V] https://raw.githubusercontent.com/kubernetes/community/master/contributors/devel/sig-testing/testing-strategy.md
- API tests: round-trip (fuzz) tests, defaulting, validation, conversion tests, set feature gates explicitly. [V] API-CHG. [U] For CRDs: test CEL rules (including transition rules via update) against envtest since the fake client does not run schema/CEL validation [V that fake client skips OpenAPI validation].
- Ginkgo/Gomega vs plain `testing`: no source in this slice weighs in. K8s core uses table-driven plain `testing` for unit/integration ("Table-driven tests are the preferred way" [V] https://raw.githubusercontent.com/kubernetes/community/master/contributors/devel/sig-testing/integration-tests.md); controller-runtime/kubebuilder ecosystem and OpenShift use Ginkgo [U]. [D] flag as a style choice; user's global standard says Ginkgo tests (`@engineer`).

## 8. Security / RBAC

- Least privilege: RoleBinding over ClusterRoleBinding where possible; no wildcards (they cover future types); watch `list`/`watch` on Secrets; never grant `escalate`, `bind`, `impersonate`, `serviceaccounts/token create`, `nodes/proxy`, webhook configuration write, namespace `patch`, PV create unless required. [V] RBAC good practices.
- Creating pods/workloads implicitly grants access to secrets/configmaps/SA tokens in the namespace; use `automountServiceAccountToken: false` where tokens aren't needed. [V] same.
- Operator: dedicated ServiceAccount, not `default`; avoid root; don't self-register CRDs. [V] operator-sdk best practices. CNCF: document RBAC scope, treat broad privilege requests as red flag; SDK-generated RBAC defaults may be too broad; narrow ClusterRoles; limit cloud IAM. [V] CNCF whitepaper.
- Pod Security "Restricted" for the controller Deployment: `runAsNonRoot: true`, `allowPrivilegeEscalation: false`, `capabilities.drop: [ALL]`, `seccompProfile: RuntimeDefault|Localhost`, volumes limited to configMap/csi/downwardAPI/emptyDir/ephemeral/PVC/projected/secret. [V] https://raw.githubusercontent.com/kubernetes/website/main/content/en/docs/concepts/security/pod-security-standards.md. Also `readOnlyRootFilesystem` is NOT in Restricted [U], but commonly added.
- Never copy referent secret values into other objects, events, or logs. [V] API-CONV.
- Image policy/signing/supply chain: CNCF whitepaper says follow CNCF supply-chain guidance incl. signing. [V] (generic). Specific ImagePolicy (OpenShift `ImagePolicy` CRD / sigstore) not verified here [U].

## 9. Observability and operations

- Logging: messages start with a capital letter, no trailing period; structured key/value pairs; past tense ("Could not delete B"); name the object type; `err` key; do not log-and-return; V(0)-V(5) verbosity (V(2) steady-state default). [V] logging.md. K8s is moving klog -> logr; use contextual logger. [V] logging.md; SAMPLE uses `klog.FromContext`. [V]. User's global standard repeats this (logr KV, Capital, no period).
- Events: record Normal on successful sync, Warning on conflicts; reasons/messages as constants; register types in scheme so events can reference them. [V] SAMPLE. Events API dependence on rate-limiting/aggregation [U].
- Metrics: expose Prometheus metrics via an HTTP endpoint (not logs/files); label properly; cover health/KPIs and SLIs (throughput, latency, availability, errors, capacity). [V] Google blog; operator-sdk best practices. OpenShift operators expose metrics via a `metrics` Service. [V] https://raw.githubusercontent.com/openshift/enhancements/master/dev-guide/operators.md. The controller-runtime built-in metric names (`controller_runtime_reconcile_total`, `workqueue_*`, etc.) = [U] (other slice).
- Leader election: use a `coordination.k8s.io` Lease; name it after your component; if multiple instances may coexist add prefix/hash to avoid collisions. [V] https://kubernetes.io/docs/concepts/architecture/leases/. Lock release on exit for kube-controller-manager is alpha 1.36 and not a general mechanism. [V] same. Controller-runtime's `LeaderElectionReleaseOnCancel` = [U] (other slice).
- Health probes (`healthz`/`readyz`), graceful shutdown (context cancel + terminationGracePeriod), resource requests/limits, PodDisruptionBudget for replicated controllers: not covered by any fetched source. [U] general practice; leader-elected replicas make PDB with minAvailable 1 meaningful.
- Run multiple replicas with leader election. [V] CTRL.
- Emit events on operator debug; `oc get events`. [V] OS operators doc.

## 10. Packaging

- Ship the CRD + the controller Deployment together; controller runs as a normal workload outside control plane. [V] https://kubernetes.io/docs/concepts/extend-kubernetes/operator/
- Version operator with semver; CRDs with k8s API versioning; leave dependency management to OLM / package manager, don't install other operators. [V] operator-sdk best practices; CNCF whitepaper.
- Operator upgrades must leave managed resources unchanged and healthy; support operands created by older operator versions; CRD versions signal stability. [V] CNCF whitepaper; operator-sdk best practices (operator fan-out / single / hybrid version models).
- Include `description` godoc on every field (generated schema docs); OpenShift: godoc starts with the JSON field name, full sentences, document defaults/omission behavior. [V] OS-API.
- Make the watched namespace configurable; don't hard-code namespace or names. [V] operator-sdk best practices.
- OpenShift build conventions: `make build|images|test-unit|verify|update|test-e2e`; `verify` checks gofmt/vet/codegen. [V] https://raw.githubusercontent.com/openshift/enhancements/master/dev-guide/operators.md. For a pure controller-gen repo, make `verify` fail if `controller-gen` output (CRDs, RBAC, deepcopy) is stale [U recommendation].

## Disagreements and gaps summary

1. Booleans: upstream "think twice" vs OpenShift "forbidden" (user standard = forbidden).
2. Condition polarity: upstream no rule; Gateway API positive summary + negative error-only; OpenShift Available/Progressing/Degraded mix.
3. Reference field naming: `xRef` (upstream, linter) vs no "Ref" suffix and group/resource (OpenShift).
4. Pointers for optional fields: upstream: pointer when zero value meaningful; OpenShift CRDs: avoid pointers unless needed, `omitzero` on Go 1.24+.
5. Testing framework (Ginkgo vs plain `testing`): not resolved by sources.
6. Not verified (no source reached): Gateway API implementers guide (URL 404), envtest limitations list, fake client `WithStatusSubresource`/SSA behavior, controller-runtime metrics list, probes/PDB/graceful shutdown/resource sizing, Secrets/ConfigMap cache-memory pitfalls, hot-loop/status-conflict blog posts, Red Hat/Cloud-Native blog posts beyond Google 2018 and CNCF whitepaper, ImagePolicy. Pages truncated at 100k chars: API-CONV (validation section, immutability, SSA tail), CRD-DOC, SSA doc head (covered via raw markdown), finalizers page tail.
7. Google blog is from 2018 and recommends Kubebuilder; treat as dated for tooling, still valid for principles.
