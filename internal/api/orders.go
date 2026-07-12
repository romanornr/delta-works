package api

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
	"time"

	"connectrpc.com/connect"
	"github.com/shopspring/decimal"
	"google.golang.org/protobuf/types/known/timestamppb"

	controlv1 "github.com/romanornr/delta-works/internal/api/gen/control/v1"
	"github.com/romanornr/delta-works/internal/domain/instrument"
	"github.com/romanornr/delta-works/internal/domain/money"
	domain "github.com/romanornr/delta-works/internal/domain/order"
	"github.com/romanornr/delta-works/internal/ports"
	orderservice "github.com/romanornr/delta-works/internal/service/order"
)

const defaultOrderLimit int32 = 50

var errInvalidArgument = errors.New("invalid argument")

// OrderServer serves control.v1.OrderService.
type OrderServer struct {
	service *orderservice.Service
	store   ports.OrderStore
}

// NewOrderServer builds the OrderService handler.
func NewOrderServer(service *orderservice.Service, store ports.OrderStore) *OrderServer {
	return &OrderServer{service: service, store: store}
}

// PlaceOrder submits an idempotent order request.
func (s *OrderServer) PlaceOrder(ctx context.Context, req *connect.Request[controlv1.PlaceOrderRequest]) (*connect.Response[controlv1.PlaceOrderResponse], error) {
	qty, err := decimal.NewFromString(req.Msg.GetQty())
	if err != nil {
		return nil, mapOrderError(fmt.Errorf("%w: qty", errInvalidArgument))
	}
	price := decimal.Zero
	if req.Msg.GetPrice() != "" {
		price, err = decimal.NewFromString(req.Msg.GetPrice())
		if err != nil {
			return nil, mapOrderError(fmt.Errorf("%w: price", errInvalidArgument))
		}
	}
	request := domain.Request{
		ClientOrderID: domain.ClientOrderID(req.Msg.GetClientOrderId()), BotID: "manual",
		Instrument: instrument.Instrument{
			Venue: instrument.NewVenueID(req.Msg.GetVenue()), Type: instrument.TypeSpot,
			Base: money.NewCurrency(req.Msg.GetBase()), Quote: money.NewCurrency(req.Msg.GetQuote()),
		},
		Side: fromProtoSide(req.Msg.GetSide()), Type: fromProtoOrderType(req.Msg.GetType()),
		Qty: qty, Price: price,
	}
	result, err := s.service.Place(ctx, request)
	unsettled := errors.Is(err, orderservice.ErrSubmitUnsettled)
	if err != nil && !unsettled {
		return nil, mapOrderError(err)
	}
	return connect.NewResponse(&controlv1.PlaceOrderResponse{
		ClientOrderId: string(result.ClientOrderID), Status: toProtoOrderStatus(result.Status), SubmitUnsettled: unsettled,
	}), nil
}

// CancelOrder records cancel intent and submits it to the venue.
func (s *OrderServer) CancelOrder(ctx context.Context, req *connect.Request[controlv1.CancelOrderRequest]) (*connect.Response[controlv1.CancelOrderResponse], error) {
	status, err := s.service.Cancel(ctx, domain.ClientOrderID(req.Msg.GetClientOrderId()))
	if err != nil {
		return nil, mapOrderError(err)
	}
	return connect.NewResponse(&controlv1.CancelOrderResponse{Status: toProtoOrderStatus(status)}), nil
}

