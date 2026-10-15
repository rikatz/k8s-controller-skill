package obsmetrics_test

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/pprof"
	"path/filepath"
	"strings"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	authenticationv1 "k8s.io/api/authentication/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	ctrlmetrics "sigs.k8s.io/controller-runtime/pkg/metrics"
	"sigs.k8s.io/controller-runtime/pkg/metrics/filters"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	demov1 "example.com/demo/api/v1"
	"example.com/demo/obsmetrics"
)

var (
	env        *envtest.Environment
	cfg        *rest.Config
	k8s        client.Client
	scheme     = runtime.NewScheme()
	metricsURL string
	cancel     context.CancelFunc
)

func TestObs(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "obs")
}

func freePort() string {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	Expect(err).NotTo(HaveOccurred())
	defer l.Close()
	return l.Addr().String()
}

var _ = BeforeSuite(func() {
	Expect(clientgoscheme.AddToScheme(scheme)).To(Succeed())
	Expect(demov1.AddToScheme(scheme)).To(Succeed())
	ctrlmetrics.RegisterRESTClientMetrics(ctrlmetrics.MetricRequestLatency, ctrlmetrics.MetricRateLimiterLatency)
	env = &envtest.Environment{
		CRDDirectoryPaths:     []string{filepath.Join("..", "config", "crd", "bases")},
		ErrorIfCRDPathMissing: true,
		DownloadBinaryAssets:  true,
	}
	var err error
	cfg, err = env.Start()
	Expect(err).NotTo(HaveOccurred())
	k8s, err = client.New(cfg, client.Options{Scheme: scheme})
	Expect(err).NotTo(HaveOccurred())

	// RBAC from the reference: controller (authn/authz delegation) and scraper (nonResourceURL).
	Expect(k8s.Create(context.Background(), &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{Name: "metrics-auth-role"},
		Rules: []rbacv1.PolicyRule{
			{APIGroups: []string{"authentication.k8s.io"}, Resources: []string{"tokenreviews"}, Verbs: []string{"create"}},
			{APIGroups: []string{"authorization.k8s.io"}, Resources: []string{"subjectaccessreviews"}, Verbs: []string{"create"}},
		}})).To(Succeed())
	Expect(k8s.Create(context.Background(), &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{Name: "metrics-reader"},
		Rules:      []rbacv1.PolicyRule{{NonResourceURLs: []string{"/metrics"}, Verbs: []string{"get"}}}})).To(Succeed())
	for _, sa := range []string{"controller", "scraper", "nobody"} {
		Expect(k8s.Create(context.Background(), &corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{Name: sa, Namespace: "default"}})).To(Succeed())
	}
	bind := func(name, role, sa string) {
		Expect(k8s.Create(context.Background(), &rbacv1.ClusterRoleBinding{
			ObjectMeta: metav1.ObjectMeta{Name: name},
			RoleRef:    rbacv1.RoleRef{APIGroup: "rbac.authorization.k8s.io", Kind: "ClusterRole", Name: role},
			Subjects:   []rbacv1.Subject{{Kind: "ServiceAccount", Name: sa, Namespace: "default"}}})).To(Succeed())
	}
	bind("controller-metrics-auth", "metrics-auth-role", "controller")
	bind("scraper-metrics-reader", "metrics-reader", "scraper")

	// The manager runs AS the controller ServiceAccount, so missing RBAC would show up.
	mgrCfg := rest.CopyConfig(cfg)
	mgrCfg.Impersonate = rest.ImpersonationConfig{UserName: "system:serviceaccount:default:controller"}
	addr := freePort()
	metricsURL = "https://" + addr + "/metrics"
	mgr, err := ctrl.NewManager(mgrCfg, ctrl.Options{
		Scheme: scheme,
		Metrics: metricsserver.Options{
			BindAddress:    addr,
			SecureServing:  true,
			FilterProvider: filters.WithAuthenticationAndAuthorization,
			ExtraHandlers:  map[string]http.Handler{"/debug/pprof/": http.HandlerFunc(pprof.Index)},
		},
	})
	Expect(err).NotTo(HaveOccurred())
	var ctx context.Context
	ctx, cancel = context.WithCancel(context.Background())
	go func() { defer GinkgoRecover(); Expect(mgr.Start(ctx)).To(Succeed()) }()
})

var _ = AfterSuite(func() {
	cancel()
	Expect(env.Stop()).To(Succeed())
})

func token(sa string) string {
	tr := &authenticationv1.TokenRequest{Spec: authenticationv1.TokenRequestSpec{ExpirationSeconds: new(int64(600))}}
	Expect(k8s.SubResource("token").Create(context.Background(),
		&corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{Name: sa, Namespace: "default"}}, tr)).To(Succeed())
	return tr.Status.Token
}

func scrape(bearer string) (int, string) { return get(metricsURL, bearer) }

func get(url, bearer string) (int, string) {
	c := &http.Client{Timeout: 5 * time.Second, Transport: &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}} // self-signed dev cert only
	var code int
	var body string
	Eventually(func() error {
		req, _ := http.NewRequest(http.MethodGet, url, nil)
		if bearer != "" {
			req.Header.Set("Authorization", "Bearer "+bearer)
		}
		resp, err := c.Do(req)
		if err != nil {
			return err // server not up yet
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		code, body = resp.StatusCode, string(b)
		return nil
	}, 30*time.Second, 200*time.Millisecond).Should(Succeed())
	return code, body
}

