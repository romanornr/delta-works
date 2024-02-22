package main

import (
	"github.com/romanornr/delta-works/util"
	"github.com/thrasher-corp/gocryptotrader/engine"
	gctlog "github.com/thrasher-corp/gocryptotrader/log"
	"github.com/thrasher-corp/gocryptotrader/signaler"
	"log"
)

const (
	// DefaultConfigPath is the default configuration file path
	DefaultConfigPath = "config.json"
)

func main() {
	if err := initLogger(); err != nil {
		log.Fatalf("Failed to initialise logger. Error: %s", err)
	}

	// Load the configuration file
	configFile := util.ConfigFile(DefaultConfigPath)

	app, err := NewBotApplication(&engine.Settings{ConfigFile: configFile})
	if err != nil {
		log.Fatalf("Failed to create bot application. Error: %s", err)
		return // Graceful exit on failure to create bot application
	}

	if err := app.Start(); err != nil {
		log.Fatalf("Failed to start bot application. Error: %s", err)
		return // Graceful exit on failure to start bot application
	}

	defer app.Stop()
	go signaler.WaitForInterrupt()
}

func initLogger() error {
	var err error
	if err == gctlog.SetupGlobalLogger("Delta-Works", true) {
		return err
	}
	gctlog.Debugf(gctlog.Global, "Logger initialised.")
	return nil
}

type BotApplication struct {
	Bot *engine.Engine
}

// NewBotApplication creates a new bot application
func NewBotApplication(settings *engine.Settings) (*BotApplication, error) {
	bot, err := engine.NewFromSettings(settings, nil)
	if err != nil {
		return nil, err
	}
	return &BotApplication{Bot: bot}, nil
}

// Start starts the bot application
func (b *BotApplication) Start() error {
	b.Bot.Settings.PrintLoadedSettings()
	if err := b.Bot.Start(); err != nil {
		// Attempt to close the logger gracefully
		if errClose := gctlog.CloseLogger(); errClose != nil {
			log.Fatalf("Failed to close logger. Error: %s", errClose)
		}
		return err
	}
	return nil
}

// Stop stops the bot application
func (b *BotApplication) Stop() {
	b.Bot.Stop()
}
