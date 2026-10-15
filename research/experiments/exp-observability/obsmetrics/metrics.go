// Package obsmetrics holds the controller's custom Prometheus metrics.
package obsmetrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

// Closed label value sets: every value a label can take is known at compile time.
var (
	Phases  = []string{"fetch", "provision", "status_update"}
	Results = []string{"success", "timeout", "rate_limited", "other"}
)

var (
	// PhaseDuration observes time spent in each reconcile phase.
	PhaseDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: "myoperator", Subsystem: "widget", Name: "reconcile_phase_duration_seconds",
		Help:    "Time spent in a reconcile phase.",
		Buckets: prometheus.DefBuckets,
	}, []string{"phase"})

	// ExternalRequests counts calls to the external API by outcome.
	ExternalRequests = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "myoperator", Subsystem: "widget", Name: "external_requests_total",
		Help: "External API requests by operation and result.",
	}, []string{"operation", "result"})

	// Ready is a per-object gauge (1 ready, 0 not ready). Cardinality = number of Widgets,
	// so it needs the delete path below.
	Ready = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "myoperator", Subsystem: "widget", Name: "ready",
		Help: "Whether a Widget is ready (1) or not (0).",
	}, []string{"namespace", "name"})
)

func init() {
	// Package init, not SetupWithManager: MustRegister panics on a second registration.
	metrics.Registry.MustRegister(PhaseDuration, ExternalRequests, Ready)
	// Pre-create bounded series so rate() and absent() work from the first scrape.
	for _, r := range Results {
		ExternalRequests.WithLabelValues("create", r)
	}
	for _, p := range Phases {
		PhaseDuration.WithLabelValues(p)
	}
}

// Forget drops every series of a deleted object. Call it when Get returns NotFound
// or after the finalizer is removed.
func Forget(namespace, name string) {
	Ready.DeletePartialMatch(prometheus.Labels{"namespace": namespace, "name": name})
}
