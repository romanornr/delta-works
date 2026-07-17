package events

import (
	"encoding/json"
	"testing"
)

func TestOrderPayloadJSONGoldenRoundTrips(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		payload any
		golden  string
	}{
		{
			"updated",
			&OrderUpdatedPayload{},
			`{"client_order_id":"cid-1","venue":"coinbase","base":"BTC","quote":"USD","status":"partially_filled","filled_qty":"0.4","source":"stream","at":"2026-07-17T12:34:56Z"}`,
		},
		{
			"filled",
			&OrderFilledPayload{},
			`{"client_order_id":"cid-1","venue":"coinbase","base":"BTC","quote":"USD","status":"filled","filled_qty":"1","qty":"0.6","price":"50000.25","fee":"0.001","fee_currency":"USD","venue_fill_id":"fill-1","at":"2026-07-17T12:34:56Z"}`,
		},
		{
			"filled optional fields omitted",
			&OrderFilledPayload{},
			`{"client_order_id":"cid-1","venue":"coinbase","base":"BTC","quote":"USD","status":"filled","filled_qty":"1","qty":"0.6","price":"50000.25","fee":"0","at":"2026-07-17T12:34:56Z"}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := json.Unmarshal([]byte(tt.golden), tt.payload); err != nil {
				t.Fatal(err)
			}
			got, err := json.Marshal(tt.payload)
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != tt.golden {
				t.Fatalf("payload JSON:\n got %s\nwant %s", got, tt.golden)
			}
		})
	}
}
