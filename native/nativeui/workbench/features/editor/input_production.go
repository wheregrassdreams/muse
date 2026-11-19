package editor

import (
	"github.com/wheregrassdreams/muse-editor/text/layout"
	mygoinputadapter "muse/nativeui/workbench/features/editor/inputadapter"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	syntax "github.com/wheregrassdreams/muse-editor/markdown/syntax"
	tablemodel "github.com/wheregrassdreams/muse-editor/markdown/table"
	helix "github.com/wheregrassdreams/muse-editor/text/helix"
	editorinput "github.com/wheregrassdreams/muse-editor/text/input"
	media "muse/nativeui/workbench/features/editor/blocks/media"
	tableui "muse/nativeui/workbench/features/editor/blocks/table"
)

// tableCellDragState exists only for the lifetime of a pointer gesture.
type tableCellDragState struct {
	lineStart            int
	anchorRow, anchorCol int
	crossed              bool
}

// input takes the editor's input as it comes: keys, text typed, the edit
// commands of the menus, and the pointer.
func (e *Editor) input(ev ui.InputEvent) bool {
	defer func() {
		e.finishInputTransaction()
		e.invalidate()
	}()
	switch ev.Kind {
	case ui.InputKeyDown:
		key, ok := mygoinputadapter.KeyEvent(ev)
		return ok && e.DispatchKey(key)
	case ui.InputText:
		event, ok := mygoinputadapter.TextEvent(ev)
		if !ok {
			return false
		}
		if e.Keymap() == "vim" && e.vimController().Mode() == helix.ModeInsert {
			intent, handled := e.Session.HandleVimKey(event)
			if !handled {
				return false
			}
			if intent.Name != "" {
				_, err := e.Session.ExecuteVimIntent(intent)
				return err == nil
			}
			return true
		}
		result, err := e.Session.Dispatch(editorinput.Event{Kind: editorinput.InsertText, Text: event.Text})
		return err == nil && result.Handled
	case ui.InputCompose:
		if !e.ModalAcceptsText() {
			return true
		}
		n := ui.UTF16Len(ev.Text)
		c := min(ui.UTF16Len(string([]rune(ev.Text)[:min(ev.Caret, len([]rune(ev.Text)))])), n)
		e.setComposition(ev.Text, ui.TextInputRange{Start: c, End: c})
		return true
	case ui.InputCommand:
		if !e.ModalAcceptsText() {
			switch ev.Text {
			case "copy", "undo", "redo", "selectAll":
			default:
				return true
			}
		}
		return e.command(ev.Text)
	case ui.InputPointerDown:
		e.pointerX, e.pointerY = ev.X, ev.Y
		if ev.Button == 1 {
			e.layoutIfNeeded()
			e.contextDiagramSource = ""
			if hit, ok := e.imageAt(ev.X, ev.Y); ok && hit.obj != nil {
				e.contextImagePath = hit.obj.imagePath
			} else {
				e.contextImagePath = ""
				if leaf, _, ok := e.diagramBodyAt(ev.X, ev.Y); ok && isMermaid(leaf) {
					e.contextDiagramSource = e.codeText(leaf)
				}
			}
		}
		if ev.Button == 0 {
			if i, ok := e.wikiSuggestionIndexAt(ev.X, ev.Y); ok {
				e.acceptWikiSuggestion(i)
				return true
			}
		}
		// A visible block scrollbar is an overlay interaction, not a text
		// press. Route it before finishing block input or clearing its state so
		// dragging the thumb preserves selections, source mode and caret.
		if ev.Button == 0 {
			e.layoutIfNeeded()
			if l, ok := e.horizontalBarAt(ev.X, ev.Y); ok {
				e.pressBlockHorizontal(l, ev.X)
				return true
			}
		}
		e.finishBlockInput()
		e.block = nil
		handled := e.press(ev)
		e.updateWikiSuggestions()
		return handled
	case ui.InputPointerMove:
		return e.drag(ev)
	case ui.InputScroll:
		return e.scrollBlock(ev)
	case ui.InputPointerUp:
		if e.horizontalDrag != nil {
			e.horizontalDrag = nil
			return true
		}
		if e.imageDrag != nil {
			d := e.imageDrag
			e.imageDrag = nil
			e.resizeImageSource(d.hit, d.gesture.PreviewWidth())
			e.assetGen.Add(1)
			e.invalidate()
			return true
		}
		if e.diagPan != nil {
			e.diagPan = nil
			return true
		}
		if e.diagDrag != nil {
			e.diagDrag = nil
			return true
		}
		if e.dragging {
			e.dragging = false
			e.tableCellDrag = nil
			if e.frozen {
				e.frozen = false
				e.setSessionHead(e.normalize(e.Caret(), 1))
			}
			return true
		}
		return false
	}
	return false
}

