package config

import (
	"strings"
	"testing"
)

func TestQuestDBHTTPConfigValidateAllowsConfiguredHost(t *testing.T) {
	cfg := QuestDBHTTPConfig{
		Address:      "questdb.internal:9000",
		ValidateHost: true,
		AllowedHosts: []string{"questdb.internal"},
	}

	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestQuestDBHTTPConfigValidateRejectsDisabledValidation(t *testing.T) {
	cfg := QuestDBHTTPConfig{
		Address:      "questdb.internal:9000",
		ValidateHost: false,
		AllowedHosts: []string{"questdb.internal"},
	}

	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestQuestDBHTTPConfigValidateAllowsExplicitWildcardOptOut(t *testing.T) {
	cfg := QuestDBHTTPConfig{
		Address:      "questdb.internal:9000",
		ValidateHost: true,
		AllowedHosts: []string{"*"},
	}

	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestQuestDBConfigPostgresConnStrEscapesValues(t *testing.T) {
	cfg := QuestDBConfig{
		Postgres: QuestDBPGConfig{
			Host:     "questdb.internal",
			Port:     8812,
			User:     "user name",
			Password: "pa:ss word",
			Database: "qdb/test?name",
			SSLMode:  "disable&application_name=evil",
		},
	}

	connStr := cfg.PostgresConnStr()

	checks := []string{
		"postgres://user%20name:pa%3Ass%20word@questdb.internal:8812/qdb/test%3Fname",
		"sslmode=disable%26application_name%3Devil",
	}

	for _, check := range checks {
		if !strings.Contains(connStr, check) {
			t.Fatalf("expected connection string to contain %q, got %q", check, connStr)
		}
	}
}
