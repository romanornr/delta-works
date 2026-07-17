package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/shopspring/decimal"

	controlv1 "github.com/romanornr/delta-works/internal/api/gen/control/v1"
	"github.com/romanornr/delta-works/internal/api/gen/control/v1/controlv1connect"
	"github.com/romanornr/delta-works/internal/bus"
	"github.com/romanornr/delta-works/internal/domain/account"
	"github.com/romanornr/delta-works/internal/domain/instrument"
	"github.com/romanornr/delta-works/internal/domain/money"
	domainorder "github.com/romanornr/delta-works/internal/domain/order"
	"github.com/romanornr/delta-works/internal/events"
	"github.com/romanornr/delta-works/internal/log"
	"github.com/romanornr/delta-works/internal/ports"
	"github.com/romanornr/delta-works/internal/service/snapshot"
)

func testEventServer(t *testing.T, eventBus bus.Bus) *EventServer {
	t.Helper()
	metrics, err := NewMetrics(prometheus.NewRegistry())
	if err != nil {
		t.Fatal(err)
	}
	return NewEventServer(eventBus, log.Nop(), metrics)
}

func testSnapshot() account.Snapshot {
	return account.Snapshot{
		Account: account.Ref{Venue: instrument.NewVenueID("bybit"), Type: account.TypeSpot},
		TakenAt: time.Date(2026, 7, 4, 12, 0, 0, 0, time.UTC),
		Balances: []account.Balance{{
			Currency: money.Currency("BTC"),
			Total:    decimal.RequireFromString("1.5"),
			Free:     decimal.RequireFromString("1.25"),
			Locked:   decimal.RequireFromString("0.25"),
		}},
	}
}

// pumpEvents publishes the events every 10ms until the test ends. Streams
// need a running publisher before they open: the client call returns only
// after the server's first message, and the at-most-once bus drops anything
// published before the subscription lands.
func pumpEvents(t *testing.T, b bus.Bus, events []bus.Event) {
	t.Helper()
	stop := make(chan struct{})
	t.Cleanup(func() { close(stop) })
	go func() {
		ticker := time.NewTicker(10 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				return
			case <-ticker.C:
				for _, e := range events {
					_ = b.Publish(context.Background(), e)
				}
			}
		}
	}()
}

// newTestServer wires the full control-plane server against a fresh
// in-proc bus, a checkpoint store with no snapshot, and an order handler
// whose dependencies are nil: order requests must be settled by the
// validation interceptor before reaching it.
func newTestServer(t *testing.T) (*http.Server, bus.Bus) {
	t.Helper()
	eventBus := bus.NewInProc()
	t.Cleanup(eventBus.Close)
	server := NewServer(NewSnapshotServer(&fakeCheckpointStore{err: ports.ErrNotFound}), testEventServer(t, eventBus), NewOrderServer(nil, nil))
	return server, eventBus
}

func TestStreamEvents(t *testing.T) {
	t.Parallel()
	server, eventBus := newTestServer(t)
	srv := httptest.NewServer(server.Handler)
	t.Cleanup(srv.Close)
	client := controlv1connect.NewEventServiceClient(srv.Client(), srv.URL)

	snap := testSnapshot()

	// A non-matching subject and an unknown payload type must both be
	// invisible to the client.
	pumpEvents(t, eventBus, []bus.Event{
		{Subject: "other.subject", At: snap.TakenAt, Payload: snap},
		{Subject: snapshot.SubjectTaken, At: snap.TakenAt, Payload: "unknown payload"},
		{Subject: snapshot.SubjectTaken, At: snap.TakenAt, Payload: snap},
	})

	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	stream, err := client.StreamEvents(ctx, connect.NewRequest(&controlv1.StreamEventsRequest{
		SubjectPrefix: "snapshot.",
	}))
	if err != nil {
		t.Fatalf("open stream: %v", err)
	}
	t.Cleanup(func() { _ = stream.Close() })

	if !stream.Receive() {
		t.Fatalf("no event received: %v", stream.Err())
	}
	got := stream.Msg().GetEvent()

	if got.GetSubject() != snapshot.SubjectTaken {
		t.Fatalf("got subject %q, want %q", got.GetSubject(), snapshot.SubjectTaken)
	}
	taken := got.GetSnapshotTaken()
	if taken == nil {
		t.Fatalf("payload is not snapshot_taken: %v", got)
	}
	if taken.GetVenue() != "bybit" || taken.GetAccount() != "spot" {
		t.Fatalf("wrong account: %s/%s", taken.GetVenue(), taken.GetAccount())
	}
	if len(taken.GetBalances()) != 1 {
		t.Fatalf("got %d balances, want 1", len(taken.GetBalances()))
	}
	b := taken.GetBalances()[0]
	if b.GetCurrency() != "BTC" || b.GetTotal() != "1.5" || b.GetFree() != "1.25" || b.GetLocked() != "0.25" {
		t.Fatalf("wrong balance: %v", b)
	}
}

