// Package telemetry owns the Prometheus registry and the operational HTTP
// server exposing /metrics, /healthz (liveness) and /readyz (readiness
// aggregated from registered ports.HealthChecker implementations).
package telemetry

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/romanornr/delta-works/internal/config"
	"github.com/romanornr/delta-works/internal/log"
	"github.com/romanornr/delta-works/internal/ports"
)

const (
	readyCheckTimeout = 5 * time.Second
	readHeaderTimeout = 5 * time.Second
)

// NewRegistry builds the application's Prometheus registry with standard
// process and Go runtime collectors.
func NewRegistry() *prometheus.Registry {
	reg := prometheus.NewRegistry()
	reg.MustRegister(
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
	)
	return reg
}

// NewServer builds the operational HTTP server. It does not start it;
// lifecycle is managed by the application (fx hooks).
func NewServer(cfg config.HTTP, reg *prometheus.Registry, checks []ports.HealthChecker, logger log.Logger) *http.Server {
	l := log.Component(logger, "telemetry")
	mux := http.NewServeMux()

	mux.Handle("GET /metrics", promhttp.HandlerFor(reg, promhttp.HandlerOpts{}))

	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok\n"))
	})

	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), readyCheckTimeout)
		defer cancel()
		for _, c := range checks {
			if err := c.Check(ctx); err != nil {
				l.Warn().Str("check", c.Name()).Err(err).Msg("readiness check failed")
				w.WriteHeader(http.StatusServiceUnavailable)
				_, _ = fmt.Fprintf(w, "not ready: %s\n", c.Name())
				return
			}
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ready\n"))
	})

	return &http.Server{
		Addr:              cfg.Addr,
		Handler:           mux,
		ReadHeaderTimeout: readHeaderTimeout,
	}
}
