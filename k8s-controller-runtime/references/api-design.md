# API design: CRD Go types and markers

Read this when you add or change a Go API type, a `+kubebuilder:` marker or a CEL rule, or when you review the generated CRD.
Install and config of controller-gen and the linter are in `codegen-workflow.md`; conditions are in `status-conditions.md`.

Tags: `run` = reproduced with controller-gen v0.23.0, kube-api-linter (KAL) 79b557a and an envtest apiserver. Others are listed in `sources.md`.
"Prefer"/"Avoid" are advice: break them only with a written reason. "Hazards" are factual bugs.

## Reference types (excerpt of types that compile, generate and pass the KAL set below)

```go
// Mode selects how fast a Gadget processes work.
// +kubebuilder:validation:Enum=Fast;Slow
type Mode string

// +kubebuilder:validation:MinProperties=1
type GadgetSpec struct {
	// mode selects the processing speed.
	// When omitted, Fast is used.
	// +kubebuilder:default=Fast
	// +optional
	Mode Mode `json:"mode,omitempty"`

	// storageClass names the class used for data volumes. It cannot change once set.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=253
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="storageClass is immutable"
	// +optional
	StorageClass string `json:"storageClass,omitempty"`

	// Deviation: pointer kept because 0 (paused) and unset (use the default) differ here.

	// replicas is the desired number of workers. 0 pauses the Gadget.
	// +kubebuilder:validation:Minimum=0
	// +optional
	Replicas *int32 `json:"replicas,omitempty"`

	// tls configures TLS for the Gadget endpoint.
	// +optional
	TLS TLSConfig `json:"tls,omitzero"`

	// ports lists the exposed ports.
	// +listType=map
	// +listMapKey=name
	// +kubebuilder:validation:MaxItems=16
	// +optional
	Ports []Port `json:"ports,omitempty"`
}

// TLSConfig configures TLS.
// +kubebuilder:validation:MinProperties=1
type TLSConfig struct {
	// minVersion is the lowest accepted TLS version.
	// +kubebuilder:validation:Enum=TLS12;TLS13
	// +optional
	MinVersion string `json:"minVersion,omitempty"`
}

// Port is an exposed port.
type Port struct {
	// name identifies the port within the Gadget.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=15
	// +required
	Name string `json:"name,omitempty"`

	// protocol is TCP or UDP. It cannot change once set.
	// +kubebuilder:validation:Enum=TCP;UDP
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="protocol is immutable"
	// +required
	Protocol string `json:"protocol,omitempty"`
}

// GadgetStatus is the observed state of a Gadget.
// +kubebuilder:validation:MinProperties=1
type GadgetStatus struct {
	// conditions represent the latest observations of the Gadget state.
	// +listType=map
	// +listMapKey=type
	// +kubebuilder:validation:MaxItems=32
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// observedGeneration is the metadata.generation last reconciled successfully.
	// +kubebuilder:validation:Minimum=1
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`

