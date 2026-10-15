# 04 - Conference talks on controller authoring (controller-runtime + controller-gen)

Method: transcripts pulled with yt-dlp (auto/manual English subs) from CNCF YouTube; timestamps below are `[m:ss]` from the transcript. Event names/dates are inferred from YouTube upload date and description (CNCF channel); "~" means inferred, not read from a sched.co page. WebSearch was unavailable (org policy), so discovery used yt-dlp `ytsearch`. Slides were NOT fetched; everything is from spoken transcript.
Name spellings confirmed from CNCF talk titles: **Stefan Büringer**, **Christian Schlotter** (not "Schloter"), **Fabrizio Pandini**. Büringer is stated in the talk (bRPfmviTi3s [0:47]) to be a controller-runtime maintainer; Pandini and Schlotter are Cluster API maintainers.

## Talks

| # | Title | Speaker(s) | Event (inferred) | URL | Status |
|---|-------|-----------|------------------|-----|--------|
| T1 | Don't Write Controllers Like Charlie Don't Does: Avoiding Common Kubernetes Controller... | Nick Young (Isovalent at Cisco; Contour/Gateway API/Cilium) | KubeCon+CloudNativeCon EU London ~Apr 2025 (uploaded 2025-04-15) | https://www.youtube.com/watch?v=tnSraS9JqZ8 | READ (full transcript) |
| T2 | Cluster API Deep Dive: Improving Performance up to 2k Clusters | Fabrizio Pandini, Stefan Büringer | KubeCon NA Chicago ~Nov 2023 (uploaded 2023-11-22; "first day of KubeCon") | https://www.youtube.com/watch?v=bRPfmviTi3s | READ |
| T3 | Kubernetes CRD Design for the Long Haul: Tips, Tricks, and... | Christian Schlotter, Fabrizio Pandini | KubeCon EU London ~Apr 2025 | https://www.youtube.com/watch?v=7IA-Vw1K7eg | READ |
| T4 | Building Better Controllers | John Howard (Google, Istio) | KubeCon NA Chicago ~Nov 2023 | https://www.youtube.com/watch?v=GKPBQDJ2Hjk | READ |
| T5 | Beyond the Limits: Scaling Kubernetes Controllers Horizontally | Tim Ebert (STACKIT/Gardener) | KubeCon EU London ~Apr 2025 | https://www.youtube.com/watch?v=OTzd9eTtLRA | READ |
| T6 | Writing a Kubernetes Operator: the Hard Parts | Sebastien Guilloux (Elastic, ECK) | KubeCon NA San Diego Nov 2019 | https://www.youtube.com/watch?v=wMqzAOp15wo | READ |
| T7 | Why Are We Copying and Pasting So Much? | Solly Ross, Phil Wittrock (Google; controller-runtime/kubebuilder creators) | KubeCon NA Seattle Dec 2018 | https://www.youtube.com/watch?v=0tX3q_WAEdE | READ |
| T8 | Beyond Kubebuilder - Generating Entire Kubernetes Controller Implementations | Amine Hilaly, Jay Pipes (AWS ACK) | KubeCon EU Amsterdam ~Apr 2023 (uploaded 2022-11-12 per metadata; likely NA Detroit 2022) | https://www.youtube.com/watch?v=rfXk6svglrA | READ |
| T9 | Automated, Distributed Systems Testing for Kubernetes Controllers (Sieve) | Lalith Suresh, Xudong Sun | KubeCon EU Valencia 2022 / uploaded 2021-10-29 (likely KubeCon NA LA 2021) | https://www.youtube.com/watch?v=6JnhjgOaZVk | READ |
| T10 | Stop Reinventing Operator Wheels (controller-idioms) | Evan Cordell (Authzed) | KubeCon NA Detroit 2022 (uploaded 2022-10-20) | https://www.youtube.com/watch?v=yHJR8d9oHFI | READ |
| T11 | Lightning Talk: Oh Sh*t! The Config Changed! | Joel Speed (Pusher) | KubeCon EU Barcelona 2019 | https://www.youtube.com/watch?v=8P7-C44Gjj8 | READ (4 min; ConfigMap-hash rollout, tangential) |
| T12 | In-place Updates with Cluster API: The Sweet Spot Between Immutable... | Fabrizio Pandini, Stefan Büringer | KubeCon ~Apr 2026 (uploaded 2026-04-09) | https://www.youtube.com/watch?v=CMf6rOPo9Z0 | SKIMMED only (grep). CAPI feature design (update extensions); no controller-runtime authoring guidance extracted. Treat as UNREAD for guidance purposes |
| T13 | Kubernetes is Your Platform: Design Patterns For Extensibility | Rafael Fernandez Lopez, Fabrizio Pandini | KubeCon EU Valencia ~May 2022 (uploaded 2022-06-02) | https://www.youtube.com/watch?v=I1-s7AxD1Ls | **UNREAD** - no subtitles available |
| T14 | Tutorial: How To Write a Reconciler Using K8s Controller-Runtime! | (speakers not identified in search output) | KubeCon EU Amsterdam 2023 / uploaded 2022-11-12 | https://www.youtube.com/watch?v=Npvz84HpO3o | **UNREAD** - no subtitles available (1h27m, likely valuable; recommend someone watch/transcribe) |
| - | Priority queue / "writing good controllers" maintainer deep-dive talks | - | - | not found | **NOT FOUND** via yt-dlp search; no talk on controller-runtime priority queue located. Also not found: controller-runtime-maintainer-only intro/deep-dive (Alvaro Aleman search returned only a Cilium/Confluent talk, not read/irrelevant) |