// DispatchKey sends a backend-neutral key through the editor's keymap and
// requests a frame when the command changes its state. Native adapters should
// translate toolkit keys into editor/input.KeyEvent before calling it.
func (e *Editor) DispatchKey(event editorinput.KeyEvent) bool {
	defer func() {
		e.finishInputTransaction()
		e.invalidate()
	}()
	return e.Session.DispatchKey(event)
}

// finishInputTransaction closes a Session-owned edit batch at the boundary of
// one native input event. A command may apply several source edits internally,
// but an unfinished batch must never escape to the next event and retain the
// Session's transaction lock.
func (e *Editor) finishInputTransaction() {
	if e.editBatch != nil {
		selection := e.Session.Selection()
		e.commit(selection.Anchor, selection.Head)
	}
}

func (e *Editor) page(down, extend bool) {
	r, ok := e.caretRect(e.Caret())
	if !ok {
		return
	}
	if !e.hasGoal {
		e.goalX, e.hasGoal = r.X, true
	}
	h := max(e.viewH-e.lk.size*3, e.lk.size*3)
	y := r.Y + r.H/2 - h
	if down {
		y = r.Y + r.H/2 + h
	}
	off := e.hitTest(e.goalX, y)
	g := e.goalX
	e.setCaretAtPoint(off, y, extend)
	e.goalX, e.hasGoal = g, true
	if down {
		e.scroll.Y = min(e.scroll.Y+h, e.scroll.MaxY)
	} else {
		e.scroll.Y = max(e.scroll.Y-h, 0)
	}
	e.invalidate()
}

