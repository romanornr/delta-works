package main

import (
	"context"
	"fmt"
	"time"

	"connectrpc.com/connect"

	controlv1 "github.com/romanornr/delta-works/internal/api/gen/control/v1"
)

func runSnapshot(ctx context.Context, c clients, args []string) error {
	if len(args) != 2 {
		return fmt.Errorf("usage: %s snapshot <venue> <account>", prog)
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	resp, err := c.snapshots.GetLastSnapshot(ctx, connect.NewRequest(&controlv1.GetLastSnapshotRequest{
		Venue:   args[0],
		Account: args[1],
	}))
	if err != nil {
		return err
	}
	cp := resp.Msg.GetCheckpoint()
	fmt.Printf("%s/%s  %s  balances=%d  status=%s",
		cp.GetVenue(), cp.GetAccount(),
		cp.GetTakenAt().AsTime().Local().Format("2006-01-02 15:04:05"),
		cp.GetBalanceCount(), statusText(cp.GetStatus()))
	if cp.GetError() != "" {
		fmt.Printf("  error=%q", cp.GetError())
	}
	fmt.Println()
	return nil
}

func statusText(s controlv1.CheckpointStatus) string {
	switch s {
	case controlv1.CheckpointStatus_CHECKPOINT_STATUS_OK:
		return "ok"
	case controlv1.CheckpointStatus_CHECKPOINT_STATUS_FAILED:
		return "failed"
	default:
		return "unknown"
	}
}
