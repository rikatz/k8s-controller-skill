# Testing with envtest

Read this when you write or review tests for a reconciler or webhook, or set up the test suite. Code blocks are excerpts of a
suite that was compiled and run (controller-runtime v0.25.2, Ginkgo v2.33.0, Gomega v1.44.0); the `[run]` tag marks results observed there.

## Choose the layer

- Default to envtest (real kube-apiserver + etcd) for reconcilers and webhooks. The fake client "is almost always better not
  to use" for anything beyond narrow logic. [CR fake/doc.go, CR FAQ, CAPI testing.md]
- Use the fake client only for narrow unit tests (see "Fake client"). Put business logic in pure input-to-output functions and test those without any client. [TALK T4, T1]
- Cover what envtest cannot (GC, Pods, namespace deletion) with e2e on a kind cluster, not with more envtest. [run]
- The two camps, stated neutrally. Ross/Wittrock (controller-runtime authors): fakes drift into a buggy mini API server, so use
  a real API server. [TALK T7 @25:36] Istio (John Howard): found envtest subpar and prefers fake clients plus kind e2e. [TALK T4 @31:19]
  Envtest fits when behaviour depends on the API server: CRD schema/CEL validation, status subresource, finalizers and
  `deletionTimestamp`, server-side apply, webhooks, resourceVersion conflicts. The fake-plus-e2e approach fits a controller whose logic is a pure
  function over a small API surface, where startup cost and the missing controllers matter more. Details of Istio's reasons were not read [unverified].

## Start envtest without setup-envtest

- Use `envtest.Environment{DownloadBinaryAssets: true, ...}`. No `setup-envtest`, no `KUBEBUILDER_ASSETS`, no Makefile download step. There is no `envtest.New`; `Start()` returns the `*rest.Config`. [CR envtest/server.go, binaries.go]
- With `DownloadBinaryAssets: true` the downloaded paths always win: `KUBEBUILDER_ASSETS` is ignored. [CR server.go configureBinaryPaths]
- Set `BinaryAssetsDirectory` to a gitignored, cached directory (e.g. `bin/envtest`). Left empty, every run creates a new temp dir and downloads again. [CR binaries.go]
  Binaries land in `<dir>/<version>-<os>-<arch>/`; `SetupEnvtestDefaultBinaryAssetsDirectory()` returns the setup-envtest layout if you want to share it. [CR]
- `DownloadBinaryAssetsVersion`: empty = latest stable, fetched from the index on every run (network needed even when cached);
  `"1.37"` also queries the index; a full `"1.37.0"` that is already cached makes no request. [run: index URL made unreachable, only the full version started]
  `DownloadBinaryAssetsIndexURL` points at a mirror (default: controller-tools `envtest-releases.yaml`). [CR]
- Set `ErrorIfCRDPathMissing: true` so a wrong `CRDDirectoryPaths` fails at start instead of later as "no matches for kind". [CR server.go doc]
- Timing: `BeforeSuite` took 2.6 to 2.9 s with binaries cached; the first run adds the download. [run]
  `ControlPlaneStartTimeout` defaults to 20 s. `AttachControlPlaneOutput: true` (or `KUBEBUILDER_ATTACH_CONTROL_PLANE_OUTPUT=true`) shows apiserver/etcd logs. [CR]
- Run with `go test ./...`; the `ginkgo` CLI is optional. [run]

## Suite skeleton

```go
var (
	ctx       context.Context
	cancel    context.CancelFunc
	testEnv   *envtest.Environment
	k8sClient client.Client // the manager's client: cached reads, direct writes
	k         komega.Komega
	mgrDone   = make(chan struct{})
)

func TestControllers(t *testing.T) { RegisterFailHandler(Fail); RunSpecs(t, "Controller suite") }

var _ = BeforeSuite(func() {
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))
	SetDefaultEventuallyTimeout(10 * time.Second)
	ctx, cancel = context.WithCancel(context.Background())

	testEnv = &envtest.Environment{
		DownloadBinaryAssets:  true,
		BinaryAssetsDirectory: filepath.Join("..", "..", "bin", "envtest"), // gitignored, reused between runs
		// DownloadBinaryAssetsVersion: "1.37.0", // optional; a full version skips the index request
		CRDDirectoryPaths:     []string{filepath.Join("..", "..", "config", "crd", "bases")},
		ErrorIfCRDPathMissing: true,
	}
	cfg, err := testEnv.Start()
	Expect(err).NotTo(HaveOccurred())

	scheme := runtime.NewScheme()
	Expect(clientgoscheme.AddToScheme(scheme)).To(Succeed())
	Expect(demov1.AddToScheme(scheme)).To(Succeed())

	opts := setup.Options(scheme) // the same cache and client options main() uses
	opts.Metrics = metricsserver.Options{BindAddress: "0"}
	opts.Controller = config.Controller{SkipNameValidation: new(true)}
	mgr, err := ctrl.NewManager(cfg, opts)
	Expect(err).NotTo(HaveOccurred())
	Expect((&controller.WidgetReconciler{Client: mgr.GetClient()}).SetupWithManager(mgr)).To(Succeed())

	go func() { defer GinkgoRecover(); defer close(mgrDone); Expect(mgr.Start(ctx)).To(Succeed()) }()
	k8sClient = mgr.GetClient()
	k = komega.New(k8sClient)
})

var _ = AfterSuite(func() {
	cancel()
	Eventually(mgrDone, 30*time.Second).Should(BeClosed()) // stop the manager before the API server
	Expect(testEnv.Stop()).To(Succeed())
	// controller-runtime recovers panics, so a panicking reconciler would otherwise pass.
	Expect(panicCount()).To(BeZero(), "a reconciler or webhook panicked, see the log")
})

func panicCount() float64 {
	mfs, err := metrics.Registry.Gather()
	Expect(err).NotTo(HaveOccurred())
	var total float64
	for _, mf := range mfs {
		if strings.HasSuffix(mf.GetName(), "_panics_total") { // reconcile, webhook, conversion webhook
			for _, m := range mf.GetMetric() { total += m.GetCounter().GetValue() }
		}
	}
	return total
}
```

