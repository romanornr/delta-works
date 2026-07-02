package exchange

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/sony/gobreaker/v2"
	"golang.org/x/time/rate"

	"github.com/romanornr/delta-works/internal/domain/instrument"
	"github.com/romanornr/delta-works/internal/ports"
)

// Registry resolves venue connections.
type Registry interface {
	Get(venue instrument.VenueID) (ports.Exchange, error)
	All() []ports.Exchange
}

type registry struct {
	byID  map[instrument.VenueID]ports.Exchange
	order []ports.Exchange
}

// NewRegistry builds a registry from already-decorated exchanges.
func NewRegistry(exchanges []ports.Exchange) Registry {
	r := &registry{byID: map[instrument.VenueID]ports.Exchange{}}
	for _, ex := range exchanges {
		if _, dup := r.byID[ex.ID()]; dup {
			continue
		}
		r.byID[ex.ID()] = ex
		r.order = append(r.order, ex)
	}
	return r
}

func (r *registry) Get(venue instrument.VenueID) (ports.Exchange, error) {
	ex, ok := r.byID[venue]
	if !ok {
		return nil, fmt.Errorf("venue %q not registered", venue)
	}
	return ex, nil
}

func (r *registry) All() []ports.Exchange { return r.order }

// Decorate applies the standard resilience stack to a raw adapter:
// rate limiter first (inner), breaker outside it, so a tripped breaker
// rejects immediately without burning limiter tokens.
func Decorate(ex ports.Exchange, rps float64, burst int) ports.Exchange {
	limited := WithRateLimit(ex, rate.NewLimiter(rate.Limit(rps), burst))
	return WithBreaker(limited, gobreaker.Settings{
		Name:         string(ex.ID()),
		Timeout:      30 * time.Second,
		IsSuccessful: isBreakerSuccess,
	})
}

// isBreakerSuccess keeps the breaker focused on venue health. Permanent
// configuration errors, caller-side cancellation, and local throttling say
// nothing about the venue, so they must not open a venue-wide circuit.
func isBreakerSuccess(err error) bool {
	return err == nil ||
		errors.Is(err, ports.ErrAuth) ||
		errors.Is(err, ports.ErrUnsupportedAccount) ||
		errors.Is(err, context.Canceled) ||
		errors.Is(err, errLimiterWait)
}
