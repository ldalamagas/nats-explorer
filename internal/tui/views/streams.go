package views

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	nc "github.com/ldalamagas/nats-explorer/internal/nats"
)

type streamItem struct{ entry nc.StreamEntry }

func (s streamItem) Title() string { return s.entry.Name }
func (s streamItem) Description() string {
	subj := strings.Join(s.entry.Subjects, ", ")
	if len(subj) > 40 {
		subj = subj[:39] + "…"
	}
	return fmt.Sprintf("%d msgs · %s · %s", s.entry.Messages, humanBytes(s.entry.Bytes), subj)
}
func (s streamItem) FilterValue() string { return s.entry.Name }

type StreamsLoadedMsg struct{ Streams []nc.StreamEntry }
type StreamsErrMsg struct{ Err error }

type streamsPane int

const (
	streamsPaneList streamsPane = iota
	streamsPaneDetail
)

type StreamsView struct {
	client     *nc.Client
	width      int
	height     int
	pane       streamsPane
	streamList list.Model
	detailView viewport.Model
	streams    []nc.StreamEntry
	selected   string
	err        error
	loading    bool
}

func NewStreamsView(client *nc.Client) StreamsView {
	delg := list.NewDefaultDelegate()
	delg.ShowDescription = true

	sl := list.New(nil, delg, 0, 0)
	sl.Title = "Streams"
	sl.SetShowStatusBar(false)
	sl.SetFilteringEnabled(true)
	sl.SetShowHelp(false)

	return StreamsView{
		client:     client,
		streamList: sl,
		detailView: viewport.New(0, 0),
	}
}

func (v StreamsView) Init() tea.Cmd {
	return v.loadStreams()
}

func (v StreamsView) loadStreams() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		streams, err := v.client.ListStreams(ctx)
		if err != nil {
			return StreamsErrMsg{Err: err}
		}
		return StreamsLoadedMsg{Streams: streams}
	}
}

func (v StreamsView) Update(msg tea.Msg) (StreamsView, tea.Cmd) {
	var cmds []tea.Cmd
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case StreamsLoadedMsg:
		v.streams = msg.Streams
		v.loading = false
		items := make([]list.Item, len(msg.Streams))
		for i, s := range msg.Streams {
			items[i] = streamItem{s}
		}
		v.streamList.SetItems(items)

	case StreamsErrMsg:
		v.err = msg.Err
		v.loading = false

	case tea.KeyMsg:
		switch msg.String() {
		case "enter":
			if v.pane == streamsPaneList {
				if sel, ok := v.streamList.SelectedItem().(streamItem); ok {
					v.selected = sel.entry.Name
					v.detailView.SetContent(renderStreamDetail(sel.entry))
					v.detailView.GotoTop()
					v.pane = streamsPaneDetail
					return v, nil
				}
			}
		case "esc", "backspace":
			if v.pane > streamsPaneList {
				v.pane--
			}
			return v, nil
		case "r":
			v.loading = true
			return v, v.loadStreams()
		}
	}

	switch v.pane {
	case streamsPaneList:
		v.streamList, cmd = v.streamList.Update(msg)
		cmds = append(cmds, cmd)
	case streamsPaneDetail:
		v.detailView, cmd = v.detailView.Update(msg)
		cmds = append(cmds, cmd)
	}

	return v, tea.Batch(cmds...)
}

func (v StreamsView) Breadcrumb() string {
	if v.pane == streamsPaneDetail {
		return "Streams > " + v.selected
	}
	return "Streams"
}

func (v *StreamsView) SetSize(w, h int) {
	v.width = w
	v.height = h
	listH := h - 2
	v.streamList.SetSize(w-2, listH)
	v.detailView.Width = w - 2
	v.detailView.Height = listH
}

func (v StreamsView) View() string {
	if v.loading {
		return "  Loading streams…"
	}
	if v.err != nil {
		return fmt.Sprintf("  Error: %s", v.err)
	}

	switch v.pane {
	case streamsPaneList:
		return v.streamList.View()
	case streamsPaneDetail:
		return v.detailView.View()
	}
	return ""
}

func renderStreamDetail(e nc.StreamEntry) string {
	var b strings.Builder

	fmt.Fprintf(&b, "Stream Name: %s\n", e.Name)
	if e.Description != "" {
		fmt.Fprintf(&b, "Description: %s\n", e.Description)
	}
	fmt.Fprintf(&b, "    Created: %s\n", e.Created.UTC().Format(time.RFC3339))

	b.WriteString("\nConfiguration:\n\n")
	if len(e.Subjects) > 0 {
		fmt.Fprintf(&b, "   Subjects: %s\n", strings.Join(e.Subjects, "\n             "))
	}
	fmt.Fprintf(&b, "  Retention: %s\n", e.Retention)
	fmt.Fprintf(&b, "    Storage: %s\n", capitalizeFirst(e.Storage))
	fmt.Fprintf(&b, "   Replicas: %d\n", e.Replicas)
	if e.Duplicates > 0 {
		fmt.Fprintf(&b, " Duplicates: %s\n", e.Duplicates)
	}
	if e.MaxMsgs <= 0 {
		b.WriteString("   Max Msgs: unlimited\n")
	} else {
		fmt.Fprintf(&b, "   Max Msgs: %d\n", e.MaxMsgs)
	}
	if e.MaxBytes <= 0 {
		b.WriteString("  Max Bytes: unlimited\n")
	} else {
		fmt.Fprintf(&b, "  Max Bytes: %s\n", humanBytes(uint64(e.MaxBytes)))
	}
	if e.MaxAge == 0 {
		b.WriteString("    Max Age: unlimited\n")
	} else {
		fmt.Fprintf(&b, "    Max Age: %s\n", e.MaxAge)
	}
	if e.MaxMsgSize <= 0 {
		b.WriteString(" Max Msg Sz: unlimited\n")
	} else {
		fmt.Fprintf(&b, " Max Msg Sz: %s\n", humanBytes(uint64(e.MaxMsgSize)))
	}

	b.WriteString("\nState:\n\n")
	fmt.Fprintf(&b, "   Messages: %d\n", e.Messages)
	fmt.Fprintf(&b, "      Bytes: %s\n", humanBytes(e.Bytes))
	fmt.Fprintf(&b, "  Consumers: %d\n", e.Consumers)
	fmt.Fprintf(&b, "   Subjects: %d\n", e.NumSubjects)
	if e.FirstSeq > 0 {
		fmt.Fprintf(&b, "   FirstSeq: %d @ %s\n", e.FirstSeq, e.FirstTime.UTC().Format(time.RFC3339))
	}
	if e.LastSeq > 0 {
		fmt.Fprintf(&b, "    LastSeq: %d @ %s\n", e.LastSeq, e.LastTime.UTC().Format(time.RFC3339))
	}
	if e.NumDeleted > 0 {
		fmt.Fprintf(&b, "   Deleted: %d\n", e.NumDeleted)
	}

	return lipgloss.NewStyle().Padding(0, 1).Render(b.String())
}
