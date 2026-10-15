# Resolutions to the independent consistency review

Each finding from `consistency-report.md` and what was done. Fixes were re-verified by the author (not just accepted from the report).

- **F1 (major) - api-design.md excerpt did not pass KAL.** FIXED. Reproduced the 5 findings on the verbatim excerpt (3x `commentstart` on the root
  `Gadget` wrapper's `ObjectMeta`/`Spec`/`Status`, 2x `requiredfields` on `Spec`). Fix: doc comment on each wrapper field starting with its JSON name;
  `Spec` is `+required` with `omitempty`; `MinProperties=1` on `GadgetSpec` so its empty object is not a valid submission, which lets `Spec` stay a
  non-pointer (the skill's own avoid-pointers rule). Re-ran KAL on the exact shipped code block: **0 issues**; `go build` clean; controller-gen produces
  `required: [spec]` and `minProperties: 1` on spec/tls/status with the deviation note still absent from the CRD. Prose at api-design.md updated.
- **F2 (minor) - metrics.md collector excerpt omits `Describe`.** FIXED. Added `func (c *WidgetCollector) Describe(ch chan<- *prometheus.Desc) { ch <- c.desc }`
  before `Collect`, with a one-line note that both methods are required. Verified: a collector with both methods registers and compiles (client_golang v1.24.0).
- **F3 (minor) - events recorder attributed to v0.25.** FIXED. `GetEventRecorder` exists since v0.23.0 (v0.25.0 only widened the return type).
  Corrected in reconcile.md and observability-rbac.md; versions.md was already correct.
- **F4 (minor) - "logs dropped after 30 s" imprecise.** FIXED. observability-rbac.md hazard now matches logging.md: logs are dropped, and the notice
  prints only on a log call made after 30 s, so earlier lines are lost silently.
- **F5 (minor) - cache-and-memory.md (360) and metrics.md (270) over the ~250 target.** WON'T FIX (accepted). These are the performance core and the
  metrics reference, loaded only on demand, not SKILL.md. Trimming ~60 lines risks dropping a verified hazard, a worse trade than the length. Decision recorded here.
- **F6 (minor) - three claims tagged verified but still listed in UNVERIFIED.md.** FIXED. The review re-verified all three (events `create;patch`,
  `<resource>/status` apply RBAC, `listType=set` under `oldSelf`); moved them out of the "still unverified" list into a "confirmed during review" section.

Outstanding after this round: none. F1, F2, F3, F4, F6 fixed and re-verified; F5 deliberately accepted.

## Round 2 (second independent review)

- **R2-1 - api-design.md `[run]` citation dangled.** FIXED. The KAL check ran in a volatile scratch dir (`/tmp/review-f1`) outside `research/`.
  Copied it verbatim to `research/experiments/exp-kal-gadget` (92K, sources only, `diff -r` identical) and pointed the citation at it.
- **R2-2 - SetLogger-notice timing still imprecise in reconcile.md and manager-setup.md.** FIXED. Both now use the wording verified in
  `logging.md` (the notice prints only on a log call made after 30 s; earlier lines are lost silently) instead of "after 30s". Tags upgraded to `[CR, run]`.
- **R2-3 - UNVERIFIED.md said "no procedure is given for testing RBAC".** FIXED. Stale: two procedures exist and ran. Removed from "still
  unverified"; the resolved section now records the `rest.Config.Impersonate` run (`observability-rbac.md` Section 5) and the ServiceAccount-token
  metrics-endpoint run (`metrics.md` Section 7).
- **R2-4 - `[unverified]` item in observability-rbac.md with no ledger entry.** FIXED. Added to the ledger (events section): whether recording
  an event for a custom resource needs its type in the manager's scheme (the run used a ConfigMap).
- **R2-5 - SKILL.md claimed every reference ends with a Hazards section.** FIXED. Verified by grep before rewording: only `versions.md`,
  `sources.md` and `review-checklist.md` lack one; SKILL.md now names those three exceptions.
- **R2-6 - file lengths.** SKIPPED at the user's direction (same acceptance as F5).

Outstanding after round 2: none. R2-1 to R2-5 fixed; R2-6 waived by the user.

## Round 3 (third independent sweep)

- **N-1 - the R2-1 fix left the "generate" half of the citation dangling.** FIXED. exp-kal-gadget backed compile +
  KAL only. Now: `go get -tool controller-gen@v0.23.0` added to its go.mod (house pattern: controller-gen in the main
  module, KAL in tools/go.mod), `GadgetList` added to gadget_types.go and both root types registered in the scheme,
  `zz_generated.deepcopy.go` regenerated (Gadget/GadgetList methods present), CRD generated to
  `config/crd/demo.example.com_gadgets.yaml`. Re-verified: `go build` clean; KAL still **0 issues**; the generated
  CRD carries every shipped schema claim (`default: Fast`, three `minProperties: 1`, `x-kubernetes-list-type: map`,
  root `required: [spec]`, status subresource, Ready printer column, deviation note absent). api-design.md:113 now
  cites the generated CRD file explicitly.
- Ratcheting coverage (user request "do it"): added the "Tightening validation on a shipped version (ratcheting)"
  block and two hazards to api-design.md, from research/notes/12-ratcheting.md (verified in exp-ratcheting).
- **N-2 - six Hazards `[run]` tags had no reproduction in research/experiments.** FIXED by reproducing, not re-tagging
  (every claim re-confirmed on 1.37.0 / controller-gen v0.23.0, quotes verbatim):
  - misspelled `MaxLenth` silently ignored (api-design.md:189): `exp-kal-gadget/misspell/types.go` +
    `genbehaviour_test.go` `TestMisspelledMarkerSilentlyIgnored` - controller-gen exits 0 and the generated CRD
    carries no `maxLength`.
  - `MinProperties=1` create rejection, "at least 1 properties" (api-design.md:191, API-server half; the
    serialization half was already backed by exp-verify): `exp-ratcheting` stage9 -
    `spec.tls in body should have at least 1 properties`.
  - `must apply minproperties to an object` (api-design.md:193): `exp-kal-gadget/minprops/types.go` +
    `genbehaviour_test.go` `TestMinPropertiesOnFieldRejected` - rejected at `types.go:28:2`.
  - field-level `self == oldSelf` does not stop removal (api-design.md:194): `exp-ratcheting` stage9 (removal
    allowed) + stage10 (parent set-once rule `!has(oldSelf.mode) || self.mode == oldSelf.mode` blocks it, failing
    with `no such key: mode`).
  - `oldSelf` under an atomic or set list (api-design.md:195): `exp-ratcheting` stage7, both variants - rule under
    `listType: atomic` (object items) and `listType: set` (scalar item) each rejected at CRD admission with
    `oldSelf cannot be used on the uncorrelatable portion of the schema`. Fields bounded (`maxItems`/`maxLength`) so
    the cost-budget rejection does not also fire; the ledger entry for the set case now points here.
  - CEL cost over budget (api-design.md:200): `exp-ratcheting` stage8 - O(n²) `all`/`exists` rule over an unbounded
    list rejected with `estimated rule cost exceeds budget by factor of more than 100x`. Probing the hazard's "or
    strings" half found it false as written: on 1.37.0 a rule over a single unbounded string (`==`, `matches`,
    `contains`) is never rejected (checked top-level, and as a negative control inside the very CRD that stage8
    rejects). api-design.md:200 reworded to "iterates an unbounded list" plus the single-string negative, keeping the
    `maxItems`+`maxLength` fix; stage8 carries the unbounded-string rule as the in-CRD negative control.
  Side effects: `waitRejected` now returns the observed error (message assertions); the stale KEP-4009 reference in
  ratcheting_test.go's header comment removed (already dropped from the notes as an unrelated device-plugin KEP);
  ratcheting_test.go gofmt-fixed (doc-comment list style). exp-kal-gadget: KAL still 0 issues on `./api/...` (the new
  probe packages live outside `^api/`), `go test ./...` green (3 tests).
- Further round-3 findings: pending (user is posting them split).
