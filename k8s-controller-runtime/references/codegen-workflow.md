# Code generation and API linting workflow

Read this when you add or change API types, RBAC or webhook markers, or set up `controller-gen` and the Kube API Linter (KAL).
Types and marker choices are in `api-design.md`; the `+kubebuilder:webhook` handler side is in `webhooks.md`.

All commands below were run with Go 1.27, controller-tools v0.23.0, controller-runtime v0.25.2, k8s.io v0.37 [run].

## Install and pin controller-gen

```sh
go get -tool sigs.k8s.io/controller-tools/cmd/controller-gen@v0.23.0   # adds a `tool` line + indirect deps to go.mod
go tool controller-gen --version                                       # Version: v0.23.0
```

- Prefer the `tool` directive over `go install`, a Makefile download, or `tools.go`: the version lives in `go.mod`/`go.sum` and `go tool` builds and caches the binary (about 7s first use) [run]. Upgrade by re-running `go get -tool ...@vX`.
- Keep controller-tools in the same minor family as your `k8s.io/*` modules; mixed minors are unsupported [CT README]. v0.23.0 requires `k8s.io/api`, `apimachinery`, `apiextensions-apiserver` v0.37.0 [run: `go list -m all`].
- The CRD embeds `controller-gen.kubebuilder.io/version: v0.23.0`, so a version bump rewrites every CRD; `crd:omitToolVersion=true` drops the annotation [run].

## Commands (one `go:generate` wrapper at the module root)

Put the directives in a file that has no other purpose; `go generate ./...` runs from that directory, so paths stay relative to the repo root [run]:

```go
// Package demo only carries the go:generate directives for the whole module.
package demo

//go:generate go tool controller-gen object paths=./api/...
//go:generate go tool controller-gen crd paths=./api/... output:crd:artifacts:config=config/crd/bases
//go:generate go tool controller-gen rbac:roleName=manager-role paths=./internal/... output:rbac:artifacts:config=config/rbac
//go:generate go tool controller-gen webhook paths=./internal/... output:webhook:artifacts:config=config/webhook
//go:generate go tool controller-gen applyconfiguration paths=./api/...
```

`go generate -x ./...` ran all five (6.6s) and produced `api/v1/zz_generated.deepcopy.go`, `config/crd/bases/<group>_<plural>.yaml`, `config/rbac/role.yaml`,
`config/webhook/manifests.yaml` and `api/v1/applyconfiguration/...` [run]. CAPV commits all generated files [CAPV]; prefer doing the same and failing CI when `go generate ./...` leaves a diff.

| Generator | Reads | Writes |
|---|---|---|
| `object` | `+kubebuilder:object:generate=true` (package or type), `+kubebuilder:object:root=true` | `zz_generated.deepcopy.go` beside the types |
| `crd` | types + validation markers | one CRD per root kind |
| `rbac:roleName=X` | `+kubebuilder:rbac` anywhere under `paths` | `role.yaml` (ClusterRole, plus a Role per `namespace=`) |
| `webhook` | `+kubebuilder:webhook` | partial `{Validating,Mutating}WebhookConfiguration` |
| `applyconfiguration` | `+kubebuilder:ac:generate=true` | `applyconfiguration/` next to the API package |

Rules for `paths=` and `output:` [CT, run]:
- `paths=` takes a package pattern and may be repeated (`paths=./api/... paths=./internal/...`); narrow it per generator so webhook and RBAC markers are read only where you put them.
- `rbac` REQUIRES `roleName=` (`unable to parse option "rbac": missing argument "roleName"`).
- Without `output:` the defaults are `config/crd` and `config/rbac` (note: not `config/crd/bases`) [run].
- `output:<gen>:artifacts:config=DIR` sets one generator; `output:dir=DIR` sends every YAML to one directory; `output:stdout` prints.
- `object` writes `zz_generated.deepcopy.go` beside the sources unless you pass `output:dir=` (then it goes there) [run]; `applyconfiguration` writes next to the API package, or where `ac:output:package` says [run].
- `controller-gen crd -www` lists every marker for a generator with docs; `-hhh` gives the long flag help. Use these instead of memory.