- `setup.Options(scheme)` is a plain function returning `ctrl.Options{Scheme, Cache: cache.Options{DefaultTransform: cache.TransformStripManagedFields()}, Client: client.Options{Cache: &client.CacheOptions{DisableFor: ...}}}`,
  called by both `main()` and the suite. Prefer this so tests exercise the production caching; override only listeners. [CAPI environment.go]
- `SkipNameValidation` is needed once two managers or suites in one process register the same controller name; the second `SetupWithManager` otherwise fails with
  "controller with name widget already exists". [CR controller.go, CAPV, run]
- Fail the suite on `controller_runtime_*_panics_total > 0`. [CAPI verifyPanicMetrics] A reconciler that panicked for one Widget (10 retried panics) left all specs green and `AfterSuite` red. [run]
- Start the manager with the test `ctx`, cancel it, and wait for `Start` to return before `testEnv.Stop()`. [run]

## Specs

```go
// newNamespace: every spec gets its own namespace. Never wait for its deletion (see limits below).
func newNamespace() string {
	GinkgoHelper()
	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{GenerateName: "test-"}}
	Expect(k8sClient.Create(ctx, ns)).To(Succeed())
	return ns.Name
}

// createAndWait: write, then poll the cached client until it sees the object. Pattern only.
func createAndWait(obj client.Object) {
	GinkgoHelper()
	Expect(k8sClient.Create(ctx, obj)).To(Succeed())
	Eventually(k.Get(obj.DeepCopyObject().(client.Object))).Should(Succeed())
}

It("adds a finalizer, owns a ConfigMap and reports observedGeneration", func() {
	w := newWidget("w") // helper building a Widget in the spec's namespace
	createAndWait(w)
	Eventually(k.Object(w)).Should(SatisfyAll(
		HaveField("Finalizers", ContainElement(controller.Finalizer)),
		HaveField("Status.ObservedGeneration", w.Generation)))
	cm := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "w", Namespace: ns}}
	Eventually(k.Get(cm)).Should(Succeed())
	Expect(metav1.IsControlledBy(cm, w)).To(BeTrue()) // assert the ownerReference, not GC
})

It("does nothing while paused, then reconciles once unpaused", func() {
	w := newWidget("paused")
	w.Annotations = map[string]string{controller.PausedAnnotation: "true"}
	createAndWait(w)
	cm := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "paused", Namespace: ns}}
	Consistently(k.Get(cm), 2*time.Second).Should(Satisfy(apierrors.IsNotFound))
	// Prove the controller is alive: Consistently also passes when it never ran.
	Eventually(k.Update(w, func() { delete(w.Annotations, controller.PausedAnnotation) })).Should(Succeed())
	Eventually(k.Get(cm)).Should(Succeed())
})
```

- Unique namespace per spec (`GenerateName`), so specs never share state and nothing waits on namespace cleanup. [KB envtest page, CAPI testing.md, run]
- Use `Eventually` for anything the reconciler does asynchronously and `Consistently` for "nothing happens", always paired with a positive control. [run]
- Cached reads right after a write can be stale: 11 to 32 of 200 immediate `Get`s after `Create` returned NotFound across four runs. [run]
  Poll (as `createAndWait`), or enable `opts.Client.Cache.EnableReadYourWritesConsistency = new(true)` (experimental; 0 of 200 stale in one run; the field is on
  `client.CacheOptions`, not `client.Options`). Test it only if production enables it too. [CR client.go, CAPI CreateAndWait, run]
- komega (`pkg/envtest/komega`) is useful with Gomega: `k.Get(obj)` returns `func() error`, `k.Object(obj)` returns `func() (client.Object, error)`,
  `k.Update(obj, fn)` does get, mutate, update and is retried by `Eventually` on conflict. `komega.New(client)` is the only constructor needed. [CR komega.go, run]
- No helper library ships with this skill; copy `createAndWait` as a pattern. [CAPI environment.go CreateAndWait/CleanupAndWait]

## Webhooks

- Generate manifests from the markers, point envtest at them, and serve from the manager on the host, port and cert dir envtest allocates. [CR envtest/webhook.go, run]
  `go tool controller-gen webhook paths=./internal/webhook output:webhook:dir=config/webhook`

