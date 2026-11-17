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
type Handle struct{ state }

func NewHandle() Handle { return Handle{state: state{page: AppearancePage}} }

func (s *state) OpenAt(page Page) {
	s.open = true
	s.page = page
	s.drag = ui.Point{}
}

func (s *state) Close()                    { s.open = false }
func (s state) IsOpen() bool               { return s.open }
func (s state) Page() Page                 { return s.page }
func (s *state) SetPage(page Page)         { s.page = page }
func (s *state) SetScrollOffset(y float32) { s.scroll.Y = y }
func (s state) ScrollOffset() float32      { return s.scroll.Y }

func (s *state) Apply(action Action) {
	switch action.Kind {
	case SelectPage:
		s.page = action.Page
	case Close:
		s.Close()
	}
}

// RenderDialog owns the dialog's local drag and scroll behavior while the
// caller supplies current persisted settings and handles preference actions.
func (h *Handle) RenderDialog(c *ui.Context, model ViewModel, dispatch func(Action)) {
	if h == nil || !h.open {
		return
	}
	w, height := c.Size()
	panelW := min(820, w-64)
	panelH := min(620, height-64)
	maxX := (w-panelW)/2 - 12
	maxY := (height-panelH)/2 - 12
	bounds := ui.Rect{X: -maxX, Y: -maxY, W: 2 * maxX, H: 2 * maxY}
	ui.DialogBase(c, &h.open, func(backdrop, panel ui.Element) {
		backdrop.Background(ui.Transparent)
		panel.Width(panelW).Height(panelH).Radius(14).Clip().
			Background(c.Theme().Surface).Border(1, c.Theme().Border).
			Shadow(0, 18, 50, 0, ui.RGBA(0, 0, 0, 0.28)).
			Draggable(&h.drag, ui.DraggableOptions{Bounds: &bounds, Rigid: true}).
			Children(func() { Render(c, h, model, dispatch) })
	})
}
