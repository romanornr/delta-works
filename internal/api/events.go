package api

import (
	"context"
	"encoding/json"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"

	controlv1 "github.com/romanornr/delta-works/internal/api/gen/control/v1"
	"github.com/romanornr/delta-works/internal/bus"
	"github.com/romanornr/delta-works/internal/domain/account"
	"github.com/romanornr/delta-works/internal/events"
	"github.com/romanornr/delta-works/internal/log"
)

// streamBuffer absorbs bursts between the bus goroutine and the stream
// writer. When it is full events are dropped, matching the bus's
// at-most-once delivery contract.
const streamBuffer = 64

// EventServer serves control.v1.EventService from the event bus.
type EventServer struct {
	bus     bus.Bus
	log     log.Logger
	metrics *Metrics
}

// NewEventServer builds the EventService handler.
func NewEventServer(b bus.Bus, logger log.Logger, metrics *Metrics) *EventServer {
	return &EventServer{bus: b, log: log.Component(logger, "api"), metrics: metrics}
}

// StreamEvents forwards bus events matching the subject prefix until the
// client disconnects or the server stops.
func (s *EventServer) StreamEvents(
	ctx context.Context,
	req *connect.Request[controlv1.StreamEventsRequest],
	stream *connect.ServerStream[controlv1.StreamEventsResponse],
) error {
	events := make(chan bus.Event, streamBuffer)
	unsubscribe, err := s.bus.Subscribe(req.Msg.GetSubjectPrefix(), func(_ context.Context, e bus.Event) {
		select {
		case events <- e:
		default:
		}
	})
	if err != nil {
		return connect.NewError(connect.CodeInternal, err)
	}
	defer unsubscribe()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case e := <-events:
			msg, ok := s.toProtoEvent(e)
			if !ok {
				continue
			}
			if err := stream.Send(msg); err != nil {
				return err
			}
		}
	}
}

// toProtoEvent maps a bus event onto the wire envelope. Payload types
// without a proto arm yet are skipped rather than sent untyped.
func (s *EventServer) toProtoEvent(e bus.Event) (*controlv1.StreamEventsResponse, bool) {
	event := &controlv1.Event{
		Subject: e.Subject,
		At:      timestamppb.New(e.At),
	}
	switch e.Subject {
	case events.SubjectOrderUpdated:
		var payload events.OrderUpdatedPayload
		if !s.decodeOrderPayload(e, &payload) {
			return nil, false
		}
		event.Payload = &controlv1.Event_OrderUpdated{OrderUpdated: &controlv1.OrderUpdated{
			ClientOrderId: string(payload.ClientOrderID), Venue: string(payload.Venue),
			Base: string(payload.Base), Quote: string(payload.Quote),
			Status: toProtoOrderStatus(payload.Status), FilledQty: payload.FilledQty.String(),
		}}
	case events.SubjectOrderFilled:
		var payload events.OrderFilledPayload
		if !s.decodeOrderPayload(e, &payload) {
			return nil, false
		}
		event.Payload = &controlv1.Event_OrderFilled{OrderFilled: &controlv1.OrderFilled{
			ClientOrderId: string(payload.ClientOrderID), Venue: string(payload.Venue),
			Base: string(payload.Base), Quote: string(payload.Quote),
			Status: toProtoOrderStatus(payload.Status), FilledQty: payload.FilledQty.String(),
			Qty: payload.Qty.String(), Price: payload.Price.String(),
		}}
	case events.SubjectReconcileOrphan:
		payload, ok := e.Payload.(events.ReconcileOrphanPayload)
		if !ok {
			s.recordMalformed(e.Subject)
			return nil, false
		}
		event.Payload = &controlv1.Event_ReconcileDiff{ReconcileDiff: &controlv1.ReconcileDiff{
			Kind:  controlv1.ReconcileDiffKind_RECONCILE_DIFF_KIND_ORPHAN,
			Venue: string(payload.Venue), VenueOrderId: payload.VenueOrderID,
			ClientOrderId: string(payload.ClientOrderID), Base: payload.Base, Quote: payload.Quote,
		}}
	default:
		payload, ok := e.Payload.(account.Snapshot)
		if !ok {
			s.recordMalformed(e.Subject)
			return nil, false
		}
		event.Payload = &controlv1.Event_SnapshotTaken{SnapshotTaken: toProtoSnapshot(payload)}
	}
	return &controlv1.StreamEventsResponse{Event: event}, true
}

func (s *EventServer) decodeOrderPayload(e bus.Event, dst any) bool {
	raw, ok := e.Payload.(json.RawMessage)
	if ok {
		ok = json.Unmarshal(raw, dst) == nil
	}
	if ok {
		return true
	}
	s.recordMalformed(e.Subject)
	return false
}

func (s *EventServer) recordMalformed(subject string) {
	s.metrics.malformed.WithLabelValues(subject).Inc()
	s.log.Error().Str("subject", subject).Msg("malformed event payload skipped")
}

func toProtoSnapshot(s account.Snapshot) *controlv1.AccountSnapshot {
	balances := make([]*controlv1.Balance, 0, len(s.Balances))
	for _, b := range s.Balances {
		balances = append(balances, &controlv1.Balance{
			Currency: string(b.Currency),
			Total:    b.Total.String(),
			Free:     b.Free.String(),
			Locked:   b.Locked.String(),
		})
	}
	return &controlv1.AccountSnapshot{
		Venue:    string(s.Account.Venue),
		Account:  string(s.Account.Type),
		TakenAt:  timestamppb.New(s.TakenAt),
		Balances: balances,
	}
}
