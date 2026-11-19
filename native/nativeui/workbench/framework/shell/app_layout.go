package shell

import (
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"muse/nativeui/workbench/framework/shell/windowcontrols"
	appearance "muse/theme"
)

// Surface identifies the workbench surface currently hosted in the context
// panel. Feature policy remains with the provider of the surface renderer.
type Surface uint8

const (
	SurfaceFiles Surface = iota
	SurfaceOutline
	SurfaceSearch
	SurfaceAgent
)

type Frame struct {
	PanelOpen        bool
	PanelWidth       float32
	PanelMinWidth    float32
	PanelMaxWidth    float32
	PanelFoldAt      float32
	PanelOpenAt      float32
	PanelResizeTop   float32
	FlatCanvas       bool
	Surface          Surface
	Background       ui.Color
	Vibrant          bool
	HideMouseCursor  bool
	CursorUntil      time.Time
	Window           *mygo.Window
	WindowButtonsTop float32
	PanelToggleLabel string
	PanelToggleIcon  *ui.SVG
	ThemeMode        uint8
	AccentID         string
}

type ActionKind uint8

const (
	PanelGeometryChanged ActionKind = iota + 1
	SelectSurface
	TogglePanel
	OpenSettings
)

type Action struct {
	Kind       ActionKind
	PanelOpen  bool
	PanelWidth float32
	Surface    Surface
}

// LayoutPorts are the complete shell-to-application boundary for composing a
// frame. The shell positions surfaces and owns the panel resize gesture; the
// root supplies surface renderers and applies resulting geometry actions.
type LayoutPorts struct {
	Frame           func() Frame
	RenderPanel     func(*ui.Context, Surface)
	RenderCanvas    func(*ui.Context, bool)
	RenderInspector func(*ui.Context)
	RenderOverlays  func(*ui.Context)
	PanelWidth      func(float32)
	ObserveInput    func(ui.InputEvent)
	Dispatch        func(Action)
}

type Layout struct {
	ports             LayoutPorts
	dragging          bool
	dragWidth         float32
	awaitingFinalEdge bool
	panelPrevTarget   float32
	panelPrevOpen     bool
	panelOpenKnown    bool
	panelTransition   bool
	panelDirect       bool
	gripHeld          bool
	dragOpen          bool
	themeMode         uint8
	accentID          string
	themeReady        bool
}

func NewLayout(ports LayoutPorts) *Layout { return &Layout{ports: ports} }

func (l *Layout) Render(c *ui.Context) {
	if l == nil || c == nil || l.ports.Frame == nil {
		return
	}
	frame := l.ports.Frame()
	l.applyTheme(c, frame)
	row := ui.Row(c).Fill()
	if frame.Vibrant || c.Vibrancy() {
		c.Root().Background(ui.Transparent)
	} else if frame.Background != (ui.Color{}) {
		row.Background(frame.Background)
	} else {
		row.Background(windowBackground(c.Theme().Dark))
	}
	if l.ports.ObserveInput != nil {
		row.ObserveInput(l.ports.ObserveInput)
	}
	if frame.HideMouseCursor {
		row.Cursor(ui.CursorNone)
		if remaining := frame.CursorUntil.Sub(c.Now()); remaining > 0 {
			c.After(remaining)
		}
	}
	row.Children(func() {
		panelWidth := l.renderPanel(c, frame)
		l.renderCanvas(c, frame, panelWidth)
		if l.ports.RenderInspector != nil {
			l.ports.RenderInspector(c)
		}
	})
	l.renderPanelToggle(c, frame)
	if l.ports.RenderOverlays != nil {
		l.ports.RenderOverlays(c)
	}
	if frame.Window != nil && windowcontrols.Visible() {
		top := frame.WindowButtonsTop
		if top <= 0 {
			top = 18
		}
		windowcontrols.Render(c, windowcontrols.Model{
			Top: top, Maximized: frame.Window.IsMaximized(),
			Minimize: frame.Window.Minimize, Maximize: frame.Window.Maximize,
			Restore: frame.Window.Unmaximize, Close: frame.Window.Close,
		})
	}
}