func (e *Editor) press(ev ui.InputEvent) bool {
	if ev.Button != 0 {
		return false
	}
	e.layoutIfNeeded()
	x, y := ev.X, ev.Y
	e.pointerX, e.pointerY = x, y
	e.tableCellDrag, e.tableDocumentRegion = nil, nil
	if h := e.foldChevronAt(x, y); h != nil {
		e.toggleFold(h)
		return true
	}
	if hit, ok := e.imageAt(x, y); ok {
		preview, source, align, grip := imageToolRects(hit.box, e.lk.size)
		switch {
		case preview.Contains(x, y):
			if e.opts.OnImagePreview != nil {
				e.opts.OnImagePreview(hit.obj.imagePath)
			}
			return true
		case source.Contains(x, y):
			if hit.leaf != nil {
				e.toggleSource(hit.leaf)
			}
			return true
		case align.Contains(x, y) && hit.obj != nil && hit.obj.block:
			e.toggleImageAlignment(hit)
			return true
		case grip.Contains(x, y):
			e.imageDrag = &imageResizeDrag{hit: hit, gesture: media.NewImageResizeGesture(x, y, hit.obj.w)}
			return true
		default:
			if hit.obj != nil && hit.obj.bmp == nil && e.opts.OnImagePreview != nil {
				e.opts.OnImagePreview(hit.obj.imagePath)
				return true
			}
		}
	}
	if l, v, box, width, ok := e.diagramBarAt(x, y); ok {
		e.pressDiagramBar(l, v, box, width, x, y)
		return true
	}
	if it, ok := e.taskBoxAt(x, y); ok {
		e.toggleTaskAt(it)
		return true
	}
	if l := e.diagramPreviewAt(x, y); l != nil {
		if e.opts.OnDiagramPreview != nil && l.view != nil && l.view.diagramModel != nil {
			e.previewDiagram(e.codeText(l))
		}
		return true
	}
	if l := e.diagramToggleAt(x, y); l != nil {
		e.toggleDiagramSource(l)
		return true
	}
	if l := e.sourceChipAt(x, y); l != nil {
		e.toggleSource(l)
		return true
	}
	if l := e.codeLabelAt(x, y); l != nil {
		e.toggleSource(l)
		return true
	}
	if l, v, ok := e.diagramBodyAt(x, y); ok {
		e.diagPan = &diagPan{leaf: l, start: x, from: v.diagX}
		e.cursor = ui.CursorGrabbing
		e.invalidate()
		return true
	}
	e.setSessionInclusive(false)
	e.setSessionLinewise(false)
	off := e.hitTest(x, y)
	if ev.Clicks <= 1 && ev.Mods&(ui.Shift|ui.Cmd|ui.Ctrl|ui.Alt) == 0 {
		if target, ok := e.footnoteTargetAt(off); ok {
			e.NavigateToOffset(target, layout.PageJumpNearest)
			return true
		}
	}
	if target, bounds := e.wikiLinkHoverAt(x, y, off); target != "" || e.opts.OnWikiLinkHover != nil && e.wikiHover != "" {
		e.setWikiHover(target, x, y, bounds)
	}
	link := e.linkAt(off)
	if strings.HasPrefix(link, "wiki:") && ev.Mods&ui.Shift == 0 {
		if r, ok := e.linkRangeAt(off); ok && e.pointInLink(x, y, r) {
			if e.opts.OnLink != nil {
				e.opts.OnLink(link)
			}
			return true
		}
	}
	if ev.Mods&ui.Cmd != 0 && link != "" {
		if e.opts.OnLink != nil {
			e.opts.OnLink(link)
		}
		return true
	}
	if e.Keymap() == "vim" && e.vimController().Mode() != helix.ModeInsert && ev.Mods&ui.Shift == 0 {
		e.modalSetMode(helix.ModeNormal)
		if ev.Clicks <= 1 {
			if _, _, object := e.objectAt(off); !object {
				off = e.vimPointerOffset(off, x, y)
			}
		}
	}
	if len(e.leaves) > 0 && ev.Clicks <= 1 && ev.Mods&ui.Shift == 0 {
		last := e.leaves[len(e.leaves)-1]
		if y > last.y+last.h+e.lk.size*0.5 && e.phantomAtEdge(len(e.leaves)-1, 1) {
			return true
		}
	}
	if ev.Clicks <= 1 && ev.Mods&ui.Shift == 0 && len(e.p.Comments()) > 0 {
		if at, ok := e.gapPhantomAt(y); ok {
			for _, comment := range e.p.Comments() {
				if at >= comment.Start && at <= comment.End {
					e.setCaret(at, false)
					return true
				}
			}
		}
	}
	if ev.Clicks <= 1 && ev.Mods&ui.Shift == 0 && !e.onALine(y) {
		if at, ok := e.gapPhantomAt(y); ok {
			e.setPhantom(&phantom{at: at})
			return true
		}
	}
	e.dragging, e.hasGoal = true, false
	if row, col, table := e.tableCellAtPoint(x, y); table != nil && ev.Clicks <= 1 {
		e.tableCellDrag = &tableCellDragState{lineStart: table.lineStart, anchorRow: row, anchorCol: col}
	}
	if ev.Mods&ui.Shift == 0 {
		e.clearTableSelection()
	}
	switch {
	case ev.Clicks >= 3:
		a, z := e.blockRangeAt(off)
		e.dropPhantom()
		e.setSessionSelection(a, z)
		e.dragUnit, e.dragFrom = 2, [2]int{a, z}
	case ev.Clicks == 2:
		a, z := e.wordRangeAt(off)
		e.dropPhantom()
		e.setSessionSelection(a, z)
		e.dragUnit, e.dragFrom = 1, [2]int{a, z}
	default:
		extend := ev.Mods&ui.Shift != 0
		if a, z, object := e.objectAt(off); object && !extend {
			if e.isInlineMathRange(a, z) {
				e.setSessionSelection(a+1, a+1)
				break
			}
			e.dropPhantom()
			e.setSessionSelection(a, z)
			e.dragUnit, e.dragFrom = 0, [2]int{a, z}
			break
		}
		e.setCaretAtPoint(off, y, extend)
		e.dragUnit, e.dragFrom = 0, [2]int{e.Session.Selection().Anchor, e.Session.Selection().Anchor}
	}
	e.frozen, e.frozenAt = true, e.Caret()
	e.blinkStart = time.Now()
	return true
}

