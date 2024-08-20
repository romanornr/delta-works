package core

import (
	"context"
	"fmt"
	"github.com/thrasher-corp/gocryptotrader/config"
	"github.com/thrasher-corp/gocryptotrader/engine"
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
		config.SetConfig(engine.Bot.Config)
	})

	return instance, err
}

func (i *Instance) Initialize(ctx context.Context) error {
	var err error

	engine.Bot, err = engine.NewFromSettings(instance.Settings, instance.FlagSet)
	if engine.Bot == nil || err != nil {
		return fmt.Errorf("failed to create engine: %v", err)
	}
	return nil
}
