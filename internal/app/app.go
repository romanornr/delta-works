// Package app is the fx composition root. It is the only place (besides
// cmd/) that knows about concrete adapters; everything else depends on
// ports and injected values.
package app

import (
	"context"
	"errors"
	"net"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/fx"

	"github.com/romanornr/delta-works/internal/adapters/gct"
	"github.com/romanornr/delta-works/internal/adapters/postgres"
	"github.com/romanornr/delta-works/internal/adapters/questdb"
	"github.com/romanornr/delta-works/internal/bus"
	"github.com/romanornr/delta-works/internal/clock"
	"github.com/romanornr/delta-works/internal/config"
	"github.com/romanornr/delta-works/internal/domain/account"
	"github.com/romanornr/delta-works/internal/domain/instrument"
	"github.com/romanornr/delta-works/internal/exchange"
	"github.com/romanornr/delta-works/internal/log"
	"github.com/romanornr/delta-works/internal/ports"
	"github.com/romanornr/delta-works/internal/service/snapshot"
	"github.com/romanornr/delta-works/internal/telemetry"
)

// startupTimeout bounds constructor-time network work (venue setup, store
// connections, migrations). fx providers run outside lifecycle hooks, so
// without this a black-holed endpoint would hang the daemon before it can
// expose health or exit for its supervisor.
const startupTimeout = 30 * time.Second

// New composes the application.
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
			newPostgres,
			fx.Annotate(postgres.NewCheckpointStore, fx.As(new(ports.CheckpointStore))),
			newQuestDB,
			fx.Annotate(postgres.NewHealth, fx.As(new(ports.HealthChecker)), fx.ResultTags(`group:"health"`)),
			fx.Annotate(newQuestDBHealth, fx.As(new(ports.HealthChecker)), fx.ResultTags(`group:"health"`)),
			snapshot.NewMetrics,
			newSnapshotService,
		),
		fx.Invoke(registerBusMetrics, startTelemetryServer, startSnapshotService, logStartup),
	)
}

// newExchangeRegistry connects every enabled venue through the GCT adapter
// and wraps it in the standard resilience stack (rate limit + breaker).
func newExchangeRegistry(cfg config.Config, l log.Logger) (exchange.Registry, error) {
	logger := log.Component(l, "exchange")
	ctx, cancel := context.WithTimeout(context.Background(), startupTimeout)
	defer cancel()
	var exchanges []ports.Exchange
	for _, name := range cfg.EnabledVenues() {
		venueCfg := cfg.Venues[name]
		ex, err := gct.New(ctx, name, venueCfg)
		if err != nil {
			return nil, err
		}
		exchanges = append(exchanges, exchange.Decorate(ex, venueCfg.Rate.RPS, venueCfg.Rate.Burst))
		logger.Info().Str("venue", name).Strs("accounts", venueCfg.Accounts).
			Bool("authenticated", venueCfg.APIKey != "").Msg("venue connected")
	}
	return exchange.NewRegistry(exchanges), nil
}

func newPostgres(lc fx.Lifecycle, cfg config.Config) (*pgxpool.Pool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), startupTimeout)
	defer cancel()
	pool, err := postgres.Connect(ctx, cfg.Postgres)
	if err != nil {
		return nil, err
	}
	lc.Append(fx.Hook{OnStop: func(context.Context) error {
		pool.Close()
		return nil
	}})
	return pool, nil
}

func newQuestDB(lc fx.Lifecycle, cfg config.Config) (ports.SeriesWriter, error) {
	ctx, cancel := context.WithTimeout(context.Background(), startupTimeout)
	defer cancel()
	w, err := questdb.New(ctx, cfg.QuestDB)
	if err != nil {
		return nil, err
	}
	lc.Append(fx.Hook{OnStop: w.Close})
	return w, nil
}

func newQuestDBHealth(cfg config.Config) *questdb.Health {
	return questdb.NewHealth(cfg.QuestDB)
}

func newSnapshotService(
	cfg config.Config,
	registry exchange.Registry,
	series ports.SeriesWriter,
	checkpoints ports.CheckpointStore,
	eventBus bus.Bus,
	clk clock.Clock,
	l log.Logger,
	m *snapshot.Metrics,
) *snapshot.Service {
	var targets []snapshot.Target
	for _, name := range cfg.EnabledVenues() {
		for _, acct := range cfg.Venues[name].Accounts {
			targets = append(targets, snapshot.Target{
				Venue:   instrument.NewVenueID(name),
				Account: account.Type(acct),
			})
		}
	}
	return snapshot.New(registry, series, checkpoints, eventBus, clk, l, cfg.Snapshot.Interval, targets, m)
}

// startSnapshotService ties the poller to the fx lifecycle. A non-nil error
// from Run means infrastructure loss; the process exits non-zero so the
// supervisor (compose, systemd) restarts it.
func startSnapshotService(lc fx.Lifecycle, svc *snapshot.Service, l log.Logger, shutdowner fx.Shutdowner) {
	logger := log.Component(l, "snapshot")
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	lc.Append(fx.Hook{
		OnStart: func(context.Context) error {
			go func() {
				defer close(done)
				if err := svc.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
					logger.Error().Err(err).Msg("snapshot service failed")
					_ = shutdowner.Shutdown(fx.ExitCode(1))
				}
			}()
			return nil
		},
		OnStop: func(stopCtx context.Context) error {
			cancel()
			select {
			case <-done:
				return nil
			case <-stopCtx.Done():
				return stopCtx.Err()
			}
		},
	})
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
		Msg("starting")
}
