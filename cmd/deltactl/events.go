package main

import (
	"context"
	"flag"
	"fmt"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/encoding/protojson"

	controlv1 "github.com/romanornr/delta-works/internal/api/gen/control/v1"
)

func runEvents(ctx context.Context, c clients, args []string) error {
	flags := flag.NewFlagSet("events", flag.ExitOnError)
	prefix := flags.String("prefix", "", "subject prefix filter (empty = all)")
	_ = flags.Parse(args)

	stream, err := c.events.StreamEvents(ctx, connect.NewRequest(&controlv1.StreamEventsRequest{
		SubjectPrefix: *prefix,
	}))
	if err != nil {
		return err
	}
	defer func() { _ = stream.Close() }()
	for stream.Receive() {
		line, err := protojson.Marshal(stream.Msg().GetEvent())
		if err != nil {
			return err
		}
		fmt.Println(string(line))
	}
	return stream.Err()
}