func (e *Editor) drag(ev ui.InputEvent) bool {
	e.pointerX, e.pointerY = ev.X, ev.Y
	if e.horizontalDrag != nil {
		e.dragBlockHorizontal(ev.X)
		return true
	}
	if e.imageDrag != nil {
		if ev.Button != 0 {
			return false
		}
		e.imageDrag.gesture.Drag(ev.X, ev.Y)
		e.assetGen.Add(1)
		e.invalidate()
		return true
	}
	if e.diagPan != nil {
		if ev.Button != 0 {
			e.diagPan = nil
			return false
		}
		e.dragDiagramPan(ev.X)
		return true
	}
	if e.diagDrag != nil {
		if ev.Button != 0 {
			return false
		}
		e.dragDiagramBar(ev.X)
		return true
	}
	if !e.dragging || ev.Button != 0 {
		e.hover(ev.X, ev.Y)
		return false
	}
	e.dragTo(ev.X, ev.Y)
	return true
}

func (e *Editor) dragTo(x, y float32) {
	off := e.hitTest(x, y)
	if capture := e.tableCellDrag; capture != nil {
		if t := e.tableByLineStart(capture.lineStart); t != nil {
			row, col, inside := tableCellIndexAt(t, x, y)
			if inside {
				if row != capture.anchorRow || col != capture.anchorCol {
					capture.crossed = true
				}
				if capture.crossed {
					e.selectTableCellRegion(t, capture.anchorRow, capture.anchorCol, row, col)
					return
				}
			} else if capture.crossed {
				if s := e.tableSelection; s != nil && s.cellRegion {
					v := *s
					e.tableDocumentRegion = &v
				}
				e.tableCellDrag = nil
			}
		}
	}
	if e.Keymap() == "vim" && (e.vimController().Mode() == helix.ModeNormal || e.vimController().Mode() == helix.ModeVisual) && e.dragUnit == 0 {
		off = e.vimPointerOffset(off, x, y)
		e.modalSetMode(helix.ModeVisual)
		e.setSessionSelectionMode(true, false)
	}
	e.clearTableSelection()
	a, z := off, off
	switch e.dragUnit {
	case 1:
		a, z = e.wordRangeAt(off)
	case 2:
		a, z = e.blockRangeAt(off)
	}
	if a < e.dragFrom[0] {
		e.setSessionSelection(e.dragFrom[1], a)
	} else {
		head := max(z, e.dragFrom[1])
		if e.dragUnit == 0 {
			head = off
		}
		e.setSessionSelection(e.dragFrom[0], head)
	}
	e.structuralSelectionForRange()
}

func (e *Editor) tableCellAtPoint(x, y float32) (row, col int, table *leaf) {
	for _, candidate := range e.leaves {
		if candidate.kind == kTable && candidate.tab != nil && candidate.geom != nil {
			if row, col, ok := tableCellIndexAt(candidate, x, y); ok {
				return row, col, candidate
			}
		}
	}
	return 0, 0, nil
}

func (e *Editor) autoScroll() bool {
	if !e.dragging || e.viewH <= 0 {
		return false
	}
	top, bottom := e.scroll.Y, e.scroll.Y+e.viewH
	var d float32
	switch {
	case e.pointerY < top:
		d = max((e.pointerY-top)/3, -40)
	case e.pointerY > bottom:
		d = min((e.pointerY-bottom)/3, 40)
	default:
		return false
	}
	y := max(min(e.scroll.Y+d, e.scroll.MaxY), 0)
	if y == e.scroll.Y {
		return false
	}
	e.pointerY += y - e.scroll.Y
	e.scroll.Y = y
	e.dragTo(e.pointerX, e.pointerY)
	return true
}

