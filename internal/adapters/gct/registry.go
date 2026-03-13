package gct

import (
	"fmt"
	"sort"
	"strings"

	"github.com/romanornr/delta-works/internal/errs"
	"github.com/rs/zerolog"
)

// adapterRegistry holds a map of exchange adapters
type adapterRegistry struct {
	exchanges map[string]ExchangeAdapter
}

// NewRegistry returns a built from the exchanges loaded by the engine
func NewRegistry(engine *Engine, log zerolog.Logger) (Registry, error) {
	if engine == nil {
		return nil, fmt.Errorf("failed to create exchange registry: %w", errs.ErrAdapterNotReady)
	}

	exchanges, err := engine.GetExchanges()
	if err != nil {
		return nil, fmt.Errorf("failed to create exchange registry: %w", err)
	}

	registry := &adapterRegistry{
		exchanges: make(map[string]ExchangeAdapter, len(exchanges)),
	}

	for _, exch := range exchanges {
		if exch == nil {
			continue
		}

		adapter := NewExchangeAdapter(exch, log)
		name := strings.ToLower(exch.GetName())
		registry.exchanges[name] = adapter
	}

	return registry, nil
}

// Get returns an exchange adapter by exchange name
func (r *adapterRegistry) Get(exchangeName string) (ExchangeAdapter, error) {
	if r == nil {
		return nil, fmt.Errorf("failed to get exchange adapter: %w", errs.ErrAdapterNotReady)
	}

	name := strings.ToLower(strings.TrimSpace(exchangeName))
	adapter, ok := r.exchanges[name]
	if !ok {
		return nil, fmt.Errorf("failed to get exchange adapter for %s: %w", exchangeName, errs.ErrExchangeNotFound)
	}
	return adapter, nil
}

// All returns all registered exchange adapters
func (r *adapterRegistry) All() []ExchangeAdapter {
	if r == nil {
		return nil
	}

	names := r.Names()
	result := make([]ExchangeAdapter, 0, len(names))
	for _, name := range names {
		result = append(result, r.exchanges[name])
	}
	return result
}
