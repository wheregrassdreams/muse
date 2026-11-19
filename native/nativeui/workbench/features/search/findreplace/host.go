package findreplace

import (
	"github.com/egoist/mygo/ui"
	editfind "github.com/wheregrassdreams/muse-editor/text/findreplace"
)

// EditorPort is the narrow current-document capability required by
// find/replace. Matching and replacement range planning stay in
// editor/findreplace; the adapter supplies source and applies the resulting
// edits through the active editor session.
type EditorPort interface {
	Source() string
	SetHighlights(matches []editfind.Match, selected int)
	NavigateToRange(start, end int)
	Focus()
	Replace(start, end int, value string) error
	ReplaceAll(source string)
}

type Ports struct {
	Editor     EditorPort
	Invalidate func()
	Replaced   func(count int)
}

// Host coordinates the find/replace panel with the current editor. It owns
// only feature state and action flow; document matching and edit ranges are
// supplied by editor/findreplace, and mutations go through EditorPort.
type Host struct {
	handle *Handle
	ports  Ports
}

func NewHost(options Options) *Host {
	handle := options.Handle
	if handle == nil {
		handle = NewHandle()
	}
	return &Host{handle: handle, ports: options.Ports}
}

type Options struct {
	Handle *Handle
	Ports  Ports
}

func (h *Host) Snapshot() Snapshot {
	if h == nil {
		return Snapshot{Selected: -1}
	}
	return h.handle.Snapshot()
}

func (h *Host) Apply(action Action) {
	if h == nil {
		return
	}
	h.handle.Apply(action)
	h.dispatch(action)
}

func (h *Host) Invalidate() {
	if h != nil {
		h.handle.Invalidate()
	}
}

// Sync refreshes matches from the active editor source and projects current
// match highlights. It is safe to call once per frame; unchanged inputs do no
// search work.
func (h *Host) Sync() {
	if h == nil || h.ports.Editor == nil || !h.handle.Sync(h.ports.Editor.Source()) {
		return
	}
	h.syncHighlights()
	h.invalidate()
}

func (h *Host) Render(c *ui.Context) {
	if h == nil {
		return
	}
	h.Sync()
	Render(c, h.handle, h.dispatch)
}

func (h *Host) dispatch(action Action) {
	if h == nil {
		return
	}
	switch action.Kind {
	case QueryChanged:
		h.Sync()
		h.invalidate()
	case ReplacementChanged:
		// Replacement text is feature state; edits occur only on replace actions.
	case OpenMatch:
		h.openMatch(action.Index)
	case ReplaceOne:
		h.replaceOne()
	case ReplaceAll:
		h.replaceAll()
	}
}

func (h *Host) openMatch(index int) {
	if h.ports.Editor == nil {
		return
	}
	snapshot := h.handle.Snapshot()
	if index < 0 || index >= len(snapshot.Matches) {
		return
	}
	h.handle.Apply(Action{Kind: OpenMatch, Index: index})
	h.syncHighlights()
	match := snapshot.Matches[index]
	h.ports.Editor.NavigateToRange(match.Start, match.End)
	h.ports.Editor.Focus()
	h.invalidate()
}

func (h *Host) replaceOne() {
	if h.ports.Editor == nil {
		return
	}
	snapshot := h.handle.Snapshot()
	index := max(snapshot.Selected, 0)
	if index >= len(snapshot.Matches) {
		return
	}
	match := snapshot.Matches[index]
	_ = h.ports.Editor.Replace(match.Start, match.End, snapshot.Replacement)
	h.Sync()
	if remaining := len(h.handle.Snapshot().Matches); remaining > 0 {
		h.openMatch(min(index, remaining-1))
	}
}

func (h *Host) replaceAll() {
	if h.ports.Editor == nil {
		return
	}
	snapshot := h.handle.Snapshot()
	if len(snapshot.Matches) == 0 {
		return
	}
	source := h.ports.Editor.Source()
	replaced, count := editfind.ReplaceAll(source, snapshot.Matches, snapshot.Replacement)
	if count == 0 {
		return
	}
	h.ports.Editor.ReplaceAll(replaced)
	h.Sync()
	if h.ports.Replaced != nil {
		h.ports.Replaced(count)
	}
	h.invalidate()
}

func (h *Host) syncHighlights() {
	if h == nil || h.ports.Editor == nil {
		return
	}
	snapshot := h.handle.Snapshot()
	h.ports.Editor.SetHighlights(snapshot.Matches, snapshot.Selected)
}

func (h *Host) invalidate() {
	if h != nil && h.ports.Invalidate != nil {
		h.ports.Invalidate()
	}
}
