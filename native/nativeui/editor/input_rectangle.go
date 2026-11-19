package editor

import (
	"encoding/json"
	"sort"
	"strings"

	"github.com/egoist/mygo/transfer"
	"github.com/egoist/mygo/ui"
	"muse/editor/input/vim"
)

const rectangleClipboardFormat transfer.Format = "application/x-muse-rectangle+json"

type rectangleClipboard struct {
	Version int      `json:"version"`
	Rows    []string `json:"rows"`
}

func encodeRectangleClipboard(rows []string) []byte {
	b, _ := json.Marshal(rectangleClipboard{Version: 1, Rows: rows})
	return b
}
func decodeRectangleClipboard(data []byte) ([]string, bool) {
	var p rectangleClipboard
	if json.Unmarshal(data, &p) != nil || p.Version != 1 || len(p.Rows) == 0 {
		return nil, false
	}
	return p.Rows, true
}

func (e *Editor) copyBlockRows() []string {
	var rows []string
	for _, r := range e.SelectionRanges() {
		rows = append(rows, e.Source()[r.Start:r.End])
	}
	return rows
}

func (e *Editor) pasteRectangle(rows []string) {
	if len(rows) == 0 {
		return
	}
	if e.block == nil {
		e.pasteBlockRegister(rows, true, 1)
		return
	}
	selected := e.blockRanges()
	visual := e.blockRows()
	if len(visual) == 0 {
		e.block = nil
		e.pasteBlockRegister(rows, true, 1)
		return
	}
	focus, ok := e.caretRect(e.Caret())
	if !ok {
		return
	}
	x := e.block.x0
	if focus.X < x {
		x = focus.X
	}
	loY, hiY := e.block.y0, focus.Y+focus.H/2
	if loY > hiY {
		loY, hiY = hiY, loY
	}
	base := 0
	for i, row := range visual {
		if row.y+0.1 >= loY && row.y-0.1 <= hiY {
			base = i
			break
		}
	}
	n := max(len(rows), len(selected))
	if n == 0 {
		return
	}
	ops := make([]Range, 0, n)
	values := make([]string, 0, n)
	var extra strings.Builder
	for i := 0; i < n; i++ {
		v := ""
		if i < len(rows) {
			v = rows[i]
		}
		var r Range
		if i < len(selected) {
			r = selected[i]
		} else {
			idx := base + i
			if idx < len(visual) {
				at := e.blockPosition(visual[idx], x)
				r = Range{Start: at, End: at}
			} else {
				if extra.Len() == 0 {
					if len(e.Source()) > 0 && !strings.HasSuffix(e.Source(), "\n") {
						extra.WriteByte('\n')
					}
				} else {
					extra.WriteByte('\n')
				}
				padding := max(0, int((x-e.colX)/max(e.lk.size*0.55, 1)))
				extra.WriteString(strings.Repeat(" ", padding))
				extra.WriteString(v)
				continue
			}
		}
		ops = append(ops, r)
		values = append(values, v)
	}
	if len(ops) == 0 && extra.Len() == 0 {
		return
	}
	e.begin(stepEdit)
	if extra.Len() > 0 {
		e.replace(len(e.Source()), len(e.Source()), extra.String())
	}
	for i := len(ops) - 1; i >= 0; i-- {
		r := ops[i]
		if r.Start != r.End || values[i] != "" {
			e.replace(r.Start, r.End, values[i])
		}
	}
	at := ops[0].Start
	e.block = nil
	e.commit(at, at)
}

// blockSelection stores a rectangle in the editor's native layout geometry.
// Y coordinates are in document-element space; x columns are physical DIPs,
// so proportional fonts and CJK glyph advances follow the actual display.
type blockSelection struct {
	x0, y0      float32
	focusX      float32
	hasFocusX   bool
	left, right float32
	top, bottom float32
	revision    uint64
	ranges      []Range
}
type blockHistoryState struct {
	selection    *blockSelection
	anchorOffset int
}

func (e *Editor) blockHistoryState() *blockHistoryState {
	if e.block == nil {
		return nil
	}
	b := *e.block
	b.ranges = append([]Range(nil), e.block.ranges...)
	return &blockHistoryState{selection: &b, anchorOffset: e.blockAnchorOffset}
}

func (e *Editor) restoreBlockHistory(s *blockHistoryState) {
	e.block = nil
	e.blockInput = nil
	if s == nil {
		if e.vimModeOverride == vim.ModeBlockVisual && e.tableSelection == nil && e.tableBridge == nil {
			e.vimModeOverride = ""
		}
		return
	}
	b := *s.selection
	b.ranges = append([]Range(nil), s.selection.ranges...)
	e.block = &b
	e.blockAnchorOffset = s.anchorOffset
	// History callbacks run while Session applies an edit and may hold its
	// write lock. Keep this UI-only mode override local instead of re-entering
	// the Session-backed Vim keymap.
	e.vimModeOverride = vim.ModeBlockVisual
	e.invalidate()
}

type blockInput struct {
	points  []int
	started bool
}
type blockVisualRow struct {
	leaf    *leaf
	view    *leafView
	line    ui.TextLine
	y       float32
	emptyAt int
}

