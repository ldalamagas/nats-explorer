package views

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	nc "github.com/ldalamagas/nats-explorer/internal/nats"
)

const maxLiveMessages = 200

type LiveMsgReceived struct{ Msg nc.LiveMessage }

type subPane int

const (
	subPaneInput subPane = iota
	subPaneMessages
)

type SubjectsView struct {
	client    *nc.Client
	width     int
	height    int
	pane      subPane
	input     textinput.Model
	msgView   viewport.Model
	messages  []nc.LiveMessage
	subject   string
	cancelSub context.CancelFunc
	msgCh     chan nc.LiveMessage
	err       error
	subscribed bool
}

func NewSubjectsView(client *nc.Client) SubjectsView {
	ti := textinput.New()
	ti.Placeholder = "e.g. orders.> or *.created"
	ti.Focus()
	ti.Width = 50

	vp := viewport.New(0, 0)

	return SubjectsView{
		client:  client,
		input:   ti,
		msgView: vp,
		msgCh:   make(chan nc.LiveMessage, 100),
	}
}

func (v SubjectsView) Init() tea.Cmd {
	return textinput.Blink
}

func (v SubjectsView) waitForMsg() tea.Cmd {
	return func() tea.Msg {
		return LiveMsgReceived{Msg: <-v.msgCh}
	}
}

func (v SubjectsView) Update(msg tea.Msg) (SubjectsView, tea.Cmd) {
	var cmds []tea.Cmd
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case LiveMsgReceived:
		v.messages = append(v.messages, msg.Msg)
		if len(v.messages) > maxLiveMessages {
			v.messages = v.messages[len(v.messages)-maxLiveMessages:]
		}
		v.msgView.SetContent(renderLiveMessages(v.messages))
		v.msgView.GotoBottom()
		cmds = append(cmds, v.waitForMsg())

	case tea.KeyMsg:
		switch msg.String() {
		case "enter":
			if v.pane == subPaneInput {
				subject := strings.TrimSpace(v.input.Value())
				if subject == "" {
					break
				}
				// Stop previous subscription
				if v.cancelSub != nil {
					v.cancelSub()
				}
				v.messages = nil
				v.subject = subject
				v.subscribed = false
				v.err = nil

				ctx, cancel := context.WithCancel(context.Background())
				v.cancelSub = cancel
				err := v.client.Subscribe(ctx, subject, v.msgCh)
				if err != nil {
					v.err = err
					cancel()
				} else {
					v.subscribed = true
					v.pane = subPaneMessages
					v.msgView.SetContent(fmt.Sprintf("Subscribed to %q — waiting for messages…", subject))
					cmds = append(cmds, v.waitForMsg())
				}
			}
		case "esc", "backspace":
			if v.pane == subPaneMessages {
				if v.cancelSub != nil {
					v.cancelSub()
					v.cancelSub = nil
				}
				v.pane = subPaneInput
				v.input.Focus()
				v.subscribed = false
				v.subject = ""
				v.messages = nil
			} else if v.pane == subPaneInput {
				// Do nothing or handle backspace in input
			}
		}
	}

	switch v.pane {
	case subPaneInput:
		v.input, cmd = v.input.Update(msg)
		cmds = append(cmds, cmd)
	case subPaneMessages:
		v.msgView, cmd = v.msgView.Update(msg)
		cmds = append(cmds, cmd)
	}

	return v, tea.Batch(cmds...)
}

func (v *SubjectsView) SetSize(w, h int) {
	v.width = w
	v.height = h
	v.input.Width = w - 10
	v.msgView.Width = w - 2
	v.msgView.Height = h - 6
}

func (v SubjectsView) View() string {
	var b strings.Builder

	b.WriteString("  Subscribe to subject: ")
	b.WriteString(v.input.View())
	b.WriteString("\n\n")

	if v.err != nil {
		b.WriteString(fmt.Sprintf("  Error: %s\n\n", v.err))
	}

	if v.subscribed {
		b.WriteString(fmt.Sprintf("  Listening on: %s  (%d msgs)\n\n", v.subject, len(v.messages)))
		b.WriteString(v.msgView.View())
	} else {
		b.WriteString("  Press Enter to subscribe. Use NATS wildcards (* and >).")
	}

	return b.String()
}

func renderLiveMessages(msgs []nc.LiveMessage) string {
	var b strings.Builder
	for _, m := range msgs {
		b.WriteString(fmt.Sprintf("[%s] %s\n", m.Time.UTC().Format(time.TimeOnly), m.Subject))
		preview := string(m.Data)
		if len(preview) > 200 {
			preview = preview[:200] + "…"
		}
		b.WriteString(fmt.Sprintf("  %s\n\n", preview))
	}
	return b.String()
}
