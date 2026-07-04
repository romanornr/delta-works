package api

import (
	"context"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"

	controlv1 "github.com/romanornr/delta-works/internal/api/gen/control/v1"
	"github.com/romanornr/delta-works/internal/bus"
	"github.com/romanornr/delta-works/internal/domain/account"
)

// streamBuffer absorbs bursts between the bus goroutine and the stream
// writer. When it is full events are dropped, matching the bus's
// at-most-once delivery contract.
const streamBuffer = 64

// EventServer serves control.v1.EventService from the event bus.
type EventServer struct {
	bus bus.Bus
}

// NewEventServer builds the EventService handler.
func NewEventServer(b bus.Bus) *EventServer {
	return &EventServer{bus: b}
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
			msg, ok := toProtoEvent(e)
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
func toProtoEvent(e bus.Event) (*controlv1.StreamEventsResponse, bool) {
	event := &controlv1.Event{
		Subject: e.Subject,
		At:      timestamppb.New(e.At),
	}
	switch p := e.Payload.(type) {
	case account.Snapshot:
		event.Payload = &controlv1.Event_SnapshotTaken{SnapshotTaken: toProtoSnapshot(p)}
	default:
		return nil, false
	}
	return &controlv1.StreamEventsResponse{Event: event}, true
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
