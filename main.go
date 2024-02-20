package main

import (
	"fmt"
	"github.com/romanornr/delta-works/util"
	"github.com/thrasher-corp/gocryptotrader/config"
	"github.com/thrasher-corp/gocryptotrader/engine"
	"log"
)

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

	engine, err := engine.NewFromSettings(&settings, nil)

	fmt.Println(engine)

	if err := engine.Start(); err != nil {
		log.Fatalf("Failed to start engine. Error: %s", err)
	}

	e, err := engine.ExchangeManager.GetExchangeByName("bybit")
	if err != nil {
		log.Fatalf("Failed to get exchange. Error: %s", err)
	}

	fmt.Println(e.GetName()) // print bybit

}