func (e *Editor) startBlockSelection() {
	r, ok := e.caretRect(e.Caret())
	if !ok {
		r = ui.Rect{X: e.lk.size, Y: 0, H: e.lk.size * e.lk.lh}
	}
	e.block = &blockSelection{x0: r.X, y0: r.Y + r.H/2, revision: 1}
	e.blockAnchorOffset = e.Caret()
	e.refreshBlockRanges()
}

func (e *Editor) blockRows() []blockVisualRow {
	e.layoutIfNeeded()
	var rows []blockVisualRow
	for _, u := range e.units() {
		if e.foldHidden(u) || u.kind == kTable || u.table != nil {
			continue
		}
		v := e.unitView(u, e.Caret())
		if v == nil || v.layout == nil {
			rows = append(rows, blockVisualRow{leaf: u, y: u.y + u.h/2, emptyAt: u.start})
			continue
		}
		for _, line := range v.layout.Lines() {
			rows = append(rows, blockVisualRow{leaf: u, view: v, line: line, y: u.y + v.ty + line.Rect.Y + line.Rect.H/2})
		}
	}
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].y < rows[j].y })
	return rows
}

func (e *Editor) blockRanges() []Range {
	if e.block == nil {
		return nil
	}
	return append([]Range(nil), e.block.ranges...)
}

func (e *Editor) refreshBlockRanges() {
	if e.block == nil {
		return
	}
	focus, ok := e.caretRect(e.Caret())
	if !ok {
		e.block.ranges = nil
		return
	}
	y1 := focus.Y + focus.H/2
	loY, hiY := e.block.y0, y1
	if loY > hiY {
		loY, hiY = hiY, loY
	}
	x1 := focus.X
	if e.block.hasFocusX {
		x1 = e.block.focusX
	}
	loX, hiX := e.block.x0, x1
	if x1 >= e.block.x0 {
		hiX = e.blockEdgeAfter(x1, y1)
	} else {
		hiX = e.blockEdgeAfter(e.block.x0, e.block.y0)
	}
	if loX > hiX {
		loX, hiX = hiX, loX
	}
	e.block.left, e.block.right = loX, hiX
	e.block.top, e.block.bottom = loY, hiY
	var out []Range
	for _, row := range e.blockRows() {
		if row.y+0.1 < loY || row.y-0.1 > hiY {
			continue
		}
		if row.view == nil || row.view.layout == nil {
			out = append(out, Range{Start: row.emptyAt, End: row.emptyAt})
			continue
		}
		base := row.line.Baseline
		p0 := row.view.layout.PositionAt(ui.Point{X: loX - unitTextX(row.leaf, row.view), Y: base})
		p1 := row.view.layout.PositionAt(ui.Point{X: hiX - unitTextX(row.leaf, row.view), Y: base})
		a := row.view.disp.dispToSrc(p0.Index, false) + row.leaf.lineStart
		z := row.view.disp.dispToSrc(p1.Index, false) + row.leaf.lineStart
		if a > z {
			a, z = z, a
		}
		out = append(out, Range{Start: a, End: z})
	}
	// Keep a zero-length range for a short or blank display row: it is a
	// meaningful line in copied blocks and a multi-caret insertion point.
	sort.SliceStable(out, func(i, j int) bool { return out[i].Start < out[j].Start })
	e.block.ranges = out
}

func (e *Editor) blockEdgeAfter(x, y float32) float32 {
	rows := e.blockRows()
	if len(rows) == 0 {
		return x
	}
	rowIndex := 0
	best := float32(1e9)
	for i, row := range rows {
		d := row.y - y
		if d < 0 {
			d = -d
		}
		if d < best {
			best = d
			rowIndex = i
		}
	}
	row := rows[rowIndex]
	if row.view == nil || row.view.layout == nil {
		return x
	}
	p := row.view.layout.PositionAt(ui.Point{X: x - unitTextX(row.leaf, row.view), Y: row.line.Baseline})
	end := row.view.layout.NextBoundary(p.Index)
	if end > row.line.End {
		end = row.line.End
	}
	r := row.view.layout.Caret(end)
	return unitTextX(row.leaf, row.view) + r.X
}

func (e *Editor) blockChanged() {
	if e.block != nil {
		e.block.revision++
		e.block.hasFocusX = false
		e.refreshBlockRanges()
	}
	e.invalidate()
}

func (e *Editor) blockChangedVertical(x float32) {
	if e.block != nil {
		e.block.revision++
		e.block.focusX = x
		e.block.hasFocusX = true
		e.refreshBlockRanges()
	}
	e.invalidate()
}

func (e *Editor) deleteBlock() int {
	ranges := e.blockRanges()
	if len(ranges) == 0 {
		return e.Caret()
	}
	e.begin(stepEdit)
	for i := len(ranges) - 1; i >= 0; i-- {
		r := ranges[i]
		if r.End > r.Start {
			e.replace(r.Start, r.End, "")
		}
	}
	at := ranges[0].Start
	e.block = nil
	e.commit(at, at)
	return at
}

