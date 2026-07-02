package main

import (
	"fmt"
	"time"

	"github.com/romanornr/delta-works/internal/domain/portfolio"
	"github.com/rs/zerolog"
	"github.com/shopspring/decimal"
	"go.uber.org/fx"

	"github.com/romanornr/delta-works/internal/config"
	"github.com/romanornr/delta-works/internal/observability"
)

func main() {
	app := fx.New(
		config.Module,        // Registers NewConfig: () → *Config
		observability.Module, // Registers NewLogger: (*Config) → zerolog.Logger  ← trailing comma!

		// fx.Invoke "pulls" the chain — forces Fx to build everything
		// This function needs zerolog.Logger,
		// so Fx must build Logger, which needs *Config, so Fx builds Config first.
		fx.Invoke(func(logger zerolog.Logger, cfg *config.Config) {
			logger.Info().
				Str("log_level", cfg.Logging.Level).      // ← dot at end continues chain
				Str("log_format", cfg.Logging.Format).    // ← dot at end continues chain
				Msg("Fx wired everything automatically!") // ← last call, no dot

			logger.Debug().Msg("This only shows if level is 'debug' or lower")
			logger.Warn().Msg("This is a warning — always visible at 'info' level")
		}),
	)

	xmrPos := portfolio.NewHolding(
		"xmr",
		decimal.New(100, 0), // total
		decimal.New(100, 0), // available
		decimal.New(100, 0), // locked
		decimal.New(0, 0),   // availableWithoutBorrow
		decimal.New(0, 0),   // borrowed
		decimal.New(0, 0),   // value
	)

	btcPos := portfolio.NewHolding(
		"BTC",
		decimal.New(120, 0), // total
		decimal.New(100, 0), // available
		decimal.New(100, 0), // locked
		decimal.New(0, 0),   // availableWithoutBorrow
		decimal.New(0, 0),   // borrowed
		decimal.New(0, 0),   // value
	)

	fmt.Printf("isZero: %v\n", xmrPos.IsZero())

	snap := portfolio.NewSnapshot("kraken", portfolio.AccountSpot, time.Now())
	snap.AddHolding(xmrPos)

	snap.AddHolding(btcPos)

	fmt.Printf("non zero: %v\n", snap.NonZeroHoldings())

	app.Run()

}
