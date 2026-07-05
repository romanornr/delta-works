package main

import (
	"context"
	"fmt"
	"sort"
	"strings"
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
	titleStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("231")).Background(lipgloss.Color("62")).Padding(0, 1)
	subtleStyle = lipgloss.NewStyle().Faint(true)
	headerStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("212")).Padding(0, 1)
	cellStyle   = lipgloss.NewStyle().Padding(0, 1)
	statusStyle = lipgloss.NewStyle().Faint(true).Padding(0, 1)
)

type watchModel struct {
	snapshots map[string]*controlv1.AccountSnapshot
	events    int
	lastAt    time.Time
	height    int
	err       error
}

func newWatchModel() watchModel {
	return watchModel{snapshots: map[string]*controlv1.AccountSnapshot{}}
}

func (m watchModel) Init() tea.Cmd { return nil }

func (m watchModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.height = msg.Height
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
	body := subtleStyle.Render("waiting for the first snapshot event…")
	if len(m.snapshots) > 0 {
		body = balanceTable(m.snapshots)
	}
	content := titleStyle.Render("balances") + "\n\n" + body

	status := fmt.Sprintf("%d accounts · %d events", len(m.snapshots), m.events)
	if !m.lastAt.IsZero() {
		status += " · updated " + m.lastAt.Local().Format("15:04:05")
	}
	if m.height > 0 {
		lines := strings.Split(content, "\n")
		switch budget := m.height - 1; {
		case len(lines) > budget:
			lines = append(lines[:budget-1], subtleStyle.Render("…"))
		case len(lines) < budget:
			lines = append(lines, make([]string, budget-len(lines))...)
		}
		content = strings.Join(lines, "\n")
	}
	content += "\n" + statusStyle.Render(status+" · q to quit")

	v := tea.NewView(content)
	v.AltScreen = true
	v.WindowTitle = "watch"
	return v
}

func balanceTable(snapshots map[string]*controlv1.AccountSnapshot) string {
	keys := make([]string, 0, len(snapshots))
	for k := range snapshots {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var rows [][]string
	for _, k := range keys {
		balances := append([]*controlv1.Balance(nil), snapshots[k].GetBalances()...)
		sort.SliceStable(balances, func(i, j int) bool {
			zi, zj := balances[i].GetTotal() == "0", balances[j].GetTotal() == "0"
			if zi != zj {
				return zj
			}
			return balances[i].GetCurrency() < balances[j].GetCurrency()
		})
		for _, b := range balances {
			rows = append(rows, []string{k, b.GetCurrency(), b.GetTotal(), b.GetFree(), b.GetLocked()})
		}
	}

	return table.New().
		Headers("ACCOUNT", "CURRENCY", "TOTAL", "FREE", "LOCKED").
		BorderStyle(subtleStyle).
		Rows(rows...).
		StyleFunc(func(row, col int) lipgloss.Style {
			if row == table.HeaderRow {
				return headerStyle
			}
			s := cellStyle
			if col >= 2 {
				s = s.Align(lipgloss.Right)
			}
			if rows[row][2] == "0" {
				s = s.Faint(true)
			}
			return s
		}).String()
}