func (e *Editor) typeBlockInput(text string) {
	b := e.blockInput
	if b == nil || len(b.points) == 0 || text == "" {
		return
	}
	points := append([]int(nil), b.points...)
	sort.Sort(sort.Reverse(sort.IntSlice(points)))
	e.beginBlockInputEdit(b, stepTyping)
	for _, at := range points {
		e.replace(at, at, text)
	}
	// Rebase each insertion point by the text inserted before it.
	for i, at := range b.points {
		shift := 0
		for _, p := range points {
			if p < at {
				shift += len(text)
			}
		}
		b.points[i] = at + shift + len(text)
	}
	if len(b.points) > 0 {
		e.setSessionSelection(b.points[len(b.points)-1], b.points[len(b.points)-1])
	}
	e.commit(e.Caret(), e.Caret())
	e.invalidate()
}

// Block insert input arrives as independent IME/native events. Close every
// Session transaction at that boundary; use MergePrevious for later events
// so a multi-row/multi-character block edit still has one undo step.
func (e *Editor) beginBlockInputEdit(b *blockInput, kind stepKind) {
	if b.started {
		e.beginMergePrevious(kind)
	} else {
		e.begin(kind)
		b.started = true
	}
}

func (e *Editor) finishBlockInput() {
	if e.blockInput == nil {
		return
	}
	if e.editBatch != nil {
		e.commit(e.Caret(), e.Caret())
	}
	e.blockInput = nil
}

func (e *Editor) editBlockInputDelete(backward bool) {
	b := e.blockInput
	if b == nil || len(b.points) == 0 {
		return
	}
	var ds []Range
	for _, p := range b.points {
		a, z := p, p
		if backward {
			a = e.previousGraphemeBoundary(p)
		} else {
			z = e.nextGraphemeBoundary(p)
		}
		if a < z {
			ds = append(ds, Range{a, z})
		}
	}
	if len(ds) == 0 {
		return
	}
	e.beginBlockInputEdit(b, stepDelete)
	for i := len(ds) - 1; i >= 0; i-- {
		r := ds[i]
		e.replace(r.Start, r.End, "")
	}
	for i, p := range b.points {
		shift := 0
		for _, r := range ds {
			if r.End <= p {
				shift += r.End - r.Start
			}
		}
		b.points[i] = p - shift
	}
	if len(b.points) > 0 {
		e.setSessionSelection(b.points[len(b.points)-1], b.points[len(b.points)-1])
	}
	e.commit(e.Caret(), e.Caret())
	e.invalidate()
}

func (e *Editor) pasteBlockRegister(rows []string, before bool, count int) {
	if len(rows) == 0 {
		return
	}
	r, ok := e.caretRect(e.Caret())
	if !ok {
		return
	}
	x := r.X
	if !before {
		_, lineEnd := e.sourceLineRange(e.Caret())
		next := e.nextGraphemeBoundary(e.Caret())
		if next <= lineEnd {
			if nr, ok := e.caretRect(next); ok && nr.Y <= r.Y+r.H {
				x = nr.X
			}
		}
	}
	visual := e.blockRows()
	if len(visual) == 0 {
		e.insertText(strings.Join(rows, "\n"))
		return
	}
	base := 0
	best := float32(1e9)
	cy := r.Y + r.H/2
	for i, row := range visual {
		d := row.y - cy
		if d < 0 {
			d = -d
		}
		if d < best {
			best = d
			base = i
		}
	}
	var edits []Range
	values := make([]string, 0, len(rows))
	var extra strings.Builder
	for i, s := range rows {
		idx := base + i
		if idx >= len(visual) {
			if extra.Len() == 0 {
				if len(e.Source()) > 0 && !strings.HasSuffix(e.Source(), "\n") {
					extra.WriteByte('\n')
				}
			} else {
				extra.WriteByte('\n')
			}
			padding := max(0, int((x-e.colX)/max(e.lk.size*0.55, 1)))
			extra.WriteString(strings.Repeat(" ", padding))
			extra.WriteString(strings.Repeat(s, max(1, count)))
			continue
		}
		row := visual[idx]
		at := e.blockPosition(row, x)
		edits = append(edits, Range{Start: at, End: at})
		values = append(values, strings.Repeat(s, max(1, count)))
	}
	if len(edits) == 0 && extra.Len() == 0 {
		return
	}
	e.begin(stepEdit)
	if extra.Len() > 0 {
		e.replace(len(e.Source()), len(e.Source()), extra.String())
	}
	for i := len(edits) - 1; i >= 0; i-- {
		e.replace(edits[i].Start, edits[i].End, values[i])
	}
	e.commit(e.Caret(), e.Caret())
}

func (e *Editor) blockPosition(row blockVisualRow, x float32) int {
	if row.view == nil || row.view.layout == nil {
		return row.emptyAt
	}
	pos := row.view.layout.PositionAt(ui.Point{X: x - unitTextX(row.leaf, row.view), Y: row.line.Baseline})
	return row.view.disp.dispToSrc(pos.Index, false) + row.leaf.lineStart
}