// ListOrders returns one keyset-paginated page.
func (s *OrderServer) ListOrders(ctx context.Context, req *connect.Request[controlv1.ListOrdersRequest]) (*connect.Response[controlv1.ListOrdersResponse], error) {
	limit := req.Msg.GetLimit()
	if limit == 0 {
		limit = defaultOrderLimit
	}
	filter, digest := orderFilter(req.Msg, limit)
	if req.Msg.GetPageToken() != "" {
		cursor, err := decodePageToken(req.Msg.GetPageToken(), digest)
		if err != nil {
			return nil, mapOrderError(err)
		}
		filter.CursorCreatedAt, filter.CursorID = &cursor.CreatedAt, &cursor.ClientOrderID
	}
	rows, err := s.store.ListOrders(ctx, filter)
	if err != nil {
		return nil, mapOrderError(err)
	}
	hasMore := len(rows) > int(limit)
	if hasMore {
		rows = rows[:limit]
	}
	response := &controlv1.ListOrdersResponse{Orders: make([]*controlv1.Order, 0, len(rows))}
	for _, row := range rows {
		response.Orders = append(response.Orders, toProtoOrder(row))
	}
	if hasMore {
		last := rows[len(rows)-1]
		response.NextPageToken, err = encodePageToken(pageToken{V: 1, CreatedAt: last.CreatedAt, ClientOrderID: string(last.ClientOrderID), FilterDigest: digest})
		if err != nil {
			return nil, mapOrderError(err)
		}
	}
	return connect.NewResponse(response), nil
}

type pageToken struct {
	V             int       `json:"v"`
	CreatedAt     time.Time `json:"created_at"`
	ClientOrderID string    `json:"client_order_id"`
	FilterDigest  string    `json:"filter_digest"`
}

func encodePageToken(token pageToken) (string, error) {
	body, err := json.Marshal(token)
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(body), nil
}

func decodePageToken(encoded, digest string) (pageToken, error) {
	body, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return pageToken{}, fmt.Errorf("%w: page token", errInvalidArgument)
	}
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.DisallowUnknownFields()
	var token pageToken
	if err := dec.Decode(&token); err != nil {
		return pageToken{}, fmt.Errorf("%w: page token", errInvalidArgument)
	}
	if err := dec.Decode(new(any)); !errors.Is(err, io.EOF) || token.V != 1 || token.CreatedAt.IsZero() || token.ClientOrderID == "" || token.FilterDigest != digest {
		return pageToken{}, fmt.Errorf("%w: page token", errInvalidArgument)
	}
	return token, nil
}

func orderFilter(req *controlv1.ListOrdersRequest, limit int32) (ports.OrderFilter, string) {
	statuses := make([]string, 0, len(req.GetStatuses()))
	for _, status := range req.GetStatuses() {
		statuses = append(statuses, string(fromProtoOrderStatus(status)))
	}
	slices.Sort(statuses)
	statuses = slices.Compact(statuses)
	venue, bot := strings.TrimSpace(req.GetVenue()), strings.TrimSpace(req.GetBotId())
	filter := ports.OrderFilter{Statuses: statuses, Limit: limit}
	if venue != "" {
		filter.Venue = &venue
	}
	if bot != "" {
		filter.BotID = &bot
	}
	canonical, _ := json.Marshal(struct {
		Venue    string   `json:"venue"`
		Statuses []string `json:"statuses"`
		BotID    string   `json:"bot_id"`
	}{venue, statuses, bot})
	digest := sha256.Sum256(canonical)
	return filter, hex.EncodeToString(digest[:])
}

func mapOrderError(err error) error {
	if err == nil {
		return nil
	}
	var code connect.Code
	var public error
	switch {
	case errors.Is(err, context.Canceled):
		code, public = connect.CodeCanceled, context.Canceled
	case errors.Is(err, context.DeadlineExceeded):
		code, public = connect.CodeDeadlineExceeded, context.DeadlineExceeded
	case errors.Is(err, errInvalidArgument):
		code, public = connect.CodeInvalidArgument, err
	case errors.Is(err, ports.ErrNotFound):
		code, public = connect.CodeNotFound, ports.ErrNotFound
	case errors.Is(err, orderservice.ErrTerminal), errors.Is(err, orderservice.ErrVenueNotConfigured):
		code, public = connect.CodeFailedPrecondition, errors.New("failed precondition")
	case errors.Is(err, orderservice.ErrIdentityMismatch):
		code, public = connect.CodeAlreadyExists, errors.New("order already exists with different identity")
	case errors.Is(err, ports.ErrAuth):
		code, public = connect.CodePermissionDenied, errors.New("permission denied")
	case errors.Is(err, ports.ErrVenueUnavailable):
		code, public = connect.CodeUnavailable, errors.New("venue unavailable")
	default:
		code, public = connect.CodeInternal, errors.New("internal error")
	}
	return connect.NewError(code, public)
}

