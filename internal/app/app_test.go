package app

import (
	"context"
	"testing"
	"time"

	"go.uber.org/fx"

	"github.com/romanornr/delta-works/internal/bus"
	"github.com/romanornr/delta-works/internal/clock/clocktest"
	"github.com/romanornr/delta-works/internal/config"
	"github.com/romanornr/delta-works/internal/log"
)

type hookRecorder struct{ hooks []fx.Hook }

func (r *hookRecorder) Append(hook fx.Hook) { r.hooks = append(r.hooks, hook) }

func appTestLogger(t *testing.T) log.Logger {
	t.Helper()
	logger, err := log.New(config.Log{Level: "error", Format: "json"})
	if err != nil {
		t.Fatal(err)
	}
	return logger
}

func TestTradingDisabledBuildsNoTradingHooks(t *testing.T) {
	t.Parallel()
	eventBus := bus.NewInProc()
	t.Cleanup(eventBus.Close)
	products, err := newExchangeProducts(config.Config{}, appTestLogger(t), eventBus, clocktest.New(time.Now()))
	if err != nil || len(products.Trading) != 0 {
		t.Fatalf("products=%+v err=%v", products, err)
	}
	lifecycle := &hookRecorder{}
	startReconcileService(lifecycle, nil, nil, appTestLogger(t), nil)
	startOrderService(lifecycle, nil, nil, nil, appTestLogger(t), nil)
	if len(lifecycle.hooks) != 0 {
		t.Fatalf("registered %d trading hooks while disabled", len(lifecycle.hooks))
	}
}

func TestOrderStartWaitsForReconcileReady(t *testing.T) {
	t.Parallel()
	lifecycle := &hookRecorder{}
	ready := make(chan struct{})
	started := make(chan struct{})
	startServiceAfter(lifecycle, "order", ready, func(ctx context.Context) error {
		close(started)
		<-ctx.Done()
		return nil
	}, appTestLogger(t), nil)
	onStartDone := make(chan error, 1)
	go func() { onStartDone <- lifecycle.hooks[0].OnStart(t.Context()) }()
	select {
	case <-onStartDone:
		t.Fatal("order hook started before reconcile readiness")
	case <-time.After(20 * time.Millisecond):
	}
	close(ready)
	if err := <-onStartDone; err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("order service did not start after readiness")
	}
	stopCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := lifecycle.hooks[0].OnStop(stopCtx); err != nil {
		t.Fatal(err)
	}
}
