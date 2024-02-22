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

func init() {
	initLogger()
}

func initLogger() {
	var err error
	if err == gctlog.SetupGlobalLogger("Delta-Works", true) {
		log.Fatalf("Failed to setup Global Logger. Error: %s", err)
	}
	gctlog.Debugf(gctlog.Global, "Logger initialised.")
}

type BotApplication struct {
	Bot *engine.Engine
}

func NewBotApplication(settings *engine.Settings) (*BotApplication, error) {
	bot, err := engine.NewFromSettings(settings, nil)
	if err != nil {
		return nil, err
	}
	return &BotApplication{Bot: bot}, nil
}

func (b *BotApplication) Start() error {
	b.Bot.Settings.PrintLoadedSettings()
	if err := b.Bot.Start(); err != nil {
		errClose := gctlog.CloseLogger()
		if errClose != nil {
			log.Printf("Failed to close logger. Error: %s", errClose)
		}
		log.Fatalf("Failed to start engine. Error: %s", err)
		return err
	}
	return nil
}

func main() {
	// Load the configuration file
	configPath := util.ConfigFile(DefaultConfigPath)

	app, err := NewBotApplication(&engine.Settings{ConfigFile: configPath})
	if err != nil {
		log.Fatalf("Failed to create bot application. Error: %s", err)
	}

	if err := app.Start(); err != nil {
		log.Fatalf("Failed to start bot application. Error: %s", err)
	}

	defer app.Stop()
	go signaler.WaitForInterrupt()
}

func (b *BotApplication) Stop() {
	b.Bot.Stop()
}
