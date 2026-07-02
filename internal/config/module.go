package config

import (
	"os"

	"go.uber.org/fx"
)

// Module provides configuration to Fx dependency injection containers.
// It loads configuration from the path specified by {EnvPrefix}_CONFIG_PATH environment variable or uses defaults if not set.
var Module = fx.Module("config", fx.Provide(NewConfig))

func NewConfig() (*Config, error) {
	path := os.Getenv(EnvPrefix + "_CONFIG_PATH")
	if path == "" {
		path = "config.yaml"
	}
	return Load(path)
}
