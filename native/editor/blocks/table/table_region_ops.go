package mdedit

import (
	"strings"

	"muse/mdedit/internal/parser"
)

// tableCellRegion is the key-independent rectangular cell selection value.
// Coordinates are inclusive and always refer to parser-projected table cells.
type tableCellRegion struct {
	lineStart            int
	anchorRow, anchorCol int
	focusRow, focusCol   int
}

func (r tableCellRegion) bounds() (top, left, bottom, right int) {
	top, bottom = r.anchorRow, r.focusRow
	left, right = r.anchorCol, r.focusCol
	if top > bottom {
		top, bottom = bottom, top
	}
	if left > right {
		left, right = right, left
	}
	return
}

// tableCellMatrix is a rectangular snapshot. Alignments are column scoped and
// retained with the copied cells for subsequent insertion/paste operations.
type tableCellMatrix struct {
	cells [][]string
	align []parser.Align
}

func (e *Editor) tableCellMatrix(t *leaf, region tableCellRegion) *tableCellMatrix {
	if t == nil || t.tab == nil || t.lineStart != region.lineStart {
		return nil
	}
	top, left, bottom, right := region.bounds()
	if top < 0 || left < 0 || bottom >= len(t.tab.rows) || left >= t.tab.cols() {
		return nil
	}
	if right >= t.tab.cols() {
		right = t.tab.cols() - 1
	}
	m := &tableCellMatrix{align: make([]parser.Align, right-left+1)}
	for c := left; c <= right; c++ {
		m.align[c-left] = alignAt(t.tab.align, c)
	}
	for row := top; row <= bottom; row++ {
		values := make([]string, right-left+1)
		for c := left; c <= right; c++ {
			if c < len(t.tab.rows[row]) {
				cell := t.tab.rows[row][c]
				values[c-left] = e.src[cell.start:cell.end]
			}
		}
		m.cells = append(m.cells, values)
	}
	return m
}

// clearTableCellRegion clears cell contents while preserving table dimensions.
// All source edits are committed as one normal editor transaction.
func (e *Editor) clearTableCellRegion(t *leaf, region tableCellRegion) bool {
	m := e.tableCellMatrix(t, region)
	if m == nil {
		return false
	}
	top, left, bottom, right := region.bounds()
	var edits [][2]int
	for row := top; row <= bottom; row++ {
		for col := left; col <= right && col < len(t.tab.rows[row]); col++ {
			c := t.tab.rows[row][col]
			edits = append(edits, [2]int{c.start, c.end})
		}
	}
	if len(edits) == 0 {
		return false
	}
	e.begin(stepEdit)
	for i := len(edits) - 1; i >= 0; i-- {
		e.replace(edits[i][0], edits[i][1], "")
	}
	e.commit(edits[0][0], edits[0][0])
	return true
}

// replaceTableCellRegion replaces the selected rectangle with matrix contents.
// Existing cells outside the target are retained; the table grows when the
// matrix exceeds the selection. The parser projection supplies cell shape.
func (e *Editor) replaceTableCellRegion(t *leaf, region tableCellRegion, matrix *tableCellMatrix) bool {
	if t == nil || t.tab == nil || matrix == nil || len(matrix.cells) == 0 || len(matrix.cells[0]) == 0 {
		return false
	}
	top, left, bottom, right := region.bounds()
	rows := max(len(t.tab.rows), top+len(matrix.cells))
	cols := max(t.tab.cols(), left+len(matrix.cells[0]))
	// The selected footprint is cleared first, including cells not covered by
	// a smaller incoming matrix.
	values := make([][]string, rows)
	for r := range values {
		values[r] = make([]string, cols)
		if r < len(t.tab.rows) {
			for c, cell := range t.tab.rows[r] {
				if c < cols {
					values[r][c] = e.src[cell.start:cell.end]
				}
			}
		}
	}
	for r := top; r <= bottom && r < len(values); r++ {
		for c := left; c <= right && c < len(values[r]); c++ {
			values[r][c] = ""
		}
	}
	for r, row := range matrix.cells {
		for c, value := range row {
			values[top+r][left+c] = value
		}
	}
	lines := e.tableLines(t)
	if len(lines) < 2 {
		return false
	}
	var b strings.Builder
	for r, row := range values {
		if r > 0 {
			b.WriteByte('\n')
		}
		b.WriteByte('|')
		for _, value := range row {
			b.WriteString(" " + value + " |")
		}
		if r == 0 {
			b.WriteString("\n|")
			for c := 0; c < cols; c++ {
				a := alignAt(t.tab.align, c)
				sep := "---"
				if c >= left && c < left+len(matrix.align) {
					a = matrix.align[c-left]
				}
				switch a {
				case parser.AlignLeft:
					sep = ":---"
				case parser.AlignCenter:
					sep = ":---:"
				case parser.AlignRight:
					sep = "---:"
				}
				b.WriteString(" " + sep + " |")
			}
		}
	}
	a, z := lines[0][0], lines[len(lines)-1][1]
	e.begin(stepEdit)
	e.replace(a, z, b.String())
	e.commit(a, a)
	return true
}