Not read, other candidates seen in search: "Cluster API Intro and Deep Dive" (Kakaraparthi & Prignano, npFO5Fixqcc/9H8flXm_lKk), "Polymorphic Reconcilers... DuckTyping" (kldVg63Utuw, Knative), "Building a Fault-Tolerant Resources Controller" (v2gAcKKDf9Y). UNREAD.

## Guidance (tagged `[Talk @ time]`)

### A. Reduce API server load / client usage
- Never call the live API for every Get/List in a reconcile; use a cache-backed client. controller-runtime's default client reads from informer cache, writes go direct. [T1 @3:50-4:36, T2 @21:28]
- Do not send no-op updates. Compare old vs new copy and skip if identical; prefer Patch over Update (only changed fields; empty patch is a no-op, and avoids "object has been modified" races). Especially for `status`. [T1 @5:22-6:53, @19:46]
- Status-update storms cause thundering herds on startup (e.g. Gateway status rewritten hundreds of times because each attached route changed). [T1 @19:46-20:32]
- Write each object at most once per reconcile: read everything up front, compute, then one deferred patch of the reconciled object at the end of Reconcile (CAPI does `defer patch`, applies on both success and error). Minimize writes to other objects. [T2 @28:22 (transcript [39])]
- Read needed objects once at the top of reconcile and pass them down rather than re-reading in several places. [T2 @27:35]
- Cache gotcha: typed objects are cached by default; **unstructured is NOT cached by default** and must be configured (in CAPI this was the single biggest perf win). Cost: memory (2-4 GB at 2k clusters). Run scale tests and check traces to confirm reads hit the cache. [T2 @22:15-23:45, @27:35]
- Stale cache is usually acceptable because a later event for the stale object triggers another reconcile that converges. [T2 @23:45]
- Client-side rate limiting (client-go QPS/burst; CAPI default 20/30) produces non-deterministic latency noise at scale; CAPI raised to 100/200 for a management cluster where it is the main client. Remove this noise before profiling. [T2 @16:05-17:39]
- Sharding/leader election: a standard controller is active/passive; scale-out needs sharding. Pattern: per-instance Lease, label objects with shard via mutating webhook, each instance sets a label-selector on the manager cache (`cache.Options`), and a "drain" handoff so only one instance reconciles an object at a time; capacity scaled ~linearly 1-5 instances. Author states: not recommended for production yet. [T5 @8:33-22:24, @35:00]