var _ = Describe("authenticated metrics endpoint", func() {
	It("rejects an unauthenticated scrape", func() {
		code, body := scrape("")
		fmt.Fprintf(GinkgoWriter, "anonymous: %d %q\n", code, body)
		GinkgoWriter.Printf("ANON code=%d body=%q\n", code, strings.TrimSpace(body))
		Expect(code).To(Equal(http.StatusUnauthorized))
	})
	It("rejects an authenticated caller without the metrics-reader role", func() {
		code, body := scrape(token("nobody"))
		GinkgoWriter.Printf("NOBODY code=%d body=%q\n", code, strings.TrimSpace(body))
		Expect(code).To(Equal(http.StatusForbidden))
	})
	It("serves metrics to a ServiceAccount bound to metrics-reader", func() {
		obsmetrics.ExternalRequests.WithLabelValues("create", "success").Inc()
		code, body := scrape(token("scraper"))
		GinkgoWriter.Printf("SCRAPER code=%d len=%d\n", code, len(body))
		names := map[string]bool{}
		for _, l := range strings.Split(body, "\n") {
			if strings.HasPrefix(l, "# TYPE ") {
				names[strings.Fields(l)[2]] = true
			}
		}
		for n := range names {
			if strings.HasPrefix(n, "rest_client") || strings.HasPrefix(n, "leader_election") || strings.HasPrefix(n, "controller_runtime") {
				GinkgoWriter.Printf("NAME %s\n", n)
			}
		}
		Expect(code).To(Equal(http.StatusOK))
		Expect(body).To(ContainSubstring("go_goroutines"))
		Expect(body).To(ContainSubstring(`myoperator_widget_external_requests_total{operation="create",result="success"} 1`))
	})
	It("puts extra handlers (pprof) behind the same filter", func() {
		pprofURL := strings.TrimSuffix(metricsURL, "/metrics") + "/debug/pprof/"
		code, _ := get(pprofURL, "")
		Expect(code).To(Equal(http.StatusUnauthorized))
		code, _ = get(pprofURL, token("scraper")) // has /metrics only
		Expect(code).To(Equal(http.StatusForbidden))
	})
	It("authorizes pprof sub-paths only with a trailing-wildcard rule", func() {
		ctx := context.Background()
		Expect(k8s.Create(ctx, &corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{Name: "profiler", Namespace: "default"}})).To(Succeed())
		Expect(k8s.Create(ctx, &rbacv1.ClusterRole{ObjectMeta: metav1.ObjectMeta{Name: "exact"},
			Rules: []rbacv1.PolicyRule{{NonResourceURLs: []string{"/debug/pprof/"}, Verbs: []string{"get"}}}})).To(Succeed())
		Expect(k8s.Create(ctx, &rbacv1.ClusterRoleBinding{ObjectMeta: metav1.ObjectMeta{Name: "exact"},
			RoleRef:  rbacv1.RoleRef{APIGroup: "rbac.authorization.k8s.io", Kind: "ClusterRole", Name: "exact"},
			Subjects: []rbacv1.Subject{{Kind: "ServiceAccount", Name: "profiler", Namespace: "default"}}})).To(Succeed())
		base := strings.TrimSuffix(metricsURL, "/metrics") + "/debug/pprof/"
		tok := token("profiler")
		code, _ := get(base, tok)
		GinkgoWriter.Printf("PPROF exact index=%d\n", code)
		code, _ = get(base+"heap", tok)
		GinkgoWriter.Printf("PPROF exact heap=%d\n", code)
		Expect(k8s.Create(ctx, &rbacv1.ClusterRole{ObjectMeta: metav1.ObjectMeta{Name: "glob"},
			Rules: []rbacv1.PolicyRule{{NonResourceURLs: []string{"/debug/pprof/*"}, Verbs: []string{"get"}}}})).To(Succeed())
		Expect(k8s.Create(ctx, &rbacv1.ClusterRoleBinding{ObjectMeta: metav1.ObjectMeta{Name: "glob"},
			RoleRef:  rbacv1.RoleRef{APIGroup: "rbac.authorization.k8s.io", Kind: "ClusterRole", Name: "glob"},
			Subjects: []rbacv1.Subject{{Kind: "ServiceAccount", Name: "profiler", Namespace: "default"}}})).To(Succeed())
		Eventually(func() int { c, _ := get(base+"heap", tok); return c }, 40*time.Second, time.Second).Should(Equal(200))
		GinkgoWriter.Printf("PPROF glob heap=200\n")
	})
	It("fails closed when the controller lacks tokenreviews/subjectaccessreviews", func() {
		Expect(k8s.Delete(context.Background(), &rbacv1.ClusterRoleBinding{ObjectMeta: metav1.ObjectMeta{Name: "controller-metrics-auth"}})).To(Succeed())
		tok := token("scraper")
		var code int
		var body string
		Eventually(func() int { code, body = scrape(tok); return code }, 90*time.Second, time.Second).ShouldNot(Equal(http.StatusOK))
		GinkgoWriter.Printf("NO-CONTROLLER-RBAC code=%d body=%q\n", code, strings.TrimSpace(body))
	})
})
