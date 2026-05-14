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

type objBucketItem struct{ info nc.ObjBucketInfo }

func (o objBucketItem) Title() string       { return o.info.Name }
func (o objBucketItem) Description() string {
	desc := humanBytes(o.info.Size)
	if o.info.Description != "" {
		desc += " · " + o.info.Description
	}
	return desc
}
func (o objBucketItem) FilterValue() string { return o.info.Name }

type objEntryItem struct{ entry nc.ObjEntry }

func (o objEntryItem) Title() string       { return o.entry.Name }
func (o objEntryItem) Description() string { return humanBytes(o.entry.Size) }
func (o objEntryItem) FilterValue() string { return o.entry.Name }

type ObjLoadedMsg struct{ Buckets []nc.ObjBucketInfo }
type ObjEntriesLoadedMsg struct{ Bucket string; Entries []nc.ObjEntry }
type ObjInfoLoadedMsg struct{ Detail *nc.ObjBucketDetail }
type ObjErrMsg struct{ Err error }

type objPane int

const (
	objPaneBuckets objPane = iota
	objPaneEntries
	objPaneDetail
)

type ObjView struct {
	client        *nc.Client
	width         int
	height        int
	pane          objPane
	bucketList    list.Model
	entryList     list.Model
	detailView    viewport.Model
	infoView      viewport.Model
	showInfo      bool
	buckets       []nc.ObjBucketInfo
	entries       []nc.ObjEntry
	selected      string
	selectedEntry string
	err           error
	loading       bool
}

func NewObjView(client *nc.Client) ObjView {
	delg := list.NewDefaultDelegate()
	delg.ShowDescription = true

	bl := list.New(nil, delg, 0, 0)
	bl.Title = "Object Buckets"
	bl.SetShowStatusBar(false)
	bl.SetFilteringEnabled(true)
	bl.SetShowHelp(false)

	el := list.New(nil, delg, 0, 0)
	el.Title = "Objects"
	el.SetShowStatusBar(false)
	el.SetFilteringEnabled(true)
	el.SetShowHelp(false)

	dv := viewport.New(0, 0)

	return ObjView{
		client:     client,
		bucketList: bl,
		entryList:  el,
		detailView: dv,
		infoView:   viewport.New(0, 0),
	}
}

func (v ObjView) Init() tea.Cmd {
	return v.loadBuckets()
}

func (v ObjView) loadBuckets() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		buckets, err := v.client.ListObjBuckets(ctx)
		if err != nil {
			return ObjErrMsg{Err: err}
		}
		return ObjLoadedMsg{Buckets: buckets}
	}
}

func (v ObjView) loadObjInfo(bucket string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		detail, err := v.client.GetObjBucketInfo(ctx, bucket)
		if err != nil {
			return ObjErrMsg{Err: err}
		}
		return ObjInfoLoadedMsg{Detail: detail}
	}
}

func (v ObjView) loadEntries(bucket string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		entries, err := v.client.ListObjEntries(ctx, bucket)
		if err != nil {
			return ObjErrMsg{Err: err}
		}
		return ObjEntriesLoadedMsg{Bucket: bucket, Entries: entries}
	}
}

func (v ObjView) Update(msg tea.Msg) (ObjView, tea.Cmd) {
	var cmds []tea.Cmd
	var cmd tea.Cmd

	if v.showInfo {
		if keyMsg, ok := msg.(tea.KeyMsg); ok {
			if keyMsg.String() == "esc" || keyMsg.String() == "backspace" {
				v.showInfo = false
				return v, nil
			}
		}
		v.infoView, cmd = v.infoView.Update(msg)
		return v, cmd
	}

	switch msg := msg.(type) {
	case ObjLoadedMsg:
		v.buckets = msg.Buckets
		v.loading = false
		items := make([]list.Item, len(msg.Buckets))
		for i, b := range msg.Buckets {
			items[i] = objBucketItem{b}
		}
		v.bucketList.SetItems(items)

	case ObjEntriesLoadedMsg:
		v.entries = msg.Entries
		items := make([]list.Item, len(msg.Entries))
		for i, e := range msg.Entries {
			items[i] = objEntryItem{e}
		}
		v.entryList.SetItems(items)
		if len(msg.Entries) > 0 {
			v.detailView.SetContent(renderObjDetail(msg.Entries[0]))
		}

	case ObjInfoLoadedMsg:
		v.infoView.SetContent(renderObjInfo(msg.Detail))
		v.infoView.GotoTop()
		v.showInfo = true

	case ObjErrMsg:
		v.err = msg.Err
		v.loading = false

	case tea.KeyMsg:
		switch msg.String() {
		case "enter":
			switch v.pane {
			case objPaneBuckets:
				if sel, ok := v.bucketList.SelectedItem().(objBucketItem); ok {
					v.selected = sel.info.Name
					v.pane = objPaneEntries
					return v, v.loadEntries(sel.info.Name)
				}
			case objPaneEntries:
				v.pane = objPaneDetail
				if sel, ok := v.entryList.SelectedItem().(objEntryItem); ok {
					v.selectedEntry = sel.entry.Name
					v.detailView.SetContent(renderObjDetail(sel.entry))
				}
			}
		case "i":
			var bucket string
			switch v.pane {
			case objPaneBuckets:
				if sel, ok := v.bucketList.SelectedItem().(objBucketItem); ok {
					bucket = sel.info.Name
				}
			case objPaneEntries, objPaneDetail:
				bucket = v.selected
			}
			if bucket != "" {
				return v, v.loadObjInfo(bucket)
			}
		case "esc", "backspace":
			if v.pane > objPaneBuckets {
				v.pane--
			}
			return v, nil
		case "r":
			switch v.pane {
			case objPaneBuckets:
				v.loading = true
				return v, v.loadBuckets()
			case objPaneEntries, objPaneDetail:
				return v, v.loadEntries(v.selected)
			}
		}
	}

	switch v.pane {
	case objPaneBuckets:
		v.bucketList, cmd = v.bucketList.Update(msg)
		cmds = append(cmds, cmd)
	case objPaneEntries:
		v.entryList, cmd = v.entryList.Update(msg)
		cmds = append(cmds, cmd)
		if _, ok := msg.(tea.KeyMsg); ok {
			if sel, ok := v.entryList.SelectedItem().(objEntryItem); ok {
				v.selectedEntry = sel.entry.Name
				v.detailView.SetContent(renderObjDetail(sel.entry))
			}
		}
	case objPaneDetail:
		v.detailView, cmd = v.detailView.Update(msg)
		cmds = append(cmds, cmd)
	}

	return v, tea.Batch(cmds...)
}

