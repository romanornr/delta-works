// Package app is the fx composition root. It is the only place (besides
// cmd/) that knows about concrete adapters; everything else depends on
// ports and injected values.
package app

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/fx"

	"github.com/romanornr/delta-works/internal/adapters/gct"
	"github.com/romanornr/delta-works/internal/adapters/postgres"
	"github.com/romanornr/delta-works/internal/adapters/questdb"
	"github.com/romanornr/delta-works/internal/api"
	"github.com/romanornr/delta-works/internal/bus"
	"github.com/romanornr/delta-works/internal/clock"
	"github.com/romanornr/delta-works/internal/config"
	"github.com/romanornr/delta-works/internal/domain/account"
	"github.com/romanornr/delta-works/internal/domain/instrument"
	"github.com/romanornr/delta-works/internal/exchange"
	"github.com/romanornr/delta-works/internal/log"
	"github.com/romanornr/delta-works/internal/ports"
	orderservice "github.com/romanornr/delta-works/internal/service/order"
	"github.com/romanornr/delta-works/internal/service/outbox"
	"github.com/romanornr/delta-works/internal/service/reconcile"
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
			newExchangeProducts,
			newPostgres,
			fx.Annotate(postgres.NewCheckpointStore, fx.As(new(ports.CheckpointStore))),
			fx.Annotate(postgres.NewOutboxStore, fx.As(new(ports.OutboxStore))),
			fx.Annotate(postgres.NewOrderStore, fx.As(new(ports.OrderStore))),
			newQuestDB,
			fx.Annotate(postgres.NewHealth, fx.As(new(ports.HealthChecker)), fx.ResultTags(`group:"health"`)),
			fx.Annotate(newQuestDBHealth, fx.As(new(ports.HealthChecker)), fx.ResultTags(`group:"health"`)),
			snapshot.NewMetrics,
			newSnapshotService,
			outbox.NewMetrics,
			newOutboxService,
			orderservice.NewMetrics,
			newOrderService,
			reconcile.NewMetrics,
			newReconcileService,
			api.NewMetrics,
			api.NewSnapshotServer,
			api.NewEventServer,
			api.NewOrderServer,
		),
		fx.Invoke(registerBusMetrics, startSnapshotService, startTelemetryServer, startOutboxService, startReconcileService, startOrderService, startAPIServer, logStartup),
	)
}

type tradingVenue struct {
	ID       instrument.VenueID
	Placer   ports.OrderPlacer
	Streamer ports.PrivateStreamer
}

type exchangeProducts struct {
	fx.Out

	Registry exchange.Registry
	Trading  []tradingVenue
}

// newExchangeProducts connects every enabled venue through the GCT adapter
// and wraps it in the standard resilience stack (rate limit + breaker).
func newExchangeProducts(cfg config.Config, l log.Logger, eventBus bus.Bus, clk clock.Clock) (exchangeProducts, error) {
	logger := log.Component(l, "exchange")
	ctx, cancel := context.WithTimeout(context.Background(), startupTimeout)
	defer cancel()
	var exchanges []ports.Exchange
	var trading []tradingVenue
	for _, name := range cfg.EnabledVenues() {
		venueCfg := cfg.Venues[name]
		ex, err := gct.New(ctx, name, venueCfg)
		if err != nil {
			return exchangeProducts{}, err
		}
		decorated := exchange.Decorate(ex, venueCfg.Rate.RPS, venueCfg.Rate.Burst)
		exchanges = append(exchanges, decorated)
		if venueCfg.Trading {
			placer, ok := decorated.(ports.OrderPlacer)
			if !ok {
				return exchangeProducts{}, fmt.Errorf("venue %q: decorated exchange does not implement order placement", name)
			}
			venueID := instrument.NewVenueID(name)
			streamer := gct.NewStreamer(ex, func() {
				_ = eventBus.Publish(context.Background(), bus.Event{
					Subject: orderservice.SubjectStreamReconnected,
					At:      clk.Now(), Payload: venueID,
				})
			})
			trading = append(trading, tradingVenue{ID: venueID, Placer: placer, Streamer: streamer})
		}
		logger.Info().Str("venue", name).Strs("accounts", venueCfg.Accounts).
			Bool("authenticated", venueCfg.APIKey != "").Msg("venue connected")
	}
	return exchangeProducts{Registry: exchange.NewRegistry(exchanges), Trading: trading}, nil
}

func newOrderService(cfg config.Config, venues []tradingVenue, store ports.OrderStore, clk clock.Clock, l log.Logger, m *orderservice.Metrics) *orderservice.Service {
	converted := make([]orderservice.Venue, 0, len(venues))
	for _, venue := range venues {
		converted = append(converted, orderservice.Venue(venue))
	}
	return orderservice.New(converted, store, clk, l, cfg.Order.SubmitBudget, m)
}

