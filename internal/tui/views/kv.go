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

// maxKeyLines caps how many lines a single key may occupy before truncation.
const maxKeyLines = 4

// kvKeyList is a scrollable key list that renders each item at exactly the
// height it needs (key lines + Op line) rather than a uniform height for all.
type kvKeyList struct {
	keys       []nc.KVEntry
	selected   int
	width      int
	height     int
	vp         viewport.Model
	itemStyles list.DefaultItemStyles
	listStyles list.Styles
}

func newKVKeyList() kvKeyList {
	return kvKeyList{
		itemStyles: list.NewDefaultItemStyles(),
		listStyles: list.DefaultStyles(),
		vp:         viewport.New(0, 0),
	}
}

func (l *kvKeyList) textWidth() int {
	return l.width - l.itemStyles.NormalTitle.GetHorizontalFrameSize()
}

func (l *kvKeyList) itemHeight(key string) int {
	textW := l.textWidth()
	if textW <= 0 {
		return 2
	}
	lines := (len(key) + textW - 1) / textW
	if lines > maxKeyLines {
		lines = maxKeyLines
	}
	return lines + 1 // +1 for Op line
}

func (l *kvKeyList) renderItem(idx int, k nc.KVEntry) string {
	selected := idx == l.selected
	var titleStyle, descStyle lipgloss.Style
	if selected {
		titleStyle = l.itemStyles.SelectedTitle
		descStyle = l.itemStyles.SelectedDesc
	} else {
		titleStyle = l.itemStyles.NormalTitle
		descStyle = l.itemStyles.NormalDesc
	}

	textW := l.textWidth()
	if textW <= 0 {
		return ""
	}

	key := k.Key
	op := k.Op
	keyRows := l.itemHeight(k.Key) - 1

	var sb strings.Builder
	for i := 0; i < keyRows; i++ {
		if i > 0 {
			sb.WriteByte('\n')
		}
		switch {
		case len(key) == 0:
			sb.WriteString(titleStyle.Render(""))
		case len(key) <= textW:
			sb.WriteString(titleStyle.Render(key))
			key = ""
		case i == keyRows-1:
			// Last available row but key still has more — truncate.
			sb.WriteString(titleStyle.Render(key[:textW-1] + "…"))
			key = ""
		default:
			sb.WriteString(titleStyle.Render(key[:textW]))
			key = key[textW:]
		}
	}
	sb.WriteByte('\n')
	sb.WriteString(descStyle.Render(op))
	return sb.String()
}

func (l *kvKeyList) refresh() {
	if l.width == 0 {
		return
	}
	var sb strings.Builder
	for i, k := range l.keys {
		if i > 0 {
			sb.WriteString("\n\n") // \n ends previous item's last line; \n adds blank spacing line
		}
		sb.WriteString(l.renderItem(i, k))
	}
	l.vp.SetContent(sb.String())
	l.scrollToSelected()
}

func (l *kvKeyList) scrollToSelected() {
	if len(l.keys) == 0 {
		return
	}
	y := 0
	for i := 0; i < l.selected; i++ {
		y += l.itemHeight(l.keys[i].Key) + 1 // +1 for spacing line
	}
	itemH := l.itemHeight(l.keys[l.selected].Key)
	if y < l.vp.YOffset {
		l.vp.SetYOffset(y)
	} else if y+itemH > l.vp.YOffset+l.vp.Height {
		l.vp.SetYOffset(y + itemH - l.vp.Height)
	}
}

func (l *kvKeyList) SetKeys(keys []nc.KVEntry) {
	l.keys = keys
	l.selected = 0
	l.vp.SetYOffset(0)
	l.refresh()
}

func (l *kvKeyList) SetSize(w, h int) {
	l.width = w
	l.height = h
	l.vp.Width = w
	l.vp.Height = h - 1 // reserve 1 line for the title bar
	if l.vp.Height < 0 {
		l.vp.Height = 0
	}
	l.refresh()
}

func (l *kvKeyList) Selected() *nc.KVEntry {
	if l.selected >= 0 && l.selected < len(l.keys) {
		return &l.keys[l.selected]
	}
	return nil
}

