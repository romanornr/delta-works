package order

import (
	"github.com/prometheus/client_golang/prometheus"

	"github.com/romanornr/delta-works/internal/domain/instrument"
)

// Metrics holds the service's Prometheus instruments.
type Metrics struct {
	dropped *prometheus.CounterVec
}

// NewMetrics registers the service metrics on the given registry.
func NewMetrics(reg *prometheus.Registry) (*Metrics, error) {
	m := &Metrics{
		dropped: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "order_events_dropped_total",
			Help: "Venue order events that produced no state change: stale, duplicate, post-terminal, anomalous or for an unknown order.",
		}, []string{"venue", "reason"}),
	}
	if err := reg.Register(m.dropped); err != nil {
		return nil, err
	}
	return m, nil
}

func (m *Metrics) observeDropped(venue instrument.VenueID, reason string) {
	m.dropped.With(prometheus.Labels{"venue": string(venue), "reason": reason}).Inc()
}
