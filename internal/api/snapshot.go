package api

import (
	"context"
	"errors"
	"fmt"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"

	controlv1 "github.com/romanornr/delta-works/internal/api/gen/control/v1"
	"github.com/romanornr/delta-works/internal/domain/account"
	"github.com/romanornr/delta-works/internal/domain/instrument"
	"github.com/romanornr/delta-works/internal/ports"
)

// SnapshotServer serves control.v1.SnapshotService from the checkpoint store.
type SnapshotServer struct {
	store ports.CheckpointStore
}

// NewSnapshotServer builds the SnapshotService handler.
func NewSnapshotServer(store ports.CheckpointStore) *SnapshotServer {
	return &SnapshotServer{store: store}
}

// GetLastSnapshot returns the most recent checkpoint for one account.
func (s *SnapshotServer) GetLastSnapshot(
	ctx context.Context,
	req *connect.Request[controlv1.GetLastSnapshotRequest],
) (*connect.Response[controlv1.GetLastSnapshotResponse], error) {
	accountType := account.Type(req.Msg.GetAccount())
	if !accountType.Valid() {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("unknown account type %q", accountType))
	}
	ref := account.Ref{
		Venue: instrument.NewVenueID(req.Msg.GetVenue()),
		Type:  accountType,
	}
	cp, err := s.store.LastSnapshot(ctx, ref)
	if errors.Is(err, ports.ErrNotFound) {
		return nil, connect.NewError(connect.CodeNotFound, err)
	}
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(&controlv1.GetLastSnapshotResponse{
		Checkpoint: &controlv1.SnapshotCheckpoint{
			Id:           cp.ID.String(),
			Venue:        string(cp.Account.Venue),
			Account:      string(cp.Account.Type),
			TakenAt:      timestamppb.New(cp.TakenAt),
			BalanceCount: int32(cp.BalanceCount), //nolint:gosec // bounded by balances per account
			Status:       checkpointStatus(cp.Status),
			Error:        cp.Error,
		},
	}), nil
}

func checkpointStatus(s ports.CheckpointStatus) controlv1.CheckpointStatus {
	switch s {
	case ports.CheckpointOK:
		return controlv1.CheckpointStatus_CHECKPOINT_STATUS_OK
	case ports.CheckpointFailed:
		return controlv1.CheckpointStatus_CHECKPOINT_STATUS_FAILED
	default:
		return controlv1.CheckpointStatus_CHECKPOINT_STATUS_UNSPECIFIED
	}
}
