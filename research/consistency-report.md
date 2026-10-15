# Consistency report: k8s-controller-runtime skill

Independent review per `research/consistency-pass-plan.md`. Reviewed: `k8s-controller-runtime/SKILL.md` and all 14
`references/*.md` (2682 lines total) against controller-runtime v0.25.2 (fresh clone, `/tmp/review-crt`), controller-tools
v0.23.0 and client-go v0.37.0 (module cache), envtest runs, and live fetches where noted. No skill, reference, research or
`.GCC` file was edited; all scratch work is under `/tmp/review-*`.

## Verdict

**SHIP AFTER FIXES.** 0 blockers, 1 major, 5 minors. The skill is structurally sound, cross-consistent, and every
spot-checked factual claim that failed is listed below; nothing else broke under adversarial checking. The one major is a
false claim in a section title (`api-design.md:9`) that a reader would rely on and then be contradicted by their own lint run.

## Findings

| id | file:line | what is wrong | evidence | severity | suggested fix |
|---|---|---|---|---|---|
| F1 | `references/api-design.md:9` | Section title claims the reference types "compile, generate and pass the KAL set below". They do not pass: the shipped types with the shipped config produce 5 KAL findings, exit code 1. | Types from `api-design.md:12-95` extracted verbatim to `/tmp/review-apidesign/api/v1/gadget_types.go`; `.golangci-kal.yml` byte-identical to `codegen-workflow.md:139-172` (python diff: MATCH). `go tool -modfile=tools/go.mod golangci-lint-kube-api-linter run -c .golangci-kal.yml ./api/...` = 5 findings on the shipped types: `gadget_types.go:89 commentstart: godoc for field Gadget.metav1.ObjectMeta should start with 'metadata ...'`; `:91 commentstart: ... Gadget.Spec should start with 'spec ...'`; `:92 requiredfields: field Gadget.Spec has a valid zero value ({"tls": {}}), but the validation is not complete ... should be a pointer`; `:92 requiredfields: field Gadget.Spec should have the omitempty tag.`; `:93 commentstart: ... Gadget.Status should start with 'status ...'`. For contrast, the experiment sample (`research/experiments/exp-kal-config`, same linter set) reports "0 issues." - its `Widget` struct gives each embedded field a prose doc comment (`widget_types.go:46 // metadata is the standard object metadata.`), which the api-design excerpt omits. | **major** | Either fix the types so the title is true (add `metadata ...`/`spec ...`/`status ...` doc comments on `Gadget`'s fields, and make `Spec` a pointer with a deviation note or complete its validation - the pointer case is already allowed by the skill's own rules), or weaken the title to "compile and generate" and state the findings the set produces on this excerpt. |
| F2 | `references/metrics.md:193-208` (excerpt), `:215` | The `WidgetCollector` excerpt shows only `Collect`; `Describe` appears nowhere in the file, but `prometheus.Collector` requires both, so `metrics.Registry.MustRegister(collector)` (`:215`) and `prometheus.CollectAndCompare` in the test section do not compile against what is shown. | `grep -n "Describe" references/metrics.md` = no matches. In the compile harness (`/tmp/review-snip/snippets/metrics`) the excerpt failed to compile until a `Describe` method was added to the collector. | minor | Add one line to the excerpt: `func (c *WidgetCollector) Describe(ch chan<- *prometheus.Desc) { ch <- c.desc }`. |
| F3 | `references/reconcile.md:225`, `references/observability-rbac.md:14` | The events.k8s.io recorder is attributed to v0.25 ("`- v0.25: rec := mgr.GetEventRecorder(...)`"; "v0.25 adds the events.k8s.io recorder"). It was added in v0.23.0; v0.25.0 only widened the return type. Contradicts `references/versions.md` (changelog rows: v0.23.0 "New events API: `mgr.GetEventRecorder(name)` writes `events.k8s.io` Events; `GetEventRecorderFor` ... is `Deprecated`"; v0.25.0 "`GetEventRecorder` now returns `recorder.EventRecorder` (adds `AnnotatedEventf`)"). | Code: `GetEventRecorder` exists at tag v0.23.3 (`pkg/cluster/internal.go`) and is absent at v0.22.1 (tag clones `/tmp/review-cr-v0233`, `/tmp/review-cr-v0221`); the `Deprecated` marker is on `GetEventRecorderFor` in `pkg/recorder/recorder.go:39` ("uses the old events API ... use GetEventRecorder instead"). The code supports versions.md; reconcile.md and observability-rbac.md carry the wrong version. | minor | Change both to v0.23 (with a note that v0.25.0 widened the return type to `recorder.EventRecorder`), matching versions.md. The `observability-rbac.md:17` field type `events.EventRecorder` still compiles at v0.25.2 (`recorder.EventRecorder` embeds it). |
| F4 | `references/observability-rbac.md:151` | "Missing `SetLogger`: all logs silently dropped after 30 s" keeps the imprecise wording the drafting agents corrected elsewhere: logs are dropped from the start; only the stderr notice waits 30 s. | `references/logging.md` (Hazards): "No `SetLogger`: every line dropped, and the notice only appears on a log call after 30 s. [CR, run]"; `references/reconcile.md` (Hazards): "a stderr notice after 30s". Plan leftover #1 names this exact wording as imprecise. | minor | Align with logging.md's wording ("every line dropped; the notice appears only on a log call after 30 s"). |
| F5 | `references/cache-and-memory.md` (360 lines), `references/metrics.md` (270 lines) | Both exceed the ~250-line target; the plan's leftover #2 asked for trimming without losing a hazard, and no trimming happened. | `wc -l`: cache-and-memory.md 360, metrics.md 270; next largest reference is 240. | minor | Trim (cache-and-memory sections 4-5 and metrics section 6 are the longest), or record the exception. Content itself is verified; this is length only. |
| F6 | `references/api-design.md:170`, `references/status-conditions.md:116`, `references/observability-rbac.md:34` | Three claims carry positive evidence tags while `research/UNVERIFIED.md` still lists the same facts as unverified: (a) `api-design.md:170` tags `oldSelf` under "an atomic **or set** list" `[K8S, run]` but the ledger (line 23) lists "`listType=set` under an `oldSelf` rule" unverified; (b) `status-conditions.md:116` "Apply needs `patch` RBAC (on the `/status` subresource ...)` [K8S, CT]" vs ledger line 24; (c) `observability-rbac.md:34` "In the run with only that rule, the event was stored [CR, run]" vs ledger line 31 ("verbs `create;patch` (inferred from the client-go sink)"). All three were re-verified TRUE during this review, so there is no factual defect - the tags overstate what had been run, or the ledger is stale. Also stale: ledger line 14 says "no procedure is given for testing [RBAC under envtest]", but `observability-rbac.md:97-99` gives the impersonation procedure and says the section 5/6 runs used it. | (a) envtest run `/tmp/review-oldself` (`elem_test.go`): CRD with a transition rule inside elements rejected for both `listType=atomic` and `listType=set` with the exact quoted message ("oldSelf cannot be used on the uncorrelatable portion of the schema"), accepted for `listType=map`; (b) `pkg/client/typed_client.go` `ApplySubResource` sends the request with `.SubResource(subResource)`, i.e. a PATCH to `<resource>/status`; (c) client-go v0.37.0 `tools/events/event_broadcaster.go`: `Create` at :88, `Patch` at :104, series aggregation calls `sink.Patch` at :240 - both verbs are used. | minor | Move the three ledger entries to UNVERIFIED.md's resolved list (they now have evidence), and update the envtest-RBAC line. If the intent was that the drafting runs never covered them, downgrade the shipped tags instead. |