func (l *kvKeyList) Update(msg tea.Msg) {
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return
	}
	switch keyMsg.String() {
	case "up", "k":
		if l.selected > 0 {
			l.selected--
			l.refresh()
		}
	case "down", "j":
		if l.selected < len(l.keys)-1 {
			l.selected++
			l.refresh()
		}
	}
}

func (l *kvKeyList) View() string {
	titleBar := l.listStyles.TitleBar.Width(l.width).Render(
		l.listStyles.Title.Render("Keys"),
	)
	return lipgloss.JoinVertical(lipgloss.Left, titleBar, l.vp.View())
}

type kvBucketItem struct{ info nc.KVBucketInfo }

func (k kvBucketItem) Title() string       { return k.info.Name }
func (k kvBucketItem) Description() string { return fmt.Sprintf("%d keys · %s", k.info.Keys, humanBytes(k.info.Bytes)) }
func (k kvBucketItem) FilterValue() string { return k.info.Name }

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
	client      *nc.Client
	width       int
	height      int
	pane        kvPane
	bucketList  list.Model
	keyList     kvKeyList
	valueView   viewport.Model
	buckets     []nc.KVBucketInfo
	selected    string
	selectedKey string
	err         error
	loading     bool
}

func NewKVView(client *nc.Client) KVView {
	bucketDelg := list.NewDefaultDelegate()
	bucketDelg.ShowDescription = true

	bl := list.New(nil, bucketDelg, 0, 0)
	bl.Title = "KV Buckets"
	bl.SetShowStatusBar(false)
	bl.SetFilteringEnabled(true)
	bl.SetShowHelp(false)

	return KVView{
		client:     client,
		bucketList: bl,
		keyList:    newKVKeyList(),
		valueView:  viewport.New(0, 0),
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
		v.keyList.SetKeys(msg.Keys)
		if sel := v.keyList.Selected(); sel != nil {
			v.valueView.SetContent(renderKVValue(*sel, v.valueView.Width))
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
				if sel := v.keyList.Selected(); sel != nil {
					v.selectedKey = sel.Key
					v.valueView.SetContent(renderKVValue(*sel, v.valueView.Width))
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
		v.keyList.Update(msg)
		if _, ok := msg.(tea.KeyMsg); ok {
			if sel := v.keyList.Selected(); sel != nil {
				v.selectedKey = sel.Key
				v.valueView.SetContent(renderKVValue(*sel, v.valueView.Width))
			}
		}
	case kvPaneValue:
		v.valueView, cmd = v.valueView.Update(msg)
		cmds = append(cmds, cmd)
	}

	return v, tea.Batch(cmds...)
}

func (v KVView) Breadcrumb() string {
	switch v.pane {
	case kvPaneKeys:
		return "KV Store > " + v.selected
	case kvPaneValue:
		return "KV Store > " + v.selected + " > " + v.selectedKey
	}
	return "KV Store"
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

func renderKVValue(e nc.KVEntry, width int) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("Key:      %s\n", e.Key))
	b.WriteString(fmt.Sprintf("Revision: %d\n", e.Revision))
	b.WriteString(fmt.Sprintf("Op:       %s\n", e.Op))
	b.WriteString(fmt.Sprintf("Created:  %s\n\n", e.Created.UTC().Format(time.RFC3339)))
	b.WriteString("Value:\n")
	var js json.RawMessage
	if json.Unmarshal(e.Value, &js) == nil {
		pretty, err := json.MarshalIndent(js, "", "  ")
		if err == nil {
			b.WriteString(string(pretty))
			return wrapText(b.String(), width)
		}
	}
	b.WriteString(string(e.Value))
	return wrapText(b.String(), width)
}

// wrapText wraps lines in s that exceed width characters, breaking at the last
// space within the limit or hard-breaking if no space is found.
func wrapText(s string, width int) string {
	if width <= 0 {
		return s
	}
	var out strings.Builder
	for i, line := range strings.Split(s, "\n") {
		if i > 0 {
			out.WriteByte('\n')
		}
		for len(line) > width {
			cut := width
			if idx := strings.LastIndex(line[:width], " "); idx > 0 {
				cut = idx + 1
			}
			out.WriteString(line[:cut])
			out.WriteByte('\n')
			line = line[cut:]
		}
		out.WriteString(line)
	}
	return out.String()
}
