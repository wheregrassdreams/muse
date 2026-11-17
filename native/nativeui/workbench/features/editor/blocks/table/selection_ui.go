package tableui

import (
	"image/color"
	"time"

	"github.com/egoist/mygo/ui"
	"muse/editor/blocks/table"
	"muse/locales"
	tabletheme "muse/theme"
)

type tableHandleKey struct {
	scopeID   uint64
	lineStart int
	axis      table.Axis
	index     int
}

// SelectionHandles draws row and column selection affordances from an
// immutable frame. Pointer gestures are translated to typed actions; all
// editor/session state remains with the Host.
func (c Component) SelectionHandles(ctx *ui.Context, viewport ui.Rect, pointer ui.Point, frame Frame, textSize float32, accent ui.Color, dark bool) {
	if c.host == nil {
		return
	}
	for _, item := range frame.Tables {
		if len(item.Rows) < 2 || len(item.Columns) < 2 {
			continue
		}
		if frame.Drag != nil {
			if frame.Drag.LineStart == item.LineStart {
				c.paintHandleDrag(ctx, viewport, item, *frame.Drag, frame.ScrollY, frame.ScopeID, dark)
			}
			continue
		}
		for row := 0; row+1 < len(item.Rows); row++ {
			if item.HorizontalOffset > 0 {
				continue // the table's true left edge is outside its viewport
			}
			r := ui.Rect{
				X: viewport.X + item.Bounds.X - 18,
				Y: viewport.Y + item.Bounds.Y + (item.Rows[row]+item.Rows[row+1])/2 - textSize*1.15/2 - frame.ScrollY,
				W: 14, H: textSize * 1.15,
			}
			if visibleHandle(r, viewport) {
				c.selectionHandle(ctx, viewport, pointer, frame, item, table.Rows, row, r, locales.EditorSelectRow(row+1), textSize, accent, dark)
			}
		}
		for col := 0; col+1 < len(item.Columns); col++ {
			left := max(item.Bounds.X, item.Bounds.X-item.HorizontalOffset+item.Columns[col])
			right := min(item.Bounds.X+item.Viewport.W, item.Bounds.X-item.HorizontalOffset+item.Columns[col+1])
			if right <= left {
				continue
			}
			r := ui.Rect{
				X: viewport.X + (left+right)/2 - 10,
				Y: viewport.Y + item.Bounds.Y - 18 - frame.ScrollY,
				W: 20, H: 14,
			}
			if visibleHandle(r, viewport) {
				c.selectionHandle(ctx, viewport, pointer, frame, item, table.Columns, col, r, locales.EditorSelectColumn(col+1), textSize, accent, dark)
			}
		}
	}
}

func (c Component) selectionHandle(ctx *ui.Context, viewport ui.Rect, pointer ui.Point, frame Frame, item Table, axis table.Axis, index int, bounds ui.Rect, label string, textSize float32, accent ui.Color, dark bool) {
	ui.Overlay(ctx, func() {
		el := ui.Box(ctx.Key(tableHandleKey{frame.ScopeID, item.LineStart, axis, index})).Absolute().Left(bounds.X).Top(bounds.Y).Width(bounds.W).Height(bounds.H).
			Label(label).Cursor(ui.CursorPointer).
			HandleInput(func(event ui.InputEvent) bool {
				switch event.Kind {
				case ui.InputScroll:
					return c.host.MenuInput(item.LineStart, bounds, viewport, event)
				case ui.InputPointerDown:
					if event.Button != 0 {
						return false
					}
					_, _ = c.Dispatch(Action{
						Kind: BeginHandleDrag, LineStart: item.LineStart, Axis: axis,
						HandleIndex: index, Extend: event.Mods&ui.Shift != 0,
						Pointer: pointerForEvent(bounds, viewport, frame.ScrollY, event),
					})
					c.host.Focus()
					return true
				case ui.InputPointerMove:
					if event.Button == 0 && frame.Drag != nil {
						_, _ = c.Dispatch(Action{Kind: MoveHandleDrag, LineStart: item.LineStart, Pointer: pointerForEvent(bounds, viewport, frame.ScrollY, event)})
						return true
					}
				case ui.InputPointerUp:
					if frame.Drag != nil {
						if !event.Canceled {
							_, _ = c.Dispatch(Action{Kind: MoveHandleDrag, LineStart: item.LineStart, Pointer: pointerForEvent(bounds, viewport, frame.ScrollY, event)})
						}
						_, _ = c.Dispatch(Action{Kind: FinishHandleDrag, LineStart: item.LineStart, Canceled: event.Canceled})
						return true
					}
				}
				return false
			})
		_, _, over := el.PointerPosition()
		hoverRow, hoverCol := HoverAxes(item, pointer)
		hot := axis == table.Rows && hoverRow == index || axis == table.Columns && hoverCol == index
		if over || hot {
			ctx.After(40 * time.Millisecond)
			el.Draw(func(p *ui.Painter, r ui.Rect) {
				colors := tabletheme.MarkdownColors(dark)
				bg := colorFromTheme(colors.TableControlBackground)
				ink := colorFromTheme(colors.TableControlInk)
				accent = accent.Alpha(0.5)
				cx, cy := r.X+r.W/2, r.Y+r.H/2
				if axis == table.Rows {
					p.Fill(ui.Rect{X: viewport.X + item.Bounds.X - 1.5, Y: cy - 9, W: 3, H: 18}, accent, 1.5)
					if over {
						p.Fill(ui.Rect{X: cx - 5, Y: cy - 12, W: 10, H: 24}, bg, 5)
						for _, dy := range []float32{-5, 0, 5} {
							p.Fill(ui.Rect{X: cx - 1, Y: cy + dy - 1, W: 2, H: 2}, ink, 1)
						}
					}
				} else {
					p.Fill(ui.Rect{X: cx - 10, Y: viewport.Y + item.Bounds.Y - frame.ScrollY - 1.5, W: 20, H: 3}, accent, 1.5)
					if over {
						p.Fill(ui.Rect{X: cx - 12, Y: cy - 5, W: 24, H: 10}, bg, 5)
						for _, dx := range []float32{-5, 0, 5} {
							p.Fill(ui.Rect{X: cx + dx - 1, Y: cy - 1, W: 2, H: 2}, ink, 1)
						}
					}
				}
			})
		}
	})
}

