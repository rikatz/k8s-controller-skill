# Admission webhooks (defaulting and validation)

Read this when you think you need a mutating or validating webhook, or when you add one to a pure controller-runtime + controller-gen
project. Tags are explained in `sources.md`; `[run]` = reproduced in a scratch envtest project (controller-runtime v0.25.2, controller-tools v0.23.0).
Conversion webhooks and multi-version CRDs are out of scope: see `WithConverter` and `pkg/conversion` (Hub/Spoke) in controller-runtime.

## Decide first: do you need a webhook?
Walk this list top to bottom and stop at the first that works. [K8S, API]
1. Schema markers on the types: enums, `Minimum`/`Maximum`, `MaxLength`, patterns, required/optional.
2. `+kubebuilder:default=...` for defaults. Schema defaults are applied on request, again after mutating webhooks, and on read from
   etcd, so objects stored before you added the default also get it. A webhook only runs on writes. [K8S]
3. CEL rules (`+kubebuilder:validation:XValidation`) for cross-field rules and transition rules such as immutability. They run in the API
   server: no pod, certificate or port to keep alive. [K8S, API]
4. `ValidatingAdmissionPolicy` for policy that needs request context or parameters. [K8S]
5. A webhook only for what the above cannot express: lookups of other objects or external systems, or a default that depends on other fields.

- Prefer markers over a defaulting webhook. Use webhook defaulting only with a documented reason (write it next to the `Default` method).
- Prefer CEL for immutability; it is evaluated on update only. Verified: changing `spec.size` was rejected with "size is immutable" [run]
  ```go
  // size is immutable once set.
  // +kubebuilder:validation:XValidation:rule="self == oldSelf",message="size is immutable"
  // +required
  Size Size `json:"size"`
  ```
- Avoid a webhook that duplicates a schema or CEL rule: two places to keep in sync, and an outage of the webhook blocks writes the schema would accept (below).

## The generic API (v0.23+)
Compiled and run in envtest (imports omitted). The handlers take `*T`, so there are no type assertions.
```go
// +kubebuilder:webhook:path=/mutate-demo-example-com-v1-widget,mutating=true,failurePolicy=fail,sideEffects=None,groups=demo.example.com,resources=widgets,verbs=create;update,versions=v1,name=mwidget.demo.example.com,admissionReviewVersions=v1
// +kubebuilder:webhook:path=/validate-demo-example-com-v1-widget,mutating=false,failurePolicy=fail,sideEffects=None,groups=demo.example.com,resources=widgets,verbs=create;update,versions=v1,name=vwidget.demo.example.com,admissionReviewVersions=v1

// WidgetWebhook implements both interfaces for *demov1.Widget.
type WidgetWebhook struct{}

var (
	_ admission.Defaulter[*demov1.Widget] = &WidgetWebhook{}
	_ admission.Validator[*demov1.Widget] = &WidgetWebhook{}
)

func SetupWidgetWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr, &demov1.Widget{}).
		WithDefaulter(&WidgetWebhook{}).
		WithValidator(&WidgetWebhook{}).
		Complete()
}

// Default fills only unset fields. A marker default cannot express "depends on size".
func (*WidgetWebhook) Default(_ context.Context, w *demov1.Widget) error {
	if w.Spec.Tier == "" {
		w.Spec.Tier = "Shared"
		if w.Spec.Size == demov1.Size("Large") {
			w.Spec.Tier = "Dedicated"
		}
	}
	return nil
}

func (*WidgetWebhook) ValidateCreate(_ context.Context, w *demov1.Widget) (admission.Warnings, error) {
	return nil, validate(w)
}

func (*WidgetWebhook) ValidateUpdate(_ context.Context, _, w *demov1.Widget) (admission.Warnings, error) {
	return nil, validate(w)
}

func (*WidgetWebhook) ValidateDelete(context.Context, *demov1.Widget) (admission.Warnings, error) {
	return nil, nil
}

// validate holds only the rule CEL cannot express: it consults another field's meaning.
func validate(w *demov1.Widget) error {
	var errs field.ErrorList
	if w.Spec.Size == demov1.Size("Large") && w.Spec.Replicas != nil && *w.Spec.Replicas > 10 {
		errs = append(errs, field.Invalid(field.NewPath("spec", "replicas"), *w.Spec.Replicas, "Large widgets allow at most 10 replicas"))
	}
	if len(errs) == 0 {
		return nil
	}
	return apierrors.NewInvalid(demov1.GroupVersion.WithKind("Widget").GroupKind(), w.Name, errs)
}

```
- `ctrl.NewWebhookManagedBy(mgr, &T{})` is generic since v0.23. `admission.CustomValidator`, `CustomDefaulter`, `WithCustomValidator` and
  `WithCustomDefaulter` are deprecated aliases of the generic types; setting both a `Defaulter` and a `CustomDefaulter` makes `Complete()`
  fail ("only one of Defaulter or CustomDefaulter can be set"). [CR]
