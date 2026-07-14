package api

import "github.com/prometheus/client_golang/prometheus"

// Metrics holds control-plane event conversion instruments.
type Metrics struct {
	malformed *prometheus.CounterVec
}

// NewMetrics registers control-plane metrics.
func NewMetrics(reg *prometheus.Registry) (*Metrics, error) {
	m := &Metrics{malformed: prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "api_event_payload_malformed_total",
		Help: "Bus events skipped because their payload could not be decoded for the subject.",
	}, []string{"subject"})}
	if err := reg.Register(m.malformed); err != nil {
		return nil, err
	}
	return m, nil
}
