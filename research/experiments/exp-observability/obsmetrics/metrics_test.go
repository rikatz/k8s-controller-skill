package obsmetrics_test

import (
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/metrics"

	demov1 "example.com/demo/api/v1"
	"example.com/demo/obsmetrics"
)

var _ = Describe("custom metrics", func() {
	It("counts by delta, not absolute value", func() {
		c := obsmetrics.ExternalRequests.WithLabelValues("create", "timeout")
		before := testutil.ToFloat64(c)
		c.Inc()
		Expect(testutil.ToFloat64(c)).To(Equal(before + 1))
	})

	It("follows Prometheus naming rules", func() {
		for _, c := range []prometheus.Collector{obsmetrics.PhaseDuration, obsmetrics.ExternalRequests, obsmetrics.Ready} {
			problems, err := testutil.CollectAndLint(c)
			Expect(err).NotTo(HaveOccurred())
			Expect(problems).To(BeEmpty())
		}
	})

	It("is served by the controller-runtime registry and not the default one", func() {
		names := func(g prometheus.Gatherer) []string {
			mfs, err := g.Gather()
			Expect(err).NotTo(HaveOccurred())
			var out []string
			for _, mf := range mfs {
				out = append(out, mf.GetName())
			}
			return out
		}
		Expect(names(metrics.Registry)).To(ContainElement("myoperator_widget_external_requests_total"))
		Expect(names(prometheus.DefaultGatherer)).NotTo(ContainElement("myoperator_widget_external_requests_total"))
	})

	It("drops every series of a deleted object", func() {
		obsmetrics.Ready.WithLabelValues("ns1", "a").Set(1)
		obsmetrics.Ready.WithLabelValues("ns1", "b").Set(1)
		Expect(testutil.CollectAndCount(obsmetrics.Ready)).To(Equal(2))
		obsmetrics.Forget("ns1", "a")
		Expect(testutil.CollectAndCount(obsmetrics.Ready)).To(Equal(1))
		obsmetrics.Forget("ns1", "b")
		Expect(testutil.CollectAndCount(obsmetrics.Ready)).To(BeZero())
	})

	It("has no reconcile panics", func() {
		mfs, err := metrics.Registry.Gather()
		Expect(err).NotTo(HaveOccurred())
		for _, mf := range mfs {
			if mf.GetName() != "controller_runtime_reconcile_panics_total" {
				continue
			}
			for _, m := range mf.GetMetric() {
				Expect(m.GetCounter().GetValue()).To(BeZero())
			}
		}
	})

	It("scrape-time collector reflects current objects without bookkeeping", func() {
		ready := demov1.Widget{ObjectMeta: metav1.ObjectMeta{Name: "a", Namespace: "d"}, Spec: demov1.WidgetSpec{Size: "Small"},
			Status: demov1.WidgetStatus{Conditions: []metav1.Condition{{Type: "Ready", Status: metav1.ConditionTrue}}}}
		fresh := demov1.Widget{ObjectMeta: metav1.ObjectMeta{Name: "b", Namespace: "d"}, Spec: demov1.WidgetSpec{Size: "Small"}}
		cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(&ready, &fresh).Build()
		col := obsmetrics.NewWidgetCollector(cl)
		expected := `
# HELP myoperator_widgets Number of Widgets by size and Ready status.
# TYPE myoperator_widgets gauge
myoperator_widgets{ready="False",size="Large"} 0
myoperator_widgets{ready="False",size="Small"} 0
myoperator_widgets{ready="True",size="Large"} 0
myoperator_widgets{ready="True",size="Small"} 1
myoperator_widgets{ready="Unknown",size="Large"} 0
myoperator_widgets{ready="Unknown",size="Small"} 1
`
		Expect(testutil.CollectAndCompare(col, strings.NewReader(expected))).To(Succeed())
		Expect(testutil.CollectAndLint(col)).To(BeEmpty())
	})
})
