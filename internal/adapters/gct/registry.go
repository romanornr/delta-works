package gct

import (
	"fmt"
	"sort"
	"strings"

	"github.com/romanornr/delta-works/internal/domain/exchange"
	"github.com/romanornr/delta-works/internal/errs"
	"github.com/rs/zerolog"
)

// exchangeRegistry holds a map of exchange implementations
type exchangeRegistry struct {
	exchanges map[string]exchange.Exchange
}

// NewRegistry returns a Registry built from the exchanges loaded by the engine
func NewRegistry(engine *Engine, log zerolog.Logger) (exchange.Registry, error) {
	if engine == nil {
		return nil, fmt.Errorf("failed to create exchange registry: %w", errs.ErrAdapterNotReady)
	}

	exchanges, err := engine.GetExchanges()
	if err != nil {
		return nil, fmt.Errorf("failed to create exchange registry: %w", err)
	}

	registry := &exchangeRegistry{
		exchanges: make(map[string]exchange.Exchange, len(exchanges)),
	}

	for _, exch := range exchanges {
		if exch == nil {
			continue
		}

		adapter := NewExchange(exch, log)
		name := strings.ToLower(exch.GetName())
		registry.exchanges[name] = adapter
	}

	return registry, nil
}

func (r *exchangeRegistry) Get(exchangeName string) (exchange.Exchange, error) {
	if r == nil {
		return nil, fmt.Errorf("failed to get exchange adapter: %w", errs.ErrAdapterNotReady)
	}

	name := strings.ToLower(strings.ToLower(exchangeName))
	adapter, ok := r.exchanges[name]
	if !ok {
		return nil, fmt.Errorf("failed to get exchange adapter for %s: %w", exchangeName, errs.ErrExchangeNotFound)
	}

	return adapter, nil
}

// All returns all registered exchanges
func (r *exchangeRegistry) All() []exchange.Exchange {
	if r == nil {
		return nil
	}

	names := r.Names()
	result := make([]exchange.Exchange, 0, len(names))
	for _, name := range names {
		result = append(result, r.exchanges[name])
	}
	return result
}

// Names returns all registered exchange names in sorted order
func (r *exchangeRegistry) Names() []string {
	if r == nil {
		return nil
	}

	names := make([]string, 0, len(r.exchanges))
	for name := range r.exchanges {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