func (e *Editor) hover(x, y float32) {
	upstream := e.upstream
	defer func() { e.upstream = upstream }()
	e.pointerX, e.pointerY = x, y
	target, bounds := e.wikiLinkHoverAt(x, y, e.hitTest(x, y))
	e.setWikiHover(target, x, y, bounds)
	foldHover := -1
	if h := e.foldHeadingAt(x, y); h != nil {
		foldHover = h.lineStart
	}
	if foldHover != e.foldHover {
		e.foldHover = foldHover
		e.invalidate()
	}
	if e.foldChevronAt(x, y) != nil {
		e.cursor = ui.CursorPointer
		e.invalidate()
		return
	}
	if _, ok := e.horizontalBarAt(x, y); ok {
		e.cursor = ui.CursorDefault
		e.invalidate()
		return
	}
	var hoverLink Range
	off := e.hitTest(x, y)
	if r, ok := e.linkRangeAt(off); ok && e.pointInLink(x, y, r) {
		hoverLink = r
	}
	if hoverLink != e.hoverLink {
		e.hoverLink = hoverLink
		e.leavesLaid = false
		e.invalidate()
	}
	if hit, ok := e.imageAt(x, y); ok {
		preview, source, align, grip := imageToolRects(hit.box, e.lk.size)
		switch {
		case preview.Contains(x, y) || source.Contains(x, y) || hit.obj != nil && hit.obj.block && align.Contains(x, y) || hit.obj != nil && hit.obj.bmp == nil && e.opts.OnImagePreview != nil:
			e.cursor = ui.CursorPointer
		case grip.Contains(x, y):
			e.cursor = ui.CursorResizeNWSE
		default:
			e.cursor = ui.CursorText
		}
		e.invalidate()
		return
	}
	if l := e.sourceChipAt(x, y); l != nil {
		if e.hoverBlock != l.lineStart {
			e.hoverBlock = l.lineStart
			e.invalidate()
		}
		e.cursor = ui.CursorPointer
		e.invalidate()
		return
	}
	block := -1
	if l := e.leafAtPoint(y); l != nil && e.gridBlock(l) {
		block = l.lineStart
	}
	if block != e.hoverBlock {
		e.hoverBlock = block
		e.invalidate()
	}
	cursor := ui.CursorText
	if _, _, _, _, ok := e.diagramBarAt(x, y); ok {
		cursor = ui.CursorDefault
	} else if _, ok := e.taskBoxAt(x, y); ok || e.diagramToggleAt(x, y) != nil || e.diagramPreviewAt(x, y) != nil || e.codeLabelAt(x, y) != nil {
		cursor = ui.CursorPointer
	} else if _, _, ok := e.diagramBodyAt(x, y); ok {
		cursor = ui.CursorGrab
	} else if e.linkAt(e.hitTest(x, y)) != "" {
		cursor = ui.CursorPointer
	}
	if cursor != e.cursor {
		e.cursor = cursor
		e.invalidate()
	}
}

func (e *Editor) diagramBodyAt(x, y float32) (*leaf, *leafView, bool) {
	for _, l := range e.leaves {
		v := l.view
		if v != nil && v.diagramModel != nil {
			if box, _ := e.diagramBox(l, v, 0, 0); box.Contains(x, y) {
				return l, v, true
			}
		}
	}
	return nil, nil, false
}

func diagramPreviewControlRect(l *leaf, size float32) ui.Rect {
	return ui.Rect{X: l.x + l.w - size*2.35, Y: l.y + size*0.18, W: size * 1.65, H: size * 1.25}
}

func (e *Editor) toggleDiagramSource(l *leaf) { e.toggleSource(l) }

func imageToolRects(box ui.Rect, size float32) (preview, source, align, grip ui.Rect) {
	button := max(size*1.4, 22)
	preview = ui.Rect{X: box.X + box.W - button*3 - 6, Y: box.Y + 4, W: button, H: button}
	align = ui.Rect{X: box.X + box.W - button*2 - 4, Y: box.Y + 4, W: button, H: button}
	source = ui.Rect{X: box.X + box.W - button - 2, Y: box.Y + 4, W: button, H: button}
	grip = ui.Rect{X: box.X + box.W - button, Y: box.Y + box.H - button, W: button, H: button}
	return
}

func (e *Editor) diagramBarAt(x, y float32) (*leaf, *leafView, ui.Rect, float32, bool) {
	for _, l := range e.leaves {
		v := l.view
		if v == nil || v.diagramModel == nil || !v.diagBarShown {
			continue
		}
		box, width := e.diagramBox(l, v, 0, 0)
		if width > box.W {
			if track, _ := diagramBarRects(box, width, v.diagX); track.Contains(x, y) {
				return l, v, box, width, true
			}
		}
	}
	return nil, nil, ui.Rect{}, 0, false
}

