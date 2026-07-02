package snapshot

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// Metrics holds the service's Prometheus instruments. The last-success
// timestamp gauge exists so staleness can be alerted on directly.
type Metrics struct {
	duration    *prometheus.HistogramVec
	errors      *prometheus.CounterVec
	lastSuccess *prometheus.GaugeVec
}

// NewMetrics registers the service metrics on the given registry.
func NewMetrics(reg *prometheus.Registry) (*Metrics, error) {
	m := &Metrics{
		duration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "snapshot_duration_seconds",
			Help:    "Time to fetch and persist one balance snapshot.",
			Buckets: prometheus.DefBuckets,
		}, []string{"venue", "account"}),
		errors: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "snapshot_errors_total",
			Help: "Snapshot attempts that failed after retries.",
		}, []string{"venue", "account"}),
		lastSuccess: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "snapshot_last_success_timestamp_seconds",
			Help: "Unix time of the last successful snapshot.",
		}, []string{"venue", "account"}),
	}
	for _, c := range []prometheus.Collector{m.duration, m.errors, m.lastSuccess} {
		if err := reg.Register(c); err != nil {
			return nil, err
		}
	}
	return m, nil
}

func (m *Metrics) observeSuccess(t Target, d time.Duration, at time.Time) {
	labels := prometheus.Labels{"venue": string(t.Venue), "account": string(t.Account)}
	m.duration.With(labels).Observe(d.Seconds())
	m.lastSuccess.With(labels).Set(float64(at.Unix()))
}

func (m *Metrics) observeError(t Target) {
	m.errors.With(prometheus.Labels{"venue": string(t.Venue), "account": string(t.Account)}).Inc()
}