No blockers. No other check produced a finding: every hazard in SKILL.md maps to a reference hazard with the same reason,
every checklist item maps to a reference rule, every tag is defined, every section cross-reference resolves, and all
compiled/run claims reproduced.

## Checks

- **A. Structure - PASS** (with F5). SKILL.md 135 lines (<300); `name` equals folder name; description present and
  trigger-rich; all 14 references appear in the "Which reference when" table and none is orphaned; all 14 open with
  "Read this when" (grep: 14/14); file sizes sane except F5.
- **B. Forbidden content - PASS.** Greps for kubebuilder CLI/PROJECT/`+kubebuilder:scaffold`/kustomize/setup-envtest,
  personas, `.GCC`, `SecureServing: false` as advice, `nil, nil` as advice, `Requeue: true`, `CustomValidator/CustomDefaulter`
  as recommended: all hits are negative context (e.g. `KUBEBUILDER_ASSETS` appears only inside a hazard saying it is
  ignored; `CustomDefaulter` only as the deprecated pre-v0.23 API). The `+kubebuilder:` marker prefix is used, as allowed.
- **C. Cross-file consistency - PASS WITH FINDINGS (F3, F4).** All 27 SKILL.md hazard bullets map to a reference hazard
  with the same reason (cache 8/8, reconcile 5/5 incl. the finalizer/`GenerationChangedPredicate` bullet split across
  cache-and-memory + reconcile, api/codegen 5/5, operations 5/5, testing 4/4). Shared topics agree: pointers/omitzero,
  defaults (marker vs webhook), KAL set (11+7+2 identical in api-design.md:156, codegen-workflow.md:136-137 prose and the
  yaml), Requeue, finalizers and generation-bump rules, `GenerationChangedPredicate`, transform composition/DefaultTransform,
  patch vs update vs SSA, status writes, events RBAC (core for the legacy leader-election recorder, events.k8s.io for
  `GetEventRecorder` - consistent, different recorders), leader-election RBAC, metrics auth, envtest limits, deviation-note
  placement. Contradictions found: F3 (version attribution) and F4 (wording). review-checklist.md: every item in all six
  sections maps to a rule or hazard in its named references. Severe reference hazards not carried in SKILL.md: the
  strongest candidate is reconcile.md's "Predicates on `For` do not cover `Owns`/`Watches`" - acceptable under SKILL.md's
  stated "cross-cutting; each reference lists the rest" split, but it is the one silent-event-loss bug a newcomer is most
  likely to hit; consider promoting it.