func (v ObjView) Breadcrumb() string {
	if v.showInfo {
		return "Object Store > " + v.selected + " > Info"
	}
	switch v.pane {
	case objPaneEntries:
		return "Object Store > " + v.selected
	case objPaneDetail:
		return "Object Store > " + v.selected + " > " + v.selectedEntry
	}
	return "Object Store"
}

func (v *ObjView) SetSize(w, h int) {
	v.width = w
	v.height = h
	half := w / 2
	listH := h - 2
	v.bucketList.SetSize(half-2, listH)
	v.entryList.SetSize(half-2, listH)
	v.detailView.Width = half - 2
	v.detailView.Height = listH
	v.infoView.Width = w - 2
	v.infoView.Height = h - 2
}

func (v ObjView) View() string {
	if v.loading {
		return "  Loading object buckets…"
	}
	if v.err != nil {
		return fmt.Sprintf("  Error: %s", v.err)
	}
	if v.showInfo {
		return v.infoView.View()
	}

	switch v.pane {
	case objPaneBuckets:
		return v.bucketList.View()
	case objPaneEntries:
		left := v.entryList.View()
		right := v.detailView.View()
		return lipgloss.JoinHorizontal(lipgloss.Top, left, "  ", right)
	case objPaneDetail:
		return v.detailView.View()
	}
	return ""
}

func renderObjInfo(d *nc.ObjBucketDetail) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Information for Object Store Bucket %s\n\n", d.Name)
	b.WriteString("Configuration:\n\n")
	fmt.Fprintf(&b, "         Bucket Name: %s\n", d.Name)
	if d.Description != "" {
		fmt.Fprintf(&b, "         Description: %s\n", d.Description)
	}
	if d.TTL == 0 {
		b.WriteString("         Maximum Age: unlimited\n")
	} else {
		fmt.Fprintf(&b, "         Maximum Age: %s\n", d.TTL)
	}
	fmt.Fprintf(&b, "  Backing Store Size: %s\n", humanBytes(d.Size))
	fmt.Fprintf(&b, "    JetStream Stream: %s\n", d.StreamName)
	fmt.Fprintf(&b, "             Storage: %s\n", capitalizeFirst(d.Storage))
	fmt.Fprintf(&b, "            Replicas: %d\n", d.Replicas)
	if d.Sealed {
		b.WriteString("              Sealed: true\n")
	}
	return b.String()
}

func renderObjDetail(e nc.ObjEntry) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("Name:     %s\n", e.Name))
	b.WriteString(fmt.Sprintf("Size:     %s\n", humanBytes(e.Size)))
	b.WriteString(fmt.Sprintf("Chunks:   %d\n", e.Chunks))
	b.WriteString(fmt.Sprintf("Modified: %s\n", e.Modified.UTC().Format(time.RFC3339)))
	if e.Description != "" {
		b.WriteString(fmt.Sprintf("Desc:     %s\n", e.Description))
	}
	if e.Digest != "" {
		b.WriteString(fmt.Sprintf("Digest:   %s\n", e.Digest))
	}
	if !e.Modified.IsZero() {
		b.WriteString(fmt.Sprintf("Modified: %s\n", e.Modified.UTC().Format(time.RFC3339)))
	}
	return b.String()
}
