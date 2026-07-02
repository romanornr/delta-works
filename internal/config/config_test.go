package config

import (
	"os"
	"path/filepath"
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
		{"env secret nested", cfg.Venues["bybit"].APIKey, "k123"},
		{"venue rate", cfg.Venues["bybit"].Rate.RPS, 5.0},
	}
	for _, tt := range tests {
		if tt.got != tt.want {
			t.Errorf("%s: got %v, want %v", tt.name, tt.got, tt.want)
		}
	}
}

func TestLoadMissingDefaultFileIsFine(t *testing.T) {
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
		{"enabled venue without accounts", func(c *Config) {
			c.Venues = map[string]Venue{"x": {Enabled: true, Rate: Rate{RPS: 1, Burst: 1}}}
		}},
		{"enabled venue without rate", func(c *Config) {
			c.Venues = map[string]Venue{"x": {Enabled: true, Accounts: []string{"spot"}}}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Config{
				Log:      Log{Level: "info", Format: "console"},
				HTTP:     HTTP{Addr: ":8080"},
				Snapshot: Snapshot{Interval: time.Minute},
			}
			tt.mutate(&cfg)
			if err := cfg.Validate(); err == nil {
				t.Error("expected validation error, got nil")
			}
		})
	}
}
