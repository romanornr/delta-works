// deltactl is the operator CLI for the daemon's control plane (ADR-0007).
// It speaks the same ConnectRPC API every other client uses.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/romanornr/delta-works/internal/api"
	"github.com/romanornr/delta-works/internal/api/gen/control/v1/controlv1connect"
	"github.com/romanornr/delta-works/internal/config"
)

// addrEnv derives from config.EnvPrefix so a project rename touches one
// constant (AGENTS.md).
var addrEnv = config.EnvPrefix + "API__ADDR"

var usage = `usage: deltactl [-addr address] <command>

commands:
  snapshot <venue> <account>   print the last snapshot checkpoint
  events [-prefix p]           stream bus events as JSON lines
  watch                        live balances view (q to quit)

The address comes from -addr or ` + addrEnv + `, in the same forms the
daemon accepts: unix:///path/to.sock or host:port.
`

type clients struct {
	snapshots controlv1connect.SnapshotServiceClient
	events    controlv1connect.EventServiceClient
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "deltactl:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	flags := flag.NewFlagSet("deltactl", flag.ExitOnError)
	flags.Usage = func() { fmt.Fprint(os.Stderr, usage) }
	addr := flags.String("addr", os.Getenv(addrEnv), "control-plane address")
	_ = flags.Parse(args)

	if flags.NArg() == 0 {
		flags.Usage()
		return fmt.Errorf("missing command")
	}
	if *addr == "" {
		return fmt.Errorf("no address: pass -addr or set %s", addrEnv)
	}

	httpClient, baseURL := api.NewHTTPClient(*addr)
	c := clients{
		snapshots: controlv1connect.NewSnapshotServiceClient(httpClient, baseURL),
		events:    controlv1connect.NewEventServiceClient(httpClient, baseURL),
	}

	ctx := context.Background()
	cmd, rest := flags.Arg(0), flags.Args()[1:]
	switch cmd {
	case "snapshot":
		return runSnapshot(ctx, c, rest)
	case "events":
		return runEvents(ctx, c, rest)
	case "watch":
		return runWatch(ctx, c)
	default:
		flags.Usage()
		return fmt.Errorf("unknown command %q", cmd)
	}
}
