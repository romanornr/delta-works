package config

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/viper"
)

// Load reads configuration from the specified YAML file path.
// If the file does not exist, it returns a config with defaults.
// Environment variables with EnvPrefix (e.g., DELTAWORKS_) override file values.
func Load(path string) (*Config, error) {
	v := viper.New()

	// Set defaults
	setDefaults(v)

	// Configure Viper for YAML
	v.SetConfigType("yaml")

	// Try to read the config file if it exists
	if path != "" {
		v.SetConfigFile(path)
		if err := v.ReadInConfig(); err != nil {
			// Only fail if the file exists but cannot be read
			if _, statErr := os.Stat(path); statErr == nil {
				return nil, fmt.Errorf("failed to read config [path=%s]: %w", path, err)
			}
			// File doesn't exist, continue with defaults
		}
	}

	// Enable environment variable overrides using the centralized prefix
	v.SetEnvPrefix(EnvPrefix)
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	// Bind specific environment variables for nested config
	bindEnvVars(v)

	// Unmarshal into Config struct
	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	// Normalize config values
	cfg.Normalize()

	// validate entire config
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	return &cfg, nil
}

// setDefaults configures all default values for the configuration.
func setDefaults(v *viper.Viper) {
	// App defaults
	v.SetDefault("app.name", "deltaworks")
	v.SetDefault("app.shutdown_timeout", 30*time.Second)

	// Logging defaults
	v.SetDefault("logging.level", "info")
	v.SetDefault("logging.format", "console")
	v.SetDefault("logging.time_format", "02/01/2006 15:04")

	// GCT defaults
	v.SetDefault("gct.config_path", "")
	v.SetDefault("gct.verbose", false)
	v.SetDefault("gct.grpc.enabled", true)
	v.SetDefault("gct.websocket.enabled", true)

	// QuestDB defaults
	v.SetDefault("questdb.http.address", "localhost:9000")
	v.SetDefault("questdb.http.validate_host", true)
	v.SetDefault("questdb.http.allowed_hosts", []string{"localhost", "127.0.0.1"})
	v.SetDefault("questdb.postgres.host", "localhost")
	v.SetDefault("questdb.postgres.port", 8812)
	v.SetDefault("questdb.postgres.user", "admin")
	v.SetDefault("questdb.postgres.password", "quest")
	v.SetDefault("questdb.postgres.database", "qdb")
	v.SetDefault("questdb.postgres.sslmode", "disable")

	// Portfolio defaults
	v.SetDefault("portfolio.observation.interval", 10*time.Minute)
	v.SetDefault("portfolio.observation.exchanges", []string{"bybit"})
	v.SetDefault("portfolio.observation.accounts", []string{"spot"})

	// Transfers defaults
	v.SetDefault("transfers.sync.enabled", true)
	v.SetDefault("transfers.sync.lookback", 720*time.Hour) // 30 days
}

// bindEnvVars explicitly binds environment variables for nested configuration.
// This ensures environment variables work correctly with nested structs.
// Uses EnvPrefix constant to build environment variable names.
func bindEnvVars(v *viper.Viper) {
	// Helper to build env var name with prefix
	env := func(name string) string {
		return EnvPrefix + "_" + name
	}

	// App
	_ = v.BindEnv("app.name", env("APP_NAME"))
	_ = v.BindEnv("app.shutdown_timeout", env("APP_SHUTDOWN_TIMEOUT"))

	// Logging
	_ = v.BindEnv("logging.level", env("LOG_LEVEL"))
	_ = v.BindEnv("logging.format", env("LOG_FORMAT"))
	_ = v.BindEnv("logging.time_format", env("LOG_TIME_FORMAT"))

	// GCT
	_ = v.BindEnv("gct.config_path", env("GCT_CONFIG_PATH"))
	_ = v.BindEnv("gct.verbose", env("GCT_VERBOSE"))
	_ = v.BindEnv("gct.grpc.enabled", env("GCT_GRPC_ENABLED"))
	_ = v.BindEnv("gct.websocket.enabled", env("GCT_WS_ENABLED"))

	// QuestDB
	_ = v.BindEnv("questdb.http.address", env("QUESTDB_HTTP_ADDR"))
	_ = v.BindEnv("questdb.http.validate_host", env("QUESTDB_HTTP_VALIDATE_HOST"))
	_ = v.BindEnv("questdb.postgres.host", env("QUESTDB_PG_HOST"))
	_ = v.BindEnv("questdb.postgres.port", env("QUESTDB_PG_PORT"))
	_ = v.BindEnv("questdb.postgres.user", env("QUESTDB_PG_USER"))
	_ = v.BindEnv("questdb.postgres.password", env("QUESTDB_PG_PASS"))
	_ = v.BindEnv("questdb.postgres.database", env("QUESTDB_PG_DB"))
	_ = v.BindEnv("questdb.postgres.sslmode", env("QUESTDB_PG_SSLMODE"))

	// Portfolio
	_ = v.BindEnv("portfolio.observation.interval", env("PORTFOLIO_INTERVAL"))

	// Transfers
	_ = v.BindEnv("transfers.sync.enabled", env("TRANSFERS_SYNC_ENABLED"))
	_ = v.BindEnv("transfers.sync.lookback", env("TRANSFERS_LOOKBACK"))
}
