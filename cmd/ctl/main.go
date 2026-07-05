// Command ctl is the operator CLI for the daemon's control plane
// (ADR-0007). It speaks the same ConnectRPC API every other client uses;
// the Makefile names the binary (bin/$(NAME)ctl) and the CLI identifies
// itself from argv[0].
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/romanornr/delta-works/internal/api"
	"github.com/romanornr/delta-works/internal/api/gen/control/v1/controlv1connect"
	"github.com/romanornr/delta-works/internal/config"
)

// addrEnv derives from config.EnvPrefix so a project rename touches one
// constant (AGENTS.md).
var (
	addrEnv = config.EnvPrefix + "API__ADDR"
	prog    = filepath.Base(os.Args[0])
	usage   = `usage: ` + prog + ` [-addr address] [-config path] <command>

commands:
  snapshot <venue> <account>   print the last snapshot checkpoint
  events [-prefix p]           stream bus events as JSON lines
  watch                        live balances view (q to quit)

The address is resolved from -addr, then ` + addrEnv + `, then api.addr in
the config file, in the same forms the daemon accepts:
unix:///path/to.sock or host:port.
`
)

type clients struct {
	snapshots controlv1connect.SnapshotServiceClient
	events    controlv1connect.EventServiceClient
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, prog+":", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	flags := flag.NewFlagSet(prog, flag.ExitOnError)
	flags.Usage = func() { fmt.Fprint(os.Stderr, usage) }
	addr := flags.String("addr", "", "control-plane address (default: env, then config file)")
	configPath := flags.String("config", "config.yaml", "path to the daemon's configuration file")
	_ = flags.Parse(args)

	if flags.NArg() == 0 {
		flags.Usage()
		return fmt.Errorf("missing command")
	}
	if *addr == "" {
		*addr = os.Getenv(addrEnv)
	}
	if *addr == "" {
		fromFile, err := config.APIAddr(*configPath)
		if err != nil {
			return err
		}
		*addr = fromFile
	}
	if *addr == "" {
		return fmt.Errorf("no address: pass -addr, set %s, or set api.addr in %s", addrEnv, *configPath)
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
