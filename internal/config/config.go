// Package config loads and validates the application configuration from a
// YAML file merged with DELTA__-prefixed environment variables (env wins).
// Secrets such as venue API keys never go in the config file: they are
// referenced by path (api_key_file) or injected via environment.
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
	Log       Log              `koanf:"log"`
	HTTP      HTTP             `koanf:"http"`
	API       API              `koanf:"api"`
	Postgres  Postgres         `koanf:"postgres"`
	QuestDB   QuestDB          `koanf:"questdb"`
	Snapshot  Snapshot         `koanf:"snapshot"`
	Outbox    Outbox           `koanf:"outbox"`
	Reconcile Reconcile        `koanf:"reconcile"`
	Order     Order            `koanf:"order"`
	Venues    map[string]Venue `koanf:"venues"`
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

// API configures the control-plane RPC server (ADR-0007). An empty Addr
// disables it. "unix:///path/to.sock" listens on a Unix socket with 0600
// permissions; anything else is a TCP host:port.
type API struct {
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

// Outbox configures the outbox relay (ADR-0008).
type Outbox struct {
	Interval time.Duration `koanf:"interval"`
	Batch    int           `koanf:"batch"`
}

// Reconcile configures the venue-vs-local reconciliation loop.
type Reconcile struct {
	Interval time.Duration `koanf:"interval"`
}

// Order configures venue order submission retries. SubmitBudget bounds one
// invocation's venue-submit retries, not the end-to-end RPC duration.
type Order struct {
	SubmitBudget time.Duration `koanf:"submit_budget"`
}

// Venue configures one exchange connection. Each credential is either a
// direct value or a path to a secret file, not both. Files carry multiline
// secrets such as PEM keys (ADR-0006).
type Venue struct {
	Enabled       bool     `koanf:"enabled"`
	Trading       bool     `koanf:"trading"`
	Accounts      []string `koanf:"accounts"`
	Rate          Rate     `koanf:"rate"`
	APIKey        string   `koanf:"api_key"`
	APISecret     string   `koanf:"api_secret"`
	APIKeyFile    string   `koanf:"api_key_file"`
	APISecretFile string   `koanf:"api_secret_file"`
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
	if c.Outbox.Interval < 200*time.Millisecond || c.Outbox.Interval > time.Second {
		errs = append(errs, fmt.Errorf("outbox.interval %s: must be between 200ms and 1s", c.Outbox.Interval))
	}
	if c.Outbox.Batch <= 0 || c.Outbox.Batch > 1000 {
		errs = append(errs, fmt.Errorf("outbox.batch %d: must be between 1 and 1000", c.Outbox.Batch))
	}
	if c.Reconcile.Interval < 5*time.Second || c.Reconcile.Interval > 5*time.Minute {
		errs = append(errs, fmt.Errorf("reconcile.interval %s: must be between 5s and 5m", c.Reconcile.Interval))
	}
	if c.Order.SubmitBudget < time.Second || c.Order.SubmitBudget > time.Minute {
		errs = append(errs, fmt.Errorf("order.submit_budget %s: must be between 1s and 1m", c.Order.SubmitBudget))
	}
	if c.Postgres.DSN == "" {
		errs = append(errs, errors.New("postgres.dsn: must not be empty"))
	}
	if c.QuestDB.Conf == "" {
		errs = append(errs, errors.New("questdb.conf: must not be empty"))
	}
	for name, v := range c.Venues {
		if v.Trading && !v.Enabled {
			errs = append(errs, fmt.Errorf("venues.%s.trading: requires enabled=true", name))
		}
		errs = append(errs, v.validate(name)...)
	}
	return errors.Join(errs...)
}

func (v Venue) validate(name string) []error {
	if !v.Enabled {
		return nil
	}
	var errs []error
	if v.Rate.RPS <= 0 || v.Rate.Burst <= 0 {
		errs = append(errs, fmt.Errorf("venues.%s.rate: rps and burst must be positive", name))
	}
	if len(v.Accounts) == 0 {
		errs = append(errs, fmt.Errorf("venues.%s.accounts: at least one account required", name))
	}
	if v.APIKey == "" || v.APISecret == "" {
		errs = append(errs, fmt.Errorf("venues.%s: api_key and api_secret are required for an enabled venue; balance snapshots authenticate", name))
	}
	return errs
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