func toProtoOrder(row ports.StoredOrder) *controlv1.Order {
	return &controlv1.Order{
		ClientOrderId: string(row.ClientOrderID), VenueOrderId: row.VenueOrderID,
		Venue: string(row.Instrument.Venue), Base: string(row.Instrument.Base), Quote: string(row.Instrument.Quote),
		Side: toProtoSide(row.Side), Type: toProtoOrderType(row.Type),
		Price: row.Price.String(), Qty: row.Qty.String(), FilledQty: row.FilledQty.String(), AvgFillPrice: row.AvgFillPrice.String(),
		Status: toProtoOrderStatus(row.Status), BotId: row.BotID,
		CreatedAt: timestamppb.New(row.CreatedAt), UpdatedAt: timestamppb.New(row.UpdatedAt),
	}
}

func fromProtoSide(side controlv1.Side) domain.Side {
	switch side {
	case controlv1.Side_SIDE_BUY:
		return domain.Buy
	case controlv1.Side_SIDE_SELL:
		return domain.Sell
	default:
		return ""
	}
}

func toProtoSide(side domain.Side) controlv1.Side {
	if side == domain.Buy {
		return controlv1.Side_SIDE_BUY
	}
	if side == domain.Sell {
		return controlv1.Side_SIDE_SELL
	}
	return controlv1.Side_SIDE_UNSPECIFIED
}

func fromProtoOrderType(kind controlv1.OrderType) domain.Type {
	switch kind {
	case controlv1.OrderType_ORDER_TYPE_LIMIT:
		return domain.Limit
	case controlv1.OrderType_ORDER_TYPE_MARKET:
		return domain.Market
	default:
		return ""
	}
}

func toProtoOrderType(kind domain.Type) controlv1.OrderType {
	if kind == domain.Limit {
		return controlv1.OrderType_ORDER_TYPE_LIMIT
	}
	if kind == domain.Market {
		return controlv1.OrderType_ORDER_TYPE_MARKET
	}
	return controlv1.OrderType_ORDER_TYPE_UNSPECIFIED
}

func fromProtoOrderStatus(status controlv1.OrderStatus) domain.Status {
	switch status {
	case controlv1.OrderStatus_ORDER_STATUS_PENDING:
		return domain.StatusPending
	case controlv1.OrderStatus_ORDER_STATUS_OPEN:
		return domain.StatusOpen
	case controlv1.OrderStatus_ORDER_STATUS_PARTIALLY_FILLED:
		return domain.StatusPartiallyFilled
	case controlv1.OrderStatus_ORDER_STATUS_FILLED:
		return domain.StatusFilled
	case controlv1.OrderStatus_ORDER_STATUS_CANCELED:
		return domain.StatusCanceled
	case controlv1.OrderStatus_ORDER_STATUS_REJECTED:
		return domain.StatusRejected
	case controlv1.OrderStatus_ORDER_STATUS_EXPIRED:
		return domain.StatusExpired
	default:
		return ""
	}
}

func toProtoOrderStatus(status domain.Status) controlv1.OrderStatus {
	switch status {
	case domain.StatusPending:
		return controlv1.OrderStatus_ORDER_STATUS_PENDING
	case domain.StatusOpen:
		return controlv1.OrderStatus_ORDER_STATUS_OPEN
	case domain.StatusPartiallyFilled:
		return controlv1.OrderStatus_ORDER_STATUS_PARTIALLY_FILLED
	case domain.StatusFilled:
		return controlv1.OrderStatus_ORDER_STATUS_FILLED
	case domain.StatusCanceled:
		return controlv1.OrderStatus_ORDER_STATUS_CANCELED
	case domain.StatusRejected:
		return controlv1.OrderStatus_ORDER_STATUS_REJECTED
	case domain.StatusExpired:
		return controlv1.OrderStatus_ORDER_STATUS_EXPIRED
	default:
		return controlv1.OrderStatus_ORDER_STATUS_UNSPECIFIED
	}
}