func newReconcileService(cfg config.Config, venues []tradingVenue, store ports.OrderStore, eventBus bus.Bus, clk clock.Clock, l log.Logger, m *reconcile.Metrics) *reconcile.Service {
	converted := make([]reconcile.Venue, 0, len(venues))
	for _, venue := range venues {
		converted = append(converted, reconcile.Venue{ID: venue.ID, Placer: venue.Placer})
	}
	return reconcile.New(converted, store, eventBus, clk, l, cfg.Reconcile.Interval, m)
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

func newOutboxService(cfg config.Config, store ports.OutboxStore, eventBus bus.Bus, clk clock.Clock, l log.Logger, m *outbox.Metrics) *outbox.Service {
	return outbox.New(store, eventBus, clk, l, cfg.Outbox.Interval, cfg.Outbox.Batch, m)
}

func startSnapshotService(lc fx.Lifecycle, svc *snapshot.Service, l log.Logger, shutdowner fx.Shutdowner) {
	startService(lc, "snapshot", svc.Run, l, shutdowner)
}

func startOutboxService(lc fx.Lifecycle, svc *outbox.Service, l log.Logger, shutdowner fx.Shutdowner) {
	startService(lc, "outbox", svc.Run, l, shutdowner)
}

func startReconcileService(lc fx.Lifecycle, venues []tradingVenue, svc *reconcile.Service, l log.Logger, shutdowner fx.Shutdowner) {
	if len(venues) > 0 {
		startService(lc, "reconcile", svc.Run, l, shutdowner)
	}
}

func startOrderService(lc fx.Lifecycle, venues []tradingVenue, svc *orderservice.Service, reconcileService *reconcile.Service, l log.Logger, shutdowner fx.Shutdowner) {
	if len(venues) == 0 {
		return
	}
	// Reconciliation is subscribed before private streams start. A stream
	// that cannot connect stays in its retry loop and does not gate readiness.
	startServiceAfter(lc, "order", reconcileService.Ready(), svc.Run, l, shutdowner)
}

// startService ties a background service to the fx lifecycle. A non-nil
// error from run means infrastructure loss; the process exits non-zero so
// the supervisor (compose, systemd) restarts it.
func startService(lc fx.Lifecycle, name string, run func(context.Context) error, l log.Logger, shutdowner fx.Shutdowner) {
	startServiceAfter(lc, name, nil, run, l, shutdowner)
}

func startServiceAfter(lc fx.Lifecycle, name string, ready <-chan struct{}, run func(context.Context) error, l log.Logger, shutdowner fx.Shutdowner) {
	logger := log.Component(l, name)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	lc.Append(fx.Hook{
		OnStart: func(startCtx context.Context) error {
			if ready != nil {
				select {
				case <-ready:
				case <-startCtx.Done():
					return startCtx.Err()
				}
			}
			go func() {
				defer close(done)
				if err := run(ctx); err != nil && !errors.Is(err, context.Canceled) {
					logger.Error().Err(err).Msg("service failed")
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

// serveHTTP runs an HTTP server under the fx lifecycle: serve in the
// background, exit the process on serve failure so the supervisor restarts
// it, shut down gracefully on stop.
func serveHTTP(lc fx.Lifecycle, name string, srv *http.Server,
	listen func(context.Context) (net.Listener, error), l log.Logger, shutdowner fx.Shutdowner,
) {
	logger := log.Component(l, name)
	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			ln, err := listen(ctx)
			if err != nil {
				return err
			}
			go func() {
				if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
					logger.Error().Err(err).Msg("server failed")
					_ = shutdowner.Shutdown(fx.ExitCode(1))
				}
			}()
			logger.Info().Str("addr", ln.Addr().String()).Msg("listening")
			return nil
		},
		OnStop: func(ctx context.Context) error {
			return srv.Shutdown(ctx)
		},
	})
}

func startTelemetryServer(lc fx.Lifecycle, srv *http.Server, l log.Logger, shutdowner fx.Shutdowner) {
	serveHTTP(lc, "telemetry", srv, func(ctx context.Context) (net.Listener, error) {
		return new(net.ListenConfig).Listen(ctx, "tcp", srv.Addr)
	}, l, shutdowner)
}

// startAPIServer serves the control plane (ADR-0007) when api.addr is
// configured. The server is built here rather than provided because fx
// already carries the telemetry *http.Server.
func startAPIServer(lc fx.Lifecycle, cfg config.Config, snapshots *api.SnapshotServer,
	events *api.EventServer, orders *api.OrderServer, l log.Logger, shutdowner fx.Shutdowner,
) {
	if cfg.API.Addr == "" {
		return
	}
	serveHTTP(lc, "api", api.NewServer(snapshots, events, orders), func(ctx context.Context) (net.Listener, error) {
		return api.Listen(ctx, cfg.API.Addr)
	}, l, shutdowner)
}

func logStartup(cfg config.Config, l log.Logger) {
	l.Info().
		Strs("venues", cfg.EnabledVenues()).
		Dur("snapshot_interval", cfg.Snapshot.Interval).
		Msg("starting")
}
