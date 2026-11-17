package editor

import (
	"time"

	"github.com/egoist/mygo/ui"
	"muse/editor/input/vim"
	"muse/editor/layout"
)

// vimImageRowCaret is a navigation position inside a rendered standalone
// image. Its source offset remains the image's real Markdown line so every
// text operation continues to address that one source row.
type vimImageRowCaret struct {
	lineStart int
	row       int
}

func (e *Editor) clearVimImageRow() { e.vimImageRow = nil }

func (e *Editor) vimImageRowsActive() bool {
	return e.opts.VimVirtualImageRows && e.Keymap() == "vim" && e.vimController().Mode() != vim.ModeInsert
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
	state := e.vimImageRow
	if state == nil || l == nil || l.lineStart != state.lineStart {
		return ui.Rect{}, false
	}
	box, rows, ok := e.vimImageGeometry(l)
	if !ok {
		e.clearVimImageRow()
		return ui.Rect{}, false
	}
	state.row = min(max(state.row, 0), rows-1)
	lineHeight := max(e.lk.size*e.lk.lh, 1)
	y := box.Y + float32(state.row)*lineHeight
	return ui.Rect{X: box.X, Y: y, W: 1, H: min(lineHeight, box.Y+box.H-y)}, true
}

// moveVimImageRow consumes vertical movement while the virtual caret is
// inside a tall image. Crossing either image edge falls through to ordinary
// document navigation.
func (e *Editor) moveVimImageRow(dir int, extend bool) bool {
	if e.vimImageRow == nil || dir == 0 {
		return false
	}
	l := e.leafAt(e.vimImageRow.lineStart)
	_, rows, ok := e.vimImageGeometry(l)
	if !ok {
		e.clearVimImageRow()
		return false
	}
	next := e.vimImageRow.row + dir
	if next < 0 || next >= rows {
		e.clearVimImageRow()
		return false
	}
	e.vimImageRow.row = next
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
	row := 0
	if dir < 0 {
		row = rows - 1
	}
	e.setCaretKeepGoal(l.lineStart, extend)
	e.vimImageRow = &vimImageRowCaret{lineStart: l.lineStart, row: row}
	if extend {
		e.setSessionInclusive(false)
		e.setSessionLinewise(true)
	}
	return true
}

func (e *Editor) vimVirtualImageLineRange() (int, int, bool) {
	if e.vimImageRow == nil || !e.vimImageRowsActive() {
		return 0, 0, false
	}
	l := e.leafAt(e.vimImageRow.lineStart)
	if _, _, ok := e.vimImageGeometry(l); !ok {
		return 0, 0, false
	}
	a, z := e.sourceLineRange(e.vimImageRow.lineStart)
	if z < len(e.Source()) {
		z++
	}
	return a, z, z > a
}
