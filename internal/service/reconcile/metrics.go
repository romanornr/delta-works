package reconcile

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/romanornr/delta-works/internal/domain/instrument"
)

// Metrics holds the service's Prometheus instruments.
type Metrics struct {
	diffs       *prometheus.CounterVec
	duration    *prometheus.HistogramVec
	lastSuccess *prometheus.GaugeVec
	orphans     *prometheus.GaugeVec
}

// NewMetrics registers the service metrics on the given registry.
func NewMetrics(reg *prometheus.Registry) (*Metrics, error) {
	m := &Metrics{
		diffs: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "reconcile_diffs_total",
			Help: "Venue and local order state differences found during reconciliation.",
		}, []string{"venue", "kind"}),
		duration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "reconcile_duration_seconds",
			Help:    "Time to complete one venue reconciliation pass.",
			Buckets: prometheus.DefBuckets,
		}, []string{"venue"}),
		lastSuccess: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "reconcile_last_success_timestamp_seconds",
			Help: "Unix time of the last successful venue reconciliation pass.",
		}, []string{"venue"}),
		orphans: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "reconcile_orphans",
			Help: "Open venue orders unknown to the local order store.",
		}, []string{"venue"}),
	}
	for _, collector := range []prometheus.Collector{m.diffs, m.duration, m.lastSuccess, m.orphans} {
		if err := reg.Register(collector); err != nil {
			return nil, err
		}
	}
	return m, nil
}

func (m *Metrics) observeDiff(venue instrument.VenueID, kind string) {
	m.diffs.With(prometheus.Labels{"venue": string(venue), "kind": kind}).Inc()
}

func (m *Metrics) observeSuccess(venue instrument.VenueID, duration time.Duration, at time.Time, orphans int) {
	labels := prometheus.Labels{"venue": string(venue)}
	m.duration.With(labels).Observe(duration.Seconds())
	m.lastSuccess.With(labels).Set(float64(at.Unix()))
	m.orphans.With(labels).Set(float64(orphans))
}
