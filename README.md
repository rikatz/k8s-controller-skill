# Kubernetes Controllers with controller-runtime

A comprehensive Claude Code skill for building, extending, and reviewing Kubernetes controllers and operators in Go using pure controller-runtime and controller-gen - without kubebuilder scaffolding.

This repository contains both the **Claude Code skill** and the complete **research + verification artifacts** that back every claim.

## What is this repository?

### The skill (in `k8s-controller-runtime/`)

A curated knowledge base for Kubernetes controller development that provides:

- **14 practical reference guides** - one for each phase of controller development (API design, reconciliation, testing, operations, etc.)
- **Best practices** - each backed by verified experiments and source attribution
- **Proven hazards** - real failure modes with explanations of how to avoid them
- **Source transparency** - every claim tagged with where it came from (`[CR]`, `[run]`, `[K8S]`, etc.)
- **No boilerplate** - pure controller-runtime patterns without CLI scaffolding

### The research (in `research/`)

Runnable experiments and verification audits that **prove every `[run]`-tagged claim**:

- **16+ experiment directories** - each with full test suites, generated CRDs, and deepcopy artifacts
- **Three rounds of independent verification** - consistency reports confirming all claims and findings
- **Ledger of unverified claims** - clear boundary between verified facts and remaining unknowns
- **Deep-dive notes** - detailed analysis of ratcheting, marker behavior, etc.