func (l *Layout) renderPanel(c *ui.Context, frame Frame) float32 {
	target := float32(0)
	width := frame.PanelWidth
	if width <= 0 {
		width = 268
	}
	if frame.PanelOpen {
		target = width
	}
	if l.panelOpenKnown && l.panelPrevOpen != frame.PanelOpen {
		l.panelTransition = true
		l.panelDirect = false
	}
	anim := ui.Animation{Duration: 180 * time.Millisecond, Ease: ui.EaseOut}
	if !l.panelTransition && frame.PanelOpen && l.panelPrevTarget > 0.5 && l.panelPrevTarget != target {
		anim.Duration = 0
	}
	panel := ui.Column(c).Key("shell-context-panel").Shrink(0).FillHeight().Clip()
	if l.panelDirect && !l.panelTransition {
		panel.Jump("width", target)
	}
	visible := panel.AnimateTo("width", target, anim)
	if l.panelTransition && panel.Settled("width") {
		l.panelTransition = false
	}
	if l.panelDirect && !l.dragging {
		l.panelDirect = false
	}
	l.panelPrevTarget, l.panelPrevOpen, l.panelOpenKnown = target, frame.PanelOpen, true
	if l.ports.PanelWidth != nil {
		l.ports.PanelWidth(visible)
	}
	panel.Width(visible).Background(c.Theme().Background.Alpha(0.8))
	if visible > 0.5 {
		panel.Children(func() {
			ui.Column(c).Width(width).FillHeight().Opacity(min(visible/width, 1)).Children(func() {
				if l.ports.RenderPanel != nil {
					l.ports.RenderPanel(c, frame.Surface)
				}
			})
			if !frame.FlatCanvas {
				l.resizeHandle(c, frame, width-12)
			}
		})
	}
	return visible
}

func (l *Layout) renderCanvas(c *ui.Context, frame Frame, panelWidth float32) {
	if l.ports.RenderCanvas == nil {
		return
	}
	theme := c.Theme()
	left := float32(0)
	padRight, padBottom := float32(0), float32(0)
	if !frame.FlatCanvas {
		left, padRight, padBottom = 8, 8, 8
		left += max(c.TitleBar().Left, 0) * (1 - clamp(panelWidth/positive(frame.PanelWidth, 268), 0, 1))
	}
	padded := ui.Row(c).Grow(1).FillHeight().MinWidth(0).Padding(padRight, padBottom, padBottom, left)
	padded.Children(func() {
		if frame.FlatCanvas {
			ui.Column(c).Key("shell-editor-canvas").Fill().Clip().Background(windowBackground(theme.Dark)).Children(func() {
				l.ports.RenderCanvas(c, true)
				l.resizeHandle(c, frame, 0)
			})
			return
		}
		card := ui.Column(c).Key("shell-editor-card").Fill().Radius(14).Clip().Background(theme.Background)
		if theme.Dark {
			card.Border(1, ui.RGBA(236, 236, 240, 0.12)).
				Shadow(0, 1, 1, 0, ui.RGBA(0, 0, 0, 0.2)).
				Shadow(0, 4, 6, -3, ui.RGBA(0, 0, 0, 0.24))
		} else {
			card.Border(1, ui.RGBA(0, 0, 0, 0.06)).
				Shadow(0, 1, 2, 0, ui.RGBA(0, 0, 0, 0.07)).
				Shadow(0, 1, 6, 0, ui.RGBA(0, 0, 0, 0.035))
		}
		card.Children(func() {
			l.ports.RenderCanvas(c, false)
			if !frame.PanelOpen {
				l.resizeHandle(c, frame, 0)
			}
		})
	})
}