## applyconfiguration (for server-side apply)

Needed only when several actors own different fields (see `reconcile.md`). Requirements, all learned by running it:

1. `+kubebuilder:ac:generate=true` on the PACKAGE comment AND on each root type; with only the package marker you get an empty `utils.go` and `internal/internal.go` [run, 09 check 3].
2. `SchemeGroupVersion = GroupVersion` in the API package: the generated `utils.go` references `v1.SchemeGroupVersion`; without it `go build` fails with `undefined: v1.SchemeGroupVersion` [run].
3. Default output is `api/v1/applyconfiguration/api/v1/widget.go` (the `api/v1` nesting is odd but real). `+kubebuilder:ac:output:package=../applyconfiguration` on the package moves it to `api/applyconfiguration/api/v1/` [run].
4. Generated structs use pointers and `omitempty` for every field, regardless of your `omitzero` API tags [run].

```go
import acv1 "example.com/demo/api/v1/applyconfiguration/api/v1"

ac := acv1.Widget("w1", "default").WithSpec(acv1.WidgetSpec().WithSize(demov1.Size("Small")))
err := c.Apply(ctx, ac, client.FieldOwner("widget-controller"), client.ForceOwnership)
err = c.Status().Apply(ctx, acv1.Widget("w1", "default").
	WithStatus(acv1.WidgetStatus().WithObservedGeneration(gen)), client.FieldOwner("widget-controller"), client.ForceOwnership)
```

Applied against envtest: `err=nil`, server default `replicas=1` appeared, status `observedGeneration=1` [run, CR `Client.Apply`].

## Markers (verified subset)

Syntax: `// +name`, `// +name=value`, `// +name:arg=v,arg2=v2`; slices `A;B`; regex in backticks [CT]. A marker line is never copied into the CRD description [run].

| Where | Marker | Verified result |
|---|---|---|
| package | `+kubebuilder:object:generate=true`, `+groupName=<group>`, `+kubebuilder:ac:generate=true` | deepcopy for all types; group name; ac generation [run] |
| root type | `+kubebuilder:object:root=true` (also on the List type), `+kubebuilder:subresource:status` | `subresources.status: {}` [run] |
| root type | `+kubebuilder:resource:path=ms,shortName=mm,scope=Cluster,categories=all` | `plural`, `shortNames`, `scope`, `categories` [run] |
| root type | ``+kubebuilder:printcolumn:name="Name",type=string,JSONPath=`.spec.name` `` | `additionalPrinterColumns` [run] |
| root type | ``+kubebuilder:selectablefield:JSONPath=`.spec.name` `` | `selectableFields` [run] |
| field | `+optional` / `+required` | `required:` list; `omitzero` counts like `omitempty`, so a field with neither tag option nor marker is required [run, 09 check 1] |
| field/type | `Enum=A;B`, `Minimum`, `MinLength`, `MaxLength`, `MinItems`, `MaxItems`, ``Pattern=`^[a-z]+$` `` | the matching schema keys [run] |
| field | `+listType=map`, `+listMapKey=type`, `+listType=set` | `x-kubernetes-list-type` / `-map-keys` [run] |
| field/type | `+kubebuilder:validation:XValidation:rule="self == oldSelf",message="..."` (CEL) | `x-kubernetes-validations` [run]; prefer it over a webhook for immutability and cross-field rules [KB] |
| field | `+kubebuilder:default=<value>` | `default:` [run] |

### `+default` versus `+kubebuilder:default` (resolved)

`+default=3`, `+default="Fast"` and `+default={"x": 1}` produced the same CRD `default:` as the `+kubebuilder:default` forms [run].
Differences from `controller-gen crd -www` and a run: bare `+default` accepts ONLY JSON (quoted strings, `[80,443]`, `{"policy":"delete"}`), while `+kubebuilder:default` also takes the relaxed form (`{policy: "delete"}`, `{80,443}`), and `+default=ref(...)` is silently ignored (no default emitted, no error) [run].
Prefer `+kubebuilder:default`, and set KAL `preferredDefaultMarker: kubebuilder:default` to match [CT, KAL].

