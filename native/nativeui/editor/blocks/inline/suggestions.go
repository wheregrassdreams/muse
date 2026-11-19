package inlineui

import (
	"github.com/egoist/mygo/ui"
	"muse/editor/input"
)

const SuggestionLimit = 8

// Suggestion is a detached row presented by the inline-link popup.
type Suggestion struct {
	Target string
	Title  string
}

// Frame is a detached read-only view of the popup, with no editor, parser,
// vault, or source-buffer references.
type Frame struct {
	open       bool
	query      string
	start      int
	end        int
	items      []Suggestion
	selected   int
	bounds     ui.Rect
	dismissed  bool
	dismissKey string
}

type popupState struct {
	open       bool
	query      string
	start      int
	end        int
	items      []Suggestion
	selected   int
	bounds     ui.Rect
	dismissed  bool
	dismissKey string
}

// Popup owns suggestion state and emits typed actions. Callers can only read a
// detached Frame, so popup transitions stay inside the inline component.
type Popup struct{ state popupState }

// Frame returns an immutable-by-convention snapshot that does not share the
// popup's item slice with the caller.
func (p *Popup) Frame() Frame {
	if p == nil {
		return Frame{}
	}
	return Frame{
		open: p.state.open, query: p.state.query, start: p.state.start, end: p.state.end,
		items: append([]Suggestion(nil), p.state.items...), selected: p.state.selected,
		bounds: p.state.bounds, dismissed: p.state.dismissed, dismissKey: p.state.dismissKey,
	}
}

func (f Frame) Open() bool              { return f.open }
func (f Frame) Query() string           { return f.query }
func (f Frame) SourceRange() (int, int) { return f.start, f.end }
func (f Frame) Items() []Suggestion     { return append([]Suggestion(nil), f.items...) }
func (f Frame) Selected() int           { return f.selected }

// DismissedFor reports whether the same source anchor and query remain
// dismissed, allowing the host to skip another vault lookup.
func (p *Popup) DismissedFor(query string, start int) bool {
	return p != nil && p.state.dismissed && p.state.dismissKey == query && p.state.start == start
}

// RefreshDismissedAnchor updates the caret end while preserving dismissal.
func (p *Popup) RefreshDismissedAnchor(query string, start, end int) {
	if p == nil || !p.DismissedFor(query, start) {
		return
	}
	p.state.query, p.state.start, p.state.end = query, start, end
}

type ActionKind uint8

const (
	ActionNone ActionKind = iota
	ActionSelectionChanged
	ActionAccept
	ActionDismiss
)

// Action is a typed request for the editor host. The UI package never edits
// source or calls back into an Editor instance.
type Action struct {
	Kind  ActionKind
	Index int
}

// Replace updates the detached popup rows while preserving a valid selection.
func (p *Popup) Replace(query string, start, end int, items []Suggestion) {
	if p == nil {
		return
	}
	s := &p.state
	old := *s
	if len(items) == 0 {
		p.Close()
		return
	}
	if old.dismissed && old.dismissKey == query && old.start == start {
		p.Close()
		s.query, s.start, s.end = query, start, end
		s.dismissed, s.dismissKey = old.dismissed, old.dismissKey
		return
	}
	selected := 0
	if old.query == query && old.start == start && old.selected >= 0 && old.selected < len(items) {
		selected = old.selected
	}
	*s = popupState{open: true, query: query, start: start, end: end, items: append([]Suggestion(nil), items...), selected: selected}
	if old.dismissed && old.dismissKey == query {
		s.dismissed, s.dismissKey = true, query
	}
}

// Close hides the popup without discarding the current dismissal key.
func (p *Popup) Close() {
	if p == nil {
		return
	}
	s := &p.state
	s.open = false
	s.items = nil
	s.bounds = ui.Rect{}
}

// Reset clears the popup and dismissal state for a new document/session.
func (p *Popup) Reset() {
	if p != nil {
		p.state = popupState{}
	}
}

// Dismiss hides the current query until the query or source location changes.
func (p *Popup) Dismiss() {
	if p == nil {
		return
	}
	s := &p.state
	s.dismissed = true
	s.dismissKey = s.query
	p.Close()
}

// HitTest maps a pointer into a suggestion row.
func (p *Popup) HitTest(x, y float32) (int, bool) {
	if p == nil {
		return 0, false
	}
	s := p.state
	if !s.open || !s.bounds.Contains(x, y) {
		return 0, false
	}
	const rowH = float32(30)
	i := int((y - s.bounds.Y - 5) / (rowH + 2))
	if i < 0 || i >= len(s.items) || y >= s.bounds.Y+5+float32(i+1)*(rowH+2)-2 {
		return 0, false
	}
	return i, true
}

// HandleKey applies popup-local navigation and returns actions for the host.
func (p *Popup) HandleKey(event input.KeyEvent) (Action, bool) {
	if p == nil {
		return Action{}, false
	}
	s := &p.state
	if !s.open || len(s.items) == 0 {
		return Action{}, false
	}
	key := event.Key
	if event.Modifiers&input.ModCtrl != 0 {
		switch key {
		case input.KeyN:
			key = input.KeyDown
		case input.KeyP:
			key = input.KeyUp
		}
	}
	switch key {
	case input.KeyUp:
		s.selected = (s.selected + len(s.items) - 1) % len(s.items)
		return Action{Kind: ActionSelectionChanged, Index: s.selected}, true
	case input.KeyDown:
		s.selected = (s.selected + 1) % len(s.items)
		return Action{Kind: ActionSelectionChanged, Index: s.selected}, true
	case input.KeyEnter, input.KeyTab:
		return Action{Kind: ActionAccept, Index: s.selected}, true
	case input.KeyEscape:
		p.Dismiss()
		return Action{Kind: ActionDismiss}, true
	default:
		return Action{}, false
	}
}

// SetBounds updates only the child-owned popup placement.
func (p *Popup) SetBounds(bounds ui.Rect) {
	if p != nil {
		p.state.bounds = bounds
	}
}

// ApplyAction applies popup-local presentation actions emitted by Render.
// Accept and dismiss remain host requests and do not mutate source here.
func (p *Popup) ApplyAction(action Action) {
	if p == nil || action.Kind != ActionSelectionChanged || action.Index < 0 || action.Index >= len(p.state.items) {
		return
	}
	p.state.selected = action.Index
}

// Bounds computes a popup rectangle from detached viewport/caret geometry.
func Bounds(viewport, caret ui.Rect, scrollY float32, itemCount int) ui.Rect {
	if itemCount <= 0 {
		return ui.Rect{}
	}
	const width = float32(340)
	const rowH = float32(30)
	height := rowH*float32(itemCount) + 2*float32(itemCount-1) + 10
	x := max(viewport.X+8, min(viewport.X+caret.X, viewport.X+viewport.W-width-8))
	y := viewport.Y + caret.Y - scrollY + caret.H + 6
	if y+height > viewport.Y+viewport.H-8 {
		y = viewport.Y + caret.Y - scrollY - height - 6
	}
	y = max(viewport.Y+8, min(y, viewport.Y+viewport.H-height-8))
	return ui.Rect{X: x, Y: y, W: width, H: height}
}