func (e *Editor) diagramToggleAt(x, y float32) *leaf {
	for _, l := range e.leaves {
		if !isMermaid(l) || l.view == nil || l.view.reveal {
			continue
		}
		if e.opts.OnDiagramPreview != nil && l.view.diagramModel != nil && diagramPreviewControlRect(l, e.lk.size).Contains(x, y) {
			continue
		}
		if y >= l.y && y <= l.y+e.lk.size*1.4 && x >= l.x+l.w-e.lk.size*6 && x <= l.x+l.w {
			return l
		}
	}
	return nil
}

func (e *Editor) diagramPreviewAt(x, y float32) *leaf {
	if e.opts.OnDiagramPreview == nil {
		return nil
	}
	for _, l := range e.leaves {
		if isMermaid(l) && l.view != nil && l.view.diagramModel != nil && !l.view.reveal && diagramPreviewControlRect(l, e.lk.size).Contains(x, y) {
			return l
		}
	}
	return nil
}

func (e *Editor) taskBoxAt(x, y float32) (cont, bool) {
	for _, l := range e.leaves {
		if y < l.y-4 || y > l.y+e.lk.size*e.lk.lh+4 {
			continue
		}
		for _, k := range l.firstOf {
			c := l.conts[k]
			if c.kind != cItem || c.task == syntax.IncrementalNoTask || e.itemPrefixOpen(l) {
				continue
			}
			x0, width := e.colX+e.indentOf(l.conts[:k]), e.indentOf(l.conts[k:k+1])
			if x >= x0 && x <= x0+width {
				return c, true
			}
		}
	}
	return cont{}, false
}

func (e *Editor) codeLabelAt(x, y float32) *leaf {
	for _, l := range e.leaves {
		if l.kind != kCode || !l.fenced || l.view == nil || l.view.reveal {
			continue
		}
		if y >= l.y && y <= l.y+e.lk.size*1.4 && x >= l.x+l.w-e.lk.size*6 && x <= l.x+l.w {
			if l.language() != "" || e.leafAt(e.Caret()) == l {
				return l
			}
		}
	}
	return nil
}

func (e *Editor) pressDiagramBar(l *leaf, v *leafView, box ui.Rect, contentW float32, x, y float32) {
	track, thumb := diagramBarRects(box, contentW, v.diagX)
	if !thumb.Contains(x, y) {
		if travel := track.W - thumb.W; travel > 0 {
			at := min(max((x-track.X-thumb.W/2)/travel, 0), 1)
			v.diagX = at * (contentW - box.W)
		}
	}
	e.diagDrag = &diagDrag{leaf: l, start: x, from: v.diagX}
	e.invalidate()
}

func (e *Editor) dragDiagramPan(x float32) {
	d := e.diagPan
	if d == nil || d.leaf.view == nil || d.leaf.view.diagramModel == nil {
		return
	}
	v := d.leaf.view
	box, contentW := e.diagramBox(d.leaf, v, 0, 0)
	v.diagX = min(max(d.from-(x-d.start), 0), max(contentW-box.W, 0))
	e.invalidate()
}

func (e *Editor) dragDiagramBar(x float32) {
	d := e.diagDrag
	if d == nil || d.leaf.view == nil || d.leaf.view.diagramModel == nil {
		return
	}
	v := d.leaf.view
	box, contentW := e.diagramBox(d.leaf, v, 0, 0)
	maxOff := contentW - box.W
	if maxOff <= 0 {
		return
	}
	track, thumb := diagramBarRects(box, contentW, v.diagX)
	if travel := track.W - thumb.W; travel > 0 {
		v.diagX = min(max(d.from+(x-d.start)*maxOff/travel, 0), maxOff)
	}
	e.invalidate()
}

func (e *Editor) footnoteTargetAt(off int) (int, bool) {
	u := e.unitAt(off)
	if u == nil || u.id < 0 {
		return 0, false
	}
	for _, in := range e.p.Parser().Inlines(u.id) {
		if in.Kind == syntax.IncrementalFootnoteRef && in.Start <= off && off <= in.End {
			if block, ok := e.p.Parser().Footnote(in.Text); ok {
				return e.p.Parser().Block(block).Start, true
			}
			return 0, false
		}
	}
	return 0, false
}

