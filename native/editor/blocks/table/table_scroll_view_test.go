package mdedit

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
	"muse/mdedit/internal/parser"
	"muse/locales"
)

func wideTableSource() string {
	var header, align, body []string
	for i := 0; i < 12; i++ {
		header = append(header, fmt.Sprintf("C%02d", i))
		align = append(align, "---")
		body = append(body, fmt.Sprintf("v%02d", i))
	}
	return "| " + strings.Join(header, " | ") + " |\n| " + strings.Join(align, " | ") + " |\n| " + strings.Join(body, " | ") + " |\n\nend"
}

func wideTableHarness(t *testing.T) *harness {
	t.Helper()
	src := wideTableSource()
	e := New(src, Options{FontSize: 16, MaxWidth: 480, PadX: 20})
	h := &harness{t: t, e: e}
	h.tt = ui.NewTester(func(c *ui.Context) {
		ui.Column(c).Fill().Children(func() { e.View(c).Grow(1) })
	}, 620, 520)
	h.frame()
	h.frame()
	h.focus()
	e.Select(len(src), len(src))
	h.frame()
	return h
}

func saveTableScrollShot(t *testing.T, h *harness, name string) {
	t.Helper()
	dir := os.Getenv("MDEDIT_SHOTS")
	if dir == "" {
		return
	}
	if err := os.MkdirAll(filepath.Clean(dir), 0o755); err != nil {
		t.Fatal(err)
	}
	h.shot(name)
}

func TestTableScrollViewportKeepsGridTextAndColumnControlsAligned(t *testing.T) {
	h := wideTableHarness(t)
	e := h.e
	table := documentTestTable(t, e)
	if table.geom.w <= tableViewportWidth(table) {
		t.Fatalf("fixture table does not overflow: grid=%v viewport=%v", table.geom.w, tableViewportWidth(table))
	}

	// Use the public tester path so this also exercises editor input routing.
	h.tt.Scroll(table.x+tableViewportWidth(table)/2, table.y+table.geom.rowY[0]+8, 170, 0)
	h.frame()
	if table.horizontal.x <= 0 {
		t.Fatal("horizontal scroll over the table did not move its viewport")
	}
	saveTableScrollShot(t, h, "table-horizontal-scroll")

	// Cells are laid out in content coordinates minus the table offset. The
	// grid uses the matching content origin, while both are clipped at the
	// stable table viewport edge.
	cell := table.tab.rows[0][4]
	padX := e.lk.size * cellPadX
	wantCellX := table.x + table.geom.colX[4] + padX - table.horizontal.x
	if cell.x != wantCellX {
		t.Fatalf("scrolled cell x=%v, want %v", cell.x, wantCellX)
	}
	contentX := tableContentX(table)
	if contentX != table.x-table.horizontal.x {
		t.Fatalf("grid content origin=%v, want %v", contentX, table.x-table.horizontal.x)
	}
	if row, col, ok := tableCellIndexAt(table, table.x+table.geom.colX[4]+table.geom.colW[4]/2-table.horizontal.x, table.y+table.geom.rowY[0]+2); !ok || row != 0 || col != 4 {
		t.Fatalf("hover mapping after scroll = (%d,%d,%t), want (0,4,true)", row, col, ok)
	}

	// The same page pixel just outside the table viewport remains untouched
	// after the content moves beneath it; leaked grid/text would change it.
	clipX := int(table.x + tableViewportWidth(table) + 3)
	probeY := int(table.y + table.geom.rowY[1])
	got := h.tt.Image().RGBAAt(clipX, probeY)
	if x := int(table.x + tableViewportWidth(table) + 30); x < 620 && got != h.tt.Image().RGBAAt(x, probeY) {
		t.Fatalf("drawing escaped the table viewport at x=%d: %v", clipX, got)
	}

	// Hovering a visible header cell places both its handle and its column menu
	// from the scrolled content geometry.
	col := 4
	centerX := table.x + table.geom.colX[col] + table.geom.colW[col]/2 - table.horizontal.x
	centerY := table.y + (table.geom.rowY[0]+table.geom.rowY[1])/2
	h.tt.Move(centerX, centerY)
	h.frame()
	if _, ok := h.tt.Find("选择第 5 列"); !ok {
		t.Fatal("hovering visible header did not expose its column handle")
	}
	menuRect, ok := h.tt.Find(locales.EditorTableCellAlignment(1, 5))
	if !ok || menuRect.X < table.x || menuRect.X+menuRect.W > table.x+tableViewportWidth(table)+1 {
		t.Fatalf("visible cell alignment control is missing or outside viewport: %+v", menuRect)
	}
	if err := h.tt.Click("第1行第5列对齐"); err != nil {
		t.Fatal(err)
	}
	if err := h.tt.ChooseMenuItem("居中（此列）"); err != nil {
		t.Fatal(err)
	}
	h.frame()
	table = documentTestTable(t, e)
	if table.horizontal.x <= 0 {
		t.Fatal("column alignment reset the table's horizontal viewport")
	}
	if got := table.tab.align[4]; got != parser.AlignCenter {
		t.Fatalf("scrolled cell menu aligned column %d as %v, want center", 4, got)
	}
}

func TestScrolledMiddleColumnHandleKeepsCaptureAndReorders(t *testing.T) {
	h := wideTableHarness(t)
	e := h.e
	table := documentTestTable(t, e)
	e.setBlockHorizontal(table, 170)
	h.frame()
	col := 5
	x := table.x + (table.geom.colX[col]+table.geom.colX[col+1])/2 - table.horizontal.x
	y := table.y + table.geom.rowY[0] + 6
	h.tt.Move(x, y)
	h.frame()
	handle, ok := h.tt.Find("选择第 6 列")
	if !ok || handle.X < table.x || handle.X+handle.W > table.x+tableViewportWidth(table)+1 {
		t.Fatalf("middle column handle not reachable after scroll: %+v", handle)
	}
	saveTableScrollShot(t, h, "table-scrolled-middle-handle-hover")
	h.tt.Press(handle.X+handle.W/2, handle.Y+handle.H/2)
	h.frame()
	if d := e.tableHandleDrag; d == nil || d.axis != tableColumns || d.index != col {
		t.Fatalf("pressing middle handle did not capture column %d: %+v", col, d)
	}
	// Drop before original column 2. The handle overlay set changes while
	// dragging, so this catches regressions in stable tableHandleKey identity.
	dropX := table.x + table.geom.colX[2] + 8 - table.horizontal.x
	h.tt.Move(dropX, y)
	h.frame()
	if d := e.tableHandleDrag; d == nil || !d.moving || d.gap != 2 {
		t.Fatalf("captured handle did not track the insertion gap after scroll: %+v", d)
	}
	saveTableScrollShot(t, h, "table-scrolled-middle-handle-drag")
	h.tt.Release(dropX, y)
	h.frame()
	if e.tableHandleDrag != nil {
		t.Fatal("release did not end the middle-column handle drag")
	}
	table = documentTestTable(t, e)
	cell := table.tab.rows[0][2]
	if got := strings.TrimSpace(e.src[cell.start:cell.end]); got != "C05" {
		t.Fatalf("column reorder did not move the dragged middle column before column 2: row=%v", table.tab.rows[0])
	}
	if table.horizontal.x <= 0 {
		t.Fatal("column reorder reset the table's horizontal viewport")
	}
}
