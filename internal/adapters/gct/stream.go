package gct

import (
	"context"
	"fmt"
	"time"

	gctstream "github.com/thrasher-corp/gocryptotrader/exchange/stream"
	gctorder "github.com/thrasher-corp/gocryptotrader/exchanges/order"

	"github.com/romanornr/delta-works/internal/domain/instrument"
	"github.com/romanornr/delta-works/internal/domain/order"
	"github.com/romanornr/delta-works/internal/ports"
)

// connectionPollInterval spaces the reconnect-detection checks. GCT's
// connection monitor owns the actual reconnecting; we only observe the
// transition so the caller can trigger reconciliation.
const connectionPollInterval = 5 * time.Second

// Streamer implements ports.PrivateStreamer over a GCT exchange's
// websocket. GCT reconnects on its own; onReconnect fires after every
// observed reconnect so the caller can publish stream.reconnected and
// close the event gap via reconciliation (docs/specs/manual-trading.md).
type Streamer struct {
	ex          *Exchange
	onReconnect func()
	pollEvery   time.Duration
}

var _ ports.PrivateStreamer = (*Streamer)(nil)

// NewStreamer wraps the exchange's websocket. onReconnect may be nil.
func NewStreamer(ex *Exchange, onReconnect func()) *Streamer {
	if onReconnect == nil {
		onReconnect = func() {}
	}
	return &Streamer{ex: ex, onReconnect: onReconnect, pollEvery: connectionPollInterval}
}

// StreamOrderEvents implements ports.PrivateStreamer. The returned channel
// closes when ctx is canceled or the venue socket is torn down for good;
// everything on the venue socket that is not an order update is ignored
// here (market data has its own path).
func (s *Streamer) StreamOrderEvents(ctx context.Context) (<-chan order.Event, error) {
	ws, err := s.ex.exch.GetWebsocket()
	if err != nil {
		return nil, fmt.Errorf("gct: websocket %s: %w", s.ex.id, err)
	}
	if !ws.IsEnabled() {
		return nil, fmt.Errorf("gct: websocket %s: not enabled", s.ex.id)
	}
	if !ws.IsConnected() {
		if err := ws.Connect(ctx); err != nil {
			return nil, fmt.Errorf("gct: websocket connect %s: %w", s.ex.id, err)
		}
	}

	out := make(chan order.Event, 64)
	go s.pump(ctx, ws.IsConnected, ws.DataHandler.C, out)
	return out, nil
}

// pump forwards order updates from the venue socket and watches for
// reconnects. It takes the connection probe and the data channel rather
// than the manager so it is testable without a live socket.
func (s *Streamer) pump(ctx context.Context, isConnected func() bool, data <-chan gctstream.Payload, out chan<- order.Event) {
	defer close(out)
	ticker := time.NewTicker(s.pollEvery)
	defer ticker.Stop()
	connected := isConnected()
	for {
		select {
		case <-ctx.Done():
			return
		case payload, ok := <-data:
			if !ok {
				// GCT tore the relay down; a closed-channel receive
				// returns immediately, so leaving would spin the select.
				return
			}
			for _, ev := range toOrderEvents(s.ex.id, payload.Data) {
				select {
				case out <- ev:
				case <-ctx.Done():
					return
				}
			}
		case <-ticker.C:
			now := isConnected()
			if now && !connected {
				s.onReconnect()
			}
			connected = now
		}
	}
}

// toOrderEvents extracts order events from one websocket payload. Payloads
// that are not order updates, and order updates whose status GCT cannot
// map, yield nothing: the stream is best-effort and reconciliation is the
// backstop for anything it misses.
func toOrderEvents(venue instrument.VenueID, data any) []order.Event {
	switch d := data.(type) {
	case *gctorder.Detail:
		if d == nil {
			return nil
		}
		if ev, err := toEvent(venue, d); err == nil {
			return []order.Event{ev}
		}
	case []gctorder.Detail:
		out := make([]order.Event, 0, len(d))
		for i := range d {
			if ev, err := toEvent(venue, &d[i]); err == nil {
				out = append(out, ev)
			}
		}
		return out
	}
	return nil
}