// command runs an edit command of the menus.
func (e *Editor) command(cmd string) bool {
	switch cmd {
	case "copy":
		e.Copy()
	case "cut":
		e.Cut()
	case "paste":
		e.Paste(false)
	case "pasteAndMatchStyle":
		e.Paste(true)
	case "selectAll":
		e.SelectAll()
	case "undo":
		e.Undo()
	case "redo":
		e.Redo()
	case "delete":
		e.deleteSelection()
	case "bold":
		e.ToggleBold()
	case "italic":
		e.ToggleItalic()
	case "underline":
		e.ToggleUnderline()
	case "strikethrough":
		e.ToggleStrike()
	default:
		return false
	}
	return true
}

// SelectAll selects the whole document.
func (e *Editor) SelectAll() {
	e.finishBlockInput()
	e.block = nil
	e.clearTableSelectionSession()
	e.tableHandleDrag = nil
	e.dropPhantom()
	e.setSessionSelection(0, len(e.Source()))
	e.setSessionInclusive(false)
	e.setSessionLinewise(false)
	e.wantReveal = false
	e.invalidate()
}

// key handles a key pressed, and reports whether it took it.
func (e *Editor) key(event editorinput.KeyEvent) bool {
	key, mods := event.Key, event.Modifiers
	shift := mods&editorinput.ModShift != 0
	m := mods &^ editorinput.ModShift
	word := (isMac && m == editorinput.ModAlt) || (!isMac && m == editorinput.ModCtrl)
	if m == editorinput.ModCommand || m == editorinput.ModCommand|editorinput.ModAlt {
		if e.shortcut(event) {
			return true
		}
	}
	switch key {
	case editorinput.KeyLeft, editorinput.KeyRight:
		dir := 1
		if key == editorinput.KeyLeft {
			dir = -1
		}
		if m == 0 && (shift || e.tableSelection != nil || e.tableBridge != nil) {
			e.nativeHorizontal(dir, shift)
			return true
		}
		switch {
		case isMac && m == editorinput.ModCommand:
			s, z := e.lineEdges(e.Caret())
			if dir < 0 {
				e.setCaret(s, shift)
			} else {
				e.setCaret(z, shift)
			}
		case word:
			e.setCaret(e.step(e.Caret(), dir, true), shift)
		case m != 0:
			return false
		case e.Session.Selection().Anchor != e.Caret() && !shift:
			a, z := e.selection()
			if dir < 0 {
				e.setCaret(a, false)
			} else {
				e.setCaret(z, false)
			}
		default:
			if e.Session.Selection().Anchor == e.Caret() {
				e.nativeHorizontal(dir, shift)
			} else {
				e.setCaret(e.step(e.Caret(), dir, false), shift)
			}
		}
		e.hasGoal = false
		e.upstream = false
		return true
	case editorinput.KeyUp, editorinput.KeyDown:
		dir := 1
		if key == editorinput.KeyUp {
			dir = -1
		}
		if m == 0 && e.moveStructuralRow(dir, shift) {
			return true
		}
		if isMac && m == editorinput.ModCommand {
			if dir < 0 {
				e.setCaret(e.firstStop(), shift)
			} else {
				e.setCaret(e.lastStop(), shift)
			}
			return true
		}
		if m != 0 {
			return false
		}
		e.nativeVertical(dir, shift)
		return true
	case editorinput.KeyHome, editorinput.KeyEnd:
		if m == editorinput.ModCtrl || m == editorinput.ModCommand {
			if key == editorinput.KeyHome {
				e.setCaret(e.firstStop(), shift)
			} else {
				e.setCaret(e.lastStop(), shift)
			}
			return true
		}
		s, z := e.lineEdges(e.Caret())
		if key == editorinput.KeyHome {
			e.setCaret(s, shift)
			e.upstream = false
		} else {
			e.setCaret(z, shift)
			e.upstream = true
		}
		e.hasGoal = false
		return true
	case editorinput.KeyPageUp, editorinput.KeyPageDown:
		e.page(key == editorinput.KeyPageDown, shift)
		return true
	case editorinput.KeyBackspace:
		if isMac && m == editorinput.ModCommand {
			s, _ := e.lineEdges(e.Caret())
			if s < e.Caret() {
				e.setSessionAnchor(s)
				e.deleteSelection()
			}
			return true
		}
		e.backspace(word)
		return true
	case editorinput.KeyDelete:
		e.deleteForward(word)
		return true
	case editorinput.KeyEnter:
		if m == editorinput.ModCommand {
			if t, r, c := e.cellPos(e.Caret()); t != nil {
				_, _ = e.tableUI().Dispatch(tableui.Action{Kind: tableui.InsertRowAfter, LineStart: t.lineStart, First: r, Focus: tablemodel.Cell{Col: c}})
				return true
			}
			return false
		}
		if m != 0 {
			return false
		}
		e.enter(shift)
		return true
	case editorinput.KeyTab:
		if m != 0 {
			return false
		}
		return e.tab(shift)
	case editorinput.KeyEscape:
		if e.imageDrag != nil {
			e.imageDrag = nil
			e.assetGen.Add(1)
			e.invalidate()
			return true
		}
		if _, _, _, ok := e.activeInlineMath(); ok && !e.inlineMath.PreviewHidden() {
			e.inlineMath.SetPreviewHidden(true)
			return true
		}
		if e.Session.Selection().Anchor != e.Caret() {
			e.setSessionAnchor(e.Caret())
			return true
		}
		return false
	}
	return false
}

