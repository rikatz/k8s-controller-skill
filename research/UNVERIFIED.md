# Unverified ledger

Everything here was NOT confirmed by reading code or running it. If a shipped file states one of these as fact, that is a defect.
Source: the drafting agents' reports (each agent listed what it omitted or marked `[unverified]`), plus the research notes. Re-check on every version bump.

## Still unverified
Testing and versions
- Istio's reasons for finding envtest "subpar" (only the word is in the notes).
- Whether a generated type converter fixes fake-client server-side apply for CRD types (the fake kept 1 condition where the API server kept 2).
- `failurePolicy: Fail` rejecting requests until the webhook listens (not run).
- Which identifier controller-runtime #3465 removed in v0.24.
- Whether a controller-gen and k8s.io minor-version mismatch breaks generation.
- Minimum Go version for v0.25 (inferred from `go.mod`); first v0.23.x patch containing `GetEventRecorder` (checked at v0.23.3 only); read-your-writes after the v0.25.2 tag.

Cache and reads
- An index on `PartialObjectMetadata` (stated from code structure only).
- Read-your-writes waiting on a write the cache never sees; stale read-after-write (stated by design, not run).
- kubernetes/kubernetes#130767 (a large `List` delaying informer writes): omitted.
- "Up to an order of magnitude" memory saving for metadata-only reads (the blog's claim, not measured); the "heavy types" list is a judgement call; per-handler listener overhead.

API and codegen
- The Ready printer column through `kubectl` (only the API server's Table API was tried); status `MinProperties` against an empty-status create.
- The default-on kube-api-linter set at each repo's pinned KAL version (HEAD docs were read).

Reconcile, webhooks and events
- Whether a real cluster enforces `/finalizers update` on the owner when `blockOwnerDeletion` is set (envtest runs as admin).
- Whether recording an event for a custom resource needs its type registered in the manager's scheme (`observability-rbac.md` states it; the run used a ConfigMap).
- cert-manager and OpenShift service-ca annotation names (not named).
- Option defaults read in code but not run: `MaxConcurrentReconciles` 1, `CacheSyncTimeout` 2m, the 5ms to 1000s limiter with no bucket, `ReconciliationTimeout` only cancelling `ctx`.
- Webhook server defaults and certificate hot-reload (read, not exercised).

Operations
- Wiring a collector to `mgr.GetCache()`; lock types other than Lease; core-events-only RBAC while using `GetEventRecorder`; why `/finalizers update` is needed (admission reason).
- A slow webhook start failing liveness; the time-to-ready histogram (a design suggestion with no upstream example).
- Resource requests and limits (omitted; no source gives controller-specific numbers); the Restricted pod-security fields were not run against a cluster.
- Claims about kube-state-metrics, cert-manager, Flux, Argo CD and Kong come from the notes and were not re-fetched.

Setup and logging
- Multi-group layout; conversion webhooks; webhook Service and certificate manifests; "more than one manager duplicates informers".
- `LeaderElectionReleaseOnCancel` behaviour (cited from the field doc only); whether CAPV fails CI on a dirty tree after generate.
- slog `ReplaceAttr` to prettify level names ("not tried"); thread-safety of klog's `-v` at runtime (observed working, not claimed safe); `ToSlogHandler` (read, not run); which sample-controller revision was read (master only).

Research coverage
- Talks T13 and T14 were unread (no subtitles; T14 is the 1h27m reconciler tutorial); T12 was skimmed only. Talk event names and dates are inferred.
- WebSearch was blocked by organisation policy, so no Red Hat or community blog posts were found. The Gateway API implementers guide URL returned 404.
- Several fetched pages were summarised excerpts (the API conventions and CRD pages were truncated at about 100k characters); re-check exact quotes before citing verbatim.

## Resolved during drafting (verified by running or reading)
Pruned-cache `Update` deletes keys while a `MergeFrom` patch is safe; `nil, nil` transform empties the store; RBAC marker `urls=/metrics`; `+default` and
`+kubebuilder:default` are identical (strict JSON only); envtest limits (namespaces stay Terminating, no GC, no controller-manager, no default ServiceAccount);
default rate limiter (per-item exponential 5ms to 1000s, no bucket with the priority queue); empty `MergeFrom` still sends `PATCH {}`; `omitzero` handling in
controller-gen and the serializer; doc-comment leakage; applyconfiguration generation; read-your-writes field path (`client.Options.Cache.EnableReadYourWritesConsistency`);
typed generics arrived in v0.18 and v0.19; `Result.Requeue` deprecated in v0.21.0; `GetEventRecorder` exists since v0.23.x; `ctrl.GetConfig` sets QPS -1 (the "20/30 default" in note 07 was wrong); RBAC under envtest is testable despite the
`system:masters` admin client, via `rest.Config.Impersonate` bound to the generated Role (`observability-rbac.md` Section 5) and via ServiceAccount
TokenRequest tokens against the metrics endpoint (`metrics.md` Section 7) - both ran.
`optionalOldSelf`: controller-tools v0.23.0 supports `optionalOldSelf=true` on the XValidation marker and emits
`optionalOldSelf: true`; at runtime the rule runs on create with `oldSelf` an empty Optional (`oldSelf.hasValue()` /
`oldSelf.value()`), and the custom-ratchet pattern works. Full ratcheting behaviour (CEL/enum ratchet per failing
field, oldSelf rules never ratcheted and skip create, `required` excused only while the parent subtree is unchanged)
verified on envtest 1.37.0 in `research/experiments/exp-ratcheting`; see `research/notes/12-ratcheting.md` [run].

## Confirmed during the independent consistency review (moved out of "still unverified")
- `events.k8s.io` RBAC `create;patch` for `GetEventRecorder`: run (the event was stored with only that rule); the shipped tags already say `[run]`.
- RBAC on `<resource>/status` for server-side apply: re-verified true.
- `listType=set` under an `oldSelf` CEL rule: re-verified true; now also reproduced at CRD admission in `exp-ratcheting` stage7, together with the `atomic` variant (`oldSelf cannot be used on the uncorrelatable portion of the schema`) [run].
Note: the consistency review also found and fixed F1 (the `api-design.md` excerpt did not actually pass KAL; now verified 0 issues after adding doc comments on the root wrapper, `omitempty` on `Spec`, and `MinProperties=1` on `GadgetSpec`).
