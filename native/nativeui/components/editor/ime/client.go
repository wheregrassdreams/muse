package ime

import "github.com/egoist/mygo/ui"

// ClientHost is the narrow capability contract needed by the platform text
// input adapter. Source conversion and composition lifecycle stay in this
// package; the host owns edits and leaf-backed geometry.
type ClientHost interface {
	TextInputSnapshot() Snapshot
	ModalAcceptsIMEText() bool
	InsertIMEText(string)
	InsertModalIMEText(string)
	ReplaceIMEBytes(start, end int, text string)
	ReconvertIMEBytes(start, end int)
	DeleteIMESelection()
	CaretBoundsForIMEOffset(int) (ui.Rect, bool)
	CompositionCaretBounds() (ui.Rect, bool)
	IMESelectionCaretWidth() float32
	HitTestIME(ui.Point) int
	InvalidateIME()
	RevealIMECaret()
}

// Client implements MyGo's platform text-input contract over a narrow host.
// It owns UTF-16 conversion and composition transitions, but delegates source
// transactions and geometry to the editor host.
type Client struct {
	host        ClientHost
	index       Index
	composition Composition
}

func NewClient(host ClientHost) *Client {
	return &Client{host: host}
}

func (c *Client) controller() Controller {
	if c == nil {
		return Controller{}
	}
	return NewController(clientControllerHost{client: c}, &c.index, c.composition)
}

type clientControllerHost struct{ client *Client }

func (h clientControllerHost) TextInputSnapshot() Snapshot {
	if h.client == nil || h.client.host == nil {
		return Snapshot{}
	}
	return h.client.host.TextInputSnapshot()
}

func (h clientControllerHost) SetIMEComposition(composition Composition) {
	if h.client != nil {
		h.client.composition = composition
	}
}

func (c *Client) Composition() Composition {
	if c == nil {
		return Composition{}
	}
	return c.composition
}

func (c *Client) SetComposition(text string, selection Range) {
	if c == nil {
		return
	}
	state := NewState(&c.index, c.composition)
	state.SetComposition(text, selection)
	c.composition = state.Composition()
}

func (c *Client) Composing() bool { return c != nil && c.composition.Text != "" }

func (c *Client) Reset() {
	if c == nil {
		return
	}
	c.index.Reset()
	c.composition = Composition{}
}

func (c *Client) RebuildIndex(source string, revision uint64) {
	if c != nil {
		c.index.Rebuild(source, revision)
	}
}

func (c *Client) IndexEmpty() bool { return c == nil || c.index.Empty() }

func (c *Client) NoteEdit(from, to int, old, replacement, source string, revision uint64) {
	if c != nil {
		c.index.NoteEdit(from, to, old, replacement, source, revision)
	}
}

func (c *Client) TextForRange(r ui.TextInputRange) (string, ui.TextInputRange) {
	text, result := c.controller().TextForRange(Range{Start: r.Start, End: r.End})
	return text, ui.TextInputRange{Start: result.Start, End: result.End}
}

func (c *Client) Selection() ui.TextInputSelection {
	selection, _ := c.controller().Selection()
	return ui.TextInputSelection{
		Range:    ui.TextInputRange{Start: selection.Range.Start, End: selection.Range.End},
		Reversed: selection.Reversed,
	}
}

func (c *Client) MarkedRange() (ui.TextInputRange, bool) {
	r, ok := c.controller().MarkedRange()
	return ui.TextInputRange{Start: r.Start, End: r.End}, ok
}

func (c *Client) ReplaceText(r *ui.TextInputRange, text string) {
	if c == nil || c.host == nil {
		return
	}
	host := c.host
	if !host.ModalAcceptsIMEText() {
		host.InsertModalIMEText(text)
		return
	}
	defer host.InvalidateIME()
	controller := c.controller()
	state := controller.State()
	snapshot := host.TextInputSnapshot()
	switch {
	case r == nil && c.composition.Text != "":
		controller.ClearMarkedText()
	case r == nil:
		host.InsertIMEText(text)
		return
	default:
		a, _ := state.VirtualByte(snapshot, min(r.Start, r.End))
		z, _ := state.VirtualByte(snapshot, max(r.Start, r.End))
		controller.ClearMarkedText()
		host.ReplaceIMEBytes(a, z, text)
		return
	}
	host.InsertIMEText(text)
}

func (c *Client) SetMarkedText(r *ui.TextInputRange, text string, selected ui.TextInputRange) {
	if c == nil || c.host == nil || !c.host.ModalAcceptsIMEText() {
		return
	}
	host := c.host
	defer host.InvalidateIME()
	controller := c.controller()
	state := controller.State()
	snapshot := host.TextInputSnapshot()
	if r != nil && !state.Composing() {
		a, _ := state.VirtualByte(snapshot, min(r.Start, r.End))
		z, _ := state.VirtualByte(snapshot, max(r.Start, r.End))
		if a != z {
			host.ReconvertIMEBytes(a, z)
		}
	} else if !state.Composing() && snapshot.Selection.Range.Start != snapshot.Selection.Range.End {
		host.DeleteIMESelection()
	}
	controller.SetMarkedText(text, Range{Start: selected.Start, End: selected.End})
	host.RevealIMECaret()
}

func (c *Client) UnmarkText() {
	if c == nil || c.host == nil {
		return
	}
	host := c.host
	controller := c.controller()
	if !host.ModalAcceptsIMEText() {
		controller.ClearMarkedText()
		return
	}
	text := controller.ClearMarkedText()
	if text == "" {
		return
	}
	host.InsertIMEText(text)
	host.InvalidateIME()
}

func (c *Client) BoundsForRange(r ui.TextInputRange) (ui.Rect, ui.TextInputRange, bool) {
	if c == nil || c.host == nil {
		return ui.Rect{}, ui.TextInputRange{}, false
	}
	host := c.host
	snapshot := host.TextInputSnapshot()
	b, extra := c.controller().VirtualByte(min(r.Start, r.End))
	var rect ui.Rect
	var ok bool
	if extra > 0 || c.composition.Text != "" && b == snapshot.Caret {
		rect, ok = host.CompositionCaretBounds()
	}
	if !ok {
		rect, ok = host.CaretBoundsForIMEOffset(b)
	}
	if !ok {
		return ui.Rect{}, ui.TextInputRange{}, false
	}
	actual := ui.TextInputRange{Start: r.Start, End: r.Start}
	if r.End > r.Start {
		rect.W = max(rect.W, host.IMESelectionCaretWidth())
		actual.End = r.Start + 1
	}
	return rect, actual, true
}

func (c *Client) IndexForPoint(point ui.Point) (int, bool) {
	if c == nil || c.host == nil {
		return 0, false
	}
	offset := c.host.HitTestIME(point)
	return c.controller().VirtualUnits(offset), true
}
