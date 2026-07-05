package main

import (
	"errors"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"google.golang.org/protobuf/types/known/timestamppb"

	controlv1 "github.com/romanornr/delta-works/internal/api/gen/control/v1"
)

func snapshotEvent(venue, account, total string) tea.Msg {
	return eventMsg{event: &controlv1.Event{
		Subject: "snapshot.taken",
		At:      timestamppb.New(time.Date(2026, 7, 4, 12, 0, 0, 0, time.UTC)),
		Payload: &controlv1.Event_SnapshotTaken{SnapshotTaken: &controlv1.AccountSnapshot{
			Venue:   venue,
			Account: account,
			Balances: []*controlv1.Balance{
				{Currency: "BTC", Total: total, Free: total, Locked: "0"},
			},
		}},
	}}
}

func TestWatchModel(t *testing.T) {
	t.Parallel()
	var m tea.Model = newWatchModel()

	view := m.View().Content
	if !strings.Contains(view, "waiting for the first snapshot") {
		t.Fatalf("empty state not rendered:\n%s", view)
	}

	m, _ = m.Update(snapshotEvent("bybit", "spot", "1.5"))
	m, _ = m.Update(snapshotEvent("bybit", "spot", "2.5"))
	m, _ = m.Update(snapshotEvent("kraken", "margin", "9"))

	view = m.View().Content
	for _, want := range []string{"bybit/spot", "2.5", "kraken/margin", "9", "2 accounts · 3 events", "· updated ", "q to quit"} {
		if !strings.Contains(view, want) {
			t.Fatalf("view missing %q:\n%s", want, view)
		}
	}
	if strings.Contains(view, "1.5") {
		t.Fatalf("stale balance not replaced by newer snapshot:\n%s", view)
	}

	m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 6})
	view = m.View().Content
	lines := strings.Split(view, "\n")
	if len(lines) != 6 {
		t.Fatalf("view is %d lines, want terminal height 6:\n%s", len(lines), view)
	}
	if !strings.Contains(view, "…") || !strings.Contains(lines[5], "q to quit") {
		t.Fatalf("overflow not cropped with status bar on the last row:\n%s", view)
	}

	m, _ = m.Update(streamErrMsg{errors.New("boom")})
	if !strings.Contains(m.View().Content, "stream error: boom") {
		t.Fatalf("error state not rendered:\n%s", m.View().Content)
	}
}
