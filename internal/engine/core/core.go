package core

import (
	"context"
	"fmt"
	"github.com/thrasher-corp/gocryptotrader/config"
	"github.com/thrasher-corp/gocryptotrader/engine"
	gctlog "github.com/thrasher-corp/gocryptotrader/log"
	"sync"
)

var (
	instance *Instance
	once     sync.Once
)

type Instance struct {
	Settings *engine.Settings
	FlagSet  map[string]bool
}

func GetInstance(ctx context.Context, settings *engine.Settings, flagset map[string]bool) (*Instance, error) {

	var err error
	once.Do(func() {
		instance = &Instance{
			Settings: settings,
			FlagSet:  flagset,
		}

		err = instance.Initialize(ctx)
		if err == nil {
			config.SetConfig(engine.Bot.Config)
		}
	})

	return instance, err
}

func (i *Instance) Initialize(ctx context.Context) error {
	var err error

	engine.Bot, err = engine.NewFromSettings(instance.Settings, instance.FlagSet)
	if err != nil {
		return fmt.Errorf("failed to create engine: %v", err)
	}

	if engine.Bot == nil {
		return fmt.Errorf("engine initialization failed: Bot is nil")
	}

	gctlog.Debugln(gctlog.Global, "Engine successfully initialized")

	return nil
}