// TestShutdownInterruptsStream proves graceful shutdown does not wait for
// stream clients to disconnect: Shutdown cancels the server's base context,
// which ends open stream handlers.
func TestShutdownInterruptsStream(t *testing.T) {
	t.Parallel()
	srv, eventBus := newTestServer(t)

	path := filepath.Join(os.TempDir(), "api-shutdown-test.sock")
	t.Cleanup(func() { _ = os.Remove(path) })
	ln, err := Listen(t.Context(), "unix://"+path)
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = srv.Serve(ln) }()

	httpClient, baseURL := NewHTTPClient("unix://" + path)
	client := controlv1connect.NewEventServiceClient(httpClient, baseURL)

	pumpEvents(t, eventBus, []bus.Event{
		{Subject: snapshot.SubjectTaken, At: time.Now(), Payload: testSnapshot()},
	})

	stream, err := client.StreamEvents(t.Context(), connect.NewRequest(&controlv1.StreamEventsRequest{}))
	if err != nil {
		t.Fatalf("open stream: %v", err)
	}
	t.Cleanup(func() { _ = stream.Close() })
	// Receiving one event proves the handler is inside its send loop.
	if !stream.Receive() {
		t.Fatalf("no event before shutdown: %v", stream.Err())
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		t.Fatalf("shutdown blocked on open stream: %v", err)
	}
}

func TestOrderAndReconcileEventMapping(t *testing.T) {
	t.Parallel()
	eventBus := bus.NewInProc()
	t.Cleanup(eventBus.Close)
	server := testEventServer(t, eventBus)
	at := time.Date(2026, 7, 12, 12, 0, 0, 0, time.UTC)
	updated, _ := json.Marshal(events.OrderUpdatedPayload{ClientOrderID: "cid", Venue: "bybit", Base: "BTC", Quote: "USDT", Status: domainorder.StatusOpen, FilledQty: decimal.RequireFromString("0.4")})
	filled, _ := json.Marshal(events.OrderFilledPayload{ClientOrderID: "cid", Venue: "bybit", Base: "BTC", Quote: "USDT", Status: domainorder.StatusPartiallyFilled, FilledQty: decimal.RequireFromString("0.4"), Qty: decimal.RequireFromString("0.4"), Price: decimal.RequireFromString("50000")})
	tests := []struct {
		name    string
		event   bus.Event
		payload func(*controlv1.Event) bool
	}{
		{"updated", bus.Event{Subject: events.SubjectOrderUpdated, At: at, Payload: json.RawMessage(updated)}, func(e *controlv1.Event) bool {
			return e.GetOrderUpdated().GetFilledQty() == "0.4" && e.GetOrderUpdated().GetBase() == "BTC"
		}},
		{"filled", bus.Event{Subject: events.SubjectOrderFilled, At: at, Payload: json.RawMessage(filled)}, func(e *controlv1.Event) bool {
			return e.GetOrderFilled().GetQty() == "0.4" && e.GetOrderFilled().GetPrice() == "50000"
		}},
		{"orphan", bus.Event{Subject: events.SubjectReconcileOrphan, At: at, Payload: events.ReconcileOrphanPayload{Venue: "bybit", VenueOrderID: "v-1", ClientOrderID: "cid", Base: "BTC", Quote: "USDT"}}, func(e *controlv1.Event) bool {
			payload := e.GetReconcileDiff()
			return e.GetSubject() == events.SubjectReconcileOrphan &&
				payload.GetKind() == controlv1.ReconcileDiffKind_RECONCILE_DIFF_KIND_ORPHAN &&
				payload.GetVenue() == "bybit" && payload.GetVenueOrderId() == "v-1" &&
				payload.GetClientOrderId() == "cid" && payload.GetBase() == "BTC" && payload.GetQuote() == "USDT"
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mapped, ok := server.toProtoEvent(tt.event)
			if !ok || !tt.payload(mapped.GetEvent()) {
				t.Fatalf("mapped = %v, ok=%v", mapped, ok)
			}
		})
	}
	if _, ok := server.toProtoEvent(bus.Event{Subject: events.SubjectOrderUpdated, At: at, Payload: json.RawMessage("{")}); ok {
		t.Fatal("malformed payload was not skipped")
	}
	if mapped, ok := server.toProtoEvent(tests[0].event); !ok || mapped.GetEvent().GetOrderUpdated() == nil {
		t.Fatal("valid payload after malformed event was not isolated")
	}
	if got := testutil.ToFloat64(server.metrics.malformed.WithLabelValues(events.SubjectOrderUpdated)); got != 1 {
		t.Fatalf("malformed counter = %v, want 1", got)
	}
}
