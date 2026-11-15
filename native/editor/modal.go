package mdedit

import (
	"github.com/egoist/mygo/ui"
	"github.com/go-text/typesetting/segmenter"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// modalState implements a deliberately small modal subset, not a Vim emulator.
// Printable commands are consumed ONLY from text events, never KeyDown.
type modalState struct {
	keymap, mode, pending string
	count, opCount        int
	line                  bool
	// vimTextVisualActive tracks only lowercase Vim `v` selection origin.
	vimTextVisualActive   bool
	register              string
	registerValid         bool
	registerLine          bool
	registerTable         *tableRegister
	registerBlock         []string
	blockInsert           *tableBlockInsert
	findChar              string
	findDir               int
	findTill              bool
	findValid             bool
	charCount             int
	awaitingCharDirection int
	awaitingCharTill      bool
	awaitingReplacement   bool
	prefixAt              time.Time
	searching             bool
	searchDir             int
	searchQuery           string
	searchStartCaret      int
	searchStartAnchor     int
	searchStartInclusive  bool
	searchStartLinewise   bool
	searchInputFocused    bool
	lastSearch            string
	lastSearchDir         int
}

func (e *Editor) SetKeymap(s string) {
	e.smoothCursorActive = false
	e.smoothCursorNavigation = false
	e.smoothCursorRectsValid = false
	e.smoothCursorFromSet = false
	e.clearVimImageRow()
	e.clearVimSearchHighlights()
	e.vimYankRanges, e.vimYankActive = nil, false
	e.finishBlockInput()
	if s != "vim" && s != "helix" {
		s = "default"
	}
	e.modal = modalState{keymap: s, mode: "Normal"}
	e.block, e.blockInput = nil, nil
	e.selectionInclusive = false
	e.selectionLinewise = false
	e.breakCoalescing()
	e.invalidate()
}
func (e *Editor) Keymap() string {
	if e.modal.keymap == "" {
		return "default"
	}
	return e.modal.keymap
}
func (e *Editor) ModeLabel() string {
	if e.Keymap() == "default" {
		return ""
	}
	label := e.Keymap() + " " + e.modal.mode
	if e.modal.opCount > 0 {
		label += " " + strconv.Itoa(e.modal.opCount)
	}
	label += e.modal.pending
	if e.modal.count > 0 {
		label += strconv.Itoa(e.modal.count)
	}
	return label
}

// ModalAcceptsText must guard all native composition/replacement entry points.
func (e *Editor) ModalAcceptsText() bool { return e.Keymap() == "default" || e.modal.mode == "Insert" }
func (e *Editor) modalKey(k ui.Key, m ui.Modifiers) bool {
	if e.Keymap() == "default" || m&ui.Cmd != 0 {
		return false
	}
	if e.modal.searching {
		switch k {
		case ui.KeyEscape:
			e.ApplyOperation(EditorOperation{Kind: OperationCancelSearch})
			return true
		case ui.KeyEnter:
			e.ApplyOperation(EditorOperation{Kind: OperationSubmitSearch})
			return true
		case ui.KeyBackspace:
			e.modalSearchBackspace()
			return true
		default:
			if m&(ui.Alt|ui.Ctrl) != 0 {
				return true
			}
		}
	}
	if k == ui.KeyR && m == ui.Ctrl {
		e.Redo()
		e.modal.mode = "Normal"
		e.modal.line = false
		e.selectionInclusive = false
		e.selectionLinewise = false
		e.anchor = e.caret
		return true
	}
	if k == ui.KeyV && m == ui.Ctrl {
		e.caret = e.selectionFocus()
		if t, row, col, ok := e.tableRegionAtCaret(); ok {
			e.tableBridge = nil
			e.tableNavigation = nil
			e.modal.mode = "BlockVisual"
			e.modal.line = false
			e.modal.pending = ""
			e.modal.count = 0
			e.anchor = e.caret
			e.block = nil
			e.selectTableCellRegion(t, row, col, row, col)
			e.selectionInclusive = false
			e.selectionLinewise = false
			e.invalidate()
			return true
		}
		e.tableBridge = nil
		e.tableNavigation = nil
		e.modal.mode = "BlockVisual"
		e.block = nil
		e.startBlockSelection()
		e.modal.line = false
		e.selectionInclusive = false
		e.selectionLinewise = false
		e.modal.pending = ""
		e.modal.count = 0
		e.anchor = e.caret
		e.tableSelection = nil
		e.invalidate()
		return true
	}
	if k == ui.KeyEscape {
		e.modal.vimTextVisualActive = false
		if e.vimHighlightsActive {
			e.clearVimSearchHighlights()
		}
		if e.hist.open != nil {
			e.commit(e.caret, e.caret)
		}
		e.caret = e.selectionFocus()
		if e.vimImageRow != nil && e.caret != e.vimImageRow.lineStart {
			e.clearVimImageRow()
		}
		e.tableNavigation = nil
		e.compose = ""
		e.composing = false
		e.modal.mode = "Normal"
		e.selectionInclusive = false
		e.selectionLinewise = false
		e.modal.pending = ""
		e.modal.awaitingCharDirection, e.modal.awaitingCharTill = 0, false
		e.modal.awaitingReplacement = false
		e.modal.prefixAt = time.Time{}
		e.modal.count = 0
		e.modal.opCount = 0
		e.modal.line = false
		e.clearTableSelectionSession()
		e.modal.blockInsert = nil
		e.block = nil
		e.blockInput = nil
		e.anchor = e.caret
		e.breakCoalescing()
		return true
	}
	if e.blockInput != nil {
		switch k {
		case ui.KeyBackspace:
			e.editBlockInputDelete(true)
			return true
		case ui.KeyDelete:
			e.editBlockInputDelete(false)
			return true
		case ui.KeyEnter:
			e.typeBlockInput("\n")
			return true
		case ui.KeyTab:
			e.typeBlockInput("\t")
			return true
		}
		e.finishBlockInput()
	}
	if e.ModalAcceptsText() {
		return false
	}
	if m&(ui.Alt|ui.Ctrl) != 0 {
		return true
	}
	switch k {
	case ui.KeyBackspace, ui.KeyDelete, ui.KeyEnter, ui.KeyTab:
		return true
	}
	// Let printable keys reach the text callback (preserves case and keyboard layout).
	switch k {
	case ui.KeyLeft:
		return e.modalText("h")
	case ui.KeyRight:
		return e.modalText("l")
	case ui.KeyUp:
		return e.modalText("k")
	case ui.KeyDown:
		return e.modalText("j")
	}
	return false
}
func (e *Editor) modalLine(at int) (int, int) {
	return e.sourceLineRange(at)
}

func (e *Editor) modalHorizontal(direction int, extend bool) {
	if e.moveTableCellRegion(0, direction) {
		return
	}
	if e.Keymap() == "vim" {
		e.ApplyOperation(EditorOperation{
			Kind: OperationMoveHorizontal, Direction: direction,
			Extend: extend, WithinLine: true,
		})
		e.promoteVisualTextToCellRegion()
		return
	}
	e.nativeHorizontal(direction, extend)
	e.promoteVisualTextToCellRegion()
}

func (e *Editor) modalEmptyLineAt(off int) bool {
	u := e.unitAt(off)
	return u != nil && (u.kind == kBlank || u.kind == kPhantom)
}

func (e *Editor) modalApply(op string, a, z int) {
	if a > z {
		a, z = z, a
	}
	e.modal.setTextRegister(e.src[a:z], e.modal.line || e.selectionLinewise)
	e.modal.line = false
	e.selectionInclusive = false
	e.selectionLinewise = false
	e.anchor, e.caret = a, z
	if op == "y" {
		e.Copy()
		e.startVimYankFlash([]Range{{Start: a, End: z}})
		e.anchor, e.caret = a, a
		return
	}
	e.begin(stepEdit)
	e.replace(a, z, "")
	e.commit(a, a)
	if op == "c" {
		e.modal.mode = "Insert"
	}
}

func (m *modalState) setTextRegister(text string, linewise bool) {
	m.register, m.registerValid, m.registerLine = text, true, linewise
	m.registerTable, m.registerBlock = nil, nil
}

func (m *modalState) setBlockRegister(rows []string) {
	m.registerBlock = append(m.registerBlock[:0], rows...)
	m.register, m.registerValid, m.registerLine = "", false, false
	m.registerTable = nil
}

func (m *modalState) setTableRegister(reg *tableRegister) {
	m.registerTable = reg
	m.registerBlock = nil
	m.register, m.registerValid, m.registerLine = "", false, false
	if reg != nil {
		m.register, m.registerValid, m.registerLine = reg.markdown(), true, true
	}
}

func linewiseRegisterPayload(text string, count int) string {
	lines := strings.Split(strings.TrimSuffix(text, "\n"), "\n")
	count = max(1, count)
	var repeated []string
	for range count {
		repeated = append(repeated, lines...)
	}
	return strings.Join(repeated, "\n")
}

func (e *Editor) putRegister(before bool, count int) bool {
	m := &e.modal
	if len(m.registerBlock) > 0 {
		e.pasteBlockRegister(m.registerBlock, before, count)
		return true
	}
	if m.registerTable != nil {
		for i := 0; i < count; i++ {
			if !e.pasteTableRegister(before) {
				return i > 0
			}
		}
		return true
	}
	if m.registerValid || m.register != "" {
		at := e.caret
		value := strings.Repeat(m.register, count)
		if m.registerLine {
			value = linewiseRegisterPayload(m.register, count)
			if before {
				at, _ = e.modalLine(at)
				value += "\n"
			} else {
				_, at = e.modalLine(at)
				if at < len(e.src) {
					at++
					value += "\n"
				} else {
					value = "\n" + value
				}
			}
		} else {
			_, lineEnd := e.modalLine(at)
			if at < lineEnd {
				at = e.nextGraphemeBoundary(at)
			}
		}
		e.begin(stepEdit)
		e.replace(at, at, value)
		e.commit(at, at)
		return true
	}
	e.anchor = e.caret
	e.Paste(false)
	return true
}

// modalGrapheme returns the first extended grapheme in a text event. A
// character command may legitimately receive multiple runes (for example a
// base letter followed by a combining mark), while all other printable
// commands remain single-rune text events.
func modalGrapheme(s string) string {
	var seg segmenter.Segmenter
	_ = seg.InitWithBytes([]byte(s))
	it := seg.GraphemeIterator()
	if !it.Next() {
		return ""
	}
	g := it.Grapheme()
	return s[g.OffsetInBytes : g.OffsetInBytes+g.LengthInBytes]
}

func (e *Editor) modalStartSearch(dir int) {
	e.clearVimSearchHighlights()
	e.modal.searching = true
	e.modal.searchDir = dir
	e.modal.searchQuery = ""
	e.modal.searchStartCaret = e.caret
	e.modal.searchStartAnchor = e.anchor
	e.modal.searchStartInclusive = e.selectionInclusive
	e.modal.searchStartLinewise = e.selectionLinewise
	e.modal.mode = "Normal"
	e.breakCoalescing()
	e.invalidate()
}

func (e *Editor) modalCancelSearch() {
	m := &e.modal
	m.searching = false
	m.searchQuery = ""
	m.mode = "Normal"
	e.caret, e.anchor = m.searchStartCaret, m.searchStartAnchor
	e.selectionInclusive, e.selectionLinewise = m.searchStartInclusive, m.searchStartLinewise
	e.clearVimSearchHighlights()
	e.breakCoalescing()
	e.Focus()
	e.invalidate()
}

func (e *Editor) modalSearchBackspace() {
	if e.compose != "" {
		e.compose = trimLastGrapheme(e.compose)
		e.composing = e.compose != ""
		e.invalidate()
		return
	}
	q := e.modal.searchQuery
	if q == "" {
		return
	}
	e.modal.searchQuery = trimLastGrapheme(q)
	e.modalRefreshSearchPreview()
	e.invalidate()
}

func trimLastGrapheme(s string) string {
	if s == "" {
		return ""
	}
	var seg segmenter.Segmenter
	_ = seg.InitWithBytes([]byte(s))
	it := seg.GraphemeIterator()
	end := 0
	for it.Next() {
		g := it.Grapheme()
		end = g.OffsetInBytes
	}
	return s[:end]
}

func (e *Editor) modalSubmitSearch() {
	m := &e.modal
	query := m.searchQuery
	if query != "" {
		m.lastSearch, m.lastSearchDir = query, m.searchDir
	} else {
		query = m.lastSearch
		if query != "" {
			m.lastSearchDir = m.searchDir
		}
	}
	matchAt, matchFound := -1, false
	if query != "" {
		if at, ok := e.modalSearch(query, m.searchDir, 1, m.searchStartCaret); ok {
			matchAt, matchFound = at, true
			e.nativeSetCaret(at, false)
			e.selectionInclusive, e.selectionLinewise = false, false
			m.mode = "Normal"
		} else {
			e.caret, e.anchor = m.searchStartCaret, m.searchStartAnchor
			e.selectionInclusive, e.selectionLinewise = m.searchStartInclusive, m.searchStartLinewise
		}
		e.modalApplySearchHighlights(query, matchAt, matchFound)
	}
	m.searching = false
	m.searchQuery = ""
	m.mode = "Normal"
	e.Focus()
	e.invalidate()
}

func (e *Editor) modalRepeatSearch(reverse bool, count int) {
	dir := e.modal.lastSearchDir
	if reverse {
		dir = -dir
	}
	if e.modal.lastSearch == "" || dir == 0 {
		return
	}
	if at, ok := e.modalSearch(e.modal.lastSearch, dir, count, e.caret); ok {
		e.nativeSetCaret(at, false)
		e.selectionInclusive, e.selectionLinewise = false, false
		e.modal.mode = "Normal"
		e.modalApplySearchHighlights(e.modal.lastSearch, at, true)
	}
}

func (e *Editor) modalSearchRanges(query string) []Range {
	if query == "" {
		return nil
	}
	edges := e.graphemeEdges()
	queryClusters := modalQueryGraphemeCount(query)
	sensitive := modalQuerySmartCase(query)
	var out []Range
	nextAllowed := 0
	for _, at := range edges {
		if at < nextAllowed {
			continue
		}
		if end, ok := e.modalSearchMatchEnd(at, query, queryClusters, sensitive); ok {
			out = append(out, Range{Start: at, End: end})
			nextAllowed = end
			if len(out) == 500 {
				break
			}
		}
	}
	return out
}

func modalQuerySmartCase(query string) bool {
	for _, r := range query {
		if unicode.IsUpper(r) {
			return true
		}
	}
	return false
}

func modalQueryGraphemeCount(query string) int {
	var seg segmenter.Segmenter
	_ = seg.InitWithBytes([]byte(query))
	it := seg.GraphemeIterator()
	n := 0
	for it.Next() {
		n++
	}
	return n
}

func (e *Editor) modalSearchMatchEnd(at int, query string, count int, sensitive bool) (int, bool) {
	edges := e.graphemeEdges()
	start := sort.SearchInts(edges, at)
	endIndex := start + count
	if start >= len(edges) || edges[start] != at || count == 0 || endIndex >= len(edges) {
		return 0, false
	}
	end := edges[endIndex]
	text := e.src[at:end]
	if sensitive {
		return end, text == query
	}
	return end, strings.EqualFold(text, query)
}

func (e *Editor) modalApplySearchHighlights(query string, at int, found bool) {
	ranges := e.modalSearchRanges(query)
	current := -1
	if found {
		for i, r := range ranges {
			if r.Start == at {
				current = i
				break
			}
		}
	}
	e.setVimSearchHighlights(ranges, current)
}

func (e *Editor) modalRefreshSearchPreview() {
	m := &e.modal
	if !m.searching {
		return
	}
	query := m.searchQuery
	at, found := e.modalSearch(query, m.searchDir, 1, m.searchStartCaret)
	e.modalApplySearchHighlights(query, at, found)
}

func (e *Editor) modalSearch(query string, dir, count, from int) (int, bool) {
	if query == "" || len(e.src) == 0 {
		return from, false
	}
	edges := e.graphemeEdges()
	queryClusters := modalQueryGraphemeCount(query)
	sensitive := modalQuerySmartCase(query)
	find := func(start, end int, forward bool) (int, bool) {
		if forward {
			for _, at := range edges {
				if at < start || at > end {
					continue
				}
				if _, ok := e.modalSearchMatchEnd(at, query, queryClusters, sensitive); ok {
					return at, true
				}
			}
		} else {
			for i := len(edges) - 1; i >= 0; i-- {
				at := edges[i]
				if at < start || at > end {
					continue
				}
				if _, ok := e.modalSearchMatchEnd(at, query, queryClusters, sensitive); ok {
					return at, true
				}
			}
		}
		return 0, false
	}
	for i := 0; i < max(1, count); i++ {
		var at int
		var ok bool
		if dir >= 0 {
			at, ok = find(from+1, len(e.src), true)
			if !ok {
				at, ok = find(0, from, true)
			}
		} else {
			at, ok = find(0, from-1, false)
			if !ok {
				at, ok = find(from, len(e.src), false)
			}
		}
		if !ok {
			return from, false
		}
		from = at
	}
	return from, true
}

func (e *Editor) modalFind(char string, dir int, till bool, count int) (int, bool) {
	if char == "" || e.caret < 0 || e.caret > len(e.src) {
		return e.caret, false
	}
	lineStart, lineEnd := e.modalLine(e.caret)
	from := e.caret
	for i := 0; i < max(1, count); i++ {
		at := -1
		if dir > 0 {
			for p := e.nextGraphemeBoundary(from); p < lineEnd; p = e.nextGraphemeBoundary(p) {
				if e.src[p:e.nextGraphemeBoundary(p)] == char {
					at = p
					break
				}
			}
		} else {
			for p := e.previousGraphemeBoundary(from); p >= lineStart && p < from; p = e.previousGraphemeBoundary(p) {
				if e.src[p:e.nextGraphemeBoundary(p)] == char {
					at = p
					break
				}
			}
		}
		if at < 0 {
			return e.caret, false
		}
		from = at
		if i == max(1, count)-1 {
			if till {
				if dir > 0 {
					return e.previousGraphemeBoundary(at), true
				}
				return e.nextGraphemeBoundary(at), true
			}
			return at, true
		}
	}
	return e.caret, false
}

func (e *Editor) modalReplace(count int, replacement string) {
	replacement = modalGrapheme(replacement)
	if replacement == "" {
		e.modal.pending = ""
		return
	}
	start := e.caret
	_, lineEnd := e.modalLine(start)
	if _, _, virtual := e.vimVirtualImageLineRange(); virtual {
		e.modal.pending = ""
		return
	}
	visual := e.modal.mode == "Visual" || e.modal.mode == "Select"
	if visual {
		start, _ = e.selection()
		_, lineEnd = e.modalLine(start)
	}
	end := start
	limit := max(1, count)
	if visual {
		_, end = e.selection()
		if e.selectionInclusive && end < len(e.src) {
			end = e.nextGraphemeBoundary(end)
		}
		limit = 0
	}
	for i := 0; i < limit && end < lineEnd; i++ {
		next := e.nextGraphemeBoundary(end)
		if next <= end || next > lineEnd {
			break
		}
		// Keep table cell separators intact: replacing one would change the
		// document structure rather than replace a cell's text.
		if e.src[end:next] == "|" {
			break
		}
		end = next
	}
	if end == start {
		e.modal.pending = ""
		return
	}
	if strings.Contains(e.src[start:end], "\n") || strings.Contains(e.src[start:end], "|") || replacement == "\n" || replacement == "|" {
		e.modal.pending = ""
		return
	}
	count = 0
	for p := start; p < end; p = e.nextGraphemeBoundary(p) {
		count++
	}
	value := strings.Repeat(replacement, count)
	e.begin(stepEdit)
	e.replace(start, end, value)
	e.commit(start, start+len(value)-len(replacement))
	e.modal.mode = "Normal"
	e.modal.line = false
	e.selectionInclusive = false
	e.selectionLinewise = false
	e.anchor = e.caret
	e.modal.pending = ""
}

func (e *Editor) modalText(s string) bool {
	previousNavigation := e.smoothCursorNavigation
	e.smoothCursorNavigation = e.modalNavigationCommand(s)
	defer func() { e.smoothCursorNavigation = previousNavigation }()
	if e.modal.searching {
		e.modal.searchQuery += s
		e.modalRefreshSearchPreview()
		e.invalidate()
		return true
	}
	if e.ModalAcceptsText() {
		return false
	}
	defer e.invalidate()
	// IME commits and multi-character payloads must never be interpreted as commands.
	if utf8.RuneCountInString(s) != 1 && !e.modal.awaitingReplacement && e.modal.awaitingCharDirection == 0 {
		return true
	}
	m := &e.modal
	if m.awaitingReplacement {
		m.awaitingReplacement = false
		e.ApplyOperation(EditorOperation{Kind: OperationReplaceCharacter, Count: m.charCount, Character: s})
		m.charCount = 0
		m.count = 0
		return true
	}
	if m.awaitingCharDirection != 0 {
		dir, till, count := m.awaitingCharDirection, m.awaitingCharTill, m.charCount
		m.awaitingCharDirection, m.awaitingCharTill = 0, false
		m.charCount, m.count = 0, 0
		e.ApplyOperation(EditorOperation{Kind: OperationFindCharacter, Character: s, Direction: dir, Till: till, Count: count})
		return true
	}
	c := s
	if c != "h" && c != "l" && c != "j" && c != "k" && !(c >= "0" && c <= "9") {
		e.tableNavigation = nil
	}
	if c >= "0" && c <= "9" && (c != "0" || m.count > 0) {
		m.count = min(9999, m.count*10+int(c[0]-'0'))
		return true
	}
	n := max(1, m.count)
	opN := max(1, m.opCount)
	m.count = 0
	if m.pending == "g" {
		m.pending = ""
		if c == "g" {
			e.nativeSetCaret(0, m.mode == "Visual" || m.mode == "Select")
		} else if c == "j" || c == "k" {
			dir := 1
			if c == "k" {
				dir = -1
			}
			e.nativeVertical(dir, m.mode == "Visual" || m.mode == "Select" || m.mode == "BlockVisual")
		}
		return true
	}
	if m.pending == "z" {
		if time.Since(m.prefixAt) > 900*time.Millisecond {
			m.pending, m.prefixAt = "", time.Time{}
		} else {
			m.pending, m.prefixAt = "", time.Time{}
			var op EditorOperation
			switch c {
			case "t":
				op = EditorOperation{Kind: OperationScrollCaretLine, Placement: CaretLineTop}
			case "z":
				op = EditorOperation{Kind: OperationScrollCaretLine, Placement: CaretLineCenter}
			case "b":
				op = EditorOperation{Kind: OperationScrollCaretLine, Placement: CaretLineBottom}
			case "c":
				op = EditorOperation{Kind: OperationFoldAtCaret, Fold: FoldClose}
			case "C":
				op = EditorOperation{Kind: OperationFoldAtCaret, Fold: FoldCloseRecursive}
			case "o":
				op = EditorOperation{Kind: OperationFoldAtCaret, Fold: FoldOpen}
			case "O":
				op = EditorOperation{Kind: OperationFoldAtCaret, Fold: FoldOpenRecursive}
			case "a":
				op = EditorOperation{Kind: OperationFoldAtCaret, Fold: FoldToggle}
			case "A":
				op = EditorOperation{Kind: OperationFoldAtCaret, Fold: FoldToggleRecursive}
			case "v":
				op = EditorOperation{Kind: OperationRevealCaret}
			case "M":
				op = EditorOperation{Kind: OperationFoldAll, Fold: FoldCloseAll}
			case "R":
				op = EditorOperation{Kind: OperationFoldAll, Fold: FoldOpenAll}
			default:
				return true
			}
			e.ApplyOperation(op)
			return true
		}
	}
	if c != "j" && c != "k" {
		e.hasGoal = false
	}
	if m.mode == "BlockVisual" && e.block != nil {
		switch c {
		case "y", "d", "c":
			ranges := e.SelectionRanges()
			var blockRegister []string
			for _, r := range ranges {
				blockRegister = append(blockRegister, e.src[r.Start:r.End])
			}
			m.setBlockRegister(blockRegister)
			if c == "y" {
				e.Copy()
				e.startVimYankFlash(ranges)
				m.mode = "Normal"
				e.block = nil
				e.anchor = e.caret
				return true
			}
			at := e.caret
			if c == "c" {
				e.begin(stepEdit)
				points := make([]int, 0, len(ranges))
				shift := 0
				for _, r := range ranges {
					points = append(points, r.Start-shift)
					shift += r.End - r.Start
				}
				for i := len(ranges) - 1; i >= 0; i-- {
					e.replace(ranges[i].Start, ranges[i].End, "")
				}
				e.blockInput = &blockInput{points: points}
				e.sync()
				if len(points) > 0 {
					at = points[0]
				}
			} else {
				at = e.deleteBlock()
			}
			e.block = nil
			e.anchor, e.caret = at, at
			if c == "c" {
				m.mode = "Insert"
			} else {
				m.mode = "Normal"
			}
			return true
		case "I", "A":
			points := make([]int, 0)
			for _, r := range e.SelectionRanges() {
				if c == "I" {
					points = append(points, r.Start)
				} else {
					points = append(points, r.End)
				}
			}
			e.blockInput = &blockInput{points: points}
			e.block = nil
			m.mode = "Insert"
			e.anchor = e.caret
			return true
		}
	}
	if m.pending != "" && c == m.pending {
		op := m.pending
		m.pending = ""
		a, z := e.modalLine(e.caret)
		for i := 0; i < n*max(1, m.opCount); i++ {
			if z < len(e.src) {
				z++
				if i+1 < n*max(1, m.opCount) {
					_, z = e.modalLine(z)
				}
			}
		}
		m.opCount = 0
		m.line = true
		if op == "c" && z > a && e.src[z-1] == '\n' {
			z--
		}
		e.modalApply(op, a, z)
		return true
	}
	extend := m.mode == "Visual" || m.mode == "Select" || m.mode == "BlockVisual" || m.pending != ""
	start := e.caret
	motion := true
	for i := 0; i < n*max(1, m.opCount); i++ {
		switch c {
		case "h":
			e.modalHorizontal(-1, extend)
		case "l":
			e.modalHorizontal(1, extend)
		case "j":
			if e.moveTableCellRegion(1, 0) {
			} else if m.line {
				e.nativeMoveLineSelection(1, 1)
			} else if m.mode == "BlockVisual" && e.moveTableVisualColumn(1, 1) {
			} else if !e.moveStructuralRow(1, extend) {
				e.nativeVertical(1, extend)
			}
		case "k":
			if e.moveTableCellRegion(-1, 0) {
			} else if m.line {
				e.nativeMoveLineSelection(-1, 1)
			} else if m.mode == "BlockVisual" && e.moveTableVisualColumn(-1, 1) {
			} else if !e.moveStructuralRow(-1, extend) {
				e.nativeVertical(-1, extend)
			}
		case "w", "W":
			e.nativeSetCaret(e.stepWithWordMode(e.caret, 1, true, c == "W"), extend)
		case "b", "B":
			e.nativeSetCaret(e.stepWithWordMode(e.caret, -1, true, c == "B"), extend)
		case "e", "E":
			bigWord := c == "E"
			z := e.wordEndAtMode(e.caret, bigWord, false)
			if i > 0 {
				if next := e.nextWordEndAcrossUnitsMode(e.caret, bigWord); next > e.caret {
					z = next
				}
			} else if z <= e.caret || m.pending == "" && e.previousGraphemeBoundary(z) <= e.caret {
				// Vim `e` advances to the next word when the caret is already
				// on the current word's final grapheme, and skips empty source
				// leaves because they have no rendered word end.
				if next := e.nextWordEndAcrossUnitsMode(e.caret, bigWord); next > e.caret {
					z = next
				}
			}
			if m.pending == "" && z > e.caret {
				z = e.previousGraphemeBoundary(z)
			}
			e.nativeSetCaret(z, extend)
		case "0":
			a := e.nativeLineStart(e.caret, false)
			e.nativeSetCaret(a, extend)
		case "^":
			a := e.nativeLineStart(e.caret, true)
			e.nativeSetCaret(a, extend)
		case "$":
			_, z := e.modalLine(e.caret)
			if m.pending == "" {
				lineStart, _ := e.modalLine(e.caret)
				if z > lineStart {
					z = e.previousGraphemeBoundary(z)
				}
			}
			e.nativeSetCaret(z, extend)
		case "G":
			e.nativeSetCaret(len(e.src), extend)
		default:
			motion = false
		}
		if extend {
			e.promoteVisualTextToCellRegion()
		}
	}
	if m.mode == "BlockVisual" && motion {
		if (c == "j" || c == "k") && e.hasGoal {
			e.blockChangedVertical(e.goalX)
		} else {
			e.blockChanged()
		}
	}
	if motion {
		if m.pending != "" {
			op := m.pending
			m.pending = ""
			m.opCount = 0
			e.modalApply(op, start, e.caret)
		} else if e.Keymap() == "helix" && !extend {
			e.anchor = start
		}
		return true
	}
	m.opCount = 0
	switch c {
	case "f", "F", "t", "T":
		if e.Keymap() == "vim" {
			dir, till := 1, false
			switch c {
			case "F":
				dir = -1
			case "t":
				till = true
			case "T":
				dir, till = -1, true
			}
			e.ApplyOperation(EditorOperation{Kind: OperationBeginCharacterFind, Direction: dir, Till: till, Count: n * opN})
		}
	case ";", ",":
		if e.Keymap() == "vim" {
			e.ApplyOperation(EditorOperation{Kind: OperationRepeatCharacterFind, Reverse: c == ",", Count: n})
		}
	case "r":
		if e.Keymap() == "vim" {
			e.ApplyOperation(EditorOperation{Kind: OperationBeginReplace, Count: n * opN})
		}
	case "/", "?":
		if e.Keymap() == "vim" {
			dir := 1
			if c == "?" {
				dir = -1
			}
			e.ApplyOperation(EditorOperation{Kind: OperationBeginSearch, Direction: dir})
		}
	case "n":
		if e.Keymap() == "vim" {
			e.ApplyOperation(EditorOperation{Kind: OperationRepeatSearch, Count: n})
		}
	case "N":
		if e.Keymap() == "vim" {
			e.ApplyOperation(EditorOperation{Kind: OperationRepeatSearch, Reverse: true, Count: n})
		}
	case "z":
		if e.Keymap() == "vim" {
			m.pending, m.prefixAt = "z", time.Now()
		}
	case "g":
		m.pending = "g"
	case "i", "a":
		focus := e.nativePrepareInsert()
		if c == "a" && !e.modalEmptyLineAt(focus) {
			e.caret = e.step(focus, 1, false)
		}
		e.anchor = e.caret
		m.mode = "Insert"
		e.breakCoalescing()
	case "I":
		focus := e.nativePrepareInsert()
		e.caret = e.nativeLineStart(focus, true)
		e.anchor = e.caret
		m.mode = "Insert"
		e.breakCoalescing()
	case "A":
		focus := e.nativePrepareInsert()
		e.caret = e.nativeLineEndAt(focus)
		e.anchor = e.caret
		m.mode = "Insert"
		e.breakCoalescing()
		e.invalidate()
	case "o":
		e.nativePrepareInsert()
		e.nativeOpenLine(false)
		m.mode = "Insert"
	case "O":
		e.nativePrepareInsert()
		e.nativeOpenLine(true)
		m.mode = "Insert"
	case "v":
		e.modal.vimTextVisualActive = e.Keymap() == "vim"
		e.caret = e.selectionFocus()
		e.tableSelection = nil
		e.tableBridge = nil
		m.line = false
		if e.Keymap() == "helix" {
			m.mode = "Select"
			e.selectionInclusive = false
			e.selectionLinewise = false
		} else {
			m.mode = "Visual"
			e.selectionInclusive = e.vimImageRow == nil
			e.selectionLinewise = e.vimImageRow != nil
		}
		e.anchor = e.caret
	case "V":
		e.modal.vimTextVisualActive = false
		e.caret = e.selectionFocus()
		e.tableSelection = nil
		e.tableBridge = nil
		m.mode = "Visual"
		m.line = true
		e.selectionInclusive = false
		e.selectionLinewise = true
		e.nativeSelectLine(e.caret)
		if n > 1 {
			e.nativeMoveLineSelection(1, n-1)
		}
	case "d", "c", "y":
		m.vimTextVisualActive = false
		if e.tableSelection != nil && e.tableBridge == nil && e.applyTableSelection(c) {
			if c != "c" {
				m.mode = "Normal"
			}
			return true
		}
		if e.Keymap() == "helix" || extend {
			a, z := e.selection()
			if m.line {
				// V mode keeps its complete source-line range in the native
				// selection throughout motion, so editing consumes it directly.
			}
			if a == z {
				z = e.step(z, 1, false)
			}
			e.modalApply(c, a, z)
			if c != "c" {
				m.mode = "Normal"
			}
		} else {
			m.pending = c
			m.opCount = n
		}
	case "p", "P":
		e.ApplyOperation(EditorOperation{Kind: OperationPutRegister, Count: n, Before: c == "P"})
	case "u":
		e.Undo()
		m.mode = "Normal"
		m.line = false
		e.selectionInclusive = false
		e.selectionLinewise = false
		e.anchor = e.caret
	case "x":
		if a, z, ok := e.vimVirtualImageLineRange(); ok {
			m.line = true
			e.modalApply("d", a, z)
		} else if a, z, ok := e.nativeSubstituteRange(n, false); ok {
			m.line = false
			e.modalApply("d", a, z)
		}
	case "X":
		if a, z, ok := e.vimVirtualImageLineRange(); ok {
			m.line = true
			e.modalApply("d", a, z)
		} else if a, z, ok := e.nativeSubstituteRange(n, true); ok {
			m.line = false
			e.modalApply("d", a, z)
		}
	case "D":
		if a, z, ok := e.vimVirtualImageLineRange(); ok {
			m.line = true
			e.modalApply("d", a, z)
		} else if a, z, ok := e.nativeLineRemainder(); ok {
			m.line = false
			e.modalApply("d", a, z)
		}
	case "C":
		if a, z, ok := e.vimVirtualImageLineRange(); ok {
			m.line = true
			e.modalApply("c", a, z)
		} else if a, z, ok := e.nativeLineRemainder(); ok {
			m.line = false
			e.modalApply("c", a, z)
		}
	}
	return true
}

func (e *Editor) modalNavigationCommand(s string) bool {
	if !e.opts.SmoothCursor || e.modal.mode != "Normal" || e.modal.searching {
		return false
	}
	if e.modal.pending == "g" {
		return s == "g" || s == "j" || s == "k"
	}
	if e.modal.pending != "" {
		return false
	}
	if e.modal.awaitingCharDirection != 0 {
		return true
	}
	switch s {
	case "h", "j", "k", "l", "w", "b", "e", "0", "^", "$", "G", ";", ",", "n", "N":
		return true
	default:
		return false
	}
}
