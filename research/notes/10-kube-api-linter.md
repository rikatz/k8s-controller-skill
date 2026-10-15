# 10 - kube-api-linter (KAL) and what major API projects enable

Tags: [V] = verified by reading code/docs or by running it here; [U] = unverified / inferred.

Commits read (shallow clones under /tmp/k8s-controller-skill-research/src/):

| Repo | Commit | Config files read |
|---|---|---|
| kubernetes-sigs/kube-api-linter | 79b557a (2026-10-12) | README.md, docs/linters.md, pkg/analysis/*, pkg/config/*, Makefile |
| openshift/api | e07c1fd9 | .golangci.yaml, .golangci.go-validated.yaml, tools/Makefile, tools/tools.go, tools/go.mod, hack/golangci-lint.sh |
| kubernetes-sigs/gateway-api | aabc423c | .golangci-kal.yml, .custom-gcl.yml, .github/workflows/kal.yml |
| kubernetes-sigs/cluster-api | c8f89360 | .golangci-kal.yml, hack/tools/.custom-gcl.yaml, Makefile |
| cluster-api-provider-vsphere | 9d3e2832 | .golangci-kal.yml, hack/tools/.custom-gcl.yaml, Makefile |

---------------------------------------------------------------------------

## 1. KAL itself

### 1.1 What it is, how it ships  [V]
README: "Kube API Linter (KAL) is a Golang based linter for Kubernetes API types ... rules ... based on the Kubernetes API Conventions ... aimed at being an assistant to API review, by catching the mechanical elements of API review."

Three delivery forms (README "Installation"):
1. Standalone binary: `go build -o ./bin ./cmd/golangci-lint-kube-api-linter`. README: "The binary builds a custom version of golangci-lint with Kube API Linter included as a module." It is a normal golangci-lint binary (same flags, same `.golangci.yml`), with KAL baked in.
2. golangci-lint v2 *module* plugin: write `.custom-gcl.yml`, run `golangci-lint custom`. README: "**Important - Version Format**: Since this repository does not have releases yet, you must use a pseudo-version `v0.0.0-YYYYMMDDHHMMSS-commithash`." (No git tags exist in the clone; `git tag` empty.)
3. Go `-buildmode=plugin` `.so` (what openshift/api uses; `type: module` is replaced by `path:`).

`--fix` applies suggested fixes: "Where fixes are available within a rule, these can be applied automatically with the `--fix` flag".

Config format = golangci-lint v2 `.golangci.yml` with a custom linter named `kubeapilinter`:

```yaml
version: "2"
linters:
  default: none
  enable: [kubeapilinter]
  settings:
    custom:
      kubeapilinter:
        type: module
        description: KAL
        settings:
          linters:           # which KAL analyzers
            enable: [...]
            disable: ["*"]   # "*" = only what is in enable
          lintersConfig:     # per-analyzer config, key = analyzer name
            optionalfields: {...}
  exclusions:
    rules:
      - linters: [kubeapilinter]
        path-except: api/*
```

Enable/disable semantics (pkg/analysis/registry/registry.go `getEnabledInitializers`) [V]: a linter runs iff not in `disable` AND (`enable:["*"]` OR in `enable` OR (`disable` is not exactly `["*"]` AND the linter is default-on)). So: with no `disable: ["*"]`, `enable` is ADDED to the default-on set; with `disable: ["*"]`, only `enable` runs. CAPI/CAPV/gateway use `disable: ["*"]` (explicit opt-in); openshift/api does not (default-on set + extras).

### 1.2 Full linter list (docs/linters.md) [V]

Default = enabled when `disable: ["*"]` is not set. Scope: Native = in-tree/go-validated, CRD = CustomResourceDefinition.

| Linter | Default | Scope | What it enforces (docs/linters.md) |
|---|---|---|---|
| arrayofstruct | on | Native, CRD | arrays of structs must have >=1 required field in the struct (ambiguous YAML otherwise) |
| commentstart | on | both | field/type comments start with the serialized (json) name |
| conditions | on | both | `Conditions []metav1.Condition` with listType=map, listMapKey=type, patchStrategy/patchMergeKey markers, `+optional`, protobuf tag; should be first status field. Config: `isFirstField`, `useProtobuf`, `usePatchStrategy` (SuggestFix/Warn/Ignore/Forbid) -- set Forbid/Ignore for CRDs |
| conflictingmarkers | off | both | user-defined sets of mutually exclusive markers; must be configured |
| defaultorrequired | on | both | field cannot be both required (`+required`, `+kubebuilder:validation:Required`, `+k8s:required`) and have a default (`+default`, `+kubebuilder:default`) |
| defaults | on | both | fields with `+default`/`+kubebuilder:default`/`+k8s:default` must be `+optional` and have `omitempty` (or `omitzero` for structs). Config `preferredDefaultMarker: default \| kubebuilder:default` (default `default`) |
| dependenttags | off | both | marker A requires marker B (configured rules) |
| duplicatemarkers | on | both | exact duplicate markers |
| forbiddenmarkers | off | both | configured list of forbidden markers (identifier, optional attributes/values). Must list >=1 marker or KAL errors out (verified, see 3.4) |
| integers | on | both | only int32/int64 |
| jsontags | on | both | json tag exists, camelCase regex `^[a-z][a-z0-9]*(?:[A-Z][a-z0-9]*)*$` |
| maxlength | off | CRD | strings need `MaxLength`, arrays `MaxItems` (CEL cost budget + abuse protection) |
| minlength | off | CRD | strings `MinLength`, arrays `MinItems`, maps `MinProperties`, structs w/o required fields `MinProperties` ("API authors should not distinguish between empty and omitted values") |
| namingconventions | off | both | user-defined regex naming rules (Inform/Drop/DropField/Replacement) |
| nobools | off | both | no `bool` ("Booleans are limited and do not evolve well ... create a string alias ... as an enum") |
| nodurations | on | both | no `time.Duration`/`metav1.Duration`; use `FooSeconds` ints |
| nofloats | on | both | no float32/float64 |
| nomaps | on | both | maps discouraged; `policy: Enforce \| AllowStringToStringMaps \| Ignore` (default AllowStringToStringMaps) |
| nonpointerstructs | on | Native only | non-pointer struct w/ required fields must be `+required`, w/o required fields `+optional`. Docs: "NOT intended to be used to check for CRD types ... For CRD types, the optionalfields and requiredfields linters should be used instead." |
| nonullable | on | both | no `nullable` marker |
| nophase | on | both | no `Phase` fields (use conditions) |
| noreferences | on | both | `Ref`/`Refs` not `Reference`/`References`; `policy: PreferAbbreviatedReference \| NoReferences` |
| notimestamp | on | both | `FooTime` not `FooTimestamp` |
| optionalfields | on | both | optional fields: pointer-ness + omitempty/omitzero (see 1.4) |
| optionalorrequired | on | both | every field has `+optional` or `+required` (prefers `+optional`/`+required` over kubebuilder:validation:* variants; `preferredOptionalMarker`/`preferredRequiredMarker`); forbids them on type declarations |
| preferredmarkers | off | both | replace equivalent markers by a preferred one (configured) |
| requiredfields | on | both | required fields: `omitempty`/`omitzero`; pointer only if zero value valid |
| ssatags | on | both | arrays need listType atomic/set/map; listMapKey for map; `listTypeSetUsage` Warn/Ignore for set on object arrays |
| statusoptional | off | both | all first-level status children `+optional` |
| statussubresource | off | CRD | `kubebuilder:object:root` with status field <=> `kubebuilder:subresource:status` marker |
| uniquemarkers | on | both | a marker that must be single is not duplicated; `customMarkers` extendable |

Note: the default-on column is from docs/linters.md at HEAD; the pinned KAL version in each repo can differ [U].

### 1.3 Which linter(s) discourage/ban `+default` / `+kubebuilder:default`?  [V]

There is NO `nodefaults` linter. The relevant mechanisms are:

1. `forbiddenmarkers` (the ban): "ensures that types and fields do not contain any markers that are forbidden. By default, `forbiddenmarkers` is not enabled." Config:
   ```yaml
   lintersConfig:
     forbiddenmarkers:
       markers:
         - identifier: "kubebuilder:default"
         - identifier: "default"
   ```
   Verified by run: message `forbiddenmarkers: field FooSpec.Mode has forbidden marker "kubebuilder:default=Fast"`. Exception mechanism: golangci-lint `exclusions.rules` with `path`+`text` (CAPI does this for the two remaining defaults, section 2.3) or `//nolint:kubeapilinter` on the field line. To allow only some default values the config supports `ruleSets` with `attributes`/`values` (docs/linters.md ForbiddenMarkers scenarios) [V].
   Docs typo: the examples show `linterConfig:` but the real key is `lintersConfig:` (README + all 4 repos use `lintersConfig`) [V].
2. `defaults` (the "defaults are OK but must be well-formed" linter, on by default): "Fields with default markers ... should also be marked as optional. Additionally, fields with default markers should have `omitempty` or `omitzero` in their json tags". Also nags about marker flavour: by default it wants `+default` and flags `+kubebuilder:default` (verified: `defaults: field FooSpec.Mode should use +default marker instead of +kubebuilder:default`). To keep `+kubebuilder:default` set:
   ```yaml
   lintersConfig:
     defaults:
       preferredDefaultMarker: kubebuilder:default   # gateway-api does this
       omitempty: {policy: SuggestFix | Warn | Ignore}
       omitzero:  {policy: SuggestFix | Warn | Forbid}   # Forbid for Go < 1.24
   ```
   Caveat: `+default` (bare) is the k8s-native marker that controller-gen does NOT necessarily understand the same way -- not verified here [U]; gateway-api prefers `kubebuilder:default` for that reason, presumably.
3. `defaultorrequired` (on by default): required + default is an error.
4. `conflictingmarkers` (off by default): CAPI/CAPV re-implement the same rule as `default_vs_required`.

Nothing else (optionalfields, requiredfields, nonpointerstructs) looks at default markers: grep for `DefaultMarker`/`kubebuilder:default` in pkg/ finds only defaults, defaultorrequired, conflictingmarkers [V].

### 1.4 Pointers vs non-pointer + omitempty/omitzero  [V]

Doc quotes (docs/linters.md, OptionalFields):
> "The `optionalfields` linter checks that all fields marked as optional adhere to being pointers and having either the `omitempty` or `omitzero` value in their `json` tag where appropriate. Currently `omitzero` is handled only for fields with struct type."
>
> "If you prefer to avoid pointers where possible, the linter can be configured with the `WhenRequired` preference to determine, based on the serialization and valid values for the field, whether the field should be a pointer or not. For example - an optional string with a non-zero minimum length does not need to be a pointer, as the zero value is not valid, and it is safe for the Go marshaller to omit the empty value. - an optional struct having omitzero json tag with a non-zero minimum properties does not need to be a pointer ..."
>
> "When the `pointers.preference` is set to `Always`, the linter will always suggest to add the pointer to the field, regardless of the validity of the zero value."

Config (defaults shown): `pointers.preference: Always | WhenRequired` (default Always), `pointers.policy: SuggestFix | Warn`, `omitempty.policy: SuggestFix | Warn | Ignore`, `omitzero.policy: SuggestFix | Warn | Forbid` (Forbid for Go < 1.24).
Go-side config key is `json:"omitempty"`; openshift/api writes `omitEmpty` (camel) -- Go's json decode is case-insensitive so it works; irrelevant because that value equals the default [V for tag, U for effect].
The code comment in config.go: "The WhenRequired option requires bounds on strings and numerical values to be able to accurately determine the correct pointer vs non-pointer decision" [V] -- i.e. you must put `MinLength`/`Enum`/`Minimum` etc. markers for the linter to decide a zero value is invalid.

Requiredfields doc: "Fields are not typically pointers. A field doesn't need to be a pointer if its zero value is not a valid value ... However, if the zero value is valid, the field should be a pointer to differentiate between an unset state and a valid zero value."

Experiment (exp-kal/, exp-kal2/ under /tmp/k8s-controller-skill-research/, KAL 79b557a, golangci-lint 2.14.0) [V]:
- `preference: WhenRequired`:
  - `Mode string omitempty` with `+optional`, `Enum=Fast;Slow`, `MaxLength`, `+kubebuilder:default=Fast`: NOT flagged for pointer (zero "" is invalid via enum).
  - `Name string omitempty` with `MinLength=1`: not flagged.
  - `Replicas int32 omitempty` with `Minimum=0`: flagged "has a valid zero value (0) and should be a pointer".
  - Optional struct `Status FooStatus json:"status,omitzero"` with all-optional children: flagged "valid zero value ({}), but the validation is not complete ... should be a pointer ... If the zero value is not a valid use case, complete the validation and remove the pointer"; adding `+kubebuilder:validation:MinProperties=1` on the field silenced it. So `omitzero` alone is NOT enough, `MinProperties=1` is also needed.
- `preference: Always` (default): every optional non-pointer was flagged "should be a pointer", plus "Status should have the omitempty tag" even with omitzero.
- `defaults` analyzer code (analyzer.go:290): pointer fields are exempt from needing `omitzero`; for non-pointer struct fields with a default it prefers `omitzero` over `omitempty` [V].

Practical reading for "avoid pointers": KAL supports it, via `WhenRequired`, but only when you give it enough validation markers to prove the zero value invalid; zero-valid scalars (0, false, empty-but-valid string) and `{}`-valid structs still must be pointers (this is also what CAPI/openshift use).

---------------------------------------------------------------------------

## 2. Configs in the four repos

### 2.1 openshift/api (.golangci.yaml, e07c1fd9) [V]
- Delivery: `.so` plugin built from vendored module (`tools/Makefile`: `go build -mod=vendor -buildmode=plugin -o .../kube-api-linter.so ./vendor/sigs.k8s.io/kube-api-linter/pkg/plugin`; `tools/tools.go` blank-imports `sigs.k8s.io/kube-api-linter/pkg/plugin`; pinned `sigs.k8s.io/kube-api-linter v0.0.0-20260716143926-092fe0c72997` in tools/go.mod). golangci-lint is also pulled via tools.go.
- No `disable: ["*"]`: so KAL default-on set + extras.
  - enable (extras): forbiddenmarkers, maxlength, minlength, namingconventions, nobools, nomaps, preferredmarkers, statussubresource
  - disable: statusoptional ("This is legacy and not something we currently recommend."), nonpointerstructs ("This is intended for native types, not CRD types.")
  - Effective set (assuming defaults as in 1.2): arrayofstruct, commentstart, conditions, defaultorrequired, defaults, duplicatemarkers, integers, jsontags, nodurations, nofloats, nomaps, nonullable, nophase, noreferences, notimestamp, optionalfields, optionalorrequired, requiredfields, ssatags, uniquemarkers + extras above [U on exact default set at their pin].
- Per-linter config:
  - conditions: `isFirstField: Warn`, `usePatchStrategy: Ignore`, `useProtobuf: Ignore`
  - forbiddenmarkers: forbids `openshift:enable:FeatureSets`, `openshift:validation:FeatureSetAwareEnum`, `openshift:validation:FeatureSetAwareXValidation`, `kubebuilder:validation:UniqueItems` (NOT default markers)
  - optionalfields: `pointers.preference: WhenRequired`, policy SuggestFix; omitEmpty SuggestFix ("optional fields should be omitted from the serialized output unless they are non-zero"); omitzero SuggestFix ("they can be omitted correctly and prevents the need for pointers to structs")
  - requiredfields: pointers/omitempty/omitzero all SuggestFix ("so that the behaviour of not setting a value for the field is the same between both structured and unstructured clients")
  - preferredmarkers: prefer `kubebuilder:validation:XValidation` over `kubebuilder:validation:Pattern` ("Use CEL expressions instead to allow more expressive error messages")
  - namingconventions: `^Kind$` -> `Resource` (Replacement)
  - noreferences: `policy: NoReferences`
  - uniquemarkers: custom openshift FeatureGate markers
  - `issues.max-issues-per-linter: 1000`
- Second config `.golangci.go-validated.yaml`: `disable: ["*"]`, only `optionalfields` (preference **Always**) + `nonpointerstructs`, applied only to machine/v1beta1 provider specs and machine/v1alpha1 openstack (Go-validated types, not CRDs). Shows that "Always pointers" is for Go-validated types and "WhenRequired" for CRDs.
- Documented exclusions: `optionalfields` on the provider-spec files; whole files `osin/v1/types.go`, `authentication/v1alpha1/types_authentication_configuration.go` ("config file APIs, not CRDs"); `operator/v1alpha1/types_clusterapi.go` noreferences (Ref is an OCI image ref); `etcd/v1/types_pacemakercluster.go` conditions "missing optional" (conditions intentionally required); `operator/v1/types_console.go` ConsoleSpec.Ingress minlength/optionalfields/omitempty ("legacy optional value struct ... preserve the existing wire behavior", "changing it to a pointer would break existing Go consumers"); `features|payload-command/`.
- openshift/api does NOT forbid defaults (defaults + defaultorrequired are default-on).

### 2.2 kubernetes-sigs/gateway-api (.golangci-kal.yml, .custom-gcl.yml, aabc423c) [V]
- Delivery: `golangci-lint custom` (`.custom-gcl.yml`: golangci-lint v2.13.2, KAL `v0.0.0-20260423112246-3fa174937a6b`, "Pin to a commit while there's no tag"); CI (`.github/workflows/kal.yml`) `go install golangci-lint@v2.13.2`, `golangci-lint custom -v`, then `GOOS=js GOARCH=wasm ./bin/golangci-kube-api-linter run -c ./.golangci-kal.yml ./...`. Scope: only `apis/` and `apisx/` (`exclusions.paths-except`), `conformance/`, `wasm/` excluded; `run.tests: false`.
- `disable: ["*"]`; enable (15): defaultorrequired, defaults, duplicatemarkers, jsontags, nodurations, nofloats, nomaps, nonullable, nophase, noreferences, notimestamp, optionalorrequired, ssatags, statussubresource, uniquemarkers.
- Config: `defaults.preferredDefaultMarker: "kubebuilder:default"`. Nothing else.
- Documented-off linters and reasons (inline comments): arrayofstruct ("BackendRef and others are already using it"), commentstart ("very verbose"), conditions ("changes how conditions are merged and we may need to check impact on existing installations"), integers ("GA APIs are already using it"), maxlength ("some GA fields do not have a maxlength and this may break"), minlength (same; "must be revisited with VAP"), nobools ("controversial"), nonpointerstructs ("Breaking change"), optionalfields ("Ensure that optional fields are pointers - controversial"), requiredfields, statusoptional ("breaking change for RouteParentStatus").
- Gateway-API is the one that USES `+kubebuilder:default` heavily and lints it with `defaults`/`defaultorrequired`. One inline `//nolint:kubeapilinter` in apisx/v1alpha1/xbackend_types.go on a map field.

### 2.3 kubernetes-sigs/cluster-api (.golangci-kal.yml, c8f89360) [V]
- Delivery: `hack/tools/.custom-gcl.yaml` (golangci-lint v2.12.2, KAL `v0.0.0-20251107220451-f87b38edda8c`); `make lint-api` = `cd api; golangci-lint-kube-api-linter run -v --config ../.golangci-kal.yml`; binary built by `cd hack/tools; golangci-lint custom`. KAL only runs on `api/` (`path-except: "api//*"`), test files and zz_generated excluded.
- `disable: ["*"]`; enable (21): commentstart, conditions, conflictingmarkers, duplicatemarkers, forbiddenmarkers, integers, jsontags, maxlength, nobools, nodurations, nofloats, nomaps, nonullable, notimestamp, optionalfields, optionalorrequired, requiredfields, ssatags, statusoptional, statussubresource, uniquemarkers. Comment: "We will manually enable new linters after understanding the impact."
- Deliberately off: nophase ("Per discussion in July 2024, we are keeping phase fields for now", refs PR 10897), defaults, defaultorrequired (redundant with the ban), noreferences, arrayofstruct, minlength, nonpointerstructs.
- Config:
  - `conflictingmarkers`: `default_vs_required` (["default","kubebuilder:default"] vs ["required","kubebuilder:validation:Required","k8s:required"])
  - **`forbiddenmarkers`: forbids `kubebuilder:default` and `default`** with comment: "We don't want to do any defaulting (including OpenAPI) anymore on API fields because we prefer to have a clear signal on user intent. This also allows us to easily change the default behavior if necessary."
  - conditions: `isFirstField: Warn`, `usePatchStrategy: Forbid`, `useProtobuf: Forbid`
  - optionalfields: `pointers.preference: WhenRequired`, policy SuggestFix, `omitempty.policy: SuggestFix`
- Exclusions (all `linters: [kubeapilinter]`, with comments): old apiVersions (v1beta1 etc., "we don't want to make any changes to these APIs"); v1beta1 condition types in V1Beta1DeprecatedStatus; nobools on existing types ("Removal of bool fields of existing types requires further discussion"); `JSONSchemaProps` json tags/maps (aligned with CRD fields); `ExtraEnvs` intentionally `*[]EnvVar`; `Bootstrap`/ClusterClass `requiredfields`/`optionalfields` ("enforced with CEL, which cannot be detected by KAL" / webhook); `FeatureGates` map aligned with kubeadm; Runtime Hooks/clusterctl types "are not CRDs, so e.g. SSA markers don't make sense there"; two existing default markers on `ValidationRule.Reason` (`forbiddenmarkers: ... "kubebuilder:default=FieldValueInvalid"` and `default=ref(...FieldValueInvalid)`) -> the documented way to grandfather a default; KAL bugs (issues #116 Taints pointer, #138 Token zero value).

### 2.4 cluster-api-provider-vsphere (.golangci-kal.yml, 9d3e2832) [V]
- Same enabled list (21), same lintersConfig (incl. forbidding `kubebuilder:default`/`default`, `optionalfields` WhenRequired, conditions Forbid/Forbid), same `disable: ["*"]` and same nophase comment as CAPI.
- Delivery: `hack/tools/.custom-gcl.yaml` golangci-lint v2.1.0 + KAL `v0.0.0-20250908163129-65a570bd22aa` (older pin than CAPI); `make lint-api` runs `cd api; $(GOLANGCI_LINT_KAL) run -v --config ../.golangci-kal.yml`.
- Exclusions: `path-except "^api//*"`; v1beta1 dirs; nobools on v1beta2; `nomaps: Capacity` on two supervisor types; plus inline `//nolint:kubeapilinter // field will be removed when v1beta1 is removed` on `FailureReason`/`FailureMessage` fields (4 places).

---------------------------------------------------------------------------

## 3. Table + recommendation

### 3.1 Linter matrix

Y = enabled, - = not enabled. OS = openshift/api (effective incl. default-on [U on exact pin]); GW = gateway-api; CAPI; CAPV.
Tension columns: (a) `+default` markers, (b) avoid-pointers preference.

| Linter | OS | GW | CAPI | CAPV | Enforces | (a) vs `+default` | (b) vs avoid-pointers |
|---|---|---|---|---|---|---|---|
| duplicatemarkers | Y | Y | Y | Y | exact duplicate markers | none | none |
| jsontags | Y | Y | Y | Y | json tag present, camelCase | none | none |
| nodurations | Y | Y | Y | Y | no Duration types | none | none |
| nofloats | Y | Y | Y | Y | no floats | none | none |
| nomaps | Y | Y | Y | Y | maps discouraged (string->string allowed) | none | none |
| nonullable | Y | Y | Y | Y | no `nullable` | none | none |
| notimestamp | Y | Y | Y | Y | no `*Timestamp` fields | none | none |
| optionalorrequired | Y | Y | Y | Y | explicit `+optional`/`+required` | none (but `defaults` requires `+optional` on defaulted fields, consistent) | none |
| ssatags | Y | Y | Y | Y | listType/listMapKey on arrays | none | none |
| statussubresource | Y | Y | Y | Y | status field <=> `kubebuilder:subresource:status` | none | none |
| uniquemarkers | Y | Y | Y | Y | marker uniqueness | none | none |
| commentstart | Y | - (verbose) | Y | Y | comment starts with json name | none | none |
| conditions | Y (Ignore/Ignore) | - (merge-impact) | Y (Forbid/Forbid) | Y | metav1.Condition layout/markers | none | none |
| forbiddenmarkers | Y | - | Y | Y | configurable marker ban | THIS is the ban: CAPI/CAPV forbid `default`+`kubebuilder:default`; OS forbids other markers | none |
| integers | Y | - (GA) | Y | Y | int32/int64 only | none | none |
| maxlength | Y | - (GA) | Y | Y | MaxLength/MaxItems everywhere | none | none |
| nobools | Y | - (controversial) | Y | Y | no bool | none | none (bool zero-value problem is the reason enums are preferred) |
| optionalfields | Y (WhenRequired) | - (controversial) | Y (WhenRequired) | Y (WhenRequired) | optional => pointer/omitempty/omitzero | no default-awareness in code; with `Always` a defaulted optional field must still be a pointer; with `WhenRequired` a defaulted field w/ enum/minLength is allowed non-pointer (verified) | CORE tension: default `Always` demands pointers; `WhenRequired` is the avoid-pointers mode; needs bounds markers; `{}`-valid structs need `MinProperties` to drop the pointer |
| requiredfields | Y | - | Y | Y | required => omitempty/omitzero, pointer only if zero valid | none (`defaultorrequired` bans required+default) | zero-valid required scalars still need pointer |
| defaultorrequired | Y (default) | Y | - | - | required XOR default | conflicts with required+default | none |
| defaults | Y (default) | Y | - | - | defaulted field is `+optional`, has omitempty/omitzero; marker flavour | the linter that *accepts* defaults; wants `+default` unless `preferredDefaultMarker: kubebuilder:default` | pushes `omitzero` for non-pointer struct, exempts pointers |
| conflictingmarkers | - | - | Y | Y | configured conflicts | used by CAPI/CAPV to encode default-vs-required | none |
| statusoptional | - (legacy) | - (breaking) | Y | Y | status children optional | none | none |
| nophase | Y (default) | Y | - (kept phase) | - | no Phase fields | none | none |
| noreferences | Y (NoReferences) | Y | - | - | Ref/Refs naming | none | none |
| minlength | Y | - | - | - | MinLength/MinItems/MinProperties | none | enables WhenRequired (non-zero min => non-pointer) |
| arrayofstruct | Y (default) | - | - | - | array-of-struct needs required field | none | none |
| nonpointerstructs | - (CRD) | - | - | - | Native-only struct optionality | none | it is the Go-validated counterpart; openshift uses it only in go-validated config |
| namingconventions | Y | - | - | - | custom naming regex | none | none |
| preferredmarkers | Y | - | - | - | prefer one equivalent marker | none | none |
| dependenttags | - | - | - | - | marker deps | none | none |

### 3.2 Counts
- 4/4 repos: duplicatemarkers, jsontags, nodurations, nofloats, nomaps, nonullable, notimestamp, optionalorrequired, ssatags, statussubresource, uniquemarkers.
- 3/4 (OS+CAPI+CAPV; gateway-api off with "breaking for GA APIs" reasons that do not apply to a new project): commentstart, conditions, forbiddenmarkers, integers, maxlength, nobools, optionalfields, requiredfields.
- 2/4: defaults + defaultorrequired (OS+GW), nophase + noreferences (OS+GW), conflictingmarkers + statusoptional (CAPI+CAPV; same repo family).

### 3.3 Proposed MINIMUM set for a pure controller-runtime project
Rationale: take the 4/4 set (nothing contentious), plus the 3/4 set, because gateway-api's reasons for disabling them are all backward-compat on GA APIs ("GA APIs are already using it", "breaking change"), which a new API does not have. Gateway-api's own comments admit these (maxlength, integers, optionalfields...) are desirable.

Core (4/4, always on): `duplicatemarkers, jsontags, nodurations, nofloats, nomaps, nonullable, notimestamp, optionalorrequired, ssatags, statussubresource, uniquemarkers`.
Add (3/4): `commentstart, conditions, integers, maxlength, nobools, optionalfields, requiredfields`.
Add (policy decision for defaults, pick exactly one):
- A. "No defaults" policy (CAPI/CAPV): `forbiddenmarkers` banning `default` + `kubebuilder:default` (+ `conflictingmarkers` optional; redundant once defaults are banned).
- B. "Defaults allowed" policy (gateway-api / openshift): `defaults` + `defaultorrequired`, with `preferredDefaultMarker: kubebuilder:default` (controller-gen's marker; avoids the nag when you write `+kubebuilder:default`).
Optional extras seen in 2/4: `statusoptional` (CAPI/CAPV), `nophase` + `noreferences` (OS+GW; CAPI deliberately keeps Phase fields).
Don't enable `nonpointerstructs` for CRD types (docs say it is Native-only), nor `statusoptional` unless you want all-optional status (it is enabled in CAPI/CAPV).

Gotcha: `forbiddenmarkers` with an empty/missing `markers` list is a hard error ("lintersConfig.forbiddenmarkers.markers: Required value: must contain at least one forbidden marker") - verified by run. So don't enable it with an empty config.

### 3.4 Config snippets for a pure controller-runtime repo (no kubebuilder) [V - run in exp-kal2]

Why `-modfile`: `go get -tool sigs.k8s.io/kube-api-linter/cmd/golangci-lint-kube-api-linter@latest` WORKS (verified with go 1.27.2) but into the main `go.mod` it adds ~215 lines (golangci-lint v2.14.0 + ~200 indirect deps, bumps `golang.org/x/tools`, etc.). Using a separate modfile keeps the project's go.mod clean (this is also how KAL's own Makefile does it: `go tool -modfile tools/go.mod github.com/golangci/golangci-lint/v2/cmd/golangci-lint`).

```sh
# one time
mkdir -p tools
printf 'module example.com/yourproject/tools\n\ngo 1.26\n' > tools/go.mod   # KAL requires go >= 1.26.0 (its go.mod says 1.26.0)
go get -tool -modfile=tools/go.mod sigs.k8s.io/kube-api-linter/cmd/golangci-lint-kube-api-linter@latest
# result in tools/go.mod:  tool sigs.k8s.io/kube-api-linter/cmd/golangci-lint-kube-api-linter
# run (verified; ~15 s, first run compiles golangci-lint)
go tool -modfile=tools/go.mod golangci-lint-kube-api-linter run -c .golangci-kal.yml ./api/...
# autofix
go tool -modfile=tools/go.mod golangci-lint-kube-api-linter run -c .golangci-kal.yml --fix ./api/...
```
`@latest` resolves to a pseudo-version (no tags exist): e.g. `v0.0.0-20261012124120-79b557aa8c8d`; commit it to tools/go.mod via the normal go.sum flow (Renovate/dependabot can bump). The tool pulls golangci-lint 2.14.0 transitively (version printed by `... version`). Alternative if you don't want the tool directive: `.custom-gcl.yml` + `golangci-lint custom` (gateway-api/CAPI/CAPV approach) [V from those repos].

Makefile target:
```make
KAL = go tool -modfile=tools/go.mod golangci-lint-kube-api-linter
.PHONY: lint-api
lint-api: ; $(KAL) run -c .golangci-kal.yml ./api/...
```

`.golangci-kal.yml` (minimum set, "defaults allowed" variant B; swap per A in comment):
```yaml
version: "2"
linters:
  default: none
  enable: [kubeapilinter]
  settings:
    custom:
      kubeapilinter:
        type: module
        description: Kube API Linter
        settings:
          linters:
            enable:
              - commentstart
              - conditions
              - duplicatemarkers
              - integers
              - jsontags
              - maxlength
              - nobools
              - nodurations
              - nofloats
              - nomaps
              - nonullable
              - notimestamp
              - optionalfields
              - optionalorrequired
              - requiredfields
              - ssatags
              - statussubresource
              - uniquemarkers
              # variant B (defaults allowed):
              - defaults
              - defaultorrequired
              # variant A (defaults banned, CAPI/CAPV): drop the two above, add
              # - forbiddenmarkers
            disable: ["*"]          # explicit opt-in; new KAL linters do not appear on upgrade
          lintersConfig:
            conditions:
              isFirstField: Warn
              usePatchStrategy: Forbid   # CRDs: no patchStrategy
              useProtobuf: Forbid        # CRDs: no protobuf tags
            optionalfields:
              pointers:
                preference: WhenRequired # avoid pointers when the zero value is invalid (needs Enum/MinLength/Minimum/MinProperties markers)
                policy: SuggestFix
              omitempty: {policy: SuggestFix}
              omitzero:  {policy: SuggestFix}   # Forbid if go < 1.24
            requiredfields:
              pointers: {policy: SuggestFix}
              omitempty: {policy: SuggestFix}
              omitzero: {policy: SuggestFix}
            defaults:                           # variant B only
              preferredDefaultMarker: kubebuilder:default
            # variant A only:
            # forbiddenmarkers:
            #   markers:
            #     - identifier: kubebuilder:default
            #     - identifier: default
  exclusions:
    generated: strict
    paths:
      - zz_generated.*\.go$
      - ".*_test.go"
    rules:
      - linters: [kubeapilinter]
        path-except: "^api/"        # CAPI/CAPV/gateway all scope KAL to the API dirs only
issues:
  max-same-issues: 0
  max-issues-per-linter: 0
  uniq-by-line: false               # IMPORTANT, see below
```

Gotchas found [V by running]:
1. golangci-lint de-duplicates diagnostics per source line by default; with `forbiddenmarkers` + `defaults` + `optionalfields` all reporting on the same field line, only the first was shown (3 of 5 issues). Set `issues.uniq-by-line: false` to see all. (CAPI/CAPV do not set this; so they may under-report [U].)
2. `forbiddenmarkers` enabled with no markers = config error (see 3.3).
3. Silence one case with `//nolint:kubeapilinter // reason` (CAPV, gateway-api do this) or an `exclusions.rules` entry with `path` + `text` regex (CAPI does this, with a comment per exclusion).
4. `statussubresource` and `maxlength`/`minlength` are CRD-only; `nonpointerstructs` Native-only.
5. Diagnostics format: `file:line:col: <linter>: <message> (kubeapilinter)`; the `<linter>:` prefix in the message is what `text:` exclusions match on (CAPI uses `text: "optionalfields: field ..."`).

### 3.5 Interaction summary for the skill author
- If the controller project puts `+kubebuilder:default` markers on fields (very common in kubebuilder scaffolds and gateway-api) use variant B. KAL requires: `+optional`, `omitempty` (or `omitzero` for struct), not also `+required`. With `pointers.preference: WhenRequired` a defaulted non-pointer string/enum passes if its zero value is invalid under the schema (enum/MinLength); a defaulted numeric whose zero is valid (e.g. `Minimum=0`) must be a pointer.
- If the project follows the CAPI line ("clear signal on user intent", no OpenAPI defaults, defaulting in controller/webhook), use variant A. Existing defaults can be grandfathered through `exclusions.rules` (text match on the forbidden marker) -- CAPI shows two examples.
- Avoid-pointers + KAL = `optionalfields.pointers.preference: WhenRequired` + `omitzero` for structs + validation markers that make the zero value invalid (`MinLength=1`, `Enum`, `Minimum=1`, `MinProperties=1`). Without the validation markers KAL will keep demanding pointers.

## 4. Not verified / open
- [U] Exact default-on linter set at each repo's pinned KAL version (I read HEAD docs only; the OS config relies on defaults).
- [U] Whether controller-gen treats bare `+default` identically to `+kubebuilder:default` (not checked here; see file 02 for controller-gen).
- [U] Whether CAPI/CAPV silently lose diagnostics to `uniq-by-line` (default behavior observed in my run only).
- [U] `go get -tool ...@latest` stays working: it resolved a pseudo-version of a repo with no tags; pin explicitly in tools/go.mod.
- Experiment dirs: /tmp/k8s-controller-skill-research/exp-kal (tool in main go.mod) and /tmp/k8s-controller-skill-research/exp-kal2 (tool in tools/go.mod via -modfile).
