package api

import (
	"context"
	"errors"
	"net/http/httptest"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"

	controlv1 "github.com/romanornr/delta-works/internal/api/gen/control/v1"
	"github.com/romanornr/delta-works/internal/api/gen/control/v1/controlv1connect"
	"github.com/romanornr/delta-works/internal/bus"
	"github.com/romanornr/delta-works/internal/domain/account"
	"github.com/romanornr/delta-works/internal/domain/instrument"
	"github.com/romanornr/delta-works/internal/ports"
	"github.com/romanornr/delta-works/internal/snapshot"
)

type fakeCheckpointStore struct {
	checkpoint snapshot.Checkpoint
	err        error
}

func (f *fakeCheckpointStore) LastSnapshot(context.Context, account.Ref) (snapshot.Checkpoint, error) {
	return f.checkpoint, f.err
}

func newTestClient(t *testing.T, store ports.SnapshotReader) controlv1connect.SnapshotServiceClient {
	t.Helper()
	eventBus := bus.NewInProc()
	t.Cleanup(eventBus.Close)
	srv := httptest.NewServer(NewServer(NewSnapshotServer(store), testEventServer(t, eventBus), nil).Handler)
	t.Cleanup(srv.Close)
	return controlv1connect.NewSnapshotServiceClient(srv.Client(), srv.URL)
}

func TestGetLastSnapshot(t *testing.T) {
	t.Parallel()
	takenAt := time.Date(2026, 7, 4, 12, 0, 0, 0, time.UTC)
	checkpoint := snapshot.Checkpoint{
		ID:           uuid.New(),
		Account:      account.Ref{Venue: instrument.NewVenueID("bybit"), Type: account.TypeSpot},
		TakenAt:      takenAt,
		BalanceCount: 3,
		Status:       snapshot.StatusOK,
	}

	tests := []struct {
		name     string
		store    *fakeCheckpointStore
		venue    string
		account  string
		wantCode connect.Code
	}{
		{
			name:    "ok",
			store:   &fakeCheckpointStore{checkpoint: checkpoint},
			venue:   "bybit",
			account: "spot",
		},
		{
			name:     "not found",
			store:    &fakeCheckpointStore{err: ports.ErrNotFound},
			venue:    "bybit",
			account:  "spot",
			wantCode: connect.CodeNotFound,
		},
		{
			name:     "store failure",
			store:    &fakeCheckpointStore{err: errors.New("connection lost")},
			venue:    "bybit",
			account:  "spot",
			wantCode: connect.CodeInternal,
		},
		{
			name:     "unknown account type rejected before the store",
			store:    &fakeCheckpointStore{checkpoint: checkpoint},
			venue:    "bybit",
			account:  "sp0t",
			wantCode: connect.CodeInvalidArgument,
		},
		{
			name:     "empty venue rejected before the store",
			store:    &fakeCheckpointStore{checkpoint: checkpoint},
			venue:    "",
			account:  "spot",
			wantCode: connect.CodeInvalidArgument,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			client := newTestClient(t, tt.store)
			resp, err := client.GetLastSnapshot(t.Context(), connect.NewRequest(&controlv1.GetLastSnapshotRequest{
				Venue:   tt.venue,
				Account: tt.account,
			}))
			if tt.wantCode != 0 {
				if connect.CodeOf(err) != tt.wantCode {
					t.Fatalf("got error %v, want code %v", err, tt.wantCode)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			got := resp.Msg.GetCheckpoint()
			if got.GetVenue() != "bybit" || got.GetAccount() != "spot" {
				t.Fatalf("wrong account: %s/%s", got.GetVenue(), got.GetAccount())
			}
			if got.GetStatus() != controlv1.CheckpointStatus_CHECKPOINT_STATUS_OK {
				t.Fatalf("got status %v, want OK", got.GetStatus())
			}
			if !got.GetTakenAt().AsTime().Equal(takenAt) {
				t.Fatalf("got taken_at %v, want %v", got.GetTakenAt().AsTime(), takenAt)
			}
			if got.GetBalanceCount() != 3 {
				t.Fatalf("got balance_count %d, want 3", got.GetBalanceCount())
			}
		})
	}
}
