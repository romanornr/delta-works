package api

import (
	"context"
	"encoding/base64"
	"errors"
	"net/http/httptest"
	"testing"
	"time"

	"connectrpc.com/connect"

	controlv1 "github.com/romanornr/delta-works/internal/api/gen/control/v1"
	"github.com/romanornr/delta-works/internal/api/gen/control/v1/controlv1connect"
	"github.com/romanornr/delta-works/internal/ports"
	orderservice "github.com/romanornr/delta-works/internal/service/order"
)

func TestPageToken(t *testing.T) {
	t.Parallel()
	request := &controlv1.ListOrdersRequest{Venue: "bybit", Statuses: []controlv1.OrderStatus{controlv1.OrderStatus_ORDER_STATUS_OPEN, controlv1.OrderStatus_ORDER_STATUS_PENDING, controlv1.OrderStatus_ORDER_STATUS_OPEN}, BotId: "manual"}
	_, digest := orderFilter(request, 50)
	want := pageToken{V: 1, CreatedAt: time.Date(2026, 7, 12, 12, 0, 0, 0, time.UTC), ClientOrderID: "cid", FilterDigest: digest}
	encoded, err := encodePageToken(want)
	if err != nil {
		t.Fatal(err)
	}
	got, err := decodePageToken(encoded, digest)
	if err != nil || got != want {
		t.Fatalf("decode = %+v, err=%v; want %+v", got, err, want)
	}
	bad := []string{
		"not-base64", base64.RawURLEncoding.EncodeToString([]byte(`{"v":2,"created_at":"2026-07-12T12:00:00Z","client_order_id":"cid","filter_digest":"` + digest + `"}`)),
		base64.RawURLEncoding.EncodeToString([]byte(`{"v":1,"created_at":"2026-07-12T12:00:00Z","client_order_id":"cid","filter_digest":"` + digest + `","extra":true}`)),
	}
	for _, token := range bad {
		if _, err := decodePageToken(token, digest); !errors.Is(err, errInvalidArgument) {
			t.Fatalf("token %q error = %v, want invalid argument", token, err)
		}
	}
	if _, err := decodePageToken(encoded, "different"); !errors.Is(err, errInvalidArgument) {
		t.Fatalf("digest mismatch error = %v", err)
	}
}

func TestOrderErrorMapping(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		err  error
		code connect.Code
	}{
		{"invalid", errInvalidArgument, connect.CodeInvalidArgument},
		{"not found", ports.ErrNotFound, connect.CodeNotFound},
		{"terminal", orderservice.ErrTerminal, connect.CodeFailedPrecondition},
		{"venue config", orderservice.ErrVenueNotConfigured, connect.CodeFailedPrecondition},
		{"identity", orderservice.ErrIdentityMismatch, connect.CodeAlreadyExists},
		{"auth", errors.Join(errors.New("secret venue text"), ports.ErrAuth), connect.CodePermissionDenied},
		{"unavailable", ports.ErrVenueUnavailable, connect.CodeUnavailable},
		{"canceled", context.Canceled, connect.CodeCanceled},
		{"deadline", context.DeadlineExceeded, connect.CodeDeadlineExceeded},
		{"internal", errors.New("database password leaked"), connect.CodeInternal},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mapped := mapOrderError(tt.err)
			if connect.CodeOf(mapped) != tt.code {
				t.Fatalf("code = %s, want %s", connect.CodeOf(mapped), tt.code)
			}
			if (tt.code == connect.CodeInternal || tt.code == connect.CodePermissionDenied) && errors.Is(mapped, tt.err) {
				t.Fatalf("mapped error exposes source: %v", mapped)
			}
		})
	}
}

func TestPlaceOrderValidation(t *testing.T) {
	t.Parallel()
	server, _ := newTestServer(t)
	srv := httptest.NewServer(server.Handler)
	t.Cleanup(srv.Close)
	client := controlv1connect.NewOrderServiceClient(srv.Client(), srv.URL)
	valid := &controlv1.PlaceOrderRequest{Venue: "bybit", Base: "BTC", Quote: "USDT", Side: controlv1.Side_SIDE_BUY, Type: controlv1.OrderType_ORDER_TYPE_LIMIT, Qty: "1", Price: "50000"}
	tests := []*controlv1.PlaceOrderRequest{
		{Venue: "bybit", Base: "BTC", Quote: "BTC", Side: valid.Side, Type: valid.Type, Qty: valid.Qty, Price: valid.Price},
		{Venue: "bybit", Base: "BTC", Quote: "USDT", Side: valid.Side, Type: valid.Type, Qty: "1e2", Price: valid.Price},
		{Venue: "bybit", Base: "BTC", Quote: "USDT", Side: valid.Side, Type: valid.Type, Qty: valid.Qty},
		{Venue: "bybit", Base: "BTC", Quote: "USDT", Side: valid.Side, Type: controlv1.OrderType_ORDER_TYPE_MARKET, Qty: valid.Qty, Price: valid.Price},
	}
	for _, request := range tests {
		_, err := client.PlaceOrder(t.Context(), connect.NewRequest(request))
		if connect.CodeOf(err) != connect.CodeInvalidArgument {
			t.Fatalf("request %+v code = %s, err=%v", request, connect.CodeOf(err), err)
		}
	}
}
