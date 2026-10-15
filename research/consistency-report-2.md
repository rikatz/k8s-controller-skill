# Independent consistency review, round 2

Date: 2026-10-15. Reviewer: independent (claude). Scope: `k8s-controller-runtime/SKILL.md` and all 14 `references/*.md`,
against the binding brief `research/consistency-pass-plan.md` and the design `.GCC/branches/k8s-controller-runtime-skill/plan.md`.
This round was produced without reading `research/consistency-report.md` or `research/consistency-resolutions.md`; every finding
below was re-derived from the current files and fresh runs. Scratch work: `/tmp/review2-ad`, `/tmp/review-snip` (extended),
`/tmp/review-crt` (tag check). No skill, reference, research or `.GCC` file was edited.

Files changed since 17:46 (after the previous review's report at 16:55): `api-design.md`, `reconcile.md`,
`observability-rbac.md` (17:46-17:47), `research/UNVERIFIED.md` (17:49), `metrics.md` (17:56). All other shipped files are
byte-unchanged since before that report (mtimes 15:46-16:11), so verifications of them from the prior round's scratch
directories remain evidence for their current bytes; everything in the changed files was re-verified from scratch this round.

## Verdict

**SHIP AFTER FIXES.** 0 blockers, 0 majors, 6 minors. Nothing states a technical falsehood that would break a user's setup.
The one finding that should be fixed before shipping is R2-1: a `[run]`-tagged claim whose cited reproduction does not exist
in `research/experiments` - the tag system is the skill's epistemic contract, and this citation dangles.

Key positive verifications this round (all reproduced, not trusted):

- The `api-design.md` reference types now genuinely pass: `go build ./...` OK; `go tool controller-gen crd` (v0.23.0) emits
  every property the file claims (`default: Fast`, `minProperties: 1` on `spec`/`tls`/`status`, `x-kubernetes-list-type: map`,
  `required: [spec]`, `status: {}` subresource, Ready printer column, no deviation-note leak into the `replicas` description);
  kube-api-linter with the shipped `.golangci-kal.yml` reports "0 issues."; and an envtest API server (1.37.0 binaries) enforces
  them: empty-spec create rejected (`spec: Required value`), `mode` defaulted to `Fast`, `storageClass` change rejected
  (`storageClass is immutable`), status subresource writable.
- `metrics.md`'s collector now shows `Describe`; `prometheus.NewPedanticRegistry().MustRegister(collector)` compiles and
  `testutil.CollectAndCompare` passes against a fake client (`--- PASS: TestCollectorWithDescribe`).
- The events-API attribution "v0.23.0 added `GetEventRecorder` (v0.25.0 only widened its return type)" is now code-verified at
  the v0.23.0 tag itself: `pkg/manager/internal.go:267` `func (cm *controllerManager) GetEventRecorder(name string)
  events.EventRecorder`, `pkg/recorder/recorder.go:35` (interface), with `GetEventRecorderFor` already `Deprecated` at
  `recorder.go:32`.

## Findings

| id | file:line | what is wrong | evidence | severity | suggested fix |
|---|---|---|---|---|---|
| R2-1 | `references/api-design.md:109-111` | The claim "These types are verbatim: they compile, generate, and pass the KAL minimum set with 0 issues (`research/experiments`)" is tagged `[KAL, CT, run]`, but no Gadget types exist anywhere in `research/experiments`. Per `SKILL.md:16` and `sources.md:5`, `run` means "reproduced in `research/experiments/`". The underlying claim is true (verified this round in `/tmp/review2-ad`: build OK, CRD properties all present, KAL 0 issues, envtest enforcement), but the cited reproduction is not there. | `grep -rln "Gadget" research/experiments --include="*.go"` = no output; experiments tree last modified 16:05:54, before the 17:46:50 edit; the codebase inventory lists no Gadget types in any experiment | minor | Land the fixed snippet as an experiment (e.g. replace the types in `exp-kal-config/api/v1`) or drop the parenthetical and re-tag the claim to what was actually done |
| R2-2 | `references/reconcile.md:239`, `references/manager-setup.md:171` | SetLogger-notice timing still imprecise: both imply the notice appears at 30 s unconditionally. The authority (`logging.md:36-37`, `[CR, run]`) and the already-fixed `observability-rbac.md:153` state it prints only on a log call made after 30 s, so a silent controller never shows it. Brief leftover 1 asked for exactly this agreement across files. | reconcile.md:239 "(a stderr notice after 30s)"; manager-setup.md:171 "(after 30s a \"log.SetLogger(...) was never called\" notice is printed)"; vs logging.md:36-37 "the stderr notice ... appears only when a log call happens after 30 s" and observability-rbac.md:153 "the notice only prints on a log call made after 30 s" | minor | Align both bullets with logging.md's wording (as observability-rbac.md now does) |
| R2-3 | `research/UNVERIFIED.md:14` | Stale ledger line: "RBAC under envtest (the test client is `system:masters`); no procedure is given for testing it." A procedure is given and was run: bind the generated Role to a user and run the manager with `rest.Config.Impersonate`. | `observability-rbac.md:97-99`: "Test it: in envtest, bind the generated Role to a user and run the manager with `rest.Config.Impersonate` set to that user; a missing verb then fails the test (the leader-election and metrics runs above used this). [run]" | minor | Reword the ledger line to the narrower true statement (the suite's own test client is `system:masters`; manager RBAC is testable via impersonation, see observability-rbac.md section 5) |
| R2-4 | `references/observability-rbac.md:32` (ledger: `research/UNVERIFIED.md`) | A shipped `[unverified]` item with no ledger entry, so the re-verification loop misses it: "Register the object's type in the scheme used by the manager so the event can reference it [unverified here; the run used a ConfigMap]." | `grep -n "scheme\|reference it\|ConfigMap" research/UNVERIFIED.md` = no match; the ledger's Reconcile/webhooks/events and Operations sections do not list it | minor | Add the item to the ledger's events section |
| R2-5 | `k8s-controller-runtime/SKILL.md:18` | "Each one opens with a 'Read this when' line and ends with its own Hazards section" is false for 3 of the 14 references: `versions.md` ends with "How to update this file", `sources.md` with the talks table, `review-checklist.md` with the Tests checklist. (All 14 do open with "Read this when".) | versions.md:83-90, sources.md:31-52, review-checklist.md:55-60 - no Hazards section in any of the three | minor | Scope the sentence to the topical references ("the topic references end with their own Hazards section; `versions.md`, `sources.md` and `review-checklist.md` are meta files") |
| R2-6 | `references/cache-and-memory.md` (360 lines), `references/metrics.md` (273 lines) | Still over the ~250-line target; `metrics.md` grew from 270 to 273 lines with the `Describe` fix. No hazard was lost; this is the outstanding trimming leftover. | `wc -l`: 360 / 273; brief leftover 2 ("check for trimming without losing a hazard") | minor | Trim when convenient; candidates are repeated cache-hazard prose and metrics section 5/6 overlap; do not drop a hazard |

## Checks A-I

- **A (structure, lengths, formatting): FAIL (minor)** - `SKILL.md` + exactly 14 references, no extra files, every reference
  opens with "Read this when"; lengths over target (R2-6) and the SKILL.md self-description is wrong for 3 meta files (R2-5).
- **B (forbidden content): PASS** - no kubebuilder CLI scaffolding, operator-sdk workflows, `make manifests`/kustomize flows.
  Only hits are the explicitly disclaimed background list (`sources.md:27`) and the out-of-scope pointer (`SKILL.md:134`).
- **C (cross-file consistency): FAIL (minor)** - events-API attribution now consistent everywhere and code-verified at v0.23.0;
  KAL set 11+7+2 identical in `api-design.md:165` and `codegen-workflow.md:140-203` (and the shipped config passes the new
  snippet); `api-design.md:128` agrees with `codegen-workflow.md:215` on `MinProperties=1` for optional structs. The
  SetLogger-notice wording disagrees across files (R2-2). Examined and accepted: `json:"spec,omitempty,omitzero"` on the root
  wrapper (compiles, KAL-clean, CRD carries `required: [spec]`; `+required`+`omitempty` is explained at api-design.md:110-111,
  the `omitzero` half is not but is harmless) and the commentless embedded `TypeMeta` (KAL passes; it has no JSON name).
- **D (tier/tag discipline): PASS** - every base tag used in the changed text (`KAL`, `CT`, `run`, `API`, `K8S`) is defined in
  `sources.md`; suffix convention documented at `sources.md:4`; advice/hazard separation maintained. Exception tracked as R2-1
  (a `run` tag whose reproduction is not in the experiments).
- **E (code verification of pinned versions): PASS** - new snippet builds against controller-runtime v0.25.2 /
  controller-gen v0.23.0 / Go toolchain 1.27.2; regenerated CRD byte-identical across two runs; KAL "0 issues.";
  `GetEventRecorder` confirmed in v0.23.0 source; envtest 1.37.0 binaries enforce the schema claims (outputs quoted above).
- **F (at least 8 checkable claims per reference): PASS** - 15 claims verified for `api-design.md` this round (compile; KAL
  clean; 8 CRD-content properties; 4 API-server-enforced behaviours); the `metrics.md` collector pair (Describe+MustRegister
  compiles; CollectAndCompare passes); `reconcile.md`/`observability-rbac.md` changed lines verified. Unchanged references
  carry the prior round's runs against identical bytes.
- **G (completeness, plan leftovers): FAIL (minor)** - leftovers 3, 4, 5, 6, 7, 8, 9 verified addressed; leftover 1 only
  partially (R2-2: one of three imprecise wordings was fixed); leftover 2 open (R2-6).
- **H (usability): PASS** - routing table, workflow, defaults screen and checklist all coherent with the new api-design text;
  the new "what makes the root wrapper pass" paragraph is actionable and matches what the harness needed.
- **I (unverified-ledger discipline): FAIL (minor)** - all 15 shipped `[unverified]` markers were enumerated: 14 map to ledger
  entries; one does not (R2-4), one ledger line is stale (R2-3). The three previously contested items (events.k8s.io RBAC
  create;patch, `/status` apply RBAC, `listType=set` under oldSelf) are now recorded as Confirmed in `UNVERIFIED.md:55-59`,
  and the F1-era note there matches what this round reproduced.

## What could not be verified

- Real-cluster behaviours the ledger already tracks: `/finalizers update` enforcement outside envtest, cert-manager and
  service-ca annotation names, lock types other than Lease, wiring the metrics collector to `mgr.GetCache()` in production
  shape, kube-state-metrics/cert-manager/Flux/Argo/Kong upstream claims (WebSearch was organisation-blocked in research; not
  re-attempted this round).
- Whether the full round-1 envtest battery still passes on the unchanged files: not re-run this round. Their bytes are
  unchanged since before the previous report (mtimes 15:46-16:11), so the prior runs remain valid evidence; this round's fresh
  envtest run covered the changed api-design claims. A cheap confirmation re-run of `exp-pruned-cache` and `exp-observability`
  would close this completely.
- Talks T13/T14 remain unread (no subtitles), as recorded in `sources.md`.
