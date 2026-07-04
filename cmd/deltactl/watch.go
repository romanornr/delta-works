package main

import (
	"context"
	"fmt"
	"sort"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"charm.land/lipgloss/v2/table"
	"connectrpc.com/connect"

	controlv1 "github.com/romanornr/delta-works/internal/api/gen/control/v1"
)

// runWatch renders live balances from the event stream. The stream reader
// feeds the bubbletea program through Send; the model only holds state.
func runWatch(ctx context.Context, c clients) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	p := tea.NewProgram(newWatchModel())
	go func() {
		stream, err := c.events.StreamEvents(ctx, connect.NewRequest(&controlv1.StreamEventsRequest{}))
		if err != nil {
			p.Send(streamErrMsg{err})
			return
		}
		defer func() { _ = stream.Close() }()
		for stream.Receive() {
			p.Send(eventMsg{stream.Msg().GetEvent()})
		}
		if err := stream.Err(); err != nil && ctx.Err() == nil {
			p.Send(streamErrMsg{err})
		}
	}()
	final, err := p.Run()
	if err != nil {
		return err
	}
	if m, ok := final.(watchModel); ok && m.err != nil {
		return m.err
	}
	return nil
}

type (
	eventMsg     struct{ event *controlv1.Event }
	streamErrMsg struct{ err error }
)

var (
	titleStyle  = lipgloss.NewStyle().Bold(true)
	subtleStyle = lipgloss.NewStyle().Faint(true)
)

type watchModel struct {
	snapshots map[string]*controlv1.AccountSnapshot
	events    int
	lastAt    time.Time
	err       error
}

func newWatchModel() watchModel {
	return watchModel{snapshots: map[string]*controlv1.AccountSnapshot{}}
}

func (m watchModel) Init() tea.Cmd { return nil }

func (m watchModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		}
	case eventMsg:
		m.events++
		m.lastAt = msg.event.GetAt().AsTime()
		if snap := msg.event.GetSnapshotTaken(); snap != nil {
			m.snapshots[snap.GetVenue()+"/"+snap.GetAccount()] = snap
		}
	case streamErrMsg:
		m.err = msg.err
		return m, tea.Quit
	}
	return m, nil
}

func (m watchModel) View() tea.View {
	if m.err != nil {
		return tea.NewView("stream error: " + m.err.Error() + "\n")
	}
	view := titleStyle.Render("balances") + "\n"
	if len(m.snapshots) == 0 {
		view += subtleStyle.Render("waiting for the first snapshot event…") + "\n"
	} else {
		view += balanceTable(m.snapshots) + "\n"
	}
	status := fmt.Sprintf("events=%d", m.events)
	if !m.lastAt.IsZero() {
		status += "  last=" + m.lastAt.Local().Format("15:04:05")
	}
	return tea.NewView(view + subtleStyle.Render(status+"  q to quit") + "\n")
}

func balanceTable(snapshots map[string]*controlv1.AccountSnapshot) string {
	keys := make([]string, 0, len(snapshots))
	for k := range snapshots {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	t := table.New().Headers("ACCOUNT", "CURRENCY", "TOTAL", "FREE", "LOCKED")
	for _, k := range keys {
		for _, b := range snapshots[k].GetBalances() {
			t.Row(k, b.GetCurrency(), b.GetTotal(), b.GetFree(), b.GetLocked())
		}
	}
	return t.String()
}
