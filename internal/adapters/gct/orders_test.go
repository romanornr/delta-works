package gct

import (
	"context"
	"errors"
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
	err error
}

func (e *noOrderExchange) GetOrderInfo(context.Context, string, currency.Pair, asset.Item) (*gctorder.Detail, error) {
	return nil, e.err
}

func TestGetOrderMapsNotFound(t *testing.T) {
	tests := []struct {
		name string
		err  error
	}{
		{"nil detail", nil},
		{"gct not-found sentinel", gctorder.ErrOrderNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exchange := &Exchange{id: "bybit", exch: &noOrderExchange{err: tt.err}}
			_, err := exchange.GetOrder(context.Background(), order.Ref{
				Instrument: instrument.Instrument{
					Venue: "bybit", Type: instrument.TypeSpot, Base: "BTC", Quote: "USDT",
				},
				ClientOrderID: "cid-1", VenueOrderID: "v-1",
			})
			if !errors.Is(err, ports.ErrNotFound) {
				t.Fatalf("GetOrder error = %v, want ErrNotFound", err)
			}
		})
	}
}