- **D. Tier discipline - PASS WITH FINDING (F6).** Advice vs hazards is cleanly separated (hazards sections carry reasons;
  metrics/envtest/tool-directive user mandates are marked as such). No unexplained must/always/never outside hazard bullets
  and user-mandated items. Every base tag used in shipped files (CR, API, K8S, KB, CT, KAL, CAPI, CAPV, TALK, BLOG, OCP, GW,
  PROM, KSM, OSS) is defined in sources.md; suffix forms (`[CR FAQ]`, `[TALK T4 @25:36]`) are declared acceptable at
  sources.md:4; `run`/`[unverified]` are defined at sources.md:5 and SKILL.md:16. Bracket-like false positives decoded:
  `drop: [ALL]` (YAML), `TypedReconciler[Request]`, `admission.Validator[T]` (Go generics), `T1`-`T14` (talk numbers,
  defined in sources.md's table). Deviation: F6 (three positive tags on ledger-unverified items).
- **E. Code - PASS WITH FINDINGS (F1, F2).** 39 fenced Go blocks extracted. Compile harness `/tmp/review-snip`
  (module example.com/demo; api/v1 built from manager-setup.md's groupversion_info + widget types; SetupWithManager made
  variadic to satisfy both files' call forms): `go build -buildvcs=false ./...` succeeds for all snippet packages;
  marker-only fragments were exercised through controller-gen instead of compilation. `research/experiments/cg-verify`
  copy builds. **exp-pruned-cache: 2 tests pass** (TestDisableForAndIndexes, TestPrunedCache). **exp-observability: 3
  suites pass across 9 packages** (TestLog, TestObs, TestRBAC; envtest downloaded its own binaries). controller-gen
  object+crd on the api-design.md snippet succeeds and the CRD verifies the file's claims (default `Fast`,
  `minProperties` on tls/status, `listType: map` on ports/conditions, status subresource, Ready printer column, no
  deviation-note leakage into descriptions). Hazards reproduced: glued RBAC markers generate nothing while separated ones
  generate the rule (`/tmp/review-rbac`); misspelled `MaxLenth` marker = controller-gen exit 0 and no `maxLength` in the
  CRD (`/tmp/review-misspell`); `oldSelf` under atomic/set rejected at CRD creation with the quoted message, map accepted
  (`/tmp/review-oldself`, this session). KAL on the shipped types: F1.
- **F. Claim spot-check - PASS.** 8+ claims per reference verified against v0.25.2 source, tag clones, or runs; all
  verified except those already filed (F1, F2, F3). Highlights: default rate limiter 5ms=1000s and `UsePriorityQueue`
  default true (controller.go:271-276); `MaxConcurrentReconciles` 1 (:263); `CacheSyncTimeout` 2m (:267); the
  RequeueAfter-with-error warning text (:495); TerminalError counted once (:482-492); `DisableFor` GVK-keyed
  (client.go:255-260); `EnableReadYourWritesConsistency` path/type and the >1-namespace caveat (:111-115); webhook builder
  "only one of Defaulter or CustomDefaulter can be set" (webhook.go:235); metrics filter TTLs (filters.go:64,83-84);
  logr↔slog bridge (slogr.go:31,59); `klog.SetLoggerWithOptions(ContextualLogger(true))` (contextual.go:72,86); `Requeue`
  Deprecated since v0.21.0; `GetEventRecorder` v0.23.3-present/v0.22.1-absent; fake client `WithInterceptorFuncs` + `Apply`;
  `builder.WithPredicates` takes `predicate.Predicate` (options.go:48); apiserver `markAsDeleting` bumps generation
  (store.go:1083-1096); default sync period 10h (cache.go:45); `scheme.Builder` deprecated (scheme.go:64);
  `TransformStripManagedFields` as DefaultTransform (cache.go:412-414); KAL key `lintersConfig`; controller-tools v0.23.0
  `optionalOldSelf` marker exists (validation.go:739); client-go events sink uses create+patch; kube-state-metrics README
  (live fetch) states "custom-resource-state is feature-frozen in favor of resource-state-metrics" as metrics.md:183 quotes.
- **G. Completeness - PASS.** A newcomer gets: layout and main.go (manager-setup), types+markers (api-design), generation
  and lint commands (codegen-workflow), reconciler (reconcile), RBAC (observability-rbac), webhooks (webhooks), authenticated
  metrics (metrics), logging (logging), envtest tests (testing-envtest), image (manager-setup Dockerfile) and packaging
  pointer (SKILL.md:135). Webhook certificate management and CRD versioning are pointer-only by declared scope, matching
  user decision 10; nothing else an agent would plausibly get wrong was found uncovered.
- **H. Usability - PASS.** The description triggers on controller-runtime/operator/reconciler/CRD/controller-gen/envtest/
  finalizer/informer/cache-tuning/kube-api-linter and excludes unrelated Go work; the table sends each task to 1-2 files;
  each reference is loadable alone (self-contained "Read this when", local `[run]` definition, pointer to sources.md);
  all "section N" cross-references resolve (SKILL.md:28 = cache-and-memory Section 8; cache-and-memory:287 = Section 4's
  `predicate.TypedFuncs` example at :210-224; metrics.md:18 = Section 1.3; observability-rbac:163 = Section 5; logging's Section 3/Section 4). Over-long
  files: F5. One wording risk examined and accepted: SKILL.md:68 "Default backend is klog" sits in a section headed
  "(all advice)" and its own parenthetical includes `ctrl.SetLogger(klog.Background())`; hazard :124 states the library
  default (no SetLogger = logs dropped), so the two do not contradict.
- **I. Unverified ledger - PASS WITH FINDING (F6).** 16 `[unverified]` markers in shipped files; every UNVERIFIED.md
  "Still unverified" item either carries a marker, uses honest "not run/not tried" wording, or is not asserted. Three
  exceptions stated as verified fact are F6 (all three re-verified true here). Additional ledger staleness: line 14
  (envtest RBAC procedure now exists), and the "Resolved during drafting" list should gain the three F6 items.

## Known leftovers from the plan (verified, not assumed)

1. Logging bullets duplicated across reconcile/observability-rbac/logging: facts agree; wording defect F4 only.
   `reconcile.md:105`'s `log.V(1).Info("Reconciled Widget", ...)` is compatible with logging.md Section 6 (V(1) debug, hidden at
   the default `-v=0`); no contradiction.
2. Lengths: F5.
3. Version corrections: versions.md is right and the code supports it; two files still carry the old wrong version - F3.
4. Tag suffixes: all base tags defined; suffixes declared acceptable in sources.md:4. No action.
5. Typed predicates: cache-and-memory.md:285-287 carries the note and points to a real typed example (:210-224). No other
   file recommends typed predicate funcs without it. No action.
6. Generation-bump claims (deletionTimestamp set bumps generation; finalizer add does not): consistent across SKILL.md:96-97,
   reconcile.md, cache-and-memory.md, and verified in apiserver code (store.go:1083-1096). No action.
7. KAL set 11+7+2 with `lintersConfig`: identical in both files and the yaml. No action.
8. klog default: unchanged, per instruction. No action.
9. rtk prefixes: no effect on shipped content. No action.

## What I could not verify

- Claims tagged `[CAPI]`, `[CAPV]`, `[GW]`, `[OCP]`, `[API]`, `[K8S]` (docs) that are not backed by `[CR]` code or a `[run]`
  reproduction were not re-fetched; verifying them would require cloning those repos at the pinned commits listed in
  sources.md. Where such a claim collided with code, the code won (F3).
- The `[BLOG]` source URL was not re-fetched; its two corrected claims are only checked for internal consistency.
- `[TALK]` claims rest on transcripts I did not re-obtain; T13/T14 are marked unread in sources.md and carry no claims.
- Webhook certificate hot-reload, real-cluster enforcement of `/finalizers` updates with `blockOwnerDeletion`, and
  read-your-writes behaviour after the v0.25.2 tag: remain unverified, and the shipped files mark them so
  (`webhooks.md:107-109` "reload not exercised", `reconcile.md:180` `[unverified]`, `cache-and-memory.md:327` `[unverified]`).
- The exact first v0.23.x patch containing `GetEventRecorder` (checked at v0.23.3 only; v0.23.0-v0.23.2 not bisected) -
  versions.md:76-81 marks this honestly.
- `research/notes/*` were treated as unverified background per the brief; shipped files won where they differed.
