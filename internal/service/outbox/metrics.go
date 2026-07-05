package outbox

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// Metrics holds the relay's Prometheus instruments. The backlog gauges
// exist so a stuck relay can be alerted on directly.
type Metrics struct {
	published   prometheus.Counter
	unpublished prometheus.Gauge
	oldestAge   prometheus.Gauge
}

// NewMetrics registers the relay metrics on the given registry.
func NewMetrics(reg *prometheus.Registry) (*Metrics, error) {
	m := &Metrics{
		published: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "outbox_published_total",
			Help: "Outbox rows published to the bus.",
		}),
		unpublished: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "outbox_unpublished_rows",
			Help: "Outbox rows not yet published.",
		}),
		oldestAge: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "outbox_oldest_unpublished_age_seconds",
			Help: "Age of the oldest unpublished outbox row.",
		}),
	}
	for _, c := range []prometheus.Collector{m.published, m.unpublished, m.oldestAge} {
		if err := reg.Register(c); err != nil {
			return nil, err
		}
	}
	return m, nil
}

func (m *Metrics) observePublished(n int) {
	m.published.Add(float64(n))
}

func (m *Metrics) observeBacklog(rows int64, oldest time.Duration) {
	m.unpublished.Set(float64(rows))
	m.oldestAge.Set(oldest.Seconds())
}