```go
// testEnv: add WebhookInstallOptions: envtest.WebhookInstallOptions{Paths: []string{filepath.Join("..", "..", "config", "webhook")}}
wh := testEnv.WebhookInstallOptions // populated by Start()
opts.WebhookServer = webhook.NewServer(webhook.Options{Host: wh.LocalServingHost, Port: wh.LocalServingPort, CertDir: wh.LocalServingCertDir})
// ... register the webhook on mgr, start mgr, then wait for the listener:
Eventually(func() error { return mgr.GetWebhookServer().StartedChecker()(nil) }).Should(Succeed())
// spec:
Expect(k8sClient.Create(ctx, w)).To(MatchError(ContainSubstring("spec.replicas must be <= 10")))
```

- envtest rewrites each `clientConfig.service` with a path into a `url` plus the generated CA, so the controller-gen manifest works unchanged. [CR webhook.go updateClientConfig]
- `Install` waits only until the configuration objects exist, not until the server listens, so wait for the server before the first request. [CR webhook.go WaitForWebhooks]
- controller-gen emitted `failurePolicy: Fail` here; requests for the matching kind are then expected to fail until the listener is up [unverified, not run].

## Fake client

Use `fake.NewClientBuilder()` only for narrow unit tests: an index extractor, a map function, a helper that reads or patches one object.

```go
c := fake.NewClientBuilder().WithScheme(scheme).
	WithStatusSubresource(&demov1.Widget{}).                              // see below
	WithIndex(&demov1.Widget{}, setup.WidgetSizeField, setup.WidgetSize). // the production extractor, exported
	WithObjects(w("a", "Small"), w("b", "Large")).Build()
err := c.List(ctx, &list, client.MatchingFields{setup.WidgetSizeField: "Small"}) // one item
```

- Without `WithStatusSubresource`, `Update` wrote status and `Status().Update` returned NotFound; with it, `Update` ignored status and `Status().Update` wrote it. [run]
- `MatchingFields` without `WithIndex` errors ("no index with name ... has been registered"). Export index extractors so tests register the same function. [run, CAPI util/index]
- Server-side apply exists in the fake client (since v0.22.0) but for CRD types it uses a deduced type converter that "might not match" the API server. Two field managers applying
  different `listType=map` conditions: API server kept 2, fake kept 1. [CR fake/client.go WithTypeConverters doc, run] Passing generated type converters may fix it [unverified].
- No schema/CEL validation; `Generation` and `ResourceVersion` do not behave like the API server (doc.go), so test `observedGeneration`, CEL and conflicts in envtest. [CR fake/doc.go]
- Inject errors with `WithInterceptorFuncs(interceptor.Funcs{...})`; the fake `doc.go` still says errors cannot be injected, which is stale. [CR fake/client.go]

## What envtest cannot prove

envtest runs only etcd and kube-apiserver; the ServiceAccount admission plugin is disabled. [CR controlplane/plane.go, apiserver.go] Each row was asserted in the suite. [run]

| Expectation | Observed in envtest | Assert instead |
|---|---|---|
| Deleted Namespace disappears | Stays `Terminating` (checked 5 s) | Unique namespace per spec; never wait for deletion |
| ownerReference GC removes dependents | Dependent ConfigMap still present 5 s after owner deleted | `metav1.IsControlledBy`; delete dependents in the test |
| Deployment creates ReplicaSets and Pods | 0 ReplicaSets, `observedGeneration` 0 (5 s) | Desired Deployment spec and ownerReference |
| Pod gets a ServiceAccount | `serviceAccountName` empty, phase Pending, no ServiceAccounts | Pod spec your controller sets |

- Finalizers your own controller handles still work (a delete spec in the suite ran green: the controller removed its finalizer and the object disappeared); finalizers owned by other controllers never clear. [run]
- RBAC is not exercised: the test client is `system:masters`. [CR server.go Start] Check generated roles in e2e on kind.
- For GC, rollouts, Pod behaviour and RBAC use e2e on kind.

## Hazards

- `cancel()` then `testEnv.Stop()` without waiting for the manager: "timeout waiting for process kube-apiserver to stop" after 20 s (failed 3 of 3 runs of a 200-object spec, passed once the suite waited). [run]
- `DownloadBinaryAssets: true` with an empty `BinaryAssetsDirectory`: a fresh temp dir and a re-download every run. [CR binaries.go]
- Exporting `KUBEBUILDER_ASSETS` while `DownloadBinaryAssets` is true: ignored, so a pinned local binary is not used. [CR server.go]
- Awaiting namespace deletion or GC in a spec: waits forever, there is no namespace or garbage-collector controller. [run]
- Reading from the cached client right after a write without polling: stale NotFound, flaky specs. [run]
- Panics inside a reconciler or webhook are recovered by controller-runtime; unchecked, tests stay green. [CR internal/controller, run]
- Fake client without `WithStatusSubresource` for a type with a status subresource: status handling differs from the API server. [run]
- Duplicate controller names across managers in one test process without `SkipNameValidation`: setup error. [run]