**Target stack:** controller-runtime v0.25.2, controller-tools v0.23.0, Go 1.26+, k8s.io/* v0.37

## What it contains

### Skill modules (`k8s-controller-runtime/`)

#### Core skill definition

- **`SKILL.md`** - Quick-start overview, workflow for new controllers, hazards, and when to use each reference

#### Reference guides (14 modules)

Each reference document covers one area end-to-end, opens with a "Read this when" line, and closes with hazard sections:

| Module | Purpose |
|--------|---------|
| `api-design.md` | CRD types, `+kubebuilder:` markers, defaults, validation, CEL rules, the KAL linter |
| `status-conditions.md` | Status patterns, `metav1.Condition`, `observedGeneration`, conditions as the primary API |
| `reconcile.md` | Reconciler logic, requeue semantics, finalizers, patch vs apply, controller options |
| `cache-and-memory.md` | Cache tuning, field pruning, selectors, transforms, indexes, metadata-only reads |
| `manager-setup.md` | Project layout, `main.go`, manager options, leader election, Dockerfile |
| `codegen-workflow.md` | controller-gen commands, kube-api-linter (KAL) config, applyconfiguration generation |
| `webhooks.md` | Validating and defaulting webhooks, CEL vs webhooks, certificate handling |
| `testing-envtest.md` | envtest with downloaded binaries, Ginkgo, Gomega, fake client patterns |
| `observability-rbac.md` | Events, RBAC markers and generation, probes, leader-election lease, pod security |
| `metrics.md` | Custom metrics, authenticated scrape endpoints, Prometheus integration |
| `logging.md` | Logger setup, structured logging conventions, backend choices (klog, slog, zap) |
| `versions.md` | API and controller-runtime version matrix, deprecations, breaking changes per release |
| `review-checklist.md` | Pre-shipping checklist for controllers; use to review existing controllers |
| `sources.md` | Tag-to-source mapping (e.g., `[CAPI]` = Cluster API, `[run]` = verified by experiment) |

### Verification & research (`research/`)

#### Experiments (`research/experiments/`)

Runnable, reproducible proofs for every `[run]`-tagged claim in the skill. Each experiment has:
- Passing test suites
- Generated artifacts (CRDs, deepcopy code)
- Proof that the documented behavior is real

**Key experiments:**
- **exp-kal-gadget** - CRD types that compile, generate, and pass kube-api-linter with 0 issues
- **exp-ratcheting** - CRD validation ratcheting (v1.33+): CEL rules, enums, transition rules, `optionalOldSelf`
- **exp-kal**, **exp-kal2**, **exp-kal-config** - KAL linter behavior and edge cases
- **exp-observability**, **exp-rbac-markers** - Event recording, RBAC marker generation
- **exp-testing**, **exp-testing-core** - envtest setup, Ginkgo suites, webhook tests
- **exp-logging** - Logging backends (klog, slog, zap), output format, caller frames
- **exp-pruned-cache**, **exp-verify**, **exp-setup** - Cache transforms, applyconfiguration, manager setup
- **exp-nil-transform**, **exp-ctl**, and 6 more

Run `go test ./...` in any experiment to verify the claim it backs.

#### Verification audits (`research/`)

- **`consistency-report-2.md`** - Round 2 audit: independent re-review of all skill claims, findings, and fixes
- **`UNVERIFIED.md`** - Ledger of claims that remain unverified (clear boundary between facts and unknowns)
- **`notes/`** - Deep dives on specific topics (e.g., `12-ratcheting.md` on CRD validation ratcheting)

## What it does

When you invoke this skill, Claude can:

1. **Design your API** - Help you write CRD types with correct markers, validation rules, and status conditions, verified by KAL
2. **Set up your project** - Hand-write `main.go`, layout, and manifests without scaffold boilerplate
3. **Build and test** - Advise on reconciler logic, cache tuning, webhooks, and envtest patterns
4. **Review your code** - Apply the hazards checklist to catch subtle bugs (cache transforms, patch semantics, CEL costs, RBAC)
5. **Troubleshoot** - Find the root cause of common failures (metrics, logging, finalizers, conditions)

The skill is **triggered automatically** when a task mentions:
- controller-runtime, Kubernetes operator, reconciler, CRD
- controller-gen, envtest, finalizers, informer, cache tuning
- kube-api-linter

You can also invoke it explicitly by name.

## Installation

### For Claude.ai and Claude Code

#### Option 1: Automatic (Codex-style links)

Share a [Codex link](https://claude.ai/codex) pointing to this repository with your team. Codex auto-installs referenced skills when you load a project.

#### Option 2: Manual installation in Claude Code CLI

Copy the skill directory to your Claude Code skills folder:

```bash
# Find your skills directory
CLAUDE_SKILLS_DIR="$HOME/.claude/skills"
mkdir -p "$CLAUDE_SKILLS_DIR"

# Copy the skill
cp -r ~/codes/k8s-controller-runtime-skill/k8s-controller-runtime "$CLAUDE_SKILLS_DIR/"

# Verify installation
ls "$CLAUDE_SKILLS_DIR/k8s-controller-runtime/"
```

Then restart Claude Code. The skill will be available as `/k8s-controller-runtime`.

#### Option 3: Via project configuration

Add to your project's `.claude/config.json` or `claude.json`:

```json
{
  "skills": [
    {
      "name": "k8s-controller-runtime",
      "path": "./k8s-controller-runtime"
    }
  ]
}
```

If the skill is in a Git submodule or adjacent directory:

```json
{
  "skills": [
    {
      "name": "k8s-controller-runtime",
      "path": "../k8s-controller-runtime-skill/k8s-controller-runtime"
    }
  ]
}
```

### For Codex (codeowners, teams)

#### Add to your Codex workspace

1. Open your [Codex workspace](https://claude.ai/codex)
2. Click **Add Project**
3. Connect to the repository containing this skill
4. Point to `/k8s-controller-runtime` as the skill root
5. Enable **Auto-load for matching tasks** (will prompt Claude when it detects Kubernetes work)

#### Or add via `.claude.yaml` at your repo root

```yaml
skills:
  - name: k8s-controller-runtime
    source: k8s-controller-runtime-skill/k8s-controller-runtime
    autoload:
      triggers:
        - "controller-runtime"
        - "kubernetes operator"
        - "envtest"
        - "reconciler"
        - "CRD"
```

## Using the skill

### In Claude.ai or Claude Code

Once installed, you can:

1. **Invoke explicitly:**
   ```
   /k8s-controller-runtime
   ```

2. **Automatic triggering** (if configured):
   Just mention "controller-runtime," "CRD," "envtest," or similar; Claude will load it

3. **Link specific sections:**
   ```
   I'm writing a reconciler. Help me follow reconcile.md section Requeue semantics.
   ```

### Typical workflows

#### Start a new controller

```
I'm building a Kubernetes operator for a custom resource. 
Use the k8s-controller-runtime skill to walk me through:
1. API design (types, markers, validation)
2. Project layout
3. A basic reconciler
4. envtest setup
```

#### Debug a cache issue

```
My controller reads field X from a Pod, but when I update it the cache doesn't 
reflect the change until I recreate the Pod. 
Per k8s-controller-runtime/cache-and-memory.md, what went wrong?
```

#### Pre-ship review

```
I have a controller ready. 
Run through k8s-controller-runtime/review-checklist.md and tell me 
if I'm missing anything before I ship.
```

## Key features

- **Verified claims** - Every assertion backed by experiment code, code review of the controller-runtime source, or Kubernetes documentation
- **Source transparency** - Every claim tagged with where it came from (`[CR]` = controller-runtime source, `[run]` = reproduced in experiments, `[K8S]` = Kubernetes docs, etc.)
- **Practical hazards** - Real failure modes with concrete examples, not just "be careful"
- **No opinion on packaging** - Covers the controller code, not OLM bundles, Helm charts, multi-cluster setup, or webhooks for CRD conversion (out of scope)
- **Cross-referenced** - Jump between modules via markdown links; each reference knows which other modules it depends on

## Quick reference: The Hazards

This skill teaches **advice** (marked "Prefer", "Avoid") alongside **factual bugs** (marked "Hazards"). Here are the top hazards:

### Cache & reads
- Transforms returning `nil, nil` empty the store; use selectors instead
- `Update` of a pruned-cache object deletes pruned keys on the server; use `Patch`
- `DisableFor` per-type lookups go live every time for `PartialObjectMetadata`

### Reconcile & writes
- Plain `MergeFrom` status patches overwrite whole `conditions` array; use `listType=map` or single writer
- `Result.Requeue` never resets backoff; use `RequeueAfter` or returned error
- Webhook defaults never run if a marker default exists (marker fires first)

### API & codegen
- Misspelled markers are silently ignored; diff the generated CRD
- `omitempty` on a struct serializes `{}`; use `omitzero`
- RBAC markers glued to comments generate nothing; leave a blank line after

### Operations
- Unauthenticated metrics are a security hole; use `SecureServing: true`
- Without `tokenreviews` and `subjectaccessreviews` RBAC, authenticated scrape returns 500
- Without `SetLogger`, all logs are dropped silently

See each reference module's Hazards section for full details and failure scenarios.

## Repository structure

```
k8s-controller-runtime-skill/
├── k8s-controller-runtime/          ← This skill
│   ├── SKILL.md                     ← Skill definition & overview
│   ├── README.md                    ← This file
│   └── references/                  ← 14 reference modules
│       ├── api-design.md
│       ├── reconcile.md
│       ├── status-conditions.md
│       └── ... (11 more)
├── research/
│   ├── experiments/                 ← Runnable proofs for [run] claims
│   │   ├── exp-kal-gadget/
│   │   ├── exp-ratcheting/
│   │   └── ... (14 more)
│   ├── consistency-report-2.md      ← Verification audit log
│   ├── UNVERIFIED.md                ← Ledger of unverified claims
│   └── notes/                       ← Deep dives (ratcheting, KEP notes, etc.)
└── README.md                        ← Repository overview
```

## Requirements

- **Go** 1.26 or later (toolchain 1.27+ recommended)
- **kubectl** 1.37+ (matches k8s.io v0.37)
- **controller-runtime** v0.25.2
- **controller-tools** v0.23.0 (installed as a `go.mod` tool)
- **kube-api-linter** (optional, for validation linting)

## Contributing

Found an error? File an issue in the repository. All claims are verifiable against:

1. **Code** in `/research/experiments/` (run `go test ./...` to reproduce)
2. **Source documents** (cited in `references/sources.md`)
3. **Controller-runtime source** at the pinned v0.25.2 tag

## License

This skill is part of the k8s-controller-runtime-skill repository. Check the repository root for license details.

## Feedback

- **Report defects** - File an issue with evidence (experiment test failure, contradicting code, broken cross-reference)
- **Request a topic** - Open a discussion; if verified, it becomes a new reference module with experiments
- **Suggest a better way** - PRs welcome; all changes require updated experiment proofs and consistency audits

---

**Quick start:** Read `SKILL.md` first. Then pick your task from the "Which reference when" table and dive in.