func (l *Layout) renderPanelToggle(c *ui.Context, frame Frame) {
	icon, label := frame.PanelToggleIcon, frame.PanelToggleLabel
	if label == "" || icon == nil {
		return
	}
	x := float32(6)
	if bar := c.TitleBar(); bar.Left+6 > x {
		x = bar.Left + 6
	}
	top := float32(18)
	if frame.FlatCanvas {
		top = max(c.TitleBar().Height, 44)/2 - 13
	}
	button := ui.ButtonBase(c).Key("shell-panel-toggle").Absolute().Left(x).Top(top).
		Size(26, 26).Padding(0).Radius(6).Label(label).Tooltip(label)
	if button.Hovered() {
		button.Background(c.Theme().SurfaceHover)
	}
	button.OnClick(func() {
		if l.ports.Dispatch != nil {
			l.ports.Dispatch(Action{Kind: TogglePanel, PanelOpen: !frame.PanelOpen, PanelWidth: frame.PanelWidth, Surface: frame.Surface})
		}
	})
	button.Children(func() { ui.Icon(c, icon).Size(16, 16).TextColor(c.Theme().TextMuted) })
}

func (l *Layout) applyTheme(c *ui.Context, frame Frame) {
	systemDark := c.Theme().Dark
	dark := frame.ThemeMode == 2 || frame.ThemeMode == 0 && systemDark
	if !l.themeReady || l.themeMode != frame.ThemeMode || l.accentID != frame.AccentID {
		l.themeReady, l.themeMode, l.accentID = true, frame.ThemeMode, frame.AccentID
		switch frame.ThemeMode {
		case 1:
			if mygo.App.IsReady() {
				mygo.Theme.SetSource(mygo.ThemeLight)
			}
		case 2:
			if mygo.App.IsReady() {
				mygo.Theme.SetSource(mygo.ThemeDark)
			}
		default:
			if mygo.App.IsReady() {
				mygo.Theme.SetSource(mygo.ThemeSystem)
			}
		}
	}
	t := *c.Theme()
	if dark {
		t.Dark = true
		t.Background, t.Surface = ui.Hex("#232328"), ui.Hex("#2a2a30")
		t.SurfaceHover, t.SurfacePressed = ui.RGBA(255, 255, 255, 0.07), ui.RGBA(255, 255, 255, 0.12)
		t.Border, t.Text, t.TextMuted = ui.RGBA(255, 255, 255, 0.07), ui.Hex("#ececf0"), ui.Hex("#a9a9b4")
	} else {
		t.Dark = false
		t.Background, t.Surface = ui.Hex("#ffffff"), ui.Hex("#f3f3f6")
		t.SurfaceHover, t.SurfacePressed = ui.RGBA(0, 0, 0, 0.045), ui.RGBA(0, 0, 0, 0.08)
		t.Border, t.Text, t.TextMuted = ui.RGBA(0, 0, 0, 0.07), ui.Hex("#1d1d1f"), ui.Hex("#6e6e78")
	}
	p := appearance.From(appearance.Color(frame.AccentID), dark)
	t.Accent, t.AccentHover, t.AccentPressed = p.Accent, p.Hover, p.Pressed
	t.AccentText, t.Selection, t.Focus = p.OnAccent, p.Selection, p.Accent
	t.Radius, t.FontSize = 7, 13
	c.SetTheme(&t)
}

func windowBackground(dark bool) ui.Color {
	if dark {
		return ui.Hex("#1a1a1a")
	}
	return ui.Hex("#ffffff")
}

func positive(value, fallback float32) float32 {
	if value > 0 {
		return value
	}
	return fallback
}

func clamp(value, minValue, maxValue float32) float32 {
	return min(max(value, minValue), maxValue)
}

