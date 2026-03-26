package views

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	nc "github.com/ldalamagas/nats-explorer/internal/nats"
)

type kvBucketItem struct{ info nc.KVBucketInfo }

func (k kvBucketItem) Title() string       { return k.info.Name }
func (k kvBucketItem) Description() string { return fmt.Sprintf("%d keys · %s", k.info.Keys, humanBytes(k.info.Bytes)) }
func (k kvBucketItem) FilterValue() string { return k.info.Name }

type kvKeyItem struct{ entry nc.KVEntry }

func (k kvKeyItem) Title() string       { return k.entry.Key }
func (k kvKeyItem) Description() string { return k.entry.Op }
func (k kvKeyItem) FilterValue() string { return k.entry.Key }

type KVLoadedMsg struct{ Buckets []nc.KVBucketInfo }
type KVKeysLoadedMsg struct{ Bucket string; Keys []nc.KVEntry }
type KVErrMsg struct{ Err error }

type kvPane int

const (
	kvPaneBuckets kvPane = iota
	kvPaneKeys
	kvPaneValue
)

type KVView struct {
	client     *nc.Client
	width      int
	height     int
	pane       kvPane
	bucketList list.Model
	keyList    list.Model
	valueView  viewport.Model
	buckets    []nc.KVBucketInfo
	keys       []nc.KVEntry
	selected   string
	err        error
	loading    bool
}

func NewKVView(client *nc.Client) KVView {
	delg := list.NewDefaultDelegate()
	delg.ShowDescription = true

	bl := list.New(nil, delg, 0, 0)
	bl.Title = "KV Buckets"
	bl.SetShowStatusBar(false)
	bl.SetFilteringEnabled(true)
	bl.SetShowHelp(false)

	kl := list.New(nil, delg, 0, 0)
	kl.Title = "Keys"
	kl.SetShowStatusBar(false)
	kl.SetFilteringEnabled(true)
	kl.SetShowHelp(false)

	vv := viewport.New(0, 0)

	return KVView{
		client:     client,
		bucketList: bl,
		keyList:    kl,
		valueView:  vv,
	}
}

func (v KVView) Init() tea.Cmd {
	return v.loadBuckets()
}

func (v KVView) loadBuckets() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		buckets, err := v.client.ListKVBuckets(ctx)
		if err != nil {
			return KVErrMsg{Err: err}
		}
		return KVLoadedMsg{Buckets: buckets}
	}
}

func (v KVView) loadKeys(bucket string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		keys, err := v.client.ListKVKeys(ctx, bucket)
		if err != nil {
			return KVErrMsg{Err: err}
		}
		return KVKeysLoadedMsg{Bucket: bucket, Keys: keys}
	}
}

func (v KVView) Update(msg tea.Msg) (KVView, tea.Cmd) {
	var cmds []tea.Cmd
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case KVLoadedMsg:
		v.buckets = msg.Buckets
		v.loading = false
		items := make([]list.Item, len(msg.Buckets))
		for i, b := range msg.Buckets {
			items[i] = kvBucketItem{b}
		}
		v.bucketList.SetItems(items)

	case KVKeysLoadedMsg:
		v.keys = msg.Keys
		items := make([]list.Item, len(msg.Keys))
		for i, k := range msg.Keys {
			items[i] = kvKeyItem{k}
		}
		v.keyList.SetItems(items)
		if len(msg.Keys) > 0 {
			v.valueView.SetContent(renderKVValue(msg.Keys[0]))
		}

	case KVErrMsg:
		v.err = msg.Err
		v.loading = false

	case tea.KeyMsg:
		switch msg.String() {
		case "enter":
			switch v.pane {
			case kvPaneBuckets:
				if sel, ok := v.bucketList.SelectedItem().(kvBucketItem); ok {
					v.selected = sel.info.Name
					v.pane = kvPaneKeys
					return v, v.loadKeys(sel.info.Name)
				}
			case kvPaneKeys:
				v.pane = kvPaneValue
				if sel, ok := v.keyList.SelectedItem().(kvKeyItem); ok {
					v.valueView.SetContent(renderKVValue(sel.entry))
				}
			}
		case "esc", "backspace":
			if v.pane > kvPaneBuckets {
				v.pane--
			}
			return v, nil
		case "r":
			v.loading = true
			return v, v.loadBuckets()
		}
	}

	switch v.pane {
	case kvPaneBuckets:
		v.bucketList, cmd = v.bucketList.Update(msg)
		cmds = append(cmds, cmd)
	case kvPaneKeys:
		v.keyList, cmd = v.keyList.Update(msg)
		cmds = append(cmds, cmd)
		if _, ok := msg.(tea.KeyMsg); ok {
			if sel, ok := v.keyList.SelectedItem().(kvKeyItem); ok {
				v.valueView.SetContent(renderKVValue(sel.entry))
			}
		}
	case kvPaneValue:
		v.valueView, cmd = v.valueView.Update(msg)
		cmds = append(cmds, cmd)
	}

	return v, tea.Batch(cmds...)
}

func (v *KVView) SetSize(w, h int) {
	v.width = w
	v.height = h
	half := w / 2
	listH := h - 2
	v.bucketList.SetSize(half-2, listH)
	v.keyList.SetSize(half-2, listH)
	v.valueView.Width = half - 2
	v.valueView.Height = listH
}

func (v KVView) View() string {
	if v.loading {
		return "  Loading KV buckets…"
	}
	if v.err != nil {
		return fmt.Sprintf("  Error: %s", v.err)
	}

	switch v.pane {
	case kvPaneBuckets:
		return v.bucketList.View()
	case kvPaneKeys:
		left := v.keyList.View()
		right := v.valueView.View()
		return lipgloss.JoinHorizontal(lipgloss.Top, left, "  ", right)
	case kvPaneValue:
		return v.valueView.View()
	}
	return ""
}

func humanBytes(b uint64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := uint64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}

func renderKVValue(e nc.KVEntry) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("Key:      %s\n", e.Key))
	b.WriteString(fmt.Sprintf("Revision: %d\n", e.Revision))
	b.WriteString(fmt.Sprintf("Op:       %s\n", e.Op))
	b.WriteString(fmt.Sprintf("Created:  %s\n\n", e.Created.Format(time.RFC3339)))
	b.WriteString("Value:\n")
	var js json.RawMessage
	if json.Unmarshal(e.Value, &js) == nil {
		pretty, err := json.MarshalIndent(js, "", "  ")
		if err == nil {
			b.WriteString(string(pretty))
			return b.String()
		}
	}
	b.WriteString(string(e.Value))
	return b.String()
}
