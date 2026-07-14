package main

import (
	"context"
	"flag"
	"fmt"
	"strings"
	"time"

	"connectrpc.com/connect"

	controlv1 "github.com/romanornr/delta-works/internal/api/gen/control/v1"
)

func runOrder(ctx context.Context, c clients, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: %s order <place|cancel|list>", prog)
	}
	switch args[0] {
	case "place":
		return runOrderPlace(ctx, c, args[1:])
	case "cancel":
		return runOrderCancel(ctx, c, args[1:])
	case "list":
		return runOrderList(ctx, c, args[1:])
	default:
		return fmt.Errorf("unknown order command %q", args[0])
	}
}

func runOrderPlace(ctx context.Context, c clients, args []string) error {
	flags := flag.NewFlagSet("order place", flag.ContinueOnError)
	venue := flags.String("venue", "", "venue")
	base := flags.String("base", "", "base currency")
	quote := flags.String("quote", "", "quote currency")
	side := flags.String("side", "", "buy or sell")
	kind := flags.String("type", "", "limit or market")
	qty := flags.String("qty", "", "quantity")
	price := flags.String("price", "", "limit price")
	clientID := flags.String("client-order-id", "", "idempotency key")
	if err := flags.Parse(args); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	resp, err := c.orders.PlaceOrder(ctx, connect.NewRequest(&controlv1.PlaceOrderRequest{
		Venue: *venue, Base: strings.ToUpper(*base), Quote: strings.ToUpper(*quote),
		Side: parseSide(*side), Type: parseOrderType(*kind), Qty: *qty, Price: *price, ClientOrderId: *clientID,
	}))
	if err != nil {
		return err
	}
	fmt.Printf("%s  %s\n", resp.Msg.GetClientOrderId(), orderStatusText(resp.Msg.GetStatus()))
	if resp.Msg.GetSubmitUnsettled() {
		fmt.Println("WARNING: venue submission is unsettled; reconciliation will determine the final state")
	}
	return nil
}

func runOrderCancel(ctx context.Context, c clients, args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: %s order cancel <client-order-id>", prog)
	}
	ctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	resp, err := c.orders.CancelOrder(ctx, connect.NewRequest(&controlv1.CancelOrderRequest{ClientOrderId: args[0]}))
	if err != nil {
		return err
	}
	fmt.Printf("%s  %s\n", args[0], orderStatusText(resp.Msg.GetStatus()))
	return nil
}

type statusFlags []string

func (s *statusFlags) String() string { return strings.Join(*s, ",") }
func (s *statusFlags) Set(value string) error {
	*s = append(*s, value)
	return nil
}

func runOrderList(ctx context.Context, c clients, args []string) error {
	flags := flag.NewFlagSet("order list", flag.ContinueOnError)
	venue := flags.String("venue", "", "venue filter")
	bot := flags.String("bot", "", "bot filter")
	limit := flags.Int("limit", 50, "maximum orders")
	var statuses statusFlags
	flags.Var(&statuses, "status", "status filter (repeatable)")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *limit < 1 {
		return fmt.Errorf("limit must be positive")
	}
	ctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	wireStatuses := make([]controlv1.OrderStatus, 0, len(statuses))
	for _, status := range statuses {
		parsed := parseOrderStatus(status)
		if parsed == controlv1.OrderStatus_ORDER_STATUS_UNSPECIFIED {
			return fmt.Errorf("invalid order status %q", status)
		}
		wireStatuses = append(wireStatuses, parsed)
	}
	pageToken, remaining := "", *limit
	for remaining > 0 {
		pageLimit := min(remaining, 500)
		resp, err := c.orders.ListOrders(ctx, connect.NewRequest(&controlv1.ListOrdersRequest{
			Venue: *venue, Statuses: wireStatuses, BotId: *bot, Limit: int32(pageLimit), PageToken: pageToken, //nolint:gosec // capped at 500
		}))
		if err != nil {
			return err
		}
		for _, order := range resp.Msg.GetOrders() {
			fmt.Printf("%s  %s  %s/%s  %s %s @ %s  %s\n", order.GetClientOrderId(), order.GetVenue(),
				order.GetBase(), order.GetQuote(), strings.ToLower(strings.TrimPrefix(order.GetSide().String(), "SIDE_")), order.GetQty(), order.GetPrice(), orderStatusText(order.GetStatus()))
			remaining--
		}
		pageToken = resp.Msg.GetNextPageToken()
		if pageToken == "" || len(resp.Msg.GetOrders()) == 0 {
			break
		}
	}
	return nil
}

func parseSide(value string) controlv1.Side {
	if strings.EqualFold(value, "buy") {
		return controlv1.Side_SIDE_BUY
	}
	if strings.EqualFold(value, "sell") {
		return controlv1.Side_SIDE_SELL
	}
	return controlv1.Side_SIDE_UNSPECIFIED
}

func parseOrderType(value string) controlv1.OrderType {
	if strings.EqualFold(value, "limit") {
		return controlv1.OrderType_ORDER_TYPE_LIMIT
	}
	if strings.EqualFold(value, "market") {
		return controlv1.OrderType_ORDER_TYPE_MARKET
	}
	return controlv1.OrderType_ORDER_TYPE_UNSPECIFIED
}

func parseOrderStatus(value string) controlv1.OrderStatus {
	normalized := "ORDER_STATUS_" + strings.ToUpper(strings.ReplaceAll(value, "-", "_"))
	return controlv1.OrderStatus(controlv1.OrderStatus_value[normalized])
}

func orderStatusText(status controlv1.OrderStatus) string {
	return strings.ToLower(strings.TrimPrefix(status.String(), "ORDER_STATUS_"))
}
