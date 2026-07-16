package gct

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/thrasher-corp/gocryptotrader/currency"
	gctexchange "github.com/thrasher-corp/gocryptotrader/exchanges"
	"github.com/thrasher-corp/gocryptotrader/exchanges/asset"
	gctorder "github.com/thrasher-corp/gocryptotrader/exchanges/order"

	"github.com/romanornr/delta-works/internal/domain/instrument"
	"github.com/romanornr/delta-works/internal/domain/order"
	"github.com/romanornr/delta-works/internal/ports"
)

type noOrderExchange struct {
	gctexchange.IBotExchange
	err          error
	getInfoCalls int
}

func (e *noOrderExchange) GetOrderInfo(context.Context, string, currency.Pair, asset.Item) (*gctorder.Detail, error) {
	e.getInfoCalls++
	return nil, e.err
}

func TestGetOrderBoundary(t *testing.T) {
	tests := []struct {
		name         string
		venueOrderID string
		venueErr     error
		wantErr      error
		wantCalls    int
		wantText     []string
	}{
		{
			name: "nil detail is not found", venueOrderID: "v-1",
			wantErr: ports.ErrNotFound, wantCalls: 1,
		},
		{
			name: "GCT sentinel is not found", venueOrderID: "v-1",
			venueErr: gctorder.ErrOrderNotFound, wantErr: ports.ErrNotFound, wantCalls: 1,
		},
		{
			name:    "missing venue order ID is rejected locally",
			wantErr: ports.ErrNoVenueOrderID, wantText: []string{"bybit", "cid-1"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			underlying := &noOrderExchange{err: tt.venueErr}
			exchange := &Exchange{id: "bybit", exch: underlying}
			_, err := exchange.GetOrder(context.Background(), order.Ref{
				Instrument: instrument.Instrument{
					Venue: "bybit", Type: instrument.TypeSpot, Base: "BTC", Quote: "USDT",
				},
				ClientOrderID: "cid-1", VenueOrderID: tt.venueOrderID,
			})
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("GetOrder error = %v, want %v", err, tt.wantErr)
			}
			if underlying.getInfoCalls != tt.wantCalls {
				t.Fatalf("GetOrderInfo calls = %d, want %d", underlying.getInfoCalls, tt.wantCalls)
			}
			for _, want := range tt.wantText {
				if !strings.Contains(err.Error(), want) {
					t.Fatalf("GetOrder error = %q, want context %q", err, want)
				}
			}
		})
	}
}
