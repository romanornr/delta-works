package main

import (
	"context"
	"testing"

	"connectrpc.com/connect"

	controlv1 "github.com/romanornr/delta-works/internal/api/gen/control/v1"
)

type fakeOrderClient struct {
	place *controlv1.PlaceOrderRequest
	list  []*controlv1.ListOrdersRequest
}

func (f *fakeOrderClient) PlaceOrder(_ context.Context, req *connect.Request[controlv1.PlaceOrderRequest]) (*connect.Response[controlv1.PlaceOrderResponse], error) {
	f.place = req.Msg
	return connect.NewResponse(&controlv1.PlaceOrderResponse{ClientOrderId: "01J00000000000000000000001", Status: controlv1.OrderStatus_ORDER_STATUS_PENDING}), nil
}

func (*fakeOrderClient) CancelOrder(context.Context, *connect.Request[controlv1.CancelOrderRequest]) (*connect.Response[controlv1.CancelOrderResponse], error) {
	return connect.NewResponse(&controlv1.CancelOrderResponse{Status: controlv1.OrderStatus_ORDER_STATUS_OPEN}), nil
}

func (f *fakeOrderClient) ListOrders(_ context.Context, req *connect.Request[controlv1.ListOrdersRequest]) (*connect.Response[controlv1.ListOrdersResponse], error) {
	f.list = append(f.list, req.Msg)
	if len(f.list) == 1 {
		return connect.NewResponse(&controlv1.ListOrdersResponse{Orders: []*controlv1.Order{{ClientOrderId: "one"}}, NextPageToken: "next"}), nil
	}
	return connect.NewResponse(&controlv1.ListOrdersResponse{Orders: []*controlv1.Order{{ClientOrderId: "two"}}}), nil
}

func TestOrderFlags(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		run     func(context.Context, clients, []string) error
		args    []string
		wantErr bool
		verify  func(*testing.T, *fakeOrderClient)
	}{
		{
			name: "place normalizes currencies and enums",
			run:  runOrderPlace,
			args: []string{"--venue", "bybit", "--base", "btc", "--quote", "usdt", "--side", "buy", "--type", "limit", "--qty", "1", "--price", "50000"},
			verify: func(t *testing.T, fake *fakeOrderClient) {
				if fake.place.GetBase() != "BTC" || fake.place.GetSide() != controlv1.Side_SIDE_BUY || fake.place.GetType() != controlv1.OrderType_ORDER_TYPE_LIMIT {
					t.Fatalf("place request = %+v", fake.place)
				}
			},
		},
		{
			name: "list follows page tokens and repeats statuses",
			run:  runOrderList,
			args: []string{"--venue", "bybit", "--status", "open", "--status", "filled", "--bot", "manual", "--limit", "2"},
			verify: func(t *testing.T, fake *fakeOrderClient) {
				if len(fake.list) != 2 || fake.list[1].GetPageToken() != "next" || len(fake.list[0].GetStatuses()) != 2 {
					t.Fatalf("list requests = %+v", fake.list)
				}
			},
		},
		{
			name:    "list rejects an unknown status before calling the API",
			run:     runOrderList,
			args:    []string{"--status", "bogus"},
			wantErr: true,
			verify: func(t *testing.T, fake *fakeOrderClient) {
				if len(fake.list) != 0 {
					t.Fatalf("list requests = %+v, want none", fake.list)
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			fake := &fakeOrderClient{}
			err := tt.run(t.Context(), clients{orders: fake}, tt.args)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr = %v", err, tt.wantErr)
			}
			tt.verify(t, fake)
		})
	}
}
