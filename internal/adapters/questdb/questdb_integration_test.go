//go:build integration

package questdb

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/romanornr/delta-works/internal/config"
	"github.com/romanornr/delta-works/internal/domain/account"
)

func startQuestDB(t *testing.T) (config.QuestDB, string) {
	t.Helper()
	ctx := context.Background()
	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:        "questdb/questdb:latest",
			ExposedPorts: []string{"9000/tcp"},
			WaitingFor: wait.ForHTTP("/ping").WithPort("9000/tcp").
				WithStatusCodeMatcher(func(status int) bool { return status < 300 }).
				WithStartupTimeout(3 * time.Minute),
		},
		Started: true,
	})
	if err != nil {
		t.Fatalf("start questdb container: %v", err)
	}
	t.Cleanup(func() {
		if err := container.Terminate(context.Background()); err != nil {
			t.Logf("terminate container: %v", err)
		}
	})

	host, err := container.Host(ctx)
	if err != nil {
		t.Fatal(err)
	}
	port, err := container.MappedPort(ctx, "9000/tcp")
	if err != nil {
		t.Fatal(err)
	}
	addr := fmt.Sprintf("%s:%s", host, port.Port())
	return config.QuestDB{Conf: fmt.Sprintf("http::addr=%s;", addr)}, addr
}

func TestBalanceSnapshotRoundTrip(t *testing.T) {
	ctx := context.Background()
	cfg, addr := startQuestDB(t)

	w, err := New(ctx, cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer w.Close(ctx)

	snap := account.Snapshot{
		Account: account.Ref{Venue: "bybit", Type: account.TypeSpot},
		TakenAt: time.Now().UTC(),
		Balances: []account.Balance{
			{Currency: "BTC", Total: decimal.NewFromFloat(1.5), Free: decimal.NewFromFloat(1), Locked: decimal.NewFromFloat(0.5)},
			{Currency: "USDT", Total: decimal.NewFromInt(100), Free: decimal.NewFromInt(100)},
			{Currency: "DUST", Total: decimal.Zero}, // filtered by NonZero
		},
	}
	if err := w.WriteBalanceSnapshot(ctx, snap); err != nil {
		t.Fatalf("WriteBalanceSnapshot: %v", err)
	}
	if err := w.Flush(ctx); err != nil {
		t.Fatalf("Flush: %v", err)
	}

	// ILP ingestion is async; poll the SQL endpoint until rows are visible.
	count := pollCount(t, addr, "select count() from balances")
	if count != 2 {
		t.Errorf("balances rows: got %d, want 2 (zero balance must be filtered)", count)
	}

	if h := NewHealth(cfg); h.Check(ctx) != nil {
		t.Errorf("health check failed: %v", h.Check(ctx))
	}
}

func pollCount(t *testing.T, addr, query string) int {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := http.Get(fmt.Sprintf("http://%s/exec?query=%s", addr, url.QueryEscape(query)))
		if err == nil {
			var body struct {
				Dataset [][]any `json:"dataset"`
			}
			if json.NewDecoder(resp.Body).Decode(&body) == nil && len(body.Dataset) > 0 {
				resp.Body.Close()
				if n, ok := body.Dataset[0][0].(float64); ok && n > 0 {
					return int(n)
				}
			} else {
				resp.Body.Close()
			}
		}
		time.Sleep(500 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for rows: %s", query)
	return 0
}
