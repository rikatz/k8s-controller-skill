# API ratcheting: tightening validation on a shipped API version

Question (user, 2026-10-15): when a new validation would break existing objects but should apply "just on updates",
does the platform give us anything? Answer: yes - CRD validation ratcheting - but the shipped skill did not cover it
(grep found it only in note 03, never in `k8s-controller-runtime/`).

Legend: [V] = read from the cited source this session. [run] = reproduced in `research/experiments/exp-ratcheting`
(envtest 1.37.0, controller-tools v0.23.0, plain `go test`; full log quoted below).

Source: CRD-DOC = https://kubernetes.io/docs/tasks/extend-kubernetes/custom-resources/custom-resource-definitions/
("Validation ratcheting" and "Transition rules" sections, fetched 2026-10-15). No KEP number cited: the
feature-gates page does not list one, and the KEP number suggested by a summarizer (4009) turned out to be an
unrelated device-plugin KEP.

## Mechanism [V, CRD-DOC]

> "The API server is willing to accept updates to resources that are not valid after the update, provided that each
> part of the resource that failed to validate was not changed by the update operation. In other words, any invalid
> part of the resource that remains invalid must have already been wrong. You cannot use this mechanism to update a
> valid resource so that it becomes invalid."

> "This feature allows authors of CRDs to confidently add new validations to the OpenAPIV3 schema under certain
> conditions. Users can update to the new schema safely without bumping the version of the object or breaking workflows."

Versions: "first available in the v1.28 release"; "Stable since Kubernetes v1.33; enabled by default"; clusters older
than v1.30 need the `CRDValidationRatcheting` feature gate. [V, CRD-DOC] On our target (1.37) it is always on.

Docs' NOT-ratcheted list ("under the implementation in Kubernetes 1.37"): quantors (`allOf`/`oneOf`/`anyOf`/`not` and
their descendants), `x-kubernetes-list-type` and `list-map-keys` changes, `required` changes, property
add/remove/rename, removing `additionalProperties`, built-in `metadata` validation; and "Transition Rules are never
ratcheted: only errors raised by rules that do not use `oldSelf` will be automatically ratcheted if their values are
unchanged". `enum` is NOT in the exception list. [V, CRD-DOC]

## Verified behaviour [run]

Method: install a loose CRD; create objects that violate future tightenings; swap in tightened CRDs; observe.
Each stage replaces the whole schema. Assertions and real API messages in `exp-ratcheting/ratcheting_test.go`.

| Tightening added later | Create after | Update of pre-existing violating object |
|---|---|---|
| CEL rule `self <= 10` on `spec.size` | rejected (`spec.size: Invalid value: 101: size must be <= 10`) | **excused while the failing field is unchanged**: annotation-only update OK, spec-touching update that leaves `size` alone OK, status-subresource update OK; worsening (size=101) and reintroducing (size=100 after a fix) rejected; the fix itself OK. Per-field, not per-object. |
| `enum: [Fast, Slow]` on `spec.mode` | rejected (`spec.mode: Unsupported value: "Turbine": supported values: "Fast", "Slow"`) | **ratchets exactly like a CEL rule** (annotation-only OK; `spec.size` changed while failing `spec.mode` unchanged OK; valid=invalid rejected; fix OK; reintroduce rejected). The docs' exception list does not mention enum, and indeed it ratchets. |
| transition rule `oldSelf <= 10` on `spec.size` | **allowed** - transition rules never run on create, so a violating object can be created directly under the tightened CRD | **never ratcheted: every update is rejected** (`spec.size: Invalid value: 100: the previous size must have been <= 10`), including metadata-only updates AND the repair itself (old value stays the failing one). The object is bricked until the rule is changed. |
| `optionalOldSelf: true` rule `self >= 4 \|\| (oldSelf.hasValue() && oldSelf.value() < 4)` | rejected below the new minimum (rule runs on create; `oldSelf` is an empty Optional) | custom ratchet in the rule: old values below the new minimum grandfathered (updates and staying below OK); dropping below the new minimum from a valid value rejected. |
| `required: [size]` on `spec` | rejected (`spec.size: Required value`) | **partially ratcheted**: annotation-only update of an object missing the field OK, but ANY update that changes `spec` is rejected (even though the missing field is untouched); the repair (adding the field) OK. So the docs' "required changes are not ratcheted" means: not ratcheted at the failing field - only while the parent subtree is byte-identical. |

## Marker support [run]

controller-tools v0.23.0 supports `optionalOldSelf` on the XValidation marker
(`pkg/crd/markers/validation.go`: `OptionalOldSelf *bool \`marker:"optionalOldSelf,optional"\``). Marker:

```go
// +kubebuilder:validation:XValidation:rule="self >= 4 || (oldSelf.hasValue() && oldSelf.value() < 4)",message="size must be >= 4 unless the old value was below 4",optionalOldSelf=true
```

generates (controller-gen v0.23.0, `go tool controller-gen crd`):

```yaml
x-kubernetes-validations:
- message: size must be >= 4 unless the old value was below 4
  optionalOldSelf: true
  rule: self >= 4 || (oldSelf.hasValue() && oldSelf.value() < 4)
```

Marker help text: "The rule always runs; you must check oldSelf.hasValue() in the rule before using oldSelf."
Also confirmed this session: in k8s.io v0.37.1 the apiextensions clientset moved to
`k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset` (the old `.../clientset/versioned` path no longer exists).

## Hazards for the skill (each verified by the run)

1. **A transition rule (`oldSelf`) that pre-existing objects fail bricks them**: no update passes, including
   metadata-only updates and the repair. Fix the rule or grandfather explicitly with `optionalOldSelf`.
2. **Adding `required` blocks every spec-touching update** of objects missing the field, until repaired;
   metadata-only and status updates still pass.
3. **Adding an `oldSelf` rule does not constrain creates** - new objects can be created directly into a state no
   update can leave.
4. Docs nuance: value constraints (CEL without `oldSelf`, `enum`) ratchet per failing field; shape changes
   (required, list-type, list-map-keys, property add/remove/rename, quantors) do not (or only while the parent is
   identical).

## Note 03 correction

Note 03 said "NOT ratcheted: ... `required` changes ..." - refined by the run: `required` IS excused while the
failing subtree is unchanged (annotation-only updates pass); it blocks only spec-touching updates. The rest of
note 03's line (transition/oldSelf rules, quantors, list-type/map-key, property renames) matches the run and the docs.
