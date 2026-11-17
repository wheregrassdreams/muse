package ime

import "github.com/egoist/mygo/ui"

// TextSession is the source and edit capability needed by the platform text
// input adapter. It exposes detached source state and explicit edit actions,
// never the editor or its mutable model.
type TextSession interface {
	TextInputSnapshot() Snapshot
	ModalAcceptsIMEText() bool
	InsertIMEText(string)
	InsertModalIMEText(string)
	ReplaceIMEBytes(start, end int, text string)
	ReconvertIMEBytes(start, end int)
	DeleteIMESelection()
}

// GeometryHost supplies the leaf-backed geometry and focus lifecycle needed
// by the MyGo text-input adapter.
type GeometryHost interface {
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
	session     TextSession
	geometry    GeometryHost
	index       Index
	composition Composition
}

func NewClient(session TextSession, geometry GeometryHost) *Client {
	return &Client{session: session, geometry: geometry}
}

func (c *Client) controller() Controller {
	if c == nil {
		return Controller{}
	}
	return NewController(clientControllerHost{client: c}, &c.index, c.composition)
}

type clientControllerHost struct{ client *Client }

func (h clientControllerHost) TextInputSnapshot() Snapshot {
	if h.client == nil || h.client.session == nil {
		return Snapshot{}
	}
	return h.client.session.TextInputSnapshot()
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
	if c == nil || c.session == nil {
		return
	}
	host := c.session
	if !host.ModalAcceptsIMEText() {
		host.InsertModalIMEText(text)
		return
	}
	if c.geometry != nil {
		defer c.geometry.InvalidateIME()
	}
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
	if c == nil || c.session == nil || !c.session.ModalAcceptsIMEText() {
		return
	}
	host := c.session
	geometry := c.geometry
	if geometry != nil {
		defer geometry.InvalidateIME()
	}
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
	if geometry != nil {
		geometry.RevealIMECaret()
	}
}

func (c *Client) UnmarkText() {
	if c == nil || c.session == nil {
		return
	}
	host := c.session
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
	if c.geometry != nil {
		c.geometry.InvalidateIME()
	}
}

func (c *Client) BoundsForRange(r ui.TextInputRange) (ui.Rect, ui.TextInputRange, bool) {
	if c == nil || c.session == nil || c.geometry == nil {
		return ui.Rect{}, ui.TextInputRange{}, false
	}
	session, host := c.session, c.geometry
	snapshot := session.TextInputSnapshot()
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
	if c == nil || c.geometry == nil {
		return 0, false
	}
	offset := c.geometry.HitTestIME(point)
	return c.controller().VirtualUnits(offset), true
}
