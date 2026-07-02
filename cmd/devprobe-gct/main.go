package main

import (
	"context"
	"fmt"
	"os"
	"time"

	gct "github.com/romanornr/delta-works/internal/adapters/gct"
	"github.com/romanornr/delta-works/internal/config"
	"github.com/romanornr/delta-works/internal/observability"
	"github.com/rs/zerolog"
	"go.uber.org/fx"
)

func main() {
	var engine *gct.Engine
	var log zerolog.Logger

	app := fx.New(
		config.Module,
		observability.Module,
		fx.Provide(gct.NewEngine),
		fx.Populate(&engine, &log),
	)

	startCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := app.Start(startCtx); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "failed to start app: %v\n", err)
		os.Exit(1)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	log.Info().Msg("about to call engine.Start")
	if err := engine.Start(ctx); err != nil {
		log.Error().Err(err).Msg("failed to start gct engine")
		_ = app.Stop(context.Background())
		os.Exit(1)
	}

	log.Info().Msg("gct engine start ok")

	registry, err := gct.NewRegistry(engine, log)
	if err != nil {
		log.Error().Err(err).Msg("failed to create exchange registry")
		_ = engine.Stop(ctx)
		_ = app.Stop(context.Background())
		os.Exit(1)
	}

	adapter, err := registry.Get("bybit")
	if err != nil {
		log.Error().Err(err).Msg("failed to resolve exchange adapter")
		_ = engine.Stop(ctx)
		_ = app.Stop(context.Background())
		os.Exit(1)
	}

	ticker, err := adapter.FetchTicker(ctx, "BTC", "USDC")
	if err != nil {
		log.Error().Err(err).Msg("failed to fetch ticker")
	} else {
		log.Info().Str("pair", ticker.Pair()).Str("last", ticker.Last.String()).Msg("ticker fetch ok")
	}

	holdings, err := adapter.FetchHoldings(ctx, "spot")
	if err != nil {
		log.Warn().Err(err).Msg("spot holdings fetch failed or is unavailable in current environment")
	} else {
		log.Info().Int("count", len(holdings)).Msg("spot holdings fetch ok")
	}

	if err := engine.Stop(ctx); err != nil {
		log.Error().Err(err).Msg("failed to stop gct engine")
		_ = app.Stop(context.Background())
		os.Exit(1)
	}

	stopCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := app.Stop(stopCtx); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "failed to stop app: %v\n", err)
	}
}
