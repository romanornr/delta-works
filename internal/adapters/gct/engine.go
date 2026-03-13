package gct

import (
	"context"
	"fmt"
	"sync"

	"github.com/romanornr/delta-works/internal/config"
	"github.com/romanornr/delta-works/internal/errs"
	"github.com/rs/zerolog"
	gctengine "github.com/thrasher-corp/gocryptotrader/engine"
	gctexchange "github.com/thrasher-corp/gocryptotrader/exchanges"
)

// Engine wraps the GoCryptoTrader/GCT engine lifecycle for Delta Works
type Engine struct {
	bot *gctengine.Engine
	log zerolog.Logger

	mu    sync.RWMutex
	ready bool
}

// NewEngine returns an Engine configured from settings
func NewEngine(cfg *config.Config, log zerolog.Logger) (e *Engine, err error) {
	if cfg == nil {
		return nil, fmt.Errorf("failed to create GCT engine: config is nil %w", errs.ErrConfigInvalid)
	}

	settings := &gctengine.Settings{
		ConfigFile: cfg.GCT.ConfigPath,
		CoreSettings: gctengine.CoreSettings{
			EnableDryRun: false,
		},
	}

	flagSet := map[string]bool{}

	bot, err := gctengine.NewFromSettings(settings, flagSet)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCT engine from settings: %w", err)
	}

	return &Engine{
		bot: bot,
		log: log.With().Str("component", "gct_engine").Logger(),
	}, nil
}

// Start starts the underlying GCT engine
func (e *Engine) Start(ctx context.Context) error {
	if e == nil || e.bot == nil {
		return fmt.Errorf("failed to start gct engine: %w", errs.ErrAdapterNotReady)
	}

	select {
	case <-ctx.Done():
		return fmt.Errorf("failed to start gct engine before context cancellation: %w", ctx.Err())
	default:
	}

	e.log.Info().Msg("starting gct engine")
	e.log.Debug().Str("config_path", e.bot.Settings.ConfigFile).Msg("invoking upstream gct engine start")
	if err := e.bot.Start(); err != nil {
		return fmt.Errorf("failed to start gct engine: %w", err)
	}

	e.mu.Lock()
	e.ready = true
	e.mu.Unlock()

	e.log.Info().Msg("gct engine started")
	return nil
}

// Stop stops the underlying GoCryptoTrader engine
func (e *Engine) Stop(ctx context.Context) error {
	if e == nil || e.bot == nil {
		return nil
	}

	select {
	case <-ctx.Done():
		return fmt.Errorf("failed to stop gct engine before context cancellation: %w", ctx.Err())
	default:
	}

	e.mu.Lock()
	e.ready = false
	e.mu.Unlock()

	e.log.Info().Msg("stopping gct engine")
	e.bot.Stop()

	e.log.Info().Msg("gct engine stopped")
	return nil
}

// Ready returns true when the engine has started successfully
func (e *Engine) Ready() bool {
	if e == nil {
		return false
	}
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.ready
}

// GetExchangeByName returns a loaded exchange by name
func (e *Engine) GetExchangeByName(name string) (gctexchange.IBotExchange, error) {
	if e == nil || e.bot == nil || !e.Ready() {
		return nil, errs.ErrAdapterNotReady
	}

	exch, err := e.bot.GetExchangeByName(name)
	if err != nil {
		return nil, fmt.Errorf("failed to get exchange by name %s: %w", name, err)
	}

	return exch, nil
}

// GetExchanges returns a list of loaded exchanges
func (e *Engine) GetExchanges() ([]gctexchange.IBotExchange, error) {
	if e == nil || e.bot == nil || !e.Ready() {
		return nil, errs.ErrAdapterNotReady
	}
	exchanges := e.bot.GetExchanges()
	if len(exchanges) == 0 {
		return nil, errs.ErrNoExchangesEnabled
	}
	return exchanges, nil
}