// Gadget is the Schema for the gadgets API.
type Gadget struct {
	metav1.TypeMeta `json:",inline"`
	// metadata is the standard object metadata.
	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`
	// spec is the desired state of the Gadget.
	// +required
	Spec GadgetSpec `json:"spec,omitempty,omitzero"`
	// status is the observed state of the Gadget.
	// +optional
	Status GadgetStatus `json:"status,omitempty,omitzero"`
}
```

These types are verbatim: they compile, generate, and pass the KAL minimum set with 0 issues (`research/experiments/exp-kal-gadget`). Note what makes the root `Gadget`
wrapper pass `commentstart` and `requiredfields`: each field has a doc comment starting with its JSON name, `Spec` is `+required` with `omitempty` and
`GadgetSpec` has `MinProperties=1` so its empty object is not a valid submission (that completeness is what lets `Spec` stay a non-pointer). `[KAL, CT, run]`

The generated schema (`config/crd/demo.example.com_gadgets.yaml` in the same experiment) carries `default: Fast`,
`minProperties: 1` on `spec`, `tls` and `status`, `x-kubernetes-list-type: map`, `required: [spec]`, the
`status: {}` subresource and the printer column; the deviation note is absent from the `replicas` description. `[CT, run]`

## Prefer

**Spec and status**
- Prefer spec for desired state and status for observed state, with the status subresource on (`+kubebuilder:subresource:status`); write status only through `Status()`. A status write does not bump `metadata.generation`. `[API, K8S, run]`
- Avoid a `phase` enum (adding values breaks clients); use conditions. Avoid large or fast-changing data in status; use another object. `[API]`

**Booleans and enums**
- Think twice about `bool`: many start boolean and grow into mutually exclusive options. Prefer a string type with named constants and `+kubebuilder:validation:Enum`; CamelCase values; state whether the set is open. `[API]` OpenShift forbids booleans outright. `[OCP]`
- `nobools` is in the KAL set. A justified bool gets a deviation note and a trailing `//nolint:kubeapilinter // reason` on the field line (not in the doc group; trailing comments never reach the CRD). `[KAL, CT, run]`

**Pointers**
- Avoid pointers for optional fields: use a value type, `+optional`, and `omitempty` (scalars, slices, maps) or `omitzero` (structs, `metav1.Time`, `metav1.Duration`). `omitempty` never omits structs, Duration or Time; `omitzero` does (Go 1.24+). `[OCP, run]`
- Make the zero value provably invalid so KAL accepts the value type (`optionalfields.pointers.preference: WhenRequired`): `Enum`, `MinLength=1`, `Minimum=1`, or `MinProperties=1` on the struct type. `[KAL, run]`
- Keep a pointer, with a deviation note, when zero and unset differ: numerics with `Minimum=0` (KAL flags the value type), bools, or a struct whose empty value is meaningful. `[OCP, KAL, run]`
- Deviation note format: its own comment group above the doc comment, separated by a blank line (see Hazards). `[CT, run]`

**Defaults**
- Prefer `+kubebuilder:default` over defaulting in code when the default is a stable part of the contract. Each one gets a justification in the doc comment ("When omitted, Fast is used.") because defaults also apply when reading old objects from etcd. `[API, K8S]`
- The field must be `+optional` with `omitempty`/`omitzero` and never `+required` (KAL `defaults`, `defaultorrequired`). `+default=` and `+kubebuilder:default=` generate the same schema. `[KAL, CT, run]`
- Documented alternative: ban schema defaults, default in the controller or webhook and document it, to keep a clear signal of user intent. `[CAPI, CAPV]`

**Validation**
- Bound everything: `MaxLength`, `MaxItems`, `MaxProperties`. KAL `maxlength` enforces it. `[K8S, KAL]`
- Prefer CEL (`+kubebuilder:validation:XValidation:rule=...,message=...`; also `reason`, `fieldPath`, `messageExpression`, `optionalOldSelf`) over a webhook for rules that look at one object. `[CT, K8S, CAPV]`
- Immutable field: `self == oldSelf` on the field. Set-once on an optional field: a rule on the parent, `!has(oldSelf.f) || self.f == oldSelf.f`. `[K8S, CAPV, run]`
- Put enum and validation markers on a named type so every use shares them. `[KB]`
- Cross-object checks, mutation and defaulting that CEL cannot express go to webhooks (`webhooks.md`).

**Tightening validation on a shipped version (ratcheting)**
- The API server accepts an update whose validation failure was already wrong in the old object and is unchanged by
  the update; you cannot use it to make a valid object invalid. Stable since v1.33 (the `CRDValidationRatcheting`
  gate is needed only before v1.30). So CEL rules and value constraints (`enum`, `pattern`, `min`/`max`, lengths)
  can be added to a live version without breaking updates of objects that already violate them: the violation is
  excused until that field is touched, the fix is always accepted, a reintroduction is rejected. Excusing is per
  failing field (other spec fields may change) and covers status-subresource updates. `[K8S, run]`
- Not ratcheted: transition rules (`oldSelf`) - see Hazards - and shape changes: `required`, list-type and
  list-map-key changes, property add/remove/rename, `allOf`/`oneOf`/`anyOf`/`not`, removing `additionalProperties`.
  `[K8S, run]` `required` is the common trap: an object missing the new field still takes metadata-only updates,
  but every update that changes `spec` is rejected until the field is added (the repair itself is accepted). `[run]`
- Custom grandfathering with `optionalOldSelf=true` (the rule then also runs on create, with `oldSelf` an empty
  Optional): a raised minimum that keeps old small values -
  `rule="self >= 4 || (oldSelf.hasValue() && oldSelf.value() < 4)",optionalOldSelf=true`. `[K8S, CT, run]`

**Lists and maps**
- Every list gets `+listType`: `map` with `+listMapKey` for named items and conditions, `set` for unique scalars, `atomic` only when one owner replaces the whole list. Unannotated CRD lists are atomic. `[API, K8S]`
- Prefer a list of named objects over a map of objects; `map[string]string` is fine for labels-like data. Avoid `+nullable`. `[API]`
- Avoid nested lists/maps under CEL rules (cost). `[K8S]`

**Structure and naming**
- Nest related settings in a struct so you can extend it later (`nodeDrain{gracePeriodSeconds, timeoutSeconds}`); give each struct a required field or `MinProperties=1`. `[TALK, OCP]`
- Avoid embedding external or third-party API types (kubeadm config, `corev1.ObjectReference`): copy what you need and convert, so upstream bumps do not leak into your schema. `metav1` types are fine. `[TALK]`
- Only `int32`/`int64`, no floats, no `time.Duration`: use `fooSeconds`; `fooTime` not `fooTimestamp`. KAL: `integers`, `nofloats`, `nodurations`, `notimestamp`. `[API, KAL]`
- References: `fooRef` (`secretRef`), same namespace by default, handle a missing referent, never copy referent secret data into your object, events or logs. OpenShift omits the `Ref` suffix and references by group and resource. `[API, OCP]`
- Write the spec as "I want a ..." sentences; avoid vague names such as `ready`. `[TALK]`

**Doc comments**
- Every field has a doc comment starting with its JSON name (`mode selects ...`); state defaults, omission behavior and limits there as well as in markers (KAL `commentstart`). `[KAL, OCP]`
- The description is the doc group adjacent to the field/type, with `+marker` lines removed; text after a blank line in the same group is kept. Type-level markers may sit in a detached group above the type doc. `[CT, run]`

**Printer columns**
- `+kubebuilder:printcolumn` with a JSONPath; the Ready column above printed `Unknown` through the apiserver Table view. Keep default columns few. `[CT, run]`

## Linter: the KAL minimum set

Run kube-api-linter on `api/` only, with `disable: ["*"]` (explicit opt-in). Enabled: `duplicatemarkers jsontags nodurations nofloats nomaps nonullable notimestamp optionalorrequired ssatags statussubresource uniquemarkers` (all four reference repos), plus `commentstart conditions integers maxlength nobools optionalfields requiredfields` (three of four), plus `defaults defaultorrequired` (defaults allowed; `forbiddenmarkers` replaces both if you choose to ban defaults). Settings that matter: `optionalfields.pointers.preference: WhenRequired`, `defaults.preferredDefaultMarker: kubebuilder:default`, `conditions` `usePatchStrategy`/`useProtobuf: Forbid`. Install and the full config are in `codegen-workflow.md`. `[KAL, CAPI, CAPV, GW, OCP, run]`

## Read the generated schema

After every type change run controller-gen and read the CRD diff; users see the schema, not your Go. Check `required`, `default`, `minProperties`, `x-kubernetes-list-type`, `x-kubernetes-validations` and each `description`. Commit generated CRDs. Use `maxDescLen=0` only for CRDs too big to apply. `[TALK, CT]`

## Hazards

- A misspelled marker (`MaxLenth`) is ignored and controller-gen exits 0; the constraint silently disappears. Diff the CRD. `[CT, run]`
- A field with no `omitempty`/`omitzero` and no marker is required, and a pointer does not make it optional; always add `+optional` or `+required`. `[CT, run]`
- `omitempty` on a struct still serializes `{}`; with `MinProperties=1` or a required child the create is rejected (`spec.tls ... at least 1 properties`). Use `omitzero`. `[run]`
- `omitzero` does not omit a non-nil empty slice or map (`[]`/`{}` are sent); use `omitempty` for slices and maps, or keep them nil. `[run]`
- controller-gen v0.23.0 rejects `MinProperties` on a struct-typed field (`must apply minproperties to an object`); put it on the struct type. `[CT, run]`
- `self == oldSelf` on an optional field does not stop users removing it; use the parent set-once rule or make the field required. `[run]`
- A transition rule (`oldSelf`) under an atomic or set list fails at CRD creation (`oldSelf cannot be used on the uncorrelatable portion of the schema`); use `listType=map`. `[K8S, run]`
- A transition rule that pre-existing objects already fail bricks them: every update is rejected, including
  metadata-only updates and the repair itself (the old value stays the failing one). Fix the rule or grandfather
  with `optionalOldSelf`. `[run]`
- Transition rules never run on create: objects can be created directly into a state no update can leave. `[K8S, run]`
- CEL that iterates an unbounded list is rejected as over budget (`estimated rule cost exceeds budget by factor of more than 100x`); add `maxItems`, and `maxLength` on the strings inside it. A rule over a single unbounded string (`==`, `matches`, `contains`) does not trip the check on v1.37. `[K8S, run]`
- A `// Deviation:` or `// nolint:` line inside the doc group, even after `+optional` or a blank `//`, leaks into the CRD description. Use a separate group above, or a trailing comment. `[CT, run]`
- Changing a list from atomic to map after release is not recommended; decide `listType` up front. `[K8S]`
