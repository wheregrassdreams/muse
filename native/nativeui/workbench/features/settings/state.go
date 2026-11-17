package settings

import "github.com/egoist/mygo/ui"

// state owns the settings panel's transient navigation and window state.
// Persisted preferences are supplied as a ViewModel and remain application
// data rather than panel state.
type state struct {
	open   bool
	page   Page
	drag   ui.Point
	scroll ui.ScrollState
}

// Handle is the opaque presentation state retained by the composition root.
// Persisted preferences remain application data supplied as a ViewModel.
type Handle struct{ state *state }

func NewHandle() Handle { return Handle{state: &state{page: AppearancePage}} }

func (h *Handle) ensureState() *state {
	if h.state == nil {
		h.state = &state{page: AppearancePage}
	}
	return h.state
}

func (h *Handle) OpenAt(page Page) {
	s := h.ensureState()
	s.open = true
	s.page = page
	s.drag = ui.Point{}
}

func (h *Handle) Close() { h.ensureState().open = false }
func (h Handle) IsOpen() bool {
	return h.state != nil && h.state.open
}
func (h Handle) Page() Page {
	if h.state == nil {
		return AppearancePage
	}
	return h.state.page
}
func (h *Handle) SetPage(page Page)         { h.ensureState().page = page }
func (h *Handle) SetScrollOffset(y float32) { h.ensureState().scroll.Y = y }
func (h Handle) ScrollOffset() float32 {
	if h.state == nil {
		return 0
	}
	return h.state.scroll.Y
}

func (h *Handle) Apply(action Action) {
	s := h.ensureState()
	switch action.Kind {
	case SelectPage:
		s.page = action.Page
	case Close:
		s.open = false
	}
}

// RenderDialog owns the dialog's local drag and scroll behavior while the
// caller supplies current persisted settings and handles preference actions.
func (h *Handle) RenderDialog(c *ui.Context, model ViewModel, dispatch func(Action)) {
	if h == nil || h.state == nil || !h.state.open {
		return
	}
	w, height := c.Size()
	panelW := min(820, w-64)
	panelH := min(620, height-64)
	maxX := (w-panelW)/2 - 12
	maxY := (height-panelH)/2 - 12
	bounds := ui.Rect{X: -maxX, Y: -maxY, W: 2 * maxX, H: 2 * maxY}
	ui.DialogBase(c, &h.state.open, func(backdrop, panel ui.Element) {
		backdrop.Background(ui.Transparent)
		panel.Width(panelW).Height(panelH).Radius(14).Clip().
			Background(c.Theme().Surface).Border(1, c.Theme().Border).
			Shadow(0, 18, 50, 0, ui.RGBA(0, 0, 0, 0.28)).
			Draggable(&h.state.drag, ui.DraggableOptions{Bounds: &bounds, Rigid: true}).
			Children(func() { Render(c, h, model, dispatch) })
	})
}
