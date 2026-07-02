// Package app is the fx composition root. It is the only place (besides
// cmd/) that knows about concrete adapters; everything else depends on
// ports and injected values.
package app

import (
	"context"
	"errors"
	"net"
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/fx"

	"github.com/romanornr/delta-works/internal/adapters/gct"
	"github.com/romanornr/delta-works/internal/bus"
	"github.com/romanornr/delta-works/internal/clock"
	"github.com/romanornr/delta-works/internal/config"
	"github.com/romanornr/delta-works/internal/exchange"
	"github.com/romanornr/delta-works/internal/log"
	"github.com/romanornr/delta-works/internal/ports"
	"github.com/romanornr/delta-works/internal/telemetry"
)

// New composes the deltad application.
func New(configPath string, configExplicit bool) *fx.App {
	return fx.New(
		fx.WithLogger(newFxLogger),
		fx.Provide(
			func() (config.Config, error) { return config.Load(configPath, configExplicit) },
			func(cfg config.Config) (log.Logger, error) { return log.New(cfg.Log) },
			clock.New,
			telemetry.NewRegistry,
			newBus,
			func(b *bus.InProc) bus.Bus { return b },
			fx.Annotate(
				func(cfg config.Config, reg *prometheus.Registry, checks []ports.HealthChecker, l log.Logger) *http.Server {
					return telemetry.NewServer(cfg.HTTP, reg, checks, l)
				},
				fx.ParamTags("", "", `group:"health"`, ""),
			),
			newExchangeRegistry,
		),
		fx.Invoke(registerBusMetrics, startTelemetryServer, logStartup),
	)
}

// newExchangeRegistry connects every enabled venue through the GCT adapter
// and wraps it in the standard resilience stack (rate limit + breaker).
func newExchangeRegistry(cfg config.Config, l log.Logger) (exchange.Registry, error) {
	logger := log.Component(l, "exchange")
	var exchanges []ports.Exchange
	for _, name := range cfg.EnabledVenues() {
		venueCfg := cfg.Venues[name]
		ex, err := gct.New(context.Background(), name, venueCfg)
		if err != nil {
			return nil, err
		}
		exchanges = append(exchanges, exchange.Decorate(ex, venueCfg.Rate.RPS, venueCfg.Rate.Burst))
		logger.Info().Str("venue", name).Strs("accounts", venueCfg.Accounts).
			Bool("authenticated", venueCfg.APIKey != "").Msg("venue connected")
	}
	return exchange.NewRegistry(exchanges), nil
}

func newBus(lc fx.Lifecycle) *bus.InProc {
	b := bus.NewInProc()
	lc.Append(fx.Hook{
		OnStop: func(context.Context) error {
			b.Close()
			return nil
		},
	})
	return b
}

func registerBusMetrics(b *bus.InProc, reg *prometheus.Registry) error {
	return reg.Register(prometheus.NewCounterFunc(prometheus.CounterOpts{
		Name: "bus_dropped_total",
		Help: "Events dropped due to slow bus subscribers.",
	}, func() float64 { return float64(b.Dropped()) }))
}

func startTelemetryServer(lc fx.Lifecycle, srv *http.Server, l log.Logger, shutdowner fx.Shutdowner) {
	logger := log.Component(l, "telemetry")
	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			ln, err := new(net.ListenConfig).Listen(ctx, "tcp", srv.Addr)
			if err != nil {
				return err
			}
			go func() {
				if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
					logger.Error().Err(err).Msg("telemetry server failed")
					_ = shutdowner.Shutdown(fx.ExitCode(1))
				}
			}()
			logger.Info().Str("addr", srv.Addr).Msg("telemetry server listening")
			return nil
		},
		OnStop: func(ctx context.Context) error {
			return srv.Shutdown(ctx)
		},
	})
}

func logStartup(cfg config.Config, l log.Logger) {
	l.Info().
		Strs("venues", cfg.EnabledVenues()).
		Dur("snapshot_interval", cfg.Snapshot.Interval).
		Msg("deltad starting")
}