// shortcut runs the editor's own shortcut of Cmd (Ctrl) with key.
func (e *Editor) shortcut(event editorinput.KeyEvent) bool {
	command := mygoinputadapter.Shortcut(event, isMac)
	switch command {
	case mygoinputadapter.ShortcutNone:
		return false
	case mygoinputadapter.ShortcutToggleTask:
		e.ToggleList("- [ ]")
	case mygoinputadapter.ShortcutInsertCodeBlock:
		e.InsertCodeBlock()
	case mygoinputadapter.ShortcutInsertTable:
		e.InsertTable(3, 3)
	case mygoinputadapter.ShortcutToggleStrike:
		e.ToggleStrike()
	case mygoinputadapter.ShortcutUndo:
		e.Undo()
	case mygoinputadapter.ShortcutRedo:
		e.Redo()
	case mygoinputadapter.ShortcutSelectAll:
		e.SelectAll()
	case mygoinputadapter.ShortcutCopy:
		e.Copy()
	case mygoinputadapter.ShortcutCut:
		e.Cut()
	case mygoinputadapter.ShortcutPaste:
		e.Paste(event.Modifiers&editorinput.ModShift != 0)
	case mygoinputadapter.ShortcutToggleBold:
		e.ToggleBold()
	case mygoinputadapter.ShortcutToggleItalic:
		e.ToggleItalic()
	case mygoinputadapter.ShortcutToggleUnderline:
		e.ToggleUnderline()
	case mygoinputadapter.ShortcutToggleCode:
		e.ToggleCode()
	case mygoinputadapter.ShortcutToggleHighlight:
		e.ToggleHighlight()
	case mygoinputadapter.ShortcutInsertLink:
		e.InsertLink()
	case mygoinputadapter.ShortcutSelectWord:
		a, z := e.wordRangeAt(e.Caret())
		e.Select(a, z)
	case mygoinputadapter.ShortcutSetHeading:
		e.SetHeading(int(event.Key[0] - '0'))
	case mygoinputadapter.ShortcutShiftHeadingUp:
		e.ShiftHeading(1)
	case mygoinputadapter.ShortcutShiftHeadingDown:
		e.ShiftHeading(-1)
	case mygoinputadapter.ShortcutToggleQuote:
		e.ToggleQuote()
	case mygoinputadapter.ShortcutToggleOrderedList:
		e.ToggleList("1.")
	case mygoinputadapter.ShortcutToggleUnorderedList:
		e.ToggleList("-")
	case mygoinputadapter.ShortcutInsertMathBlock:
		e.InsertMathBlock()
	case mygoinputadapter.ShortcutInsertTableRow:
		t, r, c := e.cellPos(e.Caret())
		if t == nil {
			return false
		}
		_, _ = e.tableUI().Dispatch(tableui.Action{Kind: tableui.InsertRowAfter, LineStart: t.lineStart, First: r, Focus: tablemodel.Cell{Col: c}})
	}
	return true
}
