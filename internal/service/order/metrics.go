package order

import (
	"github.com/prometheus/client_golang/prometheus"

	"github.com/romanornr/delta-works/internal/domain/instrument"
)

// Metrics holds the service's Prometheus instruments.
type Metrics struct {
	dropped   *prometheus.CounterVec
	unmatched *prometheus.CounterVec
}

// NewMetrics registers the service metrics on the given registry.
func NewMetrics(reg *prometheus.Registry) (*Metrics, error) {
	m := &Metrics{
		dropped: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "order_events_dropped_total",
			Help: "Venue order events dropped without effect or carrying a rejected fill claim: stale, duplicate or post-terminal with no new fill, fill regression, or for an unknown order.",
		}, []string{"venue", "reason"}),
		unmatched: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "ledger_unmatched_sells_total",
			Help: "Sell fills with quantity unmatched to open lots.",
		}, []string{"venue"}),
	}
	for _, collector := range []prometheus.Collector{m.dropped, m.unmatched} {
		if err := reg.Register(collector); err != nil {
			return nil, err
		}
	}
	return m, nil
}

func (m *Metrics) observeDropped(venue instrument.VenueID, reason string) {
	m.dropped.With(prometheus.Labels{"venue": string(venue), "reason": reason}).Inc()
}

func (m *Metrics) observeUnmatched(venue instrument.VenueID) {
	m.unmatched.With(prometheus.Labels{"venue": string(venue)}).Inc()
}