func pointerForEvent(bounds, viewport ui.Rect, scrollY float32, event ui.InputEvent) Point {
	return Point{X: bounds.X + event.X - viewport.X, Y: bounds.Y + event.Y - viewport.Y + scrollY}
}

func visibleHandle(r, viewport ui.Rect) bool {
	return r.W > 0 && r.H > 0 && r.X+r.W > viewport.X && r.X < viewport.X+viewport.W && r.Y+r.H > viewport.Y && r.Y < viewport.Y+viewport.H
}

func (c Component) paintHandleDrag(ctx *ui.Context, viewport ui.Rect, item Table, drag DragPreview, scrollY float32, scopeID uint64, dark bool) {
	x, y := viewport.X+item.Bounds.X, viewport.Y+item.Bounds.Y-scrollY
	colors := tabletheme.MarkdownColors(dark)
	background := colorFromTheme(colors.TableControlBackground).Alpha(0.7)
	ink := colorFromTheme(colors.TableControlInk)
	ctx.After(16 * time.Millisecond)
	ui.Overlay(ctx, func() {
		ui.Box(ctx.Key(tableHandleKey{scopeID, item.LineStart, drag.Axis, drag.Index})).Absolute().Left(viewport.X).Top(viewport.Y).Width(viewport.W).Height(viewport.H).
			HandleInput(func(event ui.InputEvent) bool {
				switch event.Kind {
				case ui.InputPointerMove:
					if event.Button == 0 {
						_, _ = c.Dispatch(Action{Kind: MoveHandleDrag, LineStart: item.LineStart, Pointer: Point{X: event.X, Y: event.Y + scrollY}})
						return true
					}
				case ui.InputPointerUp:
					if event.Button == 0 {
						if !event.Canceled {
							_, _ = c.Dispatch(Action{Kind: MoveHandleDrag, LineStart: item.LineStart, Pointer: Point{X: event.X, Y: event.Y + scrollY}})
						}
						_, _ = c.Dispatch(Action{Kind: FinishHandleDrag, LineStart: item.LineStart, Canceled: event.Canceled})
						return true
					}
				}
				return false
			}).Draw(func(p *ui.Painter, _ ui.Rect) {
			p.Clip(viewport, 0, func() {
				var thumb ui.Rect
				if drag.Axis == table.Rows {
					rail := ui.Rect{X: x - 17, Y: y, W: 14, H: item.Bounds.H}
					p.Fill(rail, background, 7)
					cy := max(y+14, min(viewport.Y+drag.Position-scrollY, y+item.Bounds.H-14))
					thumb = ui.Rect{X: rail.X + 1, Y: cy - 14, W: 12, H: 28}
				} else {
					rail := ui.Rect{X: x, Y: y - 17, W: item.Viewport.W, H: 14}
					p.Fill(rail, background, 7)
					cx := max(x+14, min(viewport.X+drag.Position, x+item.Viewport.W-14))
					thumb = ui.Rect{X: cx - 14, Y: rail.Y + 1, W: 28, H: 12}
				}
				p.Fill(thumb, ink, 6)
				for _, shift := range []float32{-4, 0, 4} {
					dot := ui.Rect{X: thumb.X + thumb.W/2 - 1, Y: thumb.Y + thumb.H/2 - 1, W: 2, H: 2}
					if drag.Axis == table.Rows {
						dot.Y += shift
					} else {
						dot.X += shift
					}
					p.Fill(dot, p.Theme().AccentText, 1)
				}
				if drag.Moving && (drag.Gap < drag.First || drag.Gap > drag.Last+1) {
					clip := ui.Rect{X: x - 2, Y: y - 2, W: item.Viewport.W + 4, H: item.Bounds.H + 4}
					p.Clip(clip, 0, func() {
						if drag.Axis == table.Rows && drag.Gap >= 0 && drag.Gap < len(item.Rows) {
							p.Fill(ui.Rect{X: x - 1, Y: y + item.Rows[drag.Gap] - 2, W: item.Viewport.W + 2, H: 4}, ink, 2)
						} else if drag.Axis == table.Columns && drag.Gap >= 0 && drag.Gap < len(item.Columns) {
							p.Fill(ui.Rect{X: x + item.Columns[drag.Gap] - item.HorizontalOffset - 2, Y: y - 1, W: 4, H: item.Bounds.H + 2}, ink, 2)
						}
					})
				}
			})
		})
	})
}

func colorFromTheme(c color.RGBA) ui.Color {
	return ui.RGBA(c.R, c.G, c.B, float32(c.A)/255)
}
