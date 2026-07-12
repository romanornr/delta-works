package config

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"
)

func writeFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadDefaultsFileAndEnv(t *testing.T) {
	path := writeFile(t, `
log:
  level: debug
postgres:
  dsn: "postgres://user:pass@localhost:5432/db"
questdb:
  conf: "http::addr=localhost:9000;"
snapshot:
  interval: 30s
venues:
  bybit:
    enabled: true
    accounts: [spot]
    rate:
      rps: 5
      burst: 10
`)
	t.Setenv("DELTA__LOG__FORMAT", "json")
	t.Setenv("DELTA__VENUES__BYBIT__API_KEY", "k123")
	t.Setenv("DELTA__VENUES__BYBIT__API_SECRET", "s456")
	t.Setenv("DELTA__VENUES__BYBIT__ACCOUNTS", "spot, margin")

	cfg, err := Load(path, true)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	tests := []struct {
		name string
		got  any
		want any
	}{
		{"file overrides default", cfg.Log.Level, "debug"},
		{"env overrides default", cfg.Log.Format, "json"},
		{"default survives", cfg.HTTP.Addr, ":8080"},
		{"duration parsed", cfg.Snapshot.Interval, 30 * time.Second},
		{"reconcile default", cfg.Reconcile.Interval, 30 * time.Second},
		{"order submit budget default", cfg.Order.SubmitBudget, 10 * time.Second},
		{"env secret nested", cfg.Venues["bybit"].APIKey, "k123"},
		{"venue rate", cfg.Venues["bybit"].Rate.RPS, 5.0},
	}
	for _, tt := range tests {
		if tt.got != tt.want {
			t.Errorf("%s: got %v, want %v", tt.name, tt.got, tt.want)
		}
	}
	if accounts := cfg.Venues["bybit"].Accounts; !slices.Equal(accounts, []string{"spot", "margin"}) {
		t.Errorf("env accounts list: got %v, want [spot margin]", accounts)
	}
}

