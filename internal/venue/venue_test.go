package venue

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/sony/gobreaker/v2"

	"github.com/romanornr/delta-works/internal/config"
	"github.com/romanornr/delta-works/internal/domain/account"
	"github.com/romanornr/delta-works/internal/domain/instrument"
	"github.com/romanornr/delta-works/internal/domain/marketdata"
	"github.com/romanornr/delta-works/internal/domain/order"
	"github.com/romanornr/delta-works/internal/ports"
)

type fakeConnector struct {
	support          map[string]Support
	sources          map[string]*fakeSource
	checks, connects []string
}

func (f *fakeConnector) Support(name string) (Support, error) {
	f.checks = append(f.checks, name)
	support, ok := f.support[name]
	if !ok {
		return Support{}, errors.New("unsupported")
	}
	return support, nil
}

func (f *fakeConnector) Connect(_ context.Context, name string, _ config.Venue) (Source, error) {
	f.connects = append(f.connects, name)
	return f.sources[name], nil
}

type fakeSource struct {
	support Support
	err     error
	calls   int
}

func (f *fakeSource) Account() (ports.AccountReader, bool)         { return f, f.support.Account }
func (f *fakeSource) MarketData() (ports.MarketDataReader, bool)   { return f, f.support.MarketData }
func (f *fakeSource) Orders() (ports.OrderPlacer, bool)            { return f, f.support.Orders }
func (f *fakeSource) PrivateEvents() (ports.PrivateStreamer, bool) { return f, f.support.PrivateEvents }
func (f *fakeSource) Balances(context.Context, account.Type) ([]account.Balance, error) {
	f.calls++
	return nil, f.err
}

func (f *fakeSource) Ticker(context.Context, instrument.Instrument) (marketdata.Ticker, error) {
	f.calls++
	return marketdata.Ticker{}, f.err
}

func (f *fakeSource) Instruments(context.Context, instrument.Type) ([]instrument.Instrument, error) {
	f.calls++
	return nil, f.err
}

func (f *fakeSource) PlaceOrder(context.Context, order.Request) (order.Ack, error) {
	f.calls++
	return order.Ack{}, f.err
}

func (f *fakeSource) CancelOrder(context.Context, order.Ref) error { f.calls++; return f.err }

func (f *fakeSource) OpenOrders(context.Context) ([]order.Snapshot, error) {
	f.calls++
	return nil, f.err
}

func (f *fakeSource) GetOrder(context.Context, order.Ref) (order.Snapshot, error) {
	f.calls++
	return order.Snapshot{}, f.err
}

func (f *fakeSource) StreamOrderEvents(context.Context) (<-chan order.Event, error) {
	f.calls++
	return make(chan order.Event), f.err
}

func cfg(trading bool, accounts ...string) config.Venue {
	return config.Venue{Enabled: true, Trading: trading, Accounts: accounts, Rate: config.Rate{RPS: 100, Burst: 10}}
}

func TestBuildPreflightsBeforeConnecting(t *testing.T) {
	tests := []struct {
		name     string
		configs  map[string]config.Venue
		support  map[string]Support
		noChecks bool
	}{
		{"duplicate", map[string]config.Venue{"Coinbase": cfg(false), "coinbase": cfg(false)}, nil, true},
		{"account", map[string]config.Venue{"a": cfg(false), "b": cfg(false, "spot")}, map[string]Support{"a": {}, "b": {}}, false},
		{"orders", map[string]config.Venue{"a": cfg(true)}, map[string]Support{"a": {}}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			connector := &fakeConnector{support: tt.support}
			if _, err := Build(t.Context(), tt.configs, connector); err == nil {
				t.Fatal("Build succeeded")
			}
			if len(connector.connects) != 0 {
				t.Fatalf("connected during preflight: %v", connector.connects)
			}
			if tt.noChecks && len(connector.checks) != 0 {
				t.Fatalf("support checked before duplicate rejection: %v", connector.checks)
			}
		})
	}
}

