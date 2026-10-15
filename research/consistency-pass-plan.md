# Consistency pass: plan for an independent reviewer

You are reviewing a drafted agent skill. You were NOT part of drafting it. Do not trust the drafting agents' claims or any
summary of them; trust only what you read in the files and what you can reproduce. Your output is a REPORT; do not edit the
skill files (the owner decides fixes). Write the report to `research/consistency-report.md`.

## What is being reviewed
A tool-neutral skill (Claude, Codex, others) for building Kubernetes controllers with PURE controller-runtime + controller-gen
(no kubebuilder). Target: controller-runtime v0.25.2, controller-tools v0.23.0, Go 1.26+, k8s.io/* v0.37.
Root: `k8s-controller-runtime-skill/`
- `k8s-controller-runtime/SKILL.md` and `k8s-controller-runtime/references/*.md` (SHIPPED; this is what you review)
- `research/notes/*.md` (evidence trail; may contain claims later refuted: the shipped files win where they differ and are verified)
- `research/experiments/*` (runnable fixtures behind the `run` tags)
- `.GCC/branches/k8s-controller-runtime-skill/plan.md` (the binding design; read it first)
Source for checking claims (re-create if /tmp is gone): controller-runtime tag v0.25.2 (`git clone --depth 1 --branch v0.25.2
https://github.com/kubernetes-sigs/controller-runtime`), client-go v0.37.0 from the Go module cache.

## Binding user decisions (flag any file that contradicts one)
1. Everything is ADVICE ("prefer"/"avoid"); a deviation needs a documented reason. Only a short HAZARDS list is non-negotiable,
   each with a reason. Exceptions the user made mandatory: metrics bootstrap MUST be authenticated (SecureServing + filter);
   envtest uses `envtest.Environment{DownloadBinaryAssets: true}` (no setup-envtest, no Makefile download); tools via the go.mod `tool` directive.
2. Booleans: "think twice" (not forbidden). Avoid pointers: non-pointer optional + `+optional` + `omitzero` (KAL `optionalfields` `WhenRequired`);
   pointers only for numerics with `Minimum=0` and optional structs, each with a deviation note.
3. Defaulting by markers (justified). Patching is taught as a PATTERN; no shipped helper library.
4. Deviation notes sit in their own comment group ABOVE the doc comment, separated by a blank line (so they do not leak into CRD descriptions).
5. Upstream API conventions are the default; OpenShift variants only as documented alternatives.
6. Cache: strip managedFields by default; prune to fields actually read; filter with selectors/namespaces never `nil,nil`;
   heavy types: selectors -> metadata-only -> DisableFor+APIReader last; re-verify the cache footprint whenever a reconciler's reads change.
7. Never write back (`Update`) an object read from a pruned cache (verified hazard). Use `Patch` or read via APIReader first.
8. envtest cannot prove GC, namespace deletion, Pods/Deployments, default SA: tests assert ownerReference/finalizer fields, unique namespace per test.
9. Logging: controller code uses controller-runtime's logr; simplest setup wins (slog behind logr only if clean). Capitalised message, no trailing period, KV pairs.
10. Out of scope (one-line pointers only): OLM, Helm, multi-cluster, conversion webhooks, multi-version CRDs.
11. Shipped files contain NO personas, no `.GCC`, no Claude-only features (tool names, subagents).

## Checks (do all; record every finding)
A. Structure. SKILL.md < 300 lines; frontmatter has `name` equal to the folder name and a `description`; every reference in SKILL.md's
   "which file when" table exists and no reference is orphaned; every reference opens with a "Read this when" line; file sizes sane.
B. Forbidden content. grep the shipped files for: `kubebuilder init|create|PROJECT|+kubebuilder:scaffold|kustomize|setup-envtest|KUBEBUILDER_ASSETS`
   (marker prefix `+kubebuilder:` is allowed), `@architect|@engineer|@qa|@security|\.GCC`, `SecureServing: *false`/plain-HTTP metrics shown as recommended,
   `nil, nil` shown inside a transform as a recommended pattern, `Requeue: true`, `CustomValidator|CustomDefaulter` as recommended.
C. Cross-file consistency. Build a list of every topic that appears in 2+ files (pointers/omitzero, defaults, KAL linter set,
   Requeue/RequeueAfter, finalizers, GenerationChangedPredicate, Transform composition, DefaultTransform, patch vs update vs SSA, status writes,
   events, leader election RBAC, logging, metrics auth, envtest limits, deviation-note placement). For each, confirm the files agree or one points to the other.
   Report contradictions with file:line on both sides. SKILL.md lists only the CROSS-CUTTING hazards (each reference ends with its own Hazards section):
   confirm each hazard in SKILL.md appears in a reference's Hazards section with the same reason and tag, none contradict a reference, and flag any severe
   hazard in a reference that SKILL.md should carry. Confirm every item in `references/review-checklist.md` maps to a rule in a reference.
D. Tier discipline. Every rule is advice or hazard; each hazard has a reason; no unexplained "must/always/never" outside the user-mandated items;
   every factual rule has a source tag from the tag set in plan.md and every tag is defined in `references/sources.md`; any unverified claim is marked `[unverified]`.
E. Code. Extract the fenced Go/YAML blocks. In a scratch copy of `research/experiments/cg-verify` (run `go build -buildvcs=false`), compile a sample of
   at least 5 Go snippets per file that contains them and run controller-gen / the KAL config on the API snippets. Note imports omitted from excerpts are expected;
   fail only if the code is wrong. Re-run the tests of `exp-pruned-cache` and `exp-observability` (envtest downloads its own binaries; network needed).
F. Claim spot-check. For each reference choose 8 factual claims (prefer defaults, version facts, behaviours, field/function names) and verify them against the
   v0.25.2 source or by a tiny run. Prioritise: Result/Requeue semantics, rate limiter defaults, MaxConcurrentReconciles/CacheSyncTimeout defaults, DisableFor GVK keying,
   EnableReadYourWritesConsistency path/type, webhook builder API, metrics filter RBAC verbs, applyconfiguration markers, KAL config keys, logging adapter function names.
G. Completeness vs scope. Does a newcomer building a controller from an empty directory get: layout, types+markers, generation commands, reconciler, main.go/manager,
   RBAC, webhooks, metrics (authenticated), logging, tests (envtest), image/packaging pointers? Is anything an agent would plausibly get wrong left uncovered?
H. Usability for an agent. Is SKILL.md short and does the table send each task to 1-2 files? Would loading one reference alone make sense (no hidden dependency)?
   Does the `description` trigger on controller/operator/reconciler/CRD/controller-gen/envtest tasks and NOT on unrelated Go work? Flag over-long files.
I. Unverified ledger. Compare `research/UNVERIFIED.md` with the `[unverified]` markers in shipped files; list anything unverified that is stated as fact.

## Report format
`research/consistency-report.md`: verdict (ship / ship after fixes / do not ship); a table of findings
(id | file:line | what is wrong | evidence (command/output or quote) | severity blocker/major/minor | suggested fix); then the list of
checks A-I with pass/fail and one line each; then anything you could not verify and why. Do not soften findings; do not pad with praise.

## Known leftovers to examine (reported by the drafting agents; verify, do not assume they are fixed)
1. `reconcile.md` and `observability-rbac.md` still carry logging hazard bullets (no `SetLogger`; logging or eventing a Secret value) and `reconcile.md`'s skeleton
   logs with `log.V(1).Info("Reconciled Widget", ...)`: decide keep vs pointer to `logging.md`, and check they agree with `logging.md` section 6 (which advises against
   per-reconcile Info noise). Their earlier "logs dropped after 30 s" wording is imprecise: per `logging.md`, only a log call after 30 s prints the notice.
2. `cache-and-memory.md` is 360 lines (target about 250) and `metrics.md` 270: check for trimming without losing a hazard.
3. `versions.md` contradicts parts of the original brief; the reviewer should trust the code: `GetEventRecorder` exists since v0.23.x (not v0.25);
   `Result.Requeue` was deprecated in v0.21.0 (not v0.23); the fake client has `WithInterceptorFuncs` and implements `Apply` (but not `listType=map` for CRD types).
   Check every other file for the old, wrong versions.
4. Source tags use free-text suffixes (`[CR FAQ]`, `[CAPI environment.go]`, `[TALK T4, T1]`, `[run, 09 check 3]`). Confirm each base tag is defined in `sources.md`
   and decide whether the suffixes are acceptable; two tags added late (`KSM`, `OSS`) are defined in `sources.md` and were added to plan.md's tag list.
5. `builder.WithPredicates` accepts only `client.Object` predicates; typed predicates work only through `source.Kind` or `WatchesRawSource`. Check that every
   file that recommends typed predicate funcs says this (`cache-and-memory.md` does).
6. The claim "`GenerationChangedPredicate` does not starve deletion (the API server bumps generation when deletionTimestamp is set)" and the
   finalizer-add exception both appear in different files: confirm they are consistent.
7. The linter set is 11 + 7 + 2 (not "11 + 8"): `forbiddenmarkers` is excluded because defaults are allowed and an empty `forbiddenmarkers` is a config error. Check that
   `api-design.md` and `codegen-workflow.md` give the same list and the same `.golangci-kal.yml` keys (`lintersConfig`, not `linterConfig`).
8. The logging default (klog backend through logr) follows the user's rule "simplest wins, controller-runtime's logr is the interface"; slog is documented as
   workable but not the default. Do not change this without the owner.
9. Drafting agents reported that a few early commands ran without the `rtk` prefix; this has no effect on the shipped content.
