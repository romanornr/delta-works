// Package config loads and validates the application configuration from a
// YAML file merged with DELTA__-prefixed environment variables (env wins).
// Secrets such as venue API keys are env-only by convention.
package config

import (
	"errors"
	"fmt"
	"time"
)

// EnvPrefix is the prefix for environment variable overrides, e.g.
// DELTA__LOG__LEVEL=debug maps to log.level.
const EnvPrefix = "DELTA__"

// Config is the root application configuration.
type Config struct {
	Log      Log              `koanf:"log"`
	HTTP     HTTP             `koanf:"http"`
	Postgres Postgres         `koanf:"postgres"`
	QuestDB  QuestDB          `koanf:"questdb"`
	Snapshot Snapshot         `koanf:"snapshot"`
	Venues   map[string]Venue `koanf:"venues"`
}

// Log configures logging output.
type Log struct {
	Level  string `koanf:"level"`
	Format string `koanf:"format"`
}

// HTTP configures the telemetry HTTP server (/metrics, /healthz, /readyz).
type HTTP struct {
	Addr string `koanf:"addr"`
}

// Postgres configures the durable state store.
type Postgres struct {
	DSN string `koanf:"dsn"`
}

// QuestDB configures the time-series store. Conf is a QuestDB client
// configuration string, e.g. "http::addr=localhost:9000;".
type QuestDB struct {
	Conf string `koanf:"conf"`
}

// Snapshot configures the portfolio snapshot poller.
type Snapshot struct {
	Interval time.Duration `koanf:"interval"`
}

// Venue configures one exchange connection.
type Venue struct {
	Enabled   bool     `koanf:"enabled"`
	Accounts  []string `koanf:"accounts"`
	Rate      Rate     `koanf:"rate"`
	APIKey    string   `koanf:"api_key"`
	APISecret string   `koanf:"api_secret"`
}

// Rate configures the client-side rate limiter for a venue.
type Rate struct {
	RPS   float64 `koanf:"rps"`
	Burst int     `koanf:"burst"`
}

var validLevels = map[string]bool{
	"trace": true, "debug": true, "info": true, "warn": true, "error": true,
}

// Validate checks invariants that would otherwise surface as confusing
// runtime failures.
func (c Config) Validate() error {
	var errs []error
	if !validLevels[c.Log.Level] {
		errs = append(errs, fmt.Errorf("log.level %q: must be one of trace|debug|info|warn|error", c.Log.Level))
	}
	if c.Log.Format != "console" && c.Log.Format != "json" {
		errs = append(errs, fmt.Errorf("log.format %q: must be console or json", c.Log.Format))
	}
	if c.HTTP.Addr == "" {
		errs = append(errs, errors.New("http.addr: must not be empty"))
	}
	if c.Snapshot.Interval <= 0 {
		errs = append(errs, fmt.Errorf("snapshot.interval %s: must be positive", c.Snapshot.Interval))
	}
	if c.Postgres.DSN == "" {
		errs = append(errs, errors.New("postgres.dsn: must not be empty"))
	}
	if c.QuestDB.Conf == "" {
		errs = append(errs, errors.New("questdb.conf: must not be empty"))
	}
	for name, v := range c.Venues {
		if !v.Enabled {
			continue
		}
		if v.Rate.RPS <= 0 || v.Rate.Burst <= 0 {
			errs = append(errs, fmt.Errorf("venues.%s.rate: rps and burst must be positive", name))
		}
		if len(v.Accounts) == 0 {
			errs = append(errs, fmt.Errorf("venues.%s.accounts: at least one account required", name))
		}
		if v.APIKey == "" || v.APISecret == "" {
			errs = append(errs, fmt.Errorf("venues.%s: api_key and api_secret are required for an enabled venue; balance snapshots authenticate", name))
		}
	}
	return errors.Join(errs...)
}

// EnabledVenues returns the names of venues with enabled: true.
func (c Config) EnabledVenues() []string {
	var names []string
	for name, v := range c.Venues {
		if v.Enabled {
			names = append(names, name)
		}
	}
	return names
}