func TestLoadSecretFiles(t *testing.T) {
	dir := t.TempDir()
	keyPath := filepath.Join(dir, "key")
	secretPath := filepath.Join(dir, "secret")
	if err := os.WriteFile(keyPath, []byte("k123\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	pem := "-----BEGIN EC PRIVATE KEY-----\nabc\ndef\n-----END EC PRIVATE KEY-----"
	if err := os.WriteFile(secretPath, []byte(pem+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	path := writeFile(t, `
postgres:
  dsn: "postgres://user:pass@localhost:5432/db"
questdb:
  conf: "http::addr=localhost:9000;"
venues:
  bybit:
    enabled: true
    accounts: [spot]
    rate: {rps: 5, burst: 10}
    api_key_file: "`+keyPath+`"
    api_secret_file: "`+secretPath+`"
`)
	cfg, err := Load(path, true)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := cfg.Venues["bybit"].APIKey; got != "k123" {
		t.Errorf("api key from file: got %q", got)
	}
	if got := cfg.Venues["bybit"].APISecret; got != pem {
		t.Errorf("multiline secret from file: got %q", got)
	}
}

func TestLoadSecretFileErrors(t *testing.T) {
	tests := []struct {
		name  string
		venue string
	}{
		{"value and file conflict", `
    api_key: direct
    api_key_file: /dev/null
    api_secret: s`},
		{"missing file", `
    api_key_file: /nonexistent/key
    api_secret: s`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := writeFile(t, `
postgres:
  dsn: "postgres://user:pass@localhost:5432/db"
questdb:
  conf: "http::addr=localhost:9000;"
venues:
  bybit:
    enabled: true
    accounts: [spot]
    rate: {rps: 5, burst: 10}`+tt.venue+`
`)
			if _, err := Load(path, true); err == nil {
				t.Error("expected error, got nil")
			}
		})
	}
}

func TestLoadMissingDefaultFileIsFine(t *testing.T) {
	t.Setenv("DELTA__POSTGRES__DSN", "postgres://user:pass@localhost:5432/db")
	t.Setenv("DELTA__QUESTDB__CONF", "http::addr=localhost:9000;")
	cfg, err := Load(filepath.Join(t.TempDir(), "absent.yaml"), false)
	if err != nil {
		t.Fatalf("Load with absent default file: %v", err)
	}
	if cfg.Log.Level != "info" {
		t.Errorf("expected default level info, got %q", cfg.Log.Level)
	}
}

func TestLoadMissingExplicitFileFails(t *testing.T) {
	if _, err := Load(filepath.Join(t.TempDir(), "absent.yaml"), true); err == nil {
		t.Fatal("expected error for explicitly provided missing file")
	}
}

func TestValidateRejectsBadValues(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Config)
	}{
		{"bad level", func(c *Config) { c.Log.Level = "loud" }},
		{"bad format", func(c *Config) { c.Log.Format = "xml" }},
		{"empty addr", func(c *Config) { c.HTTP.Addr = "" }},
		{"zero interval", func(c *Config) { c.Snapshot.Interval = 0 }},
		{"outbox interval out of range", func(c *Config) { c.Outbox.Interval = 5 * time.Second }},
		{"outbox batch out of range", func(c *Config) { c.Outbox.Batch = 0 }},
		{"reconcile interval too short", func(c *Config) { c.Reconcile.Interval = time.Second }},
		{"reconcile interval too long", func(c *Config) { c.Reconcile.Interval = 10 * time.Minute }},
		{"order submit budget too short", func(c *Config) { c.Order.SubmitBudget = time.Millisecond }},
		{"order submit budget too long", func(c *Config) { c.Order.SubmitBudget = 2 * time.Minute }},
		{"trading venue disabled", func(c *Config) {
			c.Venues = map[string]Venue{"x": {Trading: true}}
		}},
		{"enabled venue without accounts", func(c *Config) {
			c.Venues = map[string]Venue{"x": {Enabled: true, Rate: Rate{RPS: 1, Burst: 1}}}
		}},
		{"enabled venue without rate", func(c *Config) {
			c.Venues = map[string]Venue{"x": {Enabled: true, Accounts: []string{"spot"}}}
		}},
		{"empty postgres dsn", func(c *Config) { c.Postgres.DSN = "" }},
		{"empty questdb conf", func(c *Config) { c.QuestDB.Conf = "" }},
		{"api key without secret", func(c *Config) {
			c.Venues = map[string]Venue{"x": {
				Enabled: true, Accounts: []string{"spot"},
				Rate: Rate{RPS: 1, Burst: 1}, APIKey: "k",
			}}
		}},
		{"enabled venue without credentials", func(c *Config) {
			c.Venues = map[string]Venue{"x": {
				Enabled: true, Accounts: []string{"spot"},
				Rate: Rate{RPS: 1, Burst: 1},
			}}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Config{
				Log:       Log{Level: "info", Format: "console"},
				HTTP:      HTTP{Addr: ":8080"},
				Postgres:  Postgres{DSN: "postgres://user:pass@localhost:5432/db"},
				QuestDB:   QuestDB{Conf: "http::addr=localhost:9000;"},
				Snapshot:  Snapshot{Interval: time.Minute},
				Outbox:    Outbox{Interval: 500 * time.Millisecond, Batch: 100},
				Reconcile: Reconcile{Interval: 30 * time.Second},
				Order:     Order{SubmitBudget: 10 * time.Second},
			}
			tt.mutate(&cfg)
			if err := cfg.Validate(); err == nil {
				t.Error("expected validation error, got nil")
			}
		})
	}
}

func TestAPIAddr(t *testing.T) {
	tests := []struct {
		name    string
		content string
		missing bool
		want    string
		wantErr bool
	}{
		{name: "set", content: "api:\n  addr: unix:///tmp/x.sock\n", want: "unix:///tmp/x.sock"},
		{name: "absent key", content: "log:\n  level: info\n", want: ""},
		{name: "missing file", missing: true, want: ""},
		{name: "invalid yaml", content: "api: [broken\n", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.yaml")
			if !tt.missing {
				path = writeFile(t, tt.content)
			}
			got, err := APIAddr(path)
			if (err != nil) != tt.wantErr {
				t.Fatalf("APIAddr error = %v, wantErr %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Fatalf("APIAddr = %q, want %q", got, tt.want)
			}
		})
	}
}
