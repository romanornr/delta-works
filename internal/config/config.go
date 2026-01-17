package config

import (
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"
)

// EnvPrefix is the prefix for all environment variables.
// Change this single constant when rebranding the application.
// Example: APP_LOG_LEVEL where APP is this prefix.
const EnvPrefix = "DELTAWORKS"

// Config is the root configuration structure for the application.
type Config struct {
	App       AppConfig       `mapstructure:"app"`
	Logging   LoggingConfig   `mapstructure:"logging"`
	GCT       GCTConfig       `mapstructure:"gct"`
	QuestDB   QuestDBConfig   `mapstructure:"questdb"`
	Portfolio PortfolioConfig `mapstructure:"portfolio"`
	Transfers TransfersConfig `mapstructure:"transfers"`
}

// AppConfig contains general application settings.
type AppConfig struct {
	Name            string        `mapstructure:"name"`
	ShutdownTimeout time.Duration `mapstructure:"shutdown_timeout"`
}

// LoggingConfig contains logging settings.
type LoggingConfig struct {
	// Level is the minimum log level: trace, debug, info, warn, error, fatal, panic
	Level string `mapstructure:"level"`
	// Format is the output format: console or json
	Format string `mapstructure:"format"`
	// TimeFormat is the time format for console output (Go time format string)
	TimeFormat string `mapstructure:"time_format"`
}

// GCTConfig contains GoCryptoTrader settings.
type GCTConfig struct {
	ConfigPath string     `mapstructure:"config_path"`
	Verbose    bool       `mapstructure:"verbose"`
	GRPC       GRPCConfig `mapstructure:"grpc"`
	WebSocket  WSConfig   `mapstructure:"websocket"`
}

// GRPCConfig contains gRPC settings for GoCryptoTrader.
type GRPCConfig struct {
	Enabled bool `mapstructure:"enabled"`
}

// WSConfig contains WebSocket settings for GoCryptoTrader.
type WSConfig struct {
	Enabled bool `mapstructure:"enabled"`
}

// QuestDBConfig contains QuestDB connection settings.
type QuestDBConfig struct {
	HTTP     QuestDBHTTPConfig `mapstructure:"http"`
	Postgres QuestDBPGConfig   `mapstructure:"postgres"`
}

// QuestDBHTTPConfig contains QuestDB HTTP/ILP settings.
type QuestDBHTTPConfig struct {
	Address      string   `mapstructure:"address"`
	ValidateHost bool     `mapstructure:"validate_host"`
	AllowedHosts []string `mapstructure:"allowed_hosts"`
}

// QuestDBPGConfig contains QuestDB PostgreSQL wire protocol settings.
type QuestDBPGConfig struct {
	Host     string `mapstructure:"host"`
	Port     int    `mapstructure:"port"`
	User     string `mapstructure:"user"`
	Password string `mapstructure:"password"`
	Database string `mapstructure:"database"`
	SSLMode  string `mapstructure:"sslmode"`
}

// Validate checks that the address host is in the allowlist.
func (c *QuestDBHTTPConfig) Validate() error {
	if !c.ValidateHost {
		return nil
	}

	host, port, err := net.SplitHostPort(c.Address)
	if err != nil {
		return fmt.Errorf("invalid address format %s: %w", c.Address, err)
	}

	if _, err := strconv.Atoi(port); err != nil {
		return fmt.Errorf("invalid port %s: %w", port, err)
	}

	for _, allowedHost := range c.AllowedHosts {
		if host == allowedHost {
			return nil
		}
	}

	return fmt.Errorf("host not in allowlist [host=%s allowed=%v]", host, c.AllowedHosts)
}

// LineSenderURI returns the ILP HTTP URI for QuestDB line sender.
func (c *QuestDBConfig) LineSenderURI() string {
	return fmt.Sprintf("http::addr=%s", c.HTTP.Address)
}

// PostgresConnStr returns the PostgreSQL connection string for QuestDB queries.
func (c *QuestDBConfig) PostgresConnStr() string {
	return fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		c.Postgres.Host,
		c.Postgres.Port,
		c.Postgres.User,
		c.Postgres.Password,
		c.Postgres.Database,
		c.Postgres.SSLMode,
	)
}

// PortfolioConfig contains portfolio observation settings.
type PortfolioConfig struct {
	Observation ObservationConfig `mapstructure:"observation"`
}

// ObservationConfig contains settings for portfolio snapshot capture.
type ObservationConfig struct {
	// Interval is how often to capture portfolio snapshots
	Interval time.Duration `mapstructure:"interval"`
	// Exchanges is the list of exchange names to observe
	Exchanges []string `mapstructure:"exchanges"`
	// Accounts is the list of account types to observe (e.g., spot, margin)
	Accounts []string `mapstructure:"accounts"`
}

// TransfersConfig contains transfer tracking settings.
type TransfersConfig struct {
	Sync SyncConfig `mapstructure:"sync"`
}

// SyncConfig contains settings for transfer synchronization.
type SyncConfig struct {
	// Enabled controls whether transfer syncing is active
	Enabled bool `mapstructure:"enabled"`
	// Lookback is how far back to sync transfers on startup
	Lookback time.Duration `mapstructure:"lookback"`
}

// Validate validates the full configuration.
//
// This is intentionally strict for SSRF protection: QuestDB HTTP host must be
// explicitly allowlisted.
func (c *Config) Validate() error {
	if err := c.QuestDB.HTTP.Validate(); err != nil {
		return fmt.Errorf("invalid questdb.http config: %w", err)
	}

	if c.App.Name == "" {
		return fmt.Errorf("invalid app config: name is required")
	}

	if c.App.ShutdownTimeout <= 0 {
		return fmt.Errorf("invalid app config: shutdown_timeout must be > 0")
	}

	switch c.Logging.Format {
	case "console", "json":
		// ok
	default:
		return fmt.Errorf("invalid logging config: format must be console or json")
	}

	if c.Logging.Level == "" {
		return fmt.Errorf("invalid logging config: level is required")
	}

	if c.Portfolio.Observation.Interval <= 0 {
		return fmt.Errorf("invalid portfolio.observation config: interval must be > 0")
	}

	if len(c.Portfolio.Observation.Exchanges) == 0 {
		return fmt.Errorf("invalid portfolio.observation config: exchanges must not be empty")
	}

	if len(c.Portfolio.Observation.Accounts) == 0 {
		return fmt.Errorf("invalid portfolio.observation config: accounts must not be empty")
	}

	if c.Transfers.Sync.Lookback < 0 {
		return fmt.Errorf("invalid transfers.sync config: lookback must be >= 0")
	}

	return nil
}

// Normalize standardizes configuration values.
// Call this after loading config to ensure consistent formatting.
func (c *Config) Normalize() {
	c.Logging.Level = strings.ToLower(c.Logging.Level)
	c.Logging.Format = strings.ToLower(c.Logging.Format)

	for i, exch := range c.Portfolio.Observation.Exchanges {
		c.Portfolio.Observation.Exchanges[i] = strings.ToLower(exch)
	}

	for i, acc := range c.Portfolio.Observation.Accounts {
		c.Portfolio.Observation.Accounts[i] = strings.ToLower(acc)
	}
}
