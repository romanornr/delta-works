package container

import (
	"testing"

	"github.com/romanornr/delta-works/internal/contracts"
	"github.com/thrasher-corp/gocryptotrader/engine"
)

func TestServiceRegistration(t *testing.T) {
	logger := NewDefaultLogger()
	container := NewServiceContainer(logger)
	registration := NewServiceRegistration(container)

	settings := &engine.Settings{
		ConfigFile: "config.json",
		DataDir:    "test-data",
	}

	flagset := map[string]bool{
		"configfile": true,
		"datadir":    true,
	}

	questDBConfig := "mock-questdb-config"

	err := registration.RegisterAllServices(settings, flagset, questDBConfig)
	if err != nil {
		t.Errorf("failed to register services: %v", err)
	}
}

func TestDefaultLogger(t *testing.T) {
	logger := NewDefaultLogger()

	// Test that logger implements Logger interface
	var _ contracts.Logger = logger

	// Test logging operations
	logger.Info().Msg("Testing info level logging")
	logger.Warn().Msg("Testing warn level logging")
	logger.Error().Msg("Testing error level logging")
	logger.Debug().Msg("Testing debug level logging")

	t.Log("Logger operations completed successfully")
}

func TestServiceRegistration_InterfaceCompliance(t *testing.T) {
	logger := NewDefaultLogger()
	var _ contracts.Logger = logger
	var _ contracts.LogEvent = logger.Info()

	t.Log("Interface compliance test passed")
}
