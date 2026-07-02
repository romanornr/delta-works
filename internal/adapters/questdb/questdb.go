// Package questdb is the time-series adapter (ADR-0004: QuestDB is
// analytics, never accounting truth). Rows are sent over ILP/HTTP; tables
// are auto-created by the server on first write.
package questdb

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	qdb "github.com/questdb/go-questdb-client/v4"

	"github.com/romanornr/delta-works/internal/config"
	"github.com/romanornr/delta-works/internal/domain/account"
	"github.com/romanornr/delta-works/internal/domain/marketdata"
	"github.com/romanornr/delta-works/internal/ports"
)

// Writer implements ports.SeriesWriter over one ILP line sender. The sender
// is not safe for concurrent use, so all writes are serialized here.
type Writer struct {
	mu     sync.Mutex
	sender qdb.LineSender
}

var _ ports.SeriesWriter = (*Writer)(nil)

// New connects a line sender from a QuestDB configuration string, e.g.
// "http::addr=localhost:9000;".
func New(ctx context.Context, cfg config.QuestDB) (*Writer, error) {
	sender, err := qdb.LineSenderFromConf(ctx, cfg.Conf)
	if err != nil {
		return nil, fmt.Errorf("questdb: connect: %w", err)
	}
	return &Writer{sender: sender}, nil
}

// WriteBalanceSnapshot implements ports.SeriesWriter. decimal→float64 is
// acceptable here only because this store is analytics (ADR-0004).
func (w *Writer) WriteBalanceSnapshot(ctx context.Context, s account.Snapshot) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	for _, b := range s.NonZero() {
		err := w.sender.Table("balances").
			Symbol("venue", string(s.Account.Venue)).
			Symbol("account", string(s.Account.Type)).
			Symbol("currency", string(b.Currency)).
			Float64Column("total", b.Total.InexactFloat64()).
			Float64Column("free", b.Free.InexactFloat64()).
			Float64Column("locked", b.Locked.InexactFloat64()).
			At(ctx, s.TakenAt)
		if err != nil {
			return fmt.Errorf("questdb: write balance %s/%s: %w", s.Account.Venue, b.Currency, err)
		}
	}
	return nil
}

// WriteTicker implements ports.SeriesWriter.
func (w *Writer) WriteTicker(ctx context.Context, t marketdata.Ticker) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	err := w.sender.Table("tickers").
		Symbol("venue", string(t.Instrument.Venue)).
		Symbol("symbol", t.Instrument.Pair()).
		Float64Column("bid", t.Bid.InexactFloat64()).
		Float64Column("ask", t.Ask.InexactFloat64()).
		Float64Column("last", t.Last.InexactFloat64()).
		Float64Column("bid_size", t.BidSize.InexactFloat64()).
		Float64Column("ask_size", t.AskSize.InexactFloat64()).
		At(ctx, t.At)
	if err != nil {
		return fmt.Errorf("questdb: write ticker %s: %w", t.Instrument.Key(), err)
	}
	return nil
}

// Flush implements ports.SeriesWriter.
func (w *Writer) Flush(ctx context.Context) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if err := w.sender.Flush(ctx); err != nil {
		return fmt.Errorf("questdb: flush: %w", err)
	}
	return nil
}

// Close releases the sender.
func (w *Writer) Close(ctx context.Context) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.sender.Close(ctx)
}

// Health checks QuestDB reachability for /readyz by pinging the HTTP
// endpoint derived from the configuration string.
type Health struct {
	url string
}

// NewHealth builds the QuestDB readiness check.
func NewHealth(cfg config.QuestDB) *Health {
	addr := "localhost:9000"
	scheme := "http"
	for part := range strings.SplitSeq(cfg.Conf, ";") {
		if v, ok := strings.CutPrefix(part, "http::addr="); ok {
			addr = v
		} else if v, ok := strings.CutPrefix(part, "https::addr="); ok {
			addr = v
			scheme = "https"
		} else if v, ok := strings.CutPrefix(part, "addr="); ok {
			addr = v
		}
	}
	return &Health{url: scheme + "://" + addr + "/ping"}
}

// Name implements ports.HealthChecker.
func (h *Health) Name() string { return "questdb" }

// Check implements ports.HealthChecker.
func (h *Health) Check(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, h.url, http.NoBody)
	if err != nil {
		return err
	}
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close() //nolint:errcheck // response body unused
	if resp.StatusCode >= 300 {
		return fmt.Errorf("questdb: ping status %d", resp.StatusCode)
	}
	return nil
}
