package controller_test

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/config"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/envtest/komega"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	demov1 "example.com/demo/api/v1"
	"example.com/demo/internal/controller"
	"example.com/demo/internal/setup"
)

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
		// DownloadBinaryAssetsVersion: "1.37.0", // optional; pin a full version to skip the index request
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
	// Wait for the manager first: the API server waits on its open watches when stopping.
	Eventually(mgrDone, 30*time.Second).Should(BeClosed())
	Expect(testEnv.Stop()).To(Succeed())
	// controller-runtime recovers panics, so a panicking reconciler would otherwise pass.
	Expect(panicCount()).To(BeZero(), "a reconciler or webhook panicked, see the log")
})

func panicCount() float64 {
	mfs, err := metrics.Registry.Gather()
	Expect(err).NotTo(HaveOccurred())
	var total float64
	for _, mf := range mfs {
		if strings.HasSuffix(mf.GetName(), "_panics_total") {
			for _, m := range mf.GetMetric() { total += m.GetCounter().GetValue() }
		}
	}
	return total
}

// newNamespace gives each spec its own namespace. Never wait for its deletion:
// envtest has no namespace controller, so it stays Terminating.
func newNamespace() string {
	GinkgoHelper()
	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{GenerateName: "test-"}}
	Expect(k8sClient.Create(ctx, ns)).To(Succeed())
	return ns.Name
}

// createAndWait creates obj, then polls the cached client until it sees it, so a
// later cached read is not stale.
func createAndWait(obj client.Object) {
	GinkgoHelper()
	Expect(k8sClient.Create(ctx, obj)).To(Succeed())
	Eventually(k.Get(obj.DeepCopyObject().(client.Object))).Should(Succeed())
}