### Doc comments and deviation notes

The CRD description is the comment group directly adjacent to the field/type. A `// Deviation: ...` note must sit in its OWN group above, separated by a blank line;
inside the doc group (even after `+optional`) or as a `// nolint:...` line it leaks into the CRD [run, 09 check 2]. Shape:

```go
// Deviation: pointer kept because 0 and unset mean different things here.

// replicas is the desired count.
// +optional
Replicas *int32 `json:"replicas,omitempty"`
```

## RBAC markers: placement and least privilege

```go
// +kubebuilder:rbac:groups=demo.example.com,resources=widgets,verbs=get;list;watch
// +kubebuilder:rbac:groups=demo.example.com,resources=widgets/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=demo.example.com,resources=widgets/finalizers,verbs=update

func (r *WidgetReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
```

- Leave a BLANK LINE between the marker group and the declaration. Markers placed directly on a `func` comment produced NO rule and no error; the same markers followed by a blank line worked [run]. CAPV does this too [CAPV].
- Keep markers next to the code that needs them (reconciler file) and list only verbs the code calls: watch means `get;list;watch`, a status patch needs `<res>/status` `patch`, finalizers need `<res>/finalizers` `update` [CT, KB].
- No wildcards: `groups=*,resources=*,verbs=*` is emitted verbatim without warning [run]. Use `groups=""` for the core group (`groups=core` is normalised to `""` [run]).
- `resourceNames=` limits get/update/patch (not create) [CT docs]; `namespace=ns` produces a `Role` in `role.yaml` for that namespace [run]; `urls=/metrics` produces `nonResourceURLs: [/metrics]` [run]. Example of a scoped rule: `groups=apiextensions.k8s.io,resources=customresourcedefinitions,resourceNames=widgets.demo.example.com,verbs=get;update;patch` [run, CAPV uses this].
- Webhook output keeps the placeholders `service: {name: webhook-service, namespace: system}` and has no `caBundle`; patch or template them in your own manifests [run].

## KAL: Kube API Linter for `api/`

KAL needs Go 1.26+ and has no release tags, so it resolves to a pseudo-version (`v0.0.0-20261012124120-79b557aa8c8d` at the time of writing) [KAL, run]. Install it in a separate module so the project `go.mod` stays clean (the tool pulls a 218-line `tools/go.mod` of golangci-lint dependencies; the main `go.mod` gained none of them) [run]:

```sh
mkdir -p tools && printf 'module example.com/demo/tools\n\ngo 1.26.0\n' > tools/go.mod
go get -tool -modfile=tools/go.mod sigs.k8s.io/kube-api-linter/cmd/golangci-lint-kube-api-linter@latest   # then commit the pseudo-version
go generate ./...        # KAL type-checks, so zz_generated.deepcopy.go must already exist
go tool -modfile=tools/go.mod golangci-lint-kube-api-linter run -c .golangci-kal.yml ./api/...
```

Exit code is 1 when there are findings, 0 with "0 issues." [run]. The minimum set below is the 11 linters all four reference repos enable
(openshift/api, gateway-api, CAPI, CAPV) plus the 7 that openshift/api, CAPI and CAPV enable (the eighth, `forbiddenmarkers`, is the ban on defaults and is left out because defaults are allowed here) plus `defaults` and `defaultorrequired` [KAL, OCP, CAPI, CAPV].
`.golangci-kal.yml` (run against the sample API: 0 issues on conforming types, 16 on a deliberately bad file):

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
            disable: ["*"] # explicit opt-in: new KAL linters never appear on upgrade
            enable:
              # all four reference repos
              - duplicatemarkers
              - jsontags
              - nodurations
              - nofloats
              - nomaps
              - nonullable
              - notimestamp
              - optionalorrequired
              - ssatags
              - statussubresource
              - uniquemarkers
              # openshift/api, CAPI, CAPV
              - commentstart
              - conditions
              - integers
              - maxlength
              - nobools
              - optionalfields
              - requiredfields
              # defaults allowed, but well-formed
              - defaults
              - defaultorrequired
          lintersConfig:            # key is lintersConfig, not linterConfig (the upstream docs have the typo)
            conditions:
              isFirstField: Warn
              usePatchStrategy: Forbid   # CRDs do not use patchStrategy
              useProtobuf: Forbid        # CRDs do not use protobuf tags
            optionalfields:
              pointers:
                preference: WhenRequired # non-pointer when the zero value is invalid
                policy: SuggestFix
              omitempty: {policy: SuggestFix}
              omitzero: {policy: SuggestFix}
            requiredfields:
              pointers: {policy: SuggestFix}
              omitempty: {policy: SuggestFix}
              omitzero: {policy: SuggestFix}
            defaults:
              preferredDefaultMarker: kubebuilder:default
  exclusions:
    generated: strict
    rules:
      - linters: [kubeapilinter]
        path-except: "^api/"