func (l *Layout) resizeHandle(c *ui.Context, frame Frame, left float32) {
	handle := ui.Box(c).Key("shell-panel-resize").Absolute().Left(left).
		Top(max(frame.PanelResizeTop, 0)).Bottom(0).Width(12).
		Cursor(ui.CursorResizeEW).Label("调整面板宽度").Column().AlignItems(ui.Center)
	_, _, pointerOver := handle.PointerPosition()
	hovered := handle.Hovered() || pointerOver
	held := handle.Pressed() || handle.Dragging() || l.dragging
	if dx, _, dragged := handle.Dragged(); dragged {
		held = true
		if !l.dragging {
			l.dragging = true
			l.panelDirect = true
			l.dragWidth = 0
			l.dragOpen = frame.PanelOpen
			if frame.PanelOpen {
				l.dragWidth = frame.PanelWidth
			}
		}
		if l.awaitingFinalEdge {
			px, _, _ := handle.PointerPosition()
			if handle.Bounds().X+px >= frame.PanelWidth {
				l.awaitingFinalEdge = false
				l.dragWidth = frame.PanelWidth
				l.dragOpen = true
				l.panelTransition = false
				l.panelDirect = true
			}
			dx = 0
		}
		if !l.awaitingFinalEdge {
			l.dragWidth += dx
			minWidth, maxWidth := frame.PanelMinWidth, frame.PanelMaxWidth
			if minWidth <= 0 {
				minWidth = 224
			}
			if maxWidth < minWidth {
				maxWidth = 440
			}
			foldAt, openAt := frame.PanelFoldAt, frame.PanelOpenAt
			if foldAt <= 0 {
				foldAt = 200
			}
			if openAt <= 0 {
				openAt = 36
			}
			if l.dragOpen && l.dragWidth < foldAt {
				l.dragOpen, l.dragWidth = false, 0
			} else if !l.dragOpen && l.dragWidth >= openAt {
				l.dragOpen, l.awaitingFinalEdge, l.dragWidth = true, true, frame.PanelWidth
				l.panelTransition, l.panelDirect = true, false
			}
			if l.dragOpen {
				width := min(max(l.dragWidth, minWidth), maxWidth)
				if l.ports.Dispatch != nil {
					l.ports.Dispatch(Action{Kind: PanelGeometryChanged, PanelOpen: l.dragOpen, PanelWidth: width, Surface: frame.Surface})
				}
			} else if l.ports.Dispatch != nil {
				l.ports.Dispatch(Action{Kind: PanelGeometryChanged, PanelOpen: false, PanelWidth: frame.PanelWidth, Surface: frame.Surface})
			}
		}
	} else if l.dragging {
		l.dragging, l.awaitingFinalEdge = false, false
		l.dragWidth = 0
		l.dragOpen = false
		if !l.panelTransition {
			l.panelDirect = false
		}
	}
	if handle.DoubleClicked() && l.ports.Dispatch != nil {
		width := frame.PanelWidth
		if width <= 0 {
			width = 248
		}
		l.ports.Dispatch(Action{Kind: PanelGeometryChanged, PanelOpen: true, PanelWidth: width, Surface: frame.Surface})
	}
	handle.Children(func() {
		ui.Box(c).Grow(0.9)
		gripHeight, gripWidth, opacity := float32(8), float32(2), float32(0)
		color := c.Theme().TextMuted
		if held {
			gripHeight, gripWidth, opacity, color = 56, 4, 0.85, c.Theme().Text
		} else if hovered {
			gripHeight, gripWidth, opacity = 40, 3, 0.5
		}
		grip := ui.Box(c).Key("shell-panel-grip").Label("侧栏把手").Radius(2).Background(color.Alpha(opacity))
		if held {
			grip.Width(grip.Jump("width", gripWidth)).Height(grip.Jump("height", gripHeight))
		} else if l.gripHeld {
			motion := ui.Animation{Spring: ui.SpringOptions{Duration: 220 * time.Millisecond, Bounce: 0.16}}
			grip.Width(grip.AnimateTo("width", gripWidth, motion)).Height(grip.AnimateTo("height", gripHeight, motion))
			if grip.Settled("width") && grip.Settled("height") {
				l.gripHeld = false
			}
		} else {
			grip.Width(grip.Jump("width", gripWidth)).Height(grip.Jump("height", gripHeight))
		}
		if held {
			l.gripHeld = true
		}
		ui.Box(c).Grow(1.1)
	})
}
