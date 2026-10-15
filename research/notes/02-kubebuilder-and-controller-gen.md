# 02 - Kubebuilder book (concepts only) + controller-tools/controller-gen

Legend: [V] = verified by running it locally (scratch module /tmp/cg-verify, go1.27.2, controller-runtime v0.25.2, controller-tools v0.23.0, k8s.io/* v0.37.1) or reading module source. [B] = from book page (URL given). [?] = unverified.
Book base: https://book.kubebuilder.io

## 1. Keep: general controller-runtime / controller-gen practice

### Reconcile design
- Reconcile must be idempotent; do not branch on event type (create/update/delete). Non-idempotent logic leaves resources stuck. [B] /reference/good-practices
- One controller per Kind; no "install_all" controller (isolation, concurrency, SRP). [B] /reference/good-practices
- Adopt status `Conditions` ([]metav1.Condition, types like Available/Progressing/Degraded, status True/False/Unknown). [B] /reference/good-practices, /cronjob-tutorial/api-design
- Follow upstream k8s API conventions. [B] /reference/good-practices
- Log style for controllers: capital first letter, no trailing period, past tense, name the object type, balanced KV pairs (`log.Info("Created Deployment", "name", d.Name)`). Go error strings stay lowercase. [B] /reference/good-practices (matches user's global standard)
- Get: NotFound => `return ctrl.Result{}, nil` (or `client.IgnoreNotFound(err)`); other errors returned for backoff retry. [B] /cronjob-tutorial/controller-implementation, /reference/using-finalizers
- Terminal/user-fixable errors (e.g. invalid spec): set Degraded condition, return `ctrl.Result{}, nil` (retry is pointless until spec edit). Transient errors: return err. [B] controller-implementation
- Status writes via `r.Status().Update(ctx, obj)`; status subresource marker required. Re-fetch before update on conflict paths. [B] controller-implementation, /reference/generating-crd
- `RequeueAfter` is an upper bound; other events can trigger earlier. [B] controller-implementation
- Owner refs: `ctrl.SetControllerReference(owner, child, r.Scheme)` (alias of controllerutil's) enables GC and event mapping; `For(&Parent{}).Owns(&Child{})` re-queues parent on child change. [B] controller-implementation
- Field indexer for owner lookup: `mgr.GetFieldIndexer().IndexField(ctx, &Job{}, key, func(o client.Object) []string{...})` then `List(..., client.InNamespace(ns), client.MatchingFields{key: name})`. [B] controller-implementation
- Finalizers: add (if DeletionTimestamp zero and not contained) + Update; on deletion, if contained: run idempotent cleanup, `RemoveFinalizer`, Update, return. Helpers `controllerutil.ContainsFinalizer/AddFinalizer/RemoveFinalizer`. Needs RBAC `<res>/finalizers` verb `update`. [B] /reference/using-finalizers
- API type design: spec = desired input, status = observed output; pointers where zero != unset; `int32/int64/resource.Quantity` for numbers, `metav1.Time` for time; camelCase json tags; root object uses `omitzero` for metadata/status (book's current style); `+listType=map` `+listMapKey=type` on Conditions. [B] /cronjob-tutorial/api-design
- Put enum/validation on a named type for reuse. [B] api-design, /reference/generating-crd
- ObservedGeneration: NOT mentioned on good-practices or api-design pages [B, absent]. Take from k8s API conventions/user's standard instead (I used `status.observedGeneration int64` in verified types; [V] compiles/generates).
- Custom metrics: package-level prometheus collectors, `metrics.Registry.MustRegister(...)` in `init()` from `sigs.k8s.io/controller-runtime/pkg/metrics`. [B] /reference/metrics
- Metrics server: `ctrl.Options{Metrics: metricsserver.Options{BindAddress: ":8443", SecureServing: true, FilterProvider: filters.WithAuthenticationAndAuthorization}}` - book shows FilterProvider assignment; `filters` pkg = `sigs.k8s.io/controller-runtime/pkg/metrics/filters` [?] (import path not on page; verify). Default self-signed cert not recommended for prod; authz needs TokenReview/SubjectAccessReview RBAC (book scaffolds metrics_auth_role). [B] /reference/metrics
- Leader election: just `LeaderElection: true`, `LeaderElectionID: "<unique>"` in ctrl.Options (book's main.go). The tutorial "running" page says nothing about it. [B] /cronjob-tutorial/empty-main. Needs RBAC on leases (coordination.k8s.io) - [?] not on fetched pages; add `+kubebuilder:rbac:groups=coordination.k8s.io,resources=leases,verbs=get;list;watch;create;update;patch;delete` plus events create/patch if recorder used.
- Namespace-restricted cache: `Cache: cache.Options{DefaultNamespaces: map[string]cache.Config{ns: {}}}`. [B] empty-main
- Health probes: `mgr.AddHealthzCheck("healthz", healthz.Ping)`, `mgr.AddReadyzCheck("readyz", healthz.Ping)`. [B] empty-main; [V] compiles.

### envtest (works without kubebuilder) [B] /reference/envtest
- Install `setup-envtest` from `sigs.k8s.io/controller-runtime/tools/setup-envtest` (separate module, not in main controller-runtime module dir [V]); `setup-envtest use <k8s-ver> -p path` -> `KUBEBUILDER_ASSETS=... go test ./...`. Exact `go install ...@latest` line [?] not verified by me.
- `envtest.Environment{CRDDirectoryPaths: []string{filepath.Join("..","config","crd","bases")}}`; `Start()`/`Stop()`; `WebhookInstallOptions` for webhooks; `UseExistingCluster: true`.
- Limits: no built-in controllers (no GC; assert OwnerReferences, not deletion); namespaces cannot be deleted (use unique namespace per test); cert-manager/Prometheus belong in e2e.
- Env vars: KUBEBUILDER_ASSETS, USE_EXISTING_CLUSTER, TEST_ASSET_KUBE_APISERVER/ETCD/KUBECTL, KUBEBUILDER_ATTACH_CONTROL_PLANE_OUTPUT.

### Webhooks [B] /cronjob-tutorial/webhook-implementation; signatures [V] from controller-runtime v0.25.2 source
- Builder (current, generic): `ctrl.NewWebhookManagedBy(mgr, &v1.Widget{}).WithDefaulter(d).WithValidator(v).Complete()`. [V] `WebhookBuilder[T]` with `WithDefaulter(admission.Defaulter[T], ...)`, `WithValidator(admission.Validator[T])`, `Complete()`. `CustomDefaulter`/`CustomValidator` are now Deprecated aliases of `Defaulter[runtime.Object]`/`Validator[runtime.Object]`. OLD style `NewWebhookManagedBy(mgr).For(obj)` is pre-0.22 [?] exact cutover version unverified.
- `Default(ctx, obj *T) error`; `ValidateCreate(ctx, obj) (admission.Warnings, error)`; `ValidateUpdate(ctx, old, new)`; `ValidateDelete(ctx, obj)`. Invalid -> `apierrors.NewInvalid(gk, name, field.ErrorList)`. [B]
- Custom paths `WithValidatorCustomPath/WithDefaulterCustomPath` exist [V]. Book says need controller-runtime >= v0.21 [B].
- Local run needs certs at `/tmp/k8s-webhook-server/serving-certs/tls.{crt,key}` (book); skip with ENABLE_WEBHOOKS=false is a scaffold main.go convention, NOT controller-runtime behavior [B/?].
- Webhook server: `webhook.NewServer(webhook.Options{Port: 9443})` as `ctrl.Options.WebhookServer`. [B]
- Marker: `// +kubebuilder:webhook:path=/validate-<group-dashed>-<ver>-<kindlower>,mutating=false,failurePolicy=fail,sideEffects=None,groups=g,resources=rs,verbs=create;update,versions=v1,name=v<kind>.kb.io,admissionReviewVersions=v1` (name needs >=3 dot segments; `.kb.io` is just convention). Defaults serviceName=webhook-service, serviceNamespace=system, port 443 - the generated config uses those, so hand-rolled kustomize/helm must rename/patch (controller-gen output is a partial config; `+webhookconfiguration` sets name/mutating). [B] /reference/markers/webhook

### Multi-version / conversion [B] /multiversion-tutorial/conversion-concepts, /conversion; interfaces [V]
- One Hub (storage/internal pivot) version; others are spokes converting only to/from hub (spoke->hub->spoke). API server calls a conversion webhook for CRDs.
- Hub: `func (*Widget) Hub() {}` (package `sigs.k8s.io/controller-runtime/pkg/conversion`; `Hub interface{runtime.Object; Hub()}` [V]).
- Spoke: `ConvertTo(dst conversion.Hub) error` / `ConvertFrom(src conversion.Hub) error` (`Convertible` interface [V]); type-assert `dstRaw.(*hubv1.Widget)`; copy ObjectMeta, spec, status.
- Mark storage version: `// +kubebuilder:storageversion` on exactly one version's root type [B generating-crd]. Also `+kubebuilder:unservedversion`, `+kubebuilder:skipversion`, `+kubebuilder:deprecatedversion:warning="..."` [B /reference/markers/crd].
- Webhook registration for conversion: `ctrl.NewWebhookManagedBy(mgr, &Widget{}).Complete()` style [?] (the conversion page did not show it); the CRD needs `spec.conversion.strategy: Webhook` + caBundle + service ref, which controller-gen does NOT generate (kubebuilder does via kustomize patches - SCAFFOLDING, must be hand-rolled) [B/?]. Conversion webhook path is `/convert` [?] not verified.

## 2. controller-gen without kubebuilder

### Install / pin [V]
- Preferred (Go >=1.24 `tool` directive): `go get -tool sigs.k8s.io/controller-tools/cmd/controller-gen@v0.23.0` writes `tool sigs.k8s.io/controller-tools/cmd/controller-gen` into go.mod and pins `sigs.k8s.io/controller-tools v0.23.0 // indirect`; run with `go tool controller-gen ...`. [V] (note: it pulls a long indirect dep list into go.mod; `go tool` builds & caches the binary.) Latest at time of test: v0.23.0 [V]; README table lags (lists v0.21 -> k8s v0.36) [B] https://github.com/kubernetes-sigs/controller-tools. 
- Alternatives (not run): `go run sigs.k8s.io/controller-tools/cmd/controller-gen@v0.23.0 ...` and `go install ...@v0.23.0`. [?]
- Compatibility rule: one controller-tools minor per k8s.io/* (client-go) minor; mismatch unsupported; min Go = max of deps. controller-tools v0.23 built against k8s.io v0.37 [V from go.mod]. Pin controller-tools to the same minor family as the k8s.io/* in go.mod. [B] https://github.com/kubernetes-sigs/controller-tools README
- `go build` inside a dir with no git: add `-buildvcs=false` (error "obtaining VCS status" seen) [V] - env quirk only.
- Generated CRD embeds annotation `controller-gen.kubebuilder.io/version: v0.23.0` [V].

### Generators and flags [B] /reference/controller-gen; `controller-gen -h` [V]
- Generators: `object`, `crd`, `rbac`, `webhook`, `applyconfiguration`, `schemapatch`. Run `controller-gen -h|-hhh`, `controller-gen crd -ww` for marker docs.
- `paths=./...` (or `{./api/...,./internal/...}`; a fs path ending `...` crosses nested modules).
- Output: default per generator `output:<gen>:artifacts:config=config/<gen>`; code stays beside source (zz_generated.deepcopy.go in the package). Overrides `output:crd:artifacts:config=config/crd/bases`, `output:rbac:artifacts:config=config/rbac`, `output:dir=`, `output:stdout`, `output:none`.
- Options: crd `crdVersions`, `maxDescLen=0` (drops descriptions, shrinks big CRDs), `generateEmbeddedObjectMeta`, `allowDangerousTypes`, `ignoreUnexportedFields`; rbac `roleName`, `fileName` (default role.yaml); `headerFile`, `year` on object/crd/rbac/webhook. `trivialVersions` in the book's example is stale (v1beta1 era) - do not use. [B]
- applyconfiguration: `controller-gen applyconfiguration paths=./api/...` -> `applyconfiguration/` subdir [V from -h text only, not run]; markers `+kubebuilder:ac:generate=true` (pkg/type), `+kubebuilder:ac:output:package=...`; kubebuilder's `--ssa` flag is scaffolding. [B] /reference/markers/applyconfiguration
- Book scaffold equivalents: `controller-gen object:headerFile="hack/boilerplate.go.txt" paths="./..."` (make generate) and `controller-gen rbac:roleName=manager-role crd webhook paths="./..." output:crd:artifacts:config=config/crd/bases` (make manifests) [B generating-crd; second line [V] minus webhook].

### Verified workflow (hand-rolled)
```
# one-time
go mod init example.com/demo
go get -tool sigs.k8s.io/controller-tools/cmd/controller-gen@v0.23.0
go get sigs.k8s.io/controller-runtime@latest && go mod tidy
# every change  (put in a go:generate in api/v1/groupversion_info.go or doc.go, or a Makefile/justfile target)
go tool controller-gen object paths=./api/...
go tool controller-gen crd rbac:roleName=manager-role webhook paths=./... \
    output:crd:artifacts:config=config/crd/bases \
    output:rbac:artifacts:config=config/rbac \
    output:webhook:artifacts:config=config/webhook
```
go:generate form: `//go:generate go tool controller-gen object paths=.` then `//go:generate go tool controller-gen crd paths=. output:crd:artifacts:config=../../config/crd/bases` [?] (go:generate wrapper not run; `go tool` itself [V]). Result [V]: `api/v1/zz_generated.deepcopy.go`, `config/crd/bases/demo.example.com_widgets.yaml`, `config/rbac/role.yaml` (ClusterRole merged/sorted, one rule group per marker group+resource). Webhook output not exercised [?].

### Marker syntax [B] /reference/markers
- `// +name` (empty bool), `// +name=value` (anonymous), `// +name:arg=v,arg2=v2` (multi). Slices: `Enum=A;B;C` or `{"a","b"}`; maps `{k: v}`; regex in backticks. `+optional` and `+kubebuilder:validation:Optional` both work; `+required` supported.
- Package-level (doc.go / groupversion_info.go): `+kubebuilder:object:generate=true`, `+groupName=demo.example.com`, `+versionName=v1` [B /reference/markers/crd], `+kubebuilder:skip`.
- Type-level: `+kubebuilder:object:root=true` (root + List types), `+kubebuilder:subresource:status`, `+kubebuilder:subresource:scale:specpath=.spec.replicas,statuspath=.status.replicas,selectorpath=.status.selector`, `+kubebuilder:resource:path=,singular=,shortName=a;b,categories=all,scope=Namespaced|Cluster`, `+kubebuilder:printcolumn:name="X",type=string,JSONPath=".spec.x"` (args JSONPath, description, format, name, priority, type), `+kubebuilder:selectablefield:JSONPath=`, `+kubebuilder:metadata:annotations/labels`, `+k8s:enum`.
- Validation: `Minimum/Maximum/ExclusiveMinimum/ExclusiveMaximum/MultipleOf`, `MinLength/MaxLength/Pattern/Format`, `MinItems/MaxItems/UniqueItems`, `MinProperties/MaxProperties`, `Enum=`, `Type=`, `XIntOrString`, `XEmbeddedResource`, `Schemaless` (last resort), `+nullable`, `+kubebuilder:validation:items:Minimum` for elements. Newer (page lists, not run): `ExactlyOneOf/AtMostOneOf/AtLeastOneOf`, `+k8s:immutable`. [B] /reference/markers/crd-validation
- CEL: `+kubebuilder:validation:XValidation:rule="self == oldSelf",message="field is immutable"`; args rule, message, messageExpression, reason, fieldPath, optionalOldSelf. Prefer CEL over a webhook for immutability/cross-field. [B]
- Defaults: `+kubebuilder:default=3`, `+kubebuilder:default="ClusterIP"`, `+kubebuilder:default={replicas: 1}`; `+default=<json>` also. [B]; `default=1` [V] produced `default: 1`.
- RBAC: `// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete`; core group = `groups=""`; subresources `deployments/status`; `namespace=` => Role; `resourceNames=`; `urls=/metrics`; `roleName=` [B] /reference/markers/rbac; core pieces [V].
- Object: `+kubebuilder:object:root=true`, `+kubebuilder:object:generate=true` (pkg or type), `+k8s:deepcopy-gen:interfaces=` / `+k8s:deepcopy-gen` also accepted [B].
- Status subresource: with it, spec updates ignore status and status updates ignore spec; recommended always. [B generating-crd]

## 3. Scaffolding-specific (DROP) vs keep

DROP (kubebuilder-only): `kubebuilder init/create api/create webhook`, PROJECT file (and /reference/project-config), Makefile targets (`make manifests/generate/install/deploy/envtest/docker-build`), `+kubebuilder:scaffold:*` marker comments in main.go, `--plugins`, `--ssa`, `--spoke`, `--defaulting`, `--programmatic-validation` flags, Deploy-Image plugin, `hack/boilerplate.go.txt` convention (optional), kustomize `config/default` overlays, cert-manager/Prometheus kustomize toggles, `make run` ENABLE_WEBHOOKS convention, `config/samples`, e2e test scaffold, `cmd/main.go` flag boilerplate defaults (:8080/:8081, secureMetrics) as such.
KEEP: everything in section 1 and 2 (controller-runtime APIs, markers, controller-gen CLI, envtest, conditions, finalizers, owner refs, conversion Hub/Spoke interfaces, webhook generics, metrics registry).
Book gaps: Architecture page is only a diagram (no text) [B /architecture]; groupversion page in tutorial = concept text only; "watching externally managed resources" URL 404'd (path moved) [?] - not read; Good Practices lacks ObservedGeneration/owner/finalizer/events text; leader election has no dedicated page I could fetch.

## 4. Layout kubebuilder would produce -> hand-rolled equivalent needs
(kubebuilder layout from memory + fetched pages: [?] directory names beyond those cited are not verified from an init run.)
- `cmd/main.go` (book: cmd/main.go [B good-practices]) - manager, scheme, flags, health, leader election, register reconcilers/webhooks.
- `api/<version>/` (or `api/<group>/<version>/` for multi-group): `*_types.go`, `groupversion_info.go`, `zz_generated.deepcopy.go` [V]. Multi-group layout: `api/<group>/<version>` and controllers `internal/controller/<group>` [?].
- `internal/controller/*_controller.go` (+ `suite_test.go` envtest) [V for controller file compile].
- `internal/webhook/<version>/` for defaulter/validator [?].
- `config/crd/bases` [V generated], `config/rbac/role.yaml` [V] (+ ServiceAccount, RoleBinding/ClusterRoleBinding, leader-election Role - NOT generated by controller-gen, hand-write), `config/webhook/manifests.yaml` (generated; Service + certs hand-written), `config/manager/manager.yaml` (Deployment, hand-write), `config/samples`.
- `Dockerfile` (multi-stage `go build ./cmd`, distroless nonroot), optional `hack/boilerplate.go.txt` passed as `object:headerFile=`.
- `.gitignore`/`bin/` for tool binaries is unnecessary with `go tool`.

## 5. Minimal pure controller-runtime main.go [V compiled against controller-runtime v0.25.2]
groupversion_info.go (without scheme.Builder, book's version uses `runtime.NewSchemeBuilder`; I verified the `sigs.k8s.io/controller-runtime/pkg/scheme` Builder variant):
```go
// +kubebuilder:object:generate=true
// +groupName=demo.example.com
package v1

import (
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/scheme"
)

var (
	GroupVersion  = schema.GroupVersion{Group: "demo.example.com", Version: "v1"}
	SchemeBuilder = &scheme.Builder{GroupVersion: GroupVersion}
	AddToScheme   = SchemeBuilder.AddToScheme
)
// in types file:  func init() { SchemeBuilder.Register(&Widget{}, &WidgetList{}) }
```
main.go:
```go
var scheme = runtime.NewScheme()
func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(demov1.AddToScheme(scheme))
}
func main() {
	var metricsAddr, probeAddr string
	var leaderElect bool
	flag.StringVar(&metricsAddr, "metrics-bind-address", ":8080", "Metrics endpoint address")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "Probe endpoint address")
	flag.BoolVar(&leaderElect, "leader-elect", false, "Enable leader election")
	opts := zap.Options{}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()
	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))
	setupLog := ctrl.Log.WithName("setup")
	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:                 scheme,
		Metrics:                metricsserver.Options{BindAddress: metricsAddr},
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         leaderElect,
		LeaderElectionID:       "widget.demo.example.com",
	})
	if err != nil { setupLog.Error(err, "Unable to create manager"); os.Exit(1) }
	if err := (&controller.WidgetReconciler{Client: mgr.GetClient()}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "Unable to create controller", "controller", "Widget"); os.Exit(1)
	}
	_ = mgr.AddHealthzCheck("healthz", healthz.Ping)
	_ = mgr.AddReadyzCheck("readyz", healthz.Ping)
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil { setupLog.Error(err, "Problem running manager"); os.Exit(1) }
}
```
Imports: `k8s.io/apimachinery/pkg/runtime`, `utilruntime "k8s.io/apimachinery/pkg/util/runtime"`, `clientgoscheme "k8s.io/client-go/kubernetes/scheme"`, `ctrl "sigs.k8s.io/controller-runtime"`, `.../pkg/healthz`, `.../pkg/log/zap`, `metricsserver ".../pkg/metrics/server"`. Full verified project: /tmp/cg-verify (types with Conditions+observedGeneration, enum/default/minimum markers, rbac markers, reconciler, generated CRD+role).
Controller: `ctrl.NewControllerManagedBy(mgr).For(&v1.Widget{}).Named("widget").Complete(r)`; `Owns`, `Watches` exist on builder [V]. Use `logf.FromContext(ctx)` (`sigs.k8s.io/controller-runtime/pkg/log`) [V].
Stripped for brevity (add for prod): `ctrl.Options.LeaderElectionReleaseOnCancel`, secure metrics (`metricsserver.Options{SecureServing:true, FilterProvider: filters.WithAuthenticationAndAuthorization}` [?]), webhook server cert handling, `Cache` restriction.

## 6. Unverified / flags
- setup-envtest exact install command and flags; webhook generator output; applyconfiguration generator run; conversion webhook CRD patch/caBundle and `/convert` path; `filters` import path; leader-election lease RBAC markers; go:generate wrapper; kubebuilder default layout details (internal/webhook etc.); exact controller-runtime version where NewWebhookManagedBy became generic.
- Fetch failures: /reference/watching-resources/externally-managed (404), /reference/webhook-for-core-types (404). Architecture page has text-less diagram.