func TestBuildPreservesCapabilitiesAndOrder(t *testing.T) {
	accountOnly := &fakeSource{support: Support{Account: true, Orders: true, PrivateEvents: true}}
	all := Support{Account: true, MarketData: true, Orders: true, PrivateEvents: true}
	trading := &fakeSource{support: all}
	connector := &fakeConnector{
		support: map[string]Support{"alpha": {Account: true}, "zeta": all},
		sources: map[string]*fakeSource{"ALPHA": accountOnly, "ZETA": trading},
	}
	catalog, err := Build(t.Context(), map[string]config.Venue{
		"ZETA": cfg(true, "margin"), "ALPHA": cfg(false, "spot", "spot", "future"), "off": {},
	}, connector)
	if err != nil {
		t.Fatal(err)
	}
	entries := catalog.Entries()
	if len(entries) != 2 {
		t.Fatalf("entry count = %d, want 2", len(entries))
	}
	if got := []instrument.VenueID{entries[0].ID(), entries[1].ID()}; !slices.Equal(got, []instrument.VenueID{"alpha", "zeta"}) {
		t.Fatalf("entries = %v", got)
	}
	if !slices.Equal(connector.connects, []string{"ALPHA", "ZETA"}) {
		t.Fatalf("connects = %v", connector.connects)
	}
	wantAccounts := []account.Type{"spot", "spot", "future"}
	accounts := entries[0].Accounts()
	accounts[0] = "changed"
	if !slices.Equal(entries[0].Accounts(), wantAccounts) {
		t.Fatalf("accounts = %v", entries[0].Accounts())
	}
	reader, ok := entries[0].Account()
	if !ok {
		t.Fatal("account capability absent")
	}
	if _, broad := reader.(ports.OrderPlacer); broad {
		t.Fatal("account capability implements OrderPlacer")
	}
	if _, ok := entries[0].MarketData(); ok {
		t.Fatal("optional market data present")
	}
	if _, ok := entries[0].Orders(); ok {
		t.Fatal("orders exposed while trading disabled")
	}
	orders := catalog.OrderEntries()
	if len(orders) != 1 {
		t.Fatalf("order entry count = %d, want 1", len(orders))
	}
	streamer, streamOK := orders[0].PrivateEvents()
	if orders[0].ID() != "zeta" || !streamOK || streamer != trading {
		t.Fatalf("order entries = %v", orders)
	}
	if got, ok := catalog.Lookup("zeta"); !ok || got != entries[1] {
		t.Fatalf("Lookup = %v, %v", got, ok)
	}
}

type fakeWaiter struct {
	err   error
	calls int
}

func (w *fakeWaiter) Wait(context.Context) error { w.calls++; return w.err }

func TestGateIsSharedAndOrdered(t *testing.T) {
	waiter := &fakeWaiter{}
	gate := newGate("alpha", waiter)
	raw := &fakeSource{err: errors.New("down")}
	accounts := &guardedAccount{reader: raw, gate: gate}
	market := &guardedMarketData{reader: raw, gate: gate}
	orders := &guardedOrders{orders: raw, gate: gate}
	_, _ = accounts.Balances(t.Context(), account.TypeSpot)
	_, _ = market.Ticker(t.Context(), instrument.Instrument{})
	for range 4 {
		_, _ = accounts.Balances(t.Context(), account.TypeSpot)
	}
	waits := waiter.calls
	if _, err := orders.OpenOrders(t.Context()); !errors.Is(err, gobreaker.ErrOpenState) {
		t.Fatalf("open breaker error = %v", err)
	}
	if waiter.calls != waits || raw.calls != 6 {
		t.Fatalf("waits=%d raw calls=%d", waiter.calls, raw.calls)
	}

	waiter, raw = &fakeWaiter{err: context.DeadlineExceeded}, &fakeSource{}
	accounts = &guardedAccount{reader: raw, gate: newGate("alpha", waiter)}
	for range 10 {
		if _, err := accounts.Balances(t.Context(), account.TypeSpot); !errors.Is(err, errLimiterWait) {
			t.Fatal(err)
		}
	}
	waiter.err = nil
	if _, err := accounts.Balances(t.Context(), account.TypeSpot); err != nil || raw.calls != 1 {
		t.Fatalf("limiter probe: calls=%d err=%v", raw.calls, err)
	}
}

func TestBreakerContract(t *testing.T) {
	settings := breakerSettings("alpha")
	if settings.Name != "alpha" || settings.Timeout != 30*time.Second || settings.IsSuccessful == nil || settings.MaxRequests != 0 || settings.Interval != 0 || settings.BucketPeriod != 0 || settings.ReadyToTrip != nil || settings.OnStateChange != nil || settings.IsExcluded != nil {
		t.Fatalf("settings = %+v", settings)
	}
	for _, tt := range []struct {
		err  error
		want bool
	}{
		{nil, true},
		{ports.ErrAuth, true},
		{ports.ErrUnsupportedAccount, true},
		{ports.ErrNotFound, true},
		{ports.ErrNoVenueOrderID, true},
		{context.Canceled, true},
		{fmt.Errorf("wrapped: %w", errLimiterWait), true},
		{errors.New("down"), false},
		{context.DeadlineExceeded, false},
	} {
		if got := settings.IsSuccessful(tt.err); got != tt.want {
			t.Errorf("IsSuccessful(%v) = %v", tt.err, got)
		}
	}
}
