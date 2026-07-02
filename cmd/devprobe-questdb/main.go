package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/romanornr/delta-works/internal/adapters/questdb"
	"github.com/romanornr/delta-works/internal/config"
	"github.com/romanornr/delta-works/internal/observability"
	"github.com/rs/zerolog"
	"go.uber.org/fx"
)

func main() {
	var client *questdb.Client
	var log zerolog.Logger

	app := fx.New(
		config.Module,
		observability.Module,
		fx.Provide(questdb.NewClient),
		fx.Populate(&client, &log),
	)

	startCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := app.Start(startCtx); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "failed to start fx app: %v\n", err)
		os.Exit(1)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := client.Ping(ctx); err != nil {
		log.Error().Err(err).Msg("questdb ping failed")
		_ = app.Stop(context.Background())
		os.Exit(1)
	}
	log.Info().Msg("questdb ping ok")

	stopCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := app.Stop(stopCtx); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "failed to stop fx app: %v\n", err)
		os.Exit(1)
	}
}
