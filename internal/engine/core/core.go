package delta

import (
	"github.com/thrasher-corp/gocryptotrader/engine"
	gctlog "github.com/thrasher-corp/gocryptotrader/log"
	"sync"
)

// create a singleton instance of gocryptotrader engine
var instance *Core
var once sync.Once

type Core struct {
	Engine *engine.Engine
}

func GetInstance() *Core {
	once.Do(func() {
		instance = &Core{}
	})
	return instance
}

func (c *Core) Initialize(settings *engine.Settings, flagset map[string]bool) error {
	var err error
	c.Engine, err = engine.NewFromSettings(settings, flagset)
	if engine.Bot == nil || err != nil {
		return err
	}

	return nil
}

func (c *Core) StartEngine() error {
	if err := c.Engine.Start(); err != nil {
		errClose := gctlog.CloseLogger()
		if errClose != nil {
			return errClose
		}
		return errClose
	}
	return nil
}

func (c *Core) StopEngine() error {
	c.Engine.Stop()
	return nil
}
