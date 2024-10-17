package core

import (
	"context"
	"fmt"
	"github.com/romanornr/delta-works/internal/logger"
	"github.com/thrasher-corp/gocryptotrader/config"
	"github.com/thrasher-corp/gocryptotrader/engine"
	"sync"
	"time"
)

var (
	instance   *Instance
	instanceMu sync.Mutex
	once       sync.Once
)

// Instance manages settings and configuration flags for initializing and controlling the engine.
type Instance struct {
	Settings *engine.Settings
	FlagSet  map[string]bool
}

// GetInstance initializes and retrieves a singleton instance of the application, ensuring thread safety and context-aware setup.
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

	// Prevent concurrent reads during error handling without the lock, another goroutine might concurrently execute 'once.Do' again
	// Even if 'err' is nil, the lock ensures that the 'instance' assignment (happening inside 'once.Do') is fully visible
	// to all goroutines before we return 'instance'.
	instanceMu.Lock()
	defer instanceMu.Unlock()
	if err != nil {
		return nil, fmt.Errorf("failed to get instance: %w", err)
	}

	return instance, err
}

// Initialize configures and starts the engine based on provided settings, handling context cancellation and errors.
func (i *Instance) Initialize(ctx context.Context) error {
	errChan := make(chan error, 1)
	done := make(chan struct{})

	go func() {
		var err error
		engine.Bot, err = engine.NewFromSettings(i.Settings, i.FlagSet)
		if err != nil {
			errChan <- err
		} else {
			close(done)
		}
	}()

	select {
	case <-ctx.Done():
		return fmt.Errorf("engine initialization cancelled: %v", ctx.Err())
	case err := <-errChan:
		return fmt.Errorf("failed to create engine: %v", err)
	case <-done:
		if engine.Bot == nil {
			return fmt.Errorf("engine initialization failed: Bot is nil")
		}
		logger.Info().Msg("Engine successfully initialized")
		return nil
	}
}

func (i *Instance) StartEngine(ctx context.Context) error {
	if engine.Bot == nil {
		return fmt.Errorf("engine not initialized")
	}

	errChan := make(chan error, 1)
	go func() {
		errChan <- engine.Bot.Start()
	}()

	select {
	case <-ctx.Done():
		return fmt.Errorf("engine start cancelled: %v", ctx.Err())
	case err := <-errChan:
		if err != nil {
			return fmt.Errorf("failed to start engine: %v", err)
		}
		return nil
	}
}

func (i *Instance) StopEngine(ctx context.Context) error {
	if engine.Bot == nil {
		return fmt.Errorf("engine not initialized")
	}

	done := make(chan struct{})
	var err error

	go func() {
		engine.Bot.Stop()
		close(done)
	}()

	select {
	case <-ctx.Done():
		// Context was cancelled, but we'll still wait a bit for the engine to stop
		select {
		case <-done:
			// Engine stopped before our additional timeout
		case <-time.After(5 * time.Second):
			err = fmt.Errorf("engine stop canceled: %w", ctx.Err())
		}
	case <-done:
		// Engine stopped normally
		logger.Info().Msg("Engine stopped")
	}

	return err
}