### B. Watches, predicates, what to reconcile
- Reconcile the right (top-level) object: for composed systems (Gateway API) reconcile the parent (Gateway), not the leaf (HTTPRoute); reconciling the child caused last-writer-wins config loss and doubled reconciles. [T1 @17:27-19:46]
- Predicates on `For()` are NOT applied to other `Watches()`/`Owns()`. Repeat the relevance logic in the mapping function / predicate of every secondary watch, otherwise you enqueue and then discard many reconciles. [T1 @15:57-17:27, @20:32]
- Recalculating the whole object per reconcile is cheap when reads come from the local cache; do not track before/after diffs or tombstones. "Rebuild the state of the world every time you get a request." [T1 @15:11, @22:02-22:47]
- Use watches instead of `RequeueAfter` polling (e.g. every 10s waiting for infra to be ready): polling at scale keeps the queue busy; a watch lets the system idle. [T2 @25:19-26:50, @28:22]
- Return semantics: success = forget; error = requeue with backoff (persistent errors should slow down); RequeueAfter only when no event can signal readiness. [T2 @24:31-25:19]
- Queue latency model: `latency ≈ (objects in queue / concurrency) x reconcile duration`; periodic resync enqueues every object at once, so size for the resync burst. Levers: concurrency (default 10 in CAPI is fine; more workers just moves load to API server), reconcile duration, queue size. [T2 @9:56-12:14, @13:45-14:32]
- Use `MaxConcurrentReconciles` > 1 safely only if reconciles are idempotent and output is consistent; avoid writing no-op outputs. [T1 @23:33-24:19] (inferred from Q&A on "go wide")
- Mapping function (EnqueueRequestsFromMapFunc style) with client access to look up from cache/indexes is the supported way for non-owner relationships (ConfigMap -> Pods); custom Source/EventHandler are for rare cases. [T7 @13:01-15:17]

### C. Reconcile correctness
- Assume your cache is behind: reconcile may not see the object you just created. This is by design. [T6 @4:37-6:56]
- Rely on optimistic concurrency: create with a deterministic name (AlreadyExists -> return, requeue on next event), send `resourceVersion` on updates, send UID/resourceVersion preconditions on deletes. [T6 @7:42-10:45, @13:47]
- Deterministic naming is a prerequisite for the above; random names defeat the safety net. [T6 @13:47]
- Where API-server concurrency isn't enough (stateful/quorum workloads), use "expectations" (as ReplicaSet controller does): record in memory what you just created/deleted and return early until it shows in the cache. [T6 @11:31-13:02]
- Idempotent and "flat": each reconcile step should check/ensure its own resource independently, not nest creation of B inside "if A doesn't exist" (crash between steps or deletion of B is never repaired). Flat does not mean one 2000-line function. [T6 @14:35-17:40]
- Do not compare desired vs live with `reflect.DeepEqual` or field-by-field: defaulting, admission webhooks and other controllers mutate the live object. Either hash the desired spec at creation time and store hash in an annotation and compare hashes (StatefulSet controller revisions do this), or avoid diffs by regenerating output from a model. Hash downside: you do not retrieve original state/can't Patch a diff. [T6 @17:40-22:17, T1 @29:36-31:07]
- Prefer a user-supplied pod template embedded in your CRD (only add fields user did not set) over exposing ever more knobs. [T6 @22:17-23:48]
- Finalizers: add before taking ownership of external state; omitting finalizers lets a delete event be missed (Sieve found PVC leak in a Cassandra operator: pod deleted with no finalizer while the reconciler was slow, so the reconciler never saw `deletionTimestamp`). [T9 @15:59-22:48, T8 @28:18]
- Level-triggered: reconcilers may miss intermediate states (observability gaps); logic must reach the right final state without having seen every transition. [T9 @15:59-17:29]
- Conditions: if a controller can't make progress, set a condition and (optionally) a pause annotation rather than hot-looping; support an operator-controlled pause for debugging. [T10 @15:17-16:48]
- Pull in non-owned inputs (ConfigMap you reference) by labelling/"adopting" them so a label-filtered cache sees them; avoid watching all ConfigMaps for security/memory reasons. [T10 @12:13-13:44]

### D. Structure and testing
- Model-view style: ingestion (K8s -> domain model) -> model -> translation (model -> K8s/external). Test the two steps separately as pure functions; no mocks needed when output is a K8s object. For external systems keep an in-memory representation of external state (and push desired state out, "reconciled system is source of truth"). [T1 @25:05-31:07]
- Put business logic in pure input->output functions; unit tests on state management are the wrong tests. Fake client is fine for unit tests but doesn't cover SSA etc.; use both unit and e2e. [T4 @22:05-23:37, @31:19-32:04]
- Use envtest (real kube-apiserver + etcd) over hand-rolled fakes: fakes drift into a buggy mini API server; controller-runtime's own tests and kubebuilder scaffolds do this. (Counter-opinion from Istio: envtest was found "subpar"; they use fake clients + kind e2e.) [T7 @25:36-26:21, T4 @31:19-32:04]
- Sieve: fault-injection testing of controllers by instrumenting controller-runtime/client-go; patterns found were observability gaps, time travel, stale reads. Research tool; no source changes needed. [T9 @6:51-11:25, @32:48]
- Metrics: use controller-runtime metrics (reconcile counts/duration, workqueue depth/queue latency); define user-facing metrics (e.g. time to provision) as goals and system metrics for diagnosis; CAPI provides Grafana dashboards for any controller-runtime controller and kube-state-metrics custom-resource-state config to derive metrics from CRDs. Can't tell WHY a reconcile was triggered from controller-runtime requests (only the key is enqueued). [T2 @3:49-6:51, T1 @21:17-22:02]
- Performance method: metrics -> automation (repeatable scale test) -> mock infra (in-memory fake provider) -> iterate small changes; do not optimize without a metric pointing at a bottleneck. Typical wins: cache repeated expensive work (e.g. RSA key generation reused: -75% reconcile time), remove repeated API calls. [T2 @3:04-6:51, @18:24-20:43, @29:07-30:41]

