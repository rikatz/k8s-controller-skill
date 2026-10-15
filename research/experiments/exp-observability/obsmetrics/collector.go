package obsmetrics

import (
	"context"

	"github.com/prometheus/client_golang/prometheus"
	"k8s.io/apimachinery/pkg/api/meta"
	"sigs.k8s.io/controller-runtime/pkg/client"

	demov1 "example.com/demo/api/v1"
)

// WidgetCollector computes aggregates at scrape time from a (cached) reader, so the
// exported series always equal the current state and nothing needs deleting.
type WidgetCollector struct {
	Reader client.Reader
	desc   *prometheus.Desc
}

func NewWidgetCollector(r client.Reader) *WidgetCollector {
	return &WidgetCollector{Reader: r, desc: prometheus.NewDesc(
		"myoperator_widgets", "Number of Widgets by size and Ready status.",
		[]string{"size", "ready"}, nil)}
}

func (c *WidgetCollector) Describe(ch chan<- *prometheus.Desc) { ch <- c.desc }

func (c *WidgetCollector) Collect(ch chan<- prometheus.Metric) {
	var list demov1.WidgetList
	if err := c.Reader.List(context.Background(), &list); err != nil {
		ch <- prometheus.NewInvalidMetric(c.desc, err)
		return
	}
	counts := map[[2]string]float64{}
	for _, size := range []demov1.Size{"Small", "Large"} { // closed sets, zero-filled
		for _, ready := range []string{"True", "False", "Unknown"} {
			counts[[2]string{string(size), ready}] = 0
		}
	}
	for i := range list.Items {
		w := &list.Items[i]
		ready := "Unknown"
		if cond := meta.FindStatusCondition(w.Status.Conditions, "Ready"); cond != nil {
			ready = string(cond.Status)
		}
		counts[[2]string{string(w.Spec.Size), ready}]++
	}
	for k, v := range counts {
		ch <- prometheus.MustNewConstMetric(c.desc, prometheus.GaugeValue, v, k[0], k[1])
	}
}