issues:
  max-issues-per-linter: 0
  max-same-issues: 0
  uniq-by-line: false # otherwise several diagnostics on one field line collapse into one
```

Example findings on the bad file (`bool`, `metav1.Duration`, `float64`, no marker, default on a required field):

```
bad_types.go:9:2:  nobools: field BadSpec.Enabled should not use a bool. Use a string type with meaningful constant values as an enum.
bad_types.go:13:2: nodurations: field BadSpec.Timeout should not use a Duration. Use an integer type with units in the name ...
bad_types.go:17:2: nofloats: field BadSpec.Ratio should not use a float value because they cannot be reliably round-tripped.
bad_types.go:20:2: optionalorrequired: field BadSpec.Note must be marked as optional or required
bad_types.go:25:2: defaultorrequired: field BadSpec.Mode has conflicting markers: default_or_required: {[kubebuilder:default], [required]} ...
```

What `preference: WhenRequired` demands, all observed [run]: an optional `int64` needs `Minimum=1` (or a pointer); an optional struct with `omitzero` needs `MinProperties=1` on the struct or a pointer;
comments must start with the json name (`// replicas is ...`, from `commentstart`); `Conditions` must be the first status field; strings need `MaxLength` (an `Enum` is enough), lists `MaxItems`.
Silence one finding with `//nolint:kubeapilinter // reason` on the field line [CAPV] or an `exclusions.rules` entry with `path` and `text` [CAPI]; the two defaults CAPI grandfathered use the latter.
To ban defaults instead (CAPI/CAPV policy), drop `defaults` and `defaultorrequired` and enable `forbiddenmarkers` with `markers: [{identifier: kubebuilder:default}, {identifier: default}]` [CAPI].

## Hazards

- RBAC markers glued to a `func`/type comment generate nothing and raise no error; the controller then fails with `forbidden` at runtime. Separate the group with a blank line and diff `role.yaml` in review [run].
- `+default=ref(...)` and non-JSON `+default=` values are silently dropped; the CRD ships without the default. Use `+kubebuilder:default` and check the generated schema [run, CT docs].
- `// Deviation:` text, `// nolint:` lines or `/* */` comments inside a doc group become CRD descriptions. Keep them in a separate group above [run].
- Skipping `+kubebuilder:ac:generate=true` or `SchemeGroupVersion` leaves `applyconfiguration/` empty or non-compiling [run].
- KAL `--fix` is mechanical: it turned `Mode string` into `*string` with the tag `json:"mode,omitempty,omitempty"` and added `omitempty,omitzero` to a Duration tag. Review every fixed line [run].
- `forbiddenmarkers` enabled with an empty `markers:` list is a hard config error, and the upstream docs spell the key `linterConfig:`; KAL rejects it with `unknown field "linterconfig"` (the key is `lintersConfig`) [KAL, run].
- `issues.uniq-by-line` left at its default hides all but the first KAL finding per field line [run].
- Keep `.golangci-kal.yml` in the module root and run from there: with the same file in `/tmp`, `path-except: "^api/"` matched nothing and KAL reported "0 issues" on a file with 16 violations [run].
- A controller-gen minor that differs from the `k8s.io/*` minor is unsupported and may generate against wrong types [CT README].