### E. API/CRD design with controller-gen (T3)
- Flow is Go types -> controller-gen -> OpenAPI schema; users see the schema, so after any type change regenerate and read the CRD YAML. API guarantees apply to the OpenAPI schema; Go-package semver applies separately. [T3 @3:04-4:36, @11:33-13:54]
- Don't embed external/third-party API types (kubeadm config, core `v1.ObjectReference`) directly in your CRD; copy, adapt and convert so external bumps don't force a version bump or expose unused fields. Don't share one struct between a singleton spec and its template spec (e.g. IP address in Machine vs MachineTemplate). Reusing ObjectMeta/TypeMeta/metav1.Condition/metav1.Time is fine. [T3 @4:36-10:45]
- Markers matter: set `MaxLength` etc. (needed for CEL cost / ValidatingAdmissionPolicy), mark optional correctly, and use `+listType=map` + `+listMapKey` so multiple writers (controller vs GitOps) co-own list entries instead of fighting over an atomic list (infinite reconcile loop). [T3 @13:54-16:58]
- Use kube-api-linter (KAL; planned home sig-api-machinery) with golangci-lint. [T3 @14:41-15:26]
- Field naming: write the spec as "I want a ..." sentence; keep a glossary; avoid vague names like `ready`; nest related settings (`nodeDrain{gracePeriod,timeout}`) so you can extend; prefer enums to booleans. [T3 @19:15-25:21] (agrees with the user's global rule: no booleans in CRDs)
- Renaming a field is a breaking change; version bumps should be deliberate; storage version migration is hard. [T3 @4:36-5:18, @26:52]

### F. Framework / architecture notes
- Use a framework, not hand-written informers/workqueues: ~65% of sample-controller was boilerplate; controller-runtime's layered design (Builder on top, then Sources/EventHandlers, then raw machinery) is meant to let you break glass to lower levels. The core kube controllers do not use controller-runtime. [T1 @8:23-9:08, T7 @10:00-18:22, @31:53]
- Kubebuilder/controller-gen/controller-runtime give types, CRDs, deepcopy, leader election, metrics, webhooks scaffolding; you still write reconcile logic, webhooks, tests. Code-generate reconciler glue only if you maintain hundreds of controllers (ACK). [T8 @6:05-7:37]
- Alternative frameworks to know of (not controller-runtime): KRT (Istio; collections + dependency tracking), statedb (Cilium agent; Cilium operator uses controller-runtime). John Howard says his KRT prototype is "demo-ware"; no recommendation to adopt. [T1 @9:08-12:10, T4 @22:52-24:23]
- Reusable controller building blocks (`Handler` chain/parallel/branch with typed context, "component"/"adoption"/"static resource" idioms, hash-based change detection) exist as a library (controller-idioms), early stage. [T10]

## Consistency with controller-runtime docs (NOT independently verified in this slice)
I did not re-read controller-runtime/kubebuilder docs here; the notes below are from general knowledge and should be checked by the integrator against docs/godoc.
- Consistent: cache-backed reads; unstructured needing explicit cache config (`client.Options.Cache.Unstructured`); predicates/mappers on watches; Patch over Update; idempotency; envtest recommended.
- Divergent/opinionated: T4 (Istio) finds envtest "subpar" and prefers fake client + kind. T1 says don't track before/after state, consistent with level-triggered docs. T6 "expectations" is from the ReplicaSet controller, not a controller-runtime feature (you must implement it). T5 sharding is third-party, not upstream.
- Version caveats: T6/T7 are 2018-2019 (pre-generics, pre-`Builder.WithOptions`); T7 mentions dependency injection (`inject` pkg) that has since been removed from controller-runtime; verify before citing API details. T2's CAPI "defer patch" refers to CAPI's patch helper, not controller-runtime.
