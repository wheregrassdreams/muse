package editor

import (
	"time"

	"github.com/egoist/mygo/ui"
	helix "github.com/wheregrassdreams/muse-editor/text/helix"
	"github.com/wheregrassdreams/muse-editor/text/layout"
	media "muse/nativeui/workbench/features/editor/blocks/media"
)

func (e *Editor) clearVimImageRow() { e.vimImageRows.Clear() }

func (e *Editor) vimImageRowState() (media.VirtualImageRow, bool) { return e.vimImageRows.State() }

func (e *Editor) vimImageRowsActive() bool {
	return e.opts.VimVirtualImageRows && e.Keymap() == "vim" && e.vimController().Mode() != helix.ModeInsert
}

// vimImageGeometry returns the painted image rectangle in document
// coordinates and its number of virtual rows. Only standalone image leaves
// participate; inline images keep their ordinary text-line navigation.
func (e *Editor) vimImageGeometry(l *leaf) (ui.Rect, int, bool) {
	if !e.vimImageRowsActive() || l == nil || !e.pictureBlock(l) {
		return ui.Rect{}, 0, false
	}
	v := e.unitView(l, l.lineStart)
	if v == nil || v.layout == nil {
		return ui.Rect{}, 0, false
	}
	for _, object := range v.layout.Objects() {
		for _, piece := range v.disp.pieces {
			if piece.obj == nil || !piece.obj.block || piece.obj.imagePath == "" || piece.d0 != object.Index {
				continue
			}
			lineHeight := max(e.lk.size*e.lk.lh, 1)
			rows := layout.VirtualRows(object.Rect.H, lineHeight)
			if rows < 2 {
				return ui.Rect{}, 0, false
			}
			return ui.Rect{
				X: unitTextX(l, v) + object.Rect.X,
				Y: l.y + v.ty + object.Rect.Y,
				W: object.Rect.W,
				H: object.Rect.H,
			}, rows, true
		}
	}
	return ui.Rect{}, 0, false
}

func (e *Editor) vimImageRowCaretRect(l *leaf) (ui.Rect, bool) {
	state, active := e.vimImageRows.State()
	if !active || l == nil || l.lineStart != state.LineStart {
		return ui.Rect{}, false
	}
	box, rows, ok := e.vimImageGeometry(l)
	if !ok {
		e.clearVimImageRow()
		return ui.Rect{}, false
	}
	lineHeight := max(e.lk.size*e.lk.lh, 1)
	_, offset, height, ok := e.vimImageRows.CaretY(rows, lineHeight)
	if !ok {
		return ui.Rect{}, false
	}
	y := box.Y + offset
	return ui.Rect{X: box.X, Y: y, W: 1, H: min(height, box.Y+box.H-y)}, true
}

// moveVimImageRow consumes vertical movement while the virtual caret is
// inside a tall image. Crossing either image edge falls through to ordinary
// document navigation.
func (e *Editor) moveVimImageRow(dir int, extend bool) bool {
	state, active := e.vimImageRows.State()
	if !active || dir == 0 {
		return false
	}
	l := e.leafAt(state.LineStart)
	_, rows, ok := e.vimImageGeometry(l)
	if !ok {
		e.clearVimImageRow()
		return false
	}
	if !e.vimImageRows.Move(dir, rows) {
		return false
	}
	if extend {
		e.setSessionInclusive(false)
		e.setSessionLinewise(true)
	}
	e.wantReveal = true
	e.blinkStart = time.Now()
	e.invalidate()
	return true
}

func (e *Editor) enterVimImageRow(l *leaf, dir int, extend bool) bool {
	_, rows, ok := e.vimImageGeometry(l)
	if !ok {
		return false
	}
	if !e.vimImageRows.Enter(l.lineStart, rows, dir) {
		return false
	}
	e.setCaretKeepGoal(l.lineStart, extend)
	if extend {
		e.setSessionInclusive(false)
		e.setSessionLinewise(true)
	}
	return true
}

func (e *Editor) vimVirtualImageLineRange() (int, int, bool) {
	state, active := e.vimImageRows.State()
	if !active || !e.vimImageRowsActive() {
		return 0, 0, false
	}
	l := e.leafAt(state.LineStart)
	if _, _, ok := e.vimImageGeometry(l); !ok {
		return 0, 0, false
	}
	a, z := e.sourceLineRange(state.LineStart)
	if z < len(e.Source()) {
		z++
	}
	return a, z, z > a
}