- Validation errors: return `apierrors.NewInvalid(gk, name, field.ErrorList)`; the client saw
  `spec.replicas: Invalid value: 11: Large widgets allow at most 10 replicas` [run]. Return `admission.Warnings` for non-blocking notes.
- `Default` is not called for DELETE and must only fill unset values; make it idempotent. [CR, API]
  `admission.DefaulterRemoveUnknownOrOmitableFields` is available as an option to `WithDefaulter`. [CR]
- Panics in a handler are recovered by default; `RecoverPanic(false)` on the builder changes that. [CR]

## Markers and paths
- Generated paths are `/mutate-<group, dots to dashes>-<version>-<lowercase kind>` and `/validate-...`; the core group gives a double dash
  (`/mutate--v1-pod`). [CR] The `path=` in the controller-gen marker must equal the path the builder registers. Custom paths via
  `WithValidatorCustomPath` / `WithDefaulterCustomPath` (must match `^((/[a-zA-Z0-9-_]+)+|/)$`) need the same path in the marker. [CR]
- One marker per webhook: `mutating=true` for the defaulter, `mutating=false` for the validator. Generate the configuration with
  `go tool controller-gen webhook paths=./... output:webhook:artifacts:config=config/webhook`; it produced one
  `MutatingWebhookConfiguration` and one `ValidatingWebhookConfiguration`. [run, CT]
- The output is partial: the `clientConfig.service` is the placeholder `webhook-service` in namespace `system` and there is no `caBundle`. [run]
  Rename the service and inject the CA in the manifests you own.

## Certificates and the server are hand-written
controller-gen does not create certificates, the Service, volume mounts or CA injection. [CT, run (manifest has none)]
- Provide a Service for the webhook port, a serving certificate mounted into the pod, and a way to put its CA in the configuration's `caBundle`
  (for example cert-manager or the OpenShift service-ca operator; the exact annotations are [unverified] here, check their docs).
- Server options: `webhook.NewServer(webhook.Options{Port, CertDir, CertName, KeyName})` passed as `ctrl.Options{WebhookServer: ...}`.
  Defaults: port 9443, files `tls.crt` and `tls.key`, directory `<os.TempDir()>/k8s-webhook-server/serving-certs`. Set `CertDir`
  explicitly to your mounted secret. The server reads the files through a certificate watcher (read in code; reload not exercised). [CR]
- The webhook server is never leader-elected and starts before the caches. Use `mgr.GetWebhookServer().StartedChecker()` as a readiness check. [CR, CAPI]
- `Port: -1` disables the server (v0.25): with a registered webhook the manager still started and ran without error [run]. Use it for binaries
  or tests that do not serve webhooks.

## Test in envtest
Point `WebhookInstallOptions` at the generated directory; envtest rewrites each `clientConfig` to the local server (URL plus a CA bundle) so no
Service or cert-manager is needed. Give the manager the same host, port and cert directory. [CR, run]
```go
env := &envtest.Environment{
	CRDDirectoryPaths:     []string{filepath.Join("..", "config", "crd", "bases")},
	ErrorIfCRDPathMissing: true,
	DownloadBinaryAssets:  true,
	WebhookInstallOptions: envtest.WebhookInstallOptions{Paths: []string{filepath.Join("..", "config", "webhook")}},
}
// after env.Start():
w := &env.WebhookInstallOptions
opts.WebhookServer = webhook.NewServer(webhook.Options{Host: w.LocalServingHost, Port: w.LocalServingPort, CertDir: w.LocalServingCertDir})
```
- Assert on the object the API server stored (defaulted `spec.tier`, rejection error text), not on handler calls. Retry the first request until the
  server answers (the manager runs in a goroutine). The full harness is in `testing-envtest.md`.

## Hazards
- Marker `path` differs from the registered path: every matching write fails with `failed calling webhook ...: the server could not find the requested resource` (`failurePolicy=Fail`). [run]
- `failurePolicy=Fail` makes every matching write depend on the webhook being up and trusted: server down, wrong certificate or missing
  `caBundle` rejects the write. Use `Ignore` only for non-critical rules and say why. [K8S, run (Internal error on call failure)]
- A schema default and a webhook default on the same field: schema defaulting runs before the webhook, so the webhook sees the field already
  set (observed with `replicas` default 1) and its default is dead code. Default a field in one place. [run, K8S]
- A webhook `Default` that overwrites set values (instead of filling unset ones) fights users and other controllers and can loop. [API]
- Both `WithDefaulter` and `WithCustomDefaulter` (or the validator pair) on one builder fail at `Complete()`. [CR]
