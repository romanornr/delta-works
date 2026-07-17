// Package venue constructs the runtime catalog of configured venue capabilities.
package venue

import (
	"context"
	"fmt"
	"slices"
	"sort"

	"golang.org/x/time/rate"

	"github.com/romanornr/delta-works/internal/config"
	"github.com/romanornr/delta-works/internal/domain/account"
	"github.com/romanornr/delta-works/internal/domain/instrument"
	"github.com/romanornr/delta-works/internal/ports"
)

// Support describes the capabilities a connector can provide for a venue.
type Support struct {
	Account, MarketData, Orders, PrivateEvents bool
}

// Connector reports support without constructing an adapter, then connects it.
type Connector interface {
	Support(name string) (Support, error)
	Connect(ctx context.Context, name string, cfg config.Venue) (Source, error)
}

// Source exposes the capabilities of one connected adapter.
type Source interface {
	Account() (ports.AccountReader, bool)
	MarketData() (ports.MarketDataReader, bool)
	Orders() (ports.OrderPlacer, bool)
	PrivateEvents() (ports.PrivateStreamer, bool)
}

// Entry is the configured runtime representation of one venue.
type Entry struct {
	id            instrument.VenueID
	accounts      []account.Type
	account       ports.AccountReader
	market        ports.MarketDataReader
	orders        ports.OrderPlacer
	privateEvents ports.PrivateStreamer
}

// OrderEntry is the catalog view for order consumers. The catalog only
// yields trading-capable entries, so Orders always reports true. Fakes
// must keep that promise.
type OrderEntry interface {
	ID() instrument.VenueID
	Orders() (ports.OrderPlacer, bool)
	PrivateEvents() (ports.PrivateStreamer, bool)
}

// ID returns the canonical venue ID.
func (e *Entry) ID() instrument.VenueID { return e.id }

// Accounts returns configured account types in configuration order.
func (e *Entry) Accounts() []account.Type { return slices.Clone(e.accounts) }

// Account returns the account-reading capability when configured.
func (e *Entry) Account() (ports.AccountReader, bool) { return e.account, e.account != nil }

// MarketData returns the market-data capability when supported.
func (e *Entry) MarketData() (ports.MarketDataReader, bool) { return e.market, e.market != nil }

// Orders returns the order capability when trading is enabled.
func (e *Entry) Orders() (ports.OrderPlacer, bool) { return e.orders, e.orders != nil }

// PrivateEvents returns the private-event capability when trading is enabled and supported.
func (e *Entry) PrivateEvents() (ports.PrivateStreamer, bool) {
	return e.privateEvents, e.privateEvents != nil
}

// Catalog owns the deterministic runtime venue graph.
type Catalog struct {
	entries      []*Entry
	byID         map[instrument.VenueID]*Entry
	orderEntries []OrderEntry
}

// Entries returns every enabled venue in canonical-ID order.
func (c *Catalog) Entries() []*Entry { return slices.Clone(c.entries) }

// Lookup returns the entry with the canonical venue ID.
func (c *Catalog) Lookup(id instrument.VenueID) (*Entry, bool) {
	entry, ok := c.byID[id]
	return entry, ok
}

// OrderEntries returns trading-enabled entries in canonical-ID order.
func (c *Catalog) OrderEntries() []OrderEntry { return slices.Clone(c.orderEntries) }

type specification struct {
	name    string
	id      instrument.VenueID
	config  config.Venue
	support Support
}

// Build validates all enabled venue capabilities before connecting adapters.
func Build(ctx context.Context, configs map[string]config.Venue, connector Connector) (*Catalog, error) {
	specs, err := preflight(configs, connector)
	if err != nil {
		return nil, err
	}
	catalog := &Catalog{byID: make(map[instrument.VenueID]*Entry, len(specs))}
	for _, spec := range specs {
		source, err := connector.Connect(ctx, spec.name, spec.config)
		if err != nil {
			return nil, fmt.Errorf("connect venue %q: %w", spec.id, err)
		}
		entry, err := connectEntry(spec, source)
		if err != nil {
			return nil, err
		}
		catalog.entries = append(catalog.entries, entry)
		catalog.byID[entry.id] = entry
		if entry.orders != nil {
			catalog.orderEntries = append(catalog.orderEntries, entry)
		}
	}
	return catalog, nil
}

func preflight(configs map[string]config.Venue, connector Connector) ([]specification, error) {
	seen := make(map[instrument.VenueID]struct{}, len(configs))
	specs := make([]specification, 0, len(configs))
	for name, cfg := range configs {
		if !cfg.Enabled {
			continue
		}
		id := instrument.NewVenueID(name)
		if _, exists := seen[id]; exists {
			return nil, fmt.Errorf("duplicate canonical venue ID %q", id)
		}
		seen[id] = struct{}{}
		specs = append(specs, specification{name: name, id: id, config: cfg})
	}
	sort.Slice(specs, func(i, j int) bool { return specs[i].id < specs[j].id })
	for i := range specs {
		support, err := connector.Support(string(specs[i].id))
		if err != nil {
			return nil, fmt.Errorf("venue %q support: %w", specs[i].id, err)
		}
		if len(specs[i].config.Accounts) > 0 && !support.Account {
			return nil, fmt.Errorf("venue %q: configured accounts require account capability", specs[i].id)
		}
		if specs[i].config.Trading && !support.Orders {
			return nil, fmt.Errorf("venue %q: trading requires order capability", specs[i].id)
		}
		specs[i].support = support
	}
	return specs, nil
}

func connectEntry(spec specification, source Source) (*Entry, error) {
	accounts := make([]account.Type, len(spec.config.Accounts))
	for i, configured := range spec.config.Accounts {
		accounts[i] = account.Type(configured)
	}
	entry := &Entry{id: spec.id, accounts: accounts}
	gate := newGate(spec.id, rate.NewLimiter(rate.Limit(spec.config.Rate.RPS), spec.config.Rate.Burst))
	if spec.support.Account {
		reader, err := provided(spec.id, "account", source.Account)
		if err != nil {
			return nil, err
		}
		entry.account = &guardedAccount{reader: reader, gate: gate}
	}
	if spec.support.MarketData {
		reader, err := provided(spec.id, "market data", source.MarketData)
		if err != nil {
			return nil, err
		}
		entry.market = &guardedMarketData{reader: reader, gate: gate}
	}
	if spec.config.Trading {
		orders, err := provided(spec.id, "orders", source.Orders)
		if err != nil {
			return nil, err
		}
		entry.orders = &guardedOrders{orders: orders, gate: gate}
		if spec.support.PrivateEvents {
			streamer, err := provided(spec.id, "private events", source.PrivateEvents)
			if err != nil {
				return nil, err
			}
			entry.privateEvents = streamer
		}
	}
	return entry, nil
}

func provided[T any](id instrument.VenueID, capability string, get func() (T, bool)) (T, error) {
	value, ok := get()
	if !ok {
		return value, fmt.Errorf("venue %q connector declared %s support but did not provide it", id, capability)
	}
	return value, nil
}
