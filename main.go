package main

import (
	"fmt"
	"github.com/romanornr/delta-works/util"
	"github.com/thrasher-corp/gocryptotrader/config"
	"github.com/thrasher-corp/gocryptotrader/engine"
	gctlog "github.com/thrasher-corp/gocryptotrader/log"
	"github.com/thrasher-corp/gocryptotrader/signaler"
	"log"
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

func main() {

	str := util.ConfigFile("config.json")
	fmt.Printf("Config file: %s\n", str)

	configFile, err := config.GetAndMigrateDefaultPath(str)
	if err != nil {
		log.Fatalf("Failed to get config file. Error: %s", err)
	}

	fmt.Printf("Filepath: %s\n", configFile)

	var botconfig config.Config
	err = botconfig.LoadConfig(configFile, true)
	if err != nil {
		log.Fatalf("Failed to load config file. Error: %s", err)
	}

	var settings engine.Settings

	engine.Bot, err = engine.NewFromSettings(&settings, nil)

	if err := engine.Bot.Start(); err != nil {
		errClose := gctlog.CloseLogger()
		if errClose != nil {
			log.Printf("Failed to close logger. Error: %s", errClose)
		}
		log.Fatalf("Failed to start engine. Error: %s", err)
	}

	e, err := engine.Bot.GetExchangeByName("bybit")
	if err != nil {
		log.Fatalf("Failed to get exchange. Error: %s", err)
	}

	fmt.Println(e.GetName()) // print bybit

	go waitForInterrupt(settings.Shutdown)
	<-settings.Shutdown
	engine.Bot.Stop()
}

// waitForInterrupt waits for an interrupt signal and sends a signal on the
// waiter channel to indicate that the program should shut down.
func waitForInterrupt(waiter chan struct{}) {
	interrupt := signaler.WaitForInterrupt()
	gctlog.Infof(gctlog.Global, "Captured %v, shutdown requested.\n", interrupt)
	// Send a signal on the waiter channel to indicate that the program should shut down.
	waiter <- struct{}{}
}
