package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	nc "github.com/lefteris/nats-explorer/internal/nats"
	"github.com/lefteris/nats-explorer/internal/tui/views"
)

type tab int

const (
	tabKV tab = iota
	tabObjects
	tabSubjects
)

var tabNames = []string{"KV Store", "Object Store", "Subjects"}

type App struct {
	client   *nc.Client
	activeTab tab
	width    int
	height   int

	kvView       views.KVView
	objView      views.ObjView
	subjectsView views.SubjectsView
}

func NewApp(client *nc.Client) App {
	return App{
		client:       client,
		kvView:       views.NewKVView(client),
		objView:      views.NewObjView(client),
		subjectsView: views.NewSubjectsView(client),
	}
}

func (a App) Init() tea.Cmd {
	return tea.Batch(
		a.kvView.Init(),
		a.objView.Init(),
		a.subjectsView.Init(),
	)
}

func (a App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		a.width = msg.Width
		a.height = msg.Height
		contentH := msg.Height - 5 // header + tabs + status
		contentW := msg.Width - 2
		a.kvView.SetSize(contentW, contentH)
		a.objView.SetSize(contentW, contentH)
		a.subjectsView.SetSize(contentW, contentH)
		return a, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			return a, tea.Quit
		case "tab", "right":
			a.activeTab = (a.activeTab + 1) % tab(len(tabNames))
			return a, nil
		case "shift+tab", "left":
			a.activeTab = (a.activeTab + tab(len(tabNames)) - 1) % tab(len(tabNames))
			return a, nil
		case "1":
			a.activeTab = tabKV
			return a, nil
		case "2":
			a.activeTab = tabObjects
			return a, nil
		case "3":
			a.activeTab = tabSubjects
			return a, nil
		}
	}

	// Route messages to active view
	var cmd tea.Cmd
	switch a.activeTab {
	case tabKV:
		a.kvView, cmd = a.kvView.Update(msg)
	case tabObjects:
		a.objView, cmd = a.objView.Update(msg)
	case tabSubjects:
		a.subjectsView, cmd = a.subjectsView.Update(msg)
	}
	cmds = append(cmds, cmd)

	// Also forward data-loading messages to non-active views
	switch msg.(type) {
	case views.KVLoadedMsg, views.KVKeysLoadedMsg, views.KVErrMsg:
		if a.activeTab != tabKV {
			a.kvView, cmd = a.kvView.Update(msg)
			cmds = append(cmds, cmd)
		}
	case views.ObjLoadedMsg, views.ObjEntriesLoadedMsg, views.ObjErrMsg:
		if a.activeTab != tabObjects {
			a.objView, cmd = a.objView.Update(msg)
			cmds = append(cmds, cmd)
		}
	}

	return a, tea.Batch(cmds...)
}

func (a App) View() string {
	if a.width == 0 {
		return "Loading…"
	}

	var b strings.Builder

	// Header
	status := StyleSecondary.Render("● connected")
	if !a.client.IsConnected() {
		status = StyleError.Render("✗ disconnected")
	}
	header := StyleHeader.Render("nats-explorer") + "  " + StyleMuted.Render(a.client.ServerURL()) + "  " + status
	b.WriteString(header + "\n")
	b.WriteString(strings.Repeat("─", a.width) + "\n")

	// Tabs
	tabs := make([]string, len(tabNames))
	for i, name := range tabNames {
		label := fmt.Sprintf(" %d:%s ", i+1, name)
		if tab(i) == a.activeTab {
			tabs[i] = StyleTabActive.Render(label)
		} else {
			tabs[i] = StyleTabInactive.Render(label)
		}
	}
	b.WriteString(strings.Join(tabs, "  ") + "\n")
	b.WriteString(strings.Repeat("─", a.width) + "\n")

	// Content
	switch a.activeTab {
	case tabKV:
		b.WriteString(a.kvView.View())
	case tabObjects:
		b.WriteString(a.objView.View())
	case tabSubjects:
		b.WriteString(a.subjectsView.View())
	}

	// Status bar
	b.WriteString("\n")
	hints := []string{
		hint("tab", "switch tab"),
		hint("↑↓", "navigate"),
		hint("enter", "select"),
		hint("esc", "back"),
		hint("r", "refresh"),
		hint("q", "quit"),
	}
	b.WriteString(lipgloss.PlaceHorizontal(a.width, lipgloss.Left, strings.Join(hints, "  ")))

	return b.String()
}

func hint(key, desc string) string {
	return StyleKeyName.Render(key) + StyleKeyHint.Render(" "+desc)
}
