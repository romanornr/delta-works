// Package api serves the control plane: a ConnectRPC server exposing the
// daemon's state and event bus to clients (CLI, TUI, web) over one
// schema-defined contract (ADR-0007). Generated code lives in gen/ and is
// never edited by hand.
package api

import (
	"context"
	"net"
	"net/http"
	"time"

	"connectrpc.com/connect"
	"connectrpc.com/grpchealth"
	"connectrpc.com/grpcreflect"
	"connectrpc.com/validate"

	"github.com/romanornr/delta-works/internal/api/gen/control/v1/controlv1connect"
)

const readHeaderTimeout = 5 * time.Second

// NewServer builds the control-plane HTTP server. It does not start it;
// lifecycle is managed by the application (fx hooks). No write timeout is
// set because event streams stay open indefinitely.
func NewServer(snapshots *SnapshotServer, events *EventServer) *http.Server {
	interceptors := connect.WithInterceptors(validate.NewInterceptor())

	mux := http.NewServeMux()
	mux.Handle(controlv1connect.NewSnapshotServiceHandler(snapshots, interceptors))
	mux.Handle(controlv1connect.NewEventServiceHandler(events, interceptors))

	services := []string{
		controlv1connect.SnapshotServiceName,
		controlv1connect.EventServiceName,
	}
	mux.Handle(grpchealth.NewHandler(grpchealth.NewStaticChecker(services...)))
	reflector := grpcreflect.NewStaticReflector(services...)
	mux.Handle(grpcreflect.NewHandlerV1(reflector))
	mux.Handle(grpcreflect.NewHandlerV1Alpha(reflector))

	// gRPC clients need HTTP/2; without TLS that means h2c.
	protocols := new(http.Protocols)
	protocols.SetHTTP1(true)
	protocols.SetUnencryptedHTTP2(true)

	srv := &http.Server{
		Handler:           mux,
		Protocols:         protocols,
		ReadHeaderTimeout: readHeaderTimeout,
	}

	// Graceful shutdown waits for handlers, and event streams only end when
	// their context is canceled. Canceling the base context on Shutdown
	// propagates into every request context, so open streams terminate
	// instead of blocking shutdown until the stop deadline.
	baseCtx, cancel := context.WithCancel(context.Background())
	srv.BaseContext = func(net.Listener) context.Context { return baseCtx }
	srv.RegisterOnShutdown(cancel)
	return srv
}
