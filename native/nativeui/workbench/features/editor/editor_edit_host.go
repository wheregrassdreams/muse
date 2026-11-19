package editor

import (
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	mdedit "github.com/wheregrassdreams/muse-editor/markdown/edit"
	inlineblock "github.com/wheregrassdreams/muse-editor/markdown/inline"
	"github.com/wheregrassdreams/muse-editor/markdown/paragraph"
	syntax "github.com/wheregrassdreams/muse-editor/markdown/syntax"
	tablemodel "github.com/wheregrassdreams/muse-editor/markdown/table"
	mdtext "github.com/wheregrassdreams/muse-editor/text"
	tableui "muse/nativeui/workbench/features/editor/blocks/table"
)

// Editing works on the source, as Typora's editing reads: typing goes in
// at the caret, and the structure of the document follows from what the
// source becomes, so that "# " makes a heading and "- " a list. Keys that
// change structure (Enter, Backspace at the start of a block, Tab) write
// the Markdown that structure is, in the containers the caret is in.

// unitAt returns the unit the caret at off is in, and whether it is a
// table's cell.
func (e *Editor) unitAt(off int) *leaf {
	us := e.units()
	if i := e.unitIndex(us, off); i >= 0 {
		return us[i]
	}
	return nil
}

// leafOf returns the leaf a unit is, or the table a cell is in.
func (e *Editor) leafOf(u *leaf) *leaf {
	if t := e.tableOf(u); t != nil {
		return t
	}
	return u
}

// contPrefix returns what a continuation line of a leaf starts with, to
// stay in the leaf's containers: its first line's prefix with list markers
// blanked, "> " kept.
func (e *Editor) contPrefix(l *leaf) string {
	if l.isLine() {
		return phantomPrefix(l.conts)
	}
	end := min(max(l.syntaxStart, l.lineStart), len(e.Source()))
	return blankMarkers(e.Source()[l.lineStart:end])
}

// blankMarkers turns a line's prefix into a continuation line's: list
// markers and task boxes become spaces, quotation markers stay.
func blankMarkers(p string) string {
	return paragraph.BlankListMarkers(p)
}

// blankPrefix returns what a blank line inside a leaf's containers is:
// its quotation markers.
func blankPrefix(cont string) string {
	return paragraph.BlankParagraphPrefix(cont)
}

// phantomPrefix returns the prefix of a line in containers.
func phantomPrefix(conts []cont) string {
	kinds := make([]paragraph.ContainerKind, len(conts))
	for i, c := range conts {
		if c.kind == cQuote {
			kinds[i] = paragraph.QuoteContainer
		}
	}
	return paragraph.ContinuationPrefix(kinds)
}

// ---------------------------------------------------------------------------
// Typing

// typeText inserts text typed at the selection, as the keyboard and input
// methods type it: pairs of brackets close, a selection typed over with a
// bracket is wrapped, a third backtick opens a block of code.
func (e *Editor) typeText(text string) {
	if text == "" {
		return
	}
	defer e.updateWikiSuggestions()
	if e.blockInput != nil {
		e.typeBlockInput(text)
		return
	}
	if e.typeTableBlock(text) {
		return
	}
	if e.repairOrphanTable(text) {
		return
	}
	if e.tableSelection != nil {
		s := *e.tableSelection
		if s.axis == tableRows || s.axis == tableWhole {
			if e.applyTableSelection("c") {
				e.typeText(text)
				return
			}
		} else if s.axis == tableColumns {
			if e.typeTableColumnSelection(text) {
				return
			}
		}
	}
	e.layoutIfNeeded()
	if e.phantom != nil && e.Session.Selection().Anchor != e.Caret() {
		e.begin(stepEdit)
		e.dropPhantom()
	}
	e.trackBlankLineInput(text, stepTyping)
	if p := e.phantom; p != nil {
		e.begin(stepTyping)
		e.phantom = nil
		at := p.at
		ins, c := text, len(text)
		if block, k, ok := paragraph.BlockShortcut(text, phantomPrefix(p.conts)); ok {
			ins, c = block, k
		} else if r := []rune(text); len(r) == 1 {
			if closeCh, ok := inlineblock.PlanAutoPair(inlineblock.AutoPairInput{Open: r[0], Next: e.runeAfter(p.at)}); ok {
				ins = text + string(closeCh)
			}
		}
		e.replace(at, at, p.pre+ins+p.post)
		e.rememberInsertedBreaks(at, p.pre+ins+p.post)
		c += at + len(p.pre)
		e.commit(c, c)
		return
	}
	a, z := e.selection()
	u := e.unitAt(e.Caret())
	if t, _ := e.tableAtEdge(e.Caret()); t != nil {
		// The virtual stops are outside the table. Typing there should add
		// adjacent Markdown, without applying cell-specific rules.
		u = nil
	}
	inCode := u != nil && (u.kind == kCode || u.kind == kMath || u.kind == kHTML || u.kind == kRaw)
	cell := u != nil && e.isCell(u)
	if r := []rune(text); len(r) == 1 {
		ch := r[0]
		if cell && ch == '|' {
			text = `\|`
		} else if a != z {
			if closeCh, ok := inlineblock.WrapPairFor(ch, inCode); ok {
				e.begin(stepEdit)
				e.replace(z, z, string(closeCh))
				e.replace(a, a, string(ch))
				e.commit(a+len(string(ch)), z+len(string(ch)))
				return
			}
		} else {
			if e.skipCloser(ch) {
				return
			}
			if e.autoPair(ch, inCode) {
				return
			}
		}
	}
	if a == z && !inCode && !cell && e.blockShortcut(u, text) {
		return
	}
	kind := stepTyping
	if a != z {
		kind = stepEdit
	}
	e.begin(kind)
	e.replace(a, z, text)
	c := a + len(text)
	e.commit(c, c)
	if last, _ := utf8.DecodeLastRuneInString(text); unicode.IsSpace(last) {
		e.breakCoalescing()
	}
}

// repairOrphanTable turns a typed or pasted "| " on an empty logical line
// immediately before a GFM delimiter and data row into a matching blank
// header. The replacement is one ordinary source edit and therefore one undo.
func (e *Editor) repairOrphanTable(text string) bool {
	if text != "| " || e.Session.Selection().Anchor != e.Caret() {
		return false
	}
	lineStart := lineStartAt(e.Source(), e.Caret())
	prefix := e.Source()[lineStart:e.Caret()]
	for _, r := range prefix {
		if r != '>' && r != ' ' && r != '\t' {
			return false
		}
	}
	lineEnd := lineEndAt(e.Source(), e.Caret())
	if strings.TrimSpace(e.Source()[e.Caret():lineEnd]) != "" || lineEnd >= len(e.Source()) {
		return false
	}
	sepStart := lineEnd + 1
	sepEnd := lineEndAt(e.Source(), sepStart)
	dataStart := sepEnd + 1
	if dataStart >= len(e.Source()) {
		return false
	}
	dataEnd := lineEndAt(e.Source(), dataStart)
	data := e.Source()[dataStart:dataEnd]
	pipes := splitRow(e.Source(), [2]int{sepStart, sepEnd})
	if len(pipes) < 2 || !strings.Contains(data, "|") {
		return false
	}
	for i := 0; i+1 < len(pipes); i++ {
		cell := strings.TrimSpace(e.Source()[pipes[i]+1 : pipes[i+1]])
		cell = strings.Trim(cell, ":")
		if len(cell) < 1 || strings.Trim(cell, "-") != "" {
			return false
		}
	}
	var header strings.Builder
	header.WriteByte('|')
	for i := 0; i < len(pipes)-1; i++ {
		header.WriteString("   |")
	}
	e.begin(stepEdit)
	e.replace(e.Caret(), e.Caret(), header.String())
	caret := e.Caret() + 2
	e.commit(caret, caret)
	return true
}

// autoPair types an opening bracket and its closer, the caret between
// them, where nothing but space or a closer follows. It reports whether
// it did.
func (e *Editor) autoPair(ch rune, inCode bool) bool {
	closeCh, ok := inlineblock.PlanAutoPairAt(e.Source(), e.Caret(), ch, inCode)
	if !ok {
		return false
	}
	e.begin(stepTyping)
	s := string(ch) + string(closeCh)
	e.replace(e.Caret(), e.Caret(), s)
	c := e.Caret() - len(string(closeCh))
	e.commit(c, c)
	return true
}

// skipCloser moves the caret over the closer typed right before it. It
// reports whether it did.
func (e *Editor) skipCloser(ch rune) bool {
	if !inlineblock.ShouldSkipCloserAt(e.Source(), e.Caret(), ch) {
		return false
	}
	e.breakCoalescing()
	e.setCaret(e.Caret()+len(string(ch)), false)
	return true
}

func (e *Editor) runeAfter(off int) rune {
	if off >= len(e.Source()) {
		return 0
	}
	r := []rune(e.Source()[off:min(off+4, len(e.Source()))])
	if len(r) == 0 {
		return 0
	}
	return r[0]
}

// blockShortcut turns what is typed at the start of an empty paragraph
// into a block, as it is typed, so that the rest of the document is never
// a block of code or math for a moment: a third ` or ~ opens a fenced
// block of code, a second $ a block of math, with its closing fence. It
// reports whether it did.
func (e *Editor) blockShortcut(u *leaf, text string) bool {
	if u == nil {
		return false
	}
	ls := lineStartAt(e.Source(), e.Caret())
	contentStart := max(u.start, ls)
	plan, ok := paragraph.PlanBlockShortcut(paragraph.BlockShortcutInput{
		Source: e.Source(), Typed: text, LineStart: ls, ContentStart: contentStart,
		Caret: e.Caret(), LineEnd: lineEndAt(e.Source(), e.Caret()),
		ContainerPrefix: e.contPrefix(u), TextUnit: u.isText() && !(u.kind == kHeading && !u.pending),
		OpeningListItem: u.opensItem(),
	})
	if !ok {
		return false
	}
	e.begin(stepEdit)
	e.replace(plan.Start, plan.End, plan.Text)
	e.commit(plan.Caret, plan.Caret)
	return true
}

// ---------------------------------------------------------------------------
// Deleting

// deleteSelection deletes the selection, as one step of undo.
func (e *Editor) deleteSelection() {
	if e.block != nil {
		e.deleteBlock()
		e.block = nil
		return
	}
	if e.tableSelection != nil && e.tableBridge == nil && e.applyTableSelection("d") {
		return
	}
	if e.deleteMixedCellSelection() {
		return
	}
	// Never let a mixed structural range fall through to deletion of its
	// one-dimensional source envelope. Unsupported full-row/column mixed
	// shapes must wait for their structural edit plan rather than eat cells.
	if e.hasMixedSelectionParts() {
		return
	}
	a, z := e.selection()
	if a == z {
		return
	}
	e.begin(stepEdit)
	e.replace(a, z, "")
	e.commit(a, a)
	e.tidyEmpty()
}

func (e *Editor) hasMixedSelectionParts() bool {
	hasText, hasStructure := false, false
	for _, part := range e.selectionParts() {
		if part.structure == nil {
			hasText = true
		} else {
			hasStructure = true
		}
	}
	return hasText && hasStructure
}

// deleteMixedCellSelection removes external text ranges and a partial cell
// rectangle as one undo step. Structural row/column/table projections remain
// owned by applyTableSelection; they must not fall through to source-envelope
// deletion, which would include unselected cells.
func (e *Editor) deleteMixedCellSelection() bool {
	parts := e.selectionParts()
	var hasText, regionPart bool
	for _, part := range parts {
		if part.structure == nil {
			hasText = true
		} else if part.structure.cellRegion {
			regionPart = true
		}
	}
	if !hasText || !regionPart {
		return false
	}
	type sourceEdit struct {
		from, to int
		text     string
	}
	var edits []sourceEdit
	for _, part := range parts {
		if part.structure == nil {
			edits = append(edits, sourceEdit{from: part.source.Start, to: part.source.End})
			continue
		}
		s := part.structure
		t := e.tableByLineStart(s.lineStart)
		region, ok := e.tableSelectionCellRegion(s)
		if t == nil || !ok {
			return false
		}
		top, left, bottom, right := tableRegionBounds(region)
		lines := e.tableLines(t)
		if s.axis == tableWhole {
			z := blockEnd(e.Source(), t)
			edits = append(edits, sourceEdit{from: t.lineStart, to: z})
			continue
		}
		if s.axis == tableRows {
			lineIndex := func(row int) int {
				if row == 0 {
					return 0
				}
				return row + 1
			}
			first, last := lineIndex(s.first), lineIndex(s.last)
			if first < 0 || last >= len(lines) || first > last {
				return false
			}
			a, z := lines[first][0], lines[last][1]
			if z < len(e.Source()) {
				z++
			}
			edits = append(edits, sourceEdit{from: a, to: z})
			continue
		}
		if s.axis == tableColumns {
			if s.first < 0 || s.last >= t.tab.cols() || t.tab.cols() <= s.last-s.first+1 {
				return false
			}
			var b strings.Builder
			for li, ln := range lines {
				if li > 0 {
					b.WriteByte('\n')
				}
				pipes := splitRow(e.Source(), ln)
				if len(pipes) < 2 {
					b.WriteString(e.Source()[ln[0]:ln[1]])
					continue
				}
				b.WriteString(e.Source()[ln[0] : pipes[0]+1])
				written := 0
				for c := 0; c < t.tab.cols(); c++ {
					if c >= s.first && c <= s.last {
						continue
					}
					if written > 0 {
						b.WriteString(" |")
					}
					value := ""
					if li == 1 {
						value = " --- "
						switch alignAt(t.tab.align, c) {
						case syntax.IncrementalAlignLeft:
							value = " :-- "
						case syntax.IncrementalAlignCenter:
							value = " :-: "
						case syntax.IncrementalAlignRight:
							value = " --: "
						}
					} else {
						row := 0
						if li > 1 {
							row = li - 1
						}
						if row < len(t.tab.rows) && c < len(t.tab.rows[row]) {
							cell := t.tab.rows[row][c]
							value = e.Source()[cell.start:cell.end]
						}
					}
					b.WriteByte(' ')
					b.WriteString(value)
					written++
				}
				if written > 0 {
					b.WriteString(" |")
				}
				if pipes[len(pipes)-1]+1 < ln[1] {
					b.WriteString(e.Source()[pipes[len(pipes)-1]+1 : ln[1]])
				}
			}
			edits = append(edits, sourceEdit{from: lines[0][0], to: lines[len(lines)-1][1], text: b.String()})
			continue
		}
		if s.axis != tableCells || left == 0 && right == t.tab.cols()-1 || top == 0 && bottom == len(t.tab.rows)-1 {
			return false
		}
		for row := top; row <= bottom; row++ {
			for col := left; col <= right && col < len(t.tab.rows[row]); col++ {
				cell := t.tab.rows[row][col]
				edits = append(edits, sourceEdit{from: cell.start, to: cell.end})
			}
		}
	}
	if len(edits) == 0 {
		return false
	}
	sort.Slice(edits, func(i, j int) bool { return edits[i].from > edits[j].from })
	caret := e.selectionFocus()
	e.begin(stepEdit)
	for _, edit := range edits {
		if edit.from >= 0 && edit.to >= edit.from && edit.to <= len(e.Source()) {
			e.replace(edit.from, edit.to, edit.text)
		}
	}
	e.commit(caret, caret)
	e.tidyEmpty()
	return true
}

// tidyEmpty makes the empty paragraph a deletion leaves behind a phantom,
// so the caret stays in a line of its own, and takes its blank lines out
// of the source.
func (e *Editor) tidyEmpty() {
	if e.leafAt(e.Caret()) != nil || e.phantom != nil {
		return
	}
	// The caret is between blocks: put it in a phantom paragraph there,
	// on the blank lines the text left.
	off := e.Caret()
	s := lineStartAt(e.Source(), off)
	if strings.TrimSpace(e.Source()[s:lineEndAt(e.Source(), off)]) != "" {
		return
	}
	// The blank lines around the caret.
	a := s
	for a > 0 {
		p := lineStartAt(e.Source(), a-1)
		if strings.TrimSpace(strings.Trim(e.Source()[p:a-1], ">")) != "" {
			break
		}
		a = p
	}
	z := lineEndAt(e.Source(), off)
	for z < len(e.Source()) {
		n := lineEndAt(e.Source(), z+1)
		if strings.TrimSpace(strings.Trim(e.Source()[z+1:n], ">")) != "" {
			break
		}
		z = n
	}
	// Keep a paragraph's break: one blank line between what is above and
	// below.
	switch {
	case a == 0 && z == len(e.Source()):
		e.beginMergePrevious(stepEdit)
		e.replace(0, len(e.Source()), "")
		e.mergeLastStep()
		e.setPhantom(&phantom{at: 0})
	case a == 0:
		e.beginMergePrevious(stepEdit)
		e.replace(0, z+1, "")
		e.mergeLastStep()
		e.setPhantom(&phantom{at: 0, post: "\n", before: true})
	case z == len(e.Source()):
		e.beginMergePrevious(stepEdit)
		e.replace(a-1, len(e.Source()), "\n")
		e.mergeLastStep()
		e.setPhantom(&phantom{at: a - 1, pre: "\n"})
	default:
		e.beginMergePrevious(stepEdit)
		e.replace(a-1, z, "\n")
		e.mergeLastStep()
		e.setPhantom(&phantom{at: a - 1, pre: "\n", post: "\n"})
	}
}

// backspace deletes backward: the selection, the grapheme or word before
// the caret, or at the start of a block, the block's structure.
func (e *Editor) backspace(word bool) {
	e.layoutIfNeeded()
	if e.backspaceTableBlock() {
		return
	}
	if e.Session.Selection().Anchor != e.Caret() {
		e.deleteSelection()
		return
	}
	if p := e.phantom; p != nil {
		// An empty paragraph goes away: to the end of the leaf before it.
		at := p.at
		e.dropPhantom()
		if p.before {
			e.setCaret(e.normalize(at, 1), false)
		} else {
			e.setCaret(e.normalize(at, -1), false)
		}
		// The document may be nothing at all now, and its one line is the
		// phantom's: Backspace on an empty note keeps it.
		e.tidyEmpty()
		return
	}
	off := e.Caret()
	u := e.unitAt(off)
	if u == nil {
		return
	}
	// Heading markers are editable source positions before the content's
	// edge stop. Only Backspace at the content start converts the heading;
	// before it, delete backward normally without consuming syntax ahead.
	beforeHeadingContent := u.kind == kHeading && off < u.start
	_, tableEdge := e.tableAtEdge(off)
	if first, ok := e.edgeStop(u, false); ok && off <= first && !tableEdge && !beforeHeadingContent && !(u.kind == kCode && u.fenced && off >= u.infoStartOffset() && off <= u.infoEndOffset() && off > u.infoStartOffset()) {
		e.backspaceAtStart(u)
		return
	}
	prev := e.step(off, -1, word)
	if prev >= off {
		return
	}
	uPair := e.unitAt(off)
	inCode := uPair != nil && (uPair.kind == kCode || uPair.kind == kMath || uPair.kind == kHTML || uPair.kind == kRaw)
	if !word && !inCode && off < len(e.Source()) && e.Source()[prev:off] == "[" && e.Source()[off:off+1] == "]" {
		e.begin(stepDelete)
		e.replace(prev, off+1, "")
		e.commit(prev, prev)
		e.tidyEmpty()
		return
	}
	if !word {
		// One grapheme of what shows, which may be an atom standing for
		// more source, as a line break and the prefix after it.
		e.begin(stepDelete)
	} else {
		e.begin(stepEdit)
	}
	e.replace(prev, off, "")
	e.commit(prev, prev)
	if u.kind != kBlank || u.end == u.start {
		e.tidyEmpty()
	}
}

// backspaceAtStart handles Backspace at the start of a unit: a heading
// becomes a paragraph, an item leaves its list, a quotation's paragraph
// leaves it, a paragraph joins the one before.
func (e *Editor) backspaceAtStart(u *leaf) {
	l := e.leafOf(u)
	if e.isCell(u) {
		// To the end of the cell before.
		e.setCaret(e.step(u.start, -1, false), false)
		return
	}
	switch {
	case l.kind == kCode || l.kind == kMath:
		if strings.TrimSpace(e.codeText(l)) == "" {
			e.removeBlock(l)
			return
		}
		if l.kind == kCode && l.fenced && e.Caret() != l.infoStartOffset() {
			e.setCaret(l.infoEndOffset(), false)
			return
		}
		e.setCaret(e.step(e.Caret(), -1, false), false)
		return
	case l.kind == kHeading && !l.pending:
		e.begin(stepEdit)
		if l.setext {
			end := blockEnd(e.Source(), l)
			e.replace(lineEndAt(e.Source(), l.end), end, "")
		} else {
			e.replace(l.syntaxStart, l.start, "")
		}
		e.commit(e.Caret(), e.Caret())
		return
	case l.opensItem():
		e.leaveItem(l)
		return
	}
	if n := len(l.conts); n > 0 && l.conts[n-1].kind == cQuote && slices_contains(l.firstOf, n-1) {
		e.unquote(l)
		return
	}
	// Join the leaf before.
	i := e.indexOf(l)
	if i <= 0 {
		return
	}
	prev := e.leaves[i-1]
	switch prev.kind {
	case kPara, kHeading:
		if prev.pending {
			break
		}
		// One blank line at a time: the spaces between blocks are their
		// blank lines, and Backspace takes the last break only. Deleting the
		// whole gap would eat every blank line the user put there in one
		// press — invisible when blank lines took no room, wrong now.
		if n := strings.Count(e.Source()[prev.end:l.start], "\n"); n > 2 && l.lineStart > 0 {
			e.begin(stepEdit)
			e.replace(l.lineStart-1, l.lineStart, "")
			e.commit(l.lineStart-1, l.lineStart-1)
			return
		}
		e.begin(stepEdit)
		e.replace(prev.end, l.start, "")
		e.commit(prev.end, prev.end)
	case kRule:
		e.begin(stepEdit)
		e.replace(prev.lineStart, l.lineStart, "")
		c := e.Caret()
		e.commit(c, c)
	case kBlank, kPhantom:
		// A blank line: Backspace takes the line break above it and the
		// markers the line itself carries, so the document loses the line
		// and nothing else.
		if at := l.lineStart; at > 0 {
			e.begin(stepEdit)
			e.replace(at-1, l.start, "")
			e.commit(at-1, at-1)
			e.tidyEmpty()
		}
	default:
		e.setCaret(e.step(e.Caret(), -1, false), false)
	}
}

// slices_contains reports whether s holds v.
func slices_contains(s []int, v int) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}

// indexOf returns the index of a leaf among the leaves.
func (e *Editor) indexOf(l *leaf) int {
	for i, x := range e.leaves {
		if x == l {
			return i
		}
	}
	return -1
}

// removeBlock deletes a leaf's source, its lines, and puts the caret in a
// phantom paragraph in its place.
func (e *Editor) removeBlock(l *leaf) {
	start, end := l.lineStart, blockEnd(e.Source(), l)
	if end < len(e.Source()) {
		end++
	}
	e.begin(stepEdit)
	e.replace(start, end, "")
	e.commit(start, start)
	if e.leafAt(start) == nil || e.leafAt(start).lineStart != start {
		e.tidyEmpty()
		if e.phantom == nil {
			e.setPhantom(&phantom{at: start, post: "\n", before: true, conts: l.conts})
		}
	}
}

// deleteForward deletes forward: the selection, the grapheme or word after
// the caret, or at the end of a block, joins the block after.
func (e *Editor) deleteForward(word bool) {
	e.layoutIfNeeded()
	if e.Session.Selection().Anchor != e.Caret() {
		e.deleteSelection()
		return
	}
	if e.phantom != nil {
		at := e.phantom.at
		e.dropPhantom()
		e.setCaret(e.normalize(at, 1), false)
		e.tidyEmpty()
		return
	}
	off := e.Caret()
	if table, _ := e.tableAtEdge(off); table != nil {
		next := e.step(off, 1, word)
		if next <= off {
			return
		}
		e.begin(stepDelete)
		e.replace(off, next, "")
		e.commit(off, off)
		e.tidyEmpty()
		return
	}
	u := e.unitAt(off)
	if u == nil {
		return
	}
	blankText := u.kind == kBlank && u.end > u.start
	if last, ok := e.edgeStop(u, true); ok && off >= last {
		l := e.leafOf(u)
		if e.isCell(u) || l.kind == kCode || l.kind == kMath || l.kind == kHTML || l.kind == kRaw {
			return
		}
		i := e.indexOf(l)
		if i < 0 || i+1 >= len(e.leaves) {
			return
		}
		next := e.leaves[i+1]
		switch next.kind {
		case kPara, kHeading:
			e.begin(stepEdit)
			e.replace(l.end, next.start, "")
			e.commit(l.end, l.end)
		case kBlank, kPhantom:
			// A blank line below: Delete takes the line break that starts
			// it, and the markers it carries, one line at a time, as
			// Backspace does above.
			if next.start > l.end {
				e.begin(stepEdit)
				e.replace(l.end, next.start, "")
				e.commit(l.end, l.end)
				e.tidyEmpty()
			}
		case kRule:
			e.begin(stepEdit)
			end := blockEnd(e.Source(), next)
			if end < len(e.Source()) {
				end++
			}
			e.replace(lineEndAt(e.Source(), l.end), end-1, "")
			e.commit(off, off)
		}
		return
	}
	next := e.step(off, 1, word)
	if blankText && off < u.end {
		next = e.nextGraphemeBoundary(off)
	}
	if next <= off {
		return
	}
	e.begin(stepDelete)
	e.replace(off, next, "")
	e.commit(off, off)
	if !blankText {
		e.tidyEmpty()
	}
}

// ---------------------------------------------------------------------------
// Enter

// enter breaks the line at the caret as Typora does: a new paragraph, a
// new item, the next row of a table, a new line of code.
func (e *Editor) enter(shift bool) {
	previous := e.preserveInsertedBreaks
	e.preserveInsertedBreaks = e.opts.PreserveTypedLineBreaks
	defer func() { e.preserveInsertedBreaks = previous }()
	e.layoutIfNeeded()
	if e.Session.Selection().Anchor != e.Caret() {
		e.deleteSelection()
	}
	if p := e.phantom; p != nil {
		// An empty paragraph in a container leaves it.
		if len(p.conts) > 0 {
			np := *p
			np.conts = p.conts[:len(p.conts)-1]
			np.pre = "\n" + phantomPrefix(np.conts)
			if np.before {
				np.pre = ""
				np.post = "\n"
			}
			e.setPhantom(&np)
			return
		}
		// Enter in an empty paragraph adds a blank line: Markdown has no
		// empty paragraphs, so the extra Enters are kept as the blank
		// lines they are, and typing in the paragraph writes them there.
		e.begin(stepEdit)
		e.replace(p.at, p.at, "\n")
		np := *p
		if !np.before {
			np.at++
		}
		e.phantom = &np
		e.phantomStays = true
		e.commit(np.at, np.at)
		return
	}
	off := e.Caret()
	u := e.unitAt(off)
	if u == nil {
		// Between blocks — a blank line of its own, or an empty note — no
		// leaf holds the caret, and Enter was a no-op there: the gutter grew
		// while the text did not. Seat the phantom paragraph every other
		// empty paragraph is, and let it take the line.
		e.setPhantom(&phantom{at: off})
		e.enter(shift)
		return
	}
	if e.isCell(u) {
		e.nextRow()
		return
	}
	l := u
	cont := e.contPrefix(l)
	switch l.kind {
	case kCode, kMath, kHTML, kRaw:
		if l.kind == kCode && l.fenced && off >= l.infoStartOffset() && off <= l.infoEndOffset() {
			// From the info string into the code.
			if len(l.codeLines()) == 0 {
				e.begin(stepEdit)
				e.replace(l.infoEndOffset(), l.infoEndOffset(), "\n"+cont)
				c := l.infoEndOffset() + 1 + len(cont)
				e.commit(c, c)
				return
			}
			e.setCaret(l.start, false)
			return
		}
		indent := ""
		if l.kind == kCode {
			ls := e.lineContentStart(l, off)
			for i := ls; i < off && (e.Source()[i] == ' ' || e.Source()[i] == '\t'); i++ {
				indent += string(e.Source()[i])
			}
		}
		e.begin(stepEdit)
		ins := "\n" + cont + indent
		e.replace(off, off, ins)
		e.commit(off+len(ins), off+len(ins))
		return
	case kBlank, kPhantom:
		// A blank line: Enter opens another one below it — except in a
		// quotation, where an empty line is how the quotation ends: its
		// markers go, and the caret stays on the line.
		if len(l.conts) > 0 {
			at := l.lineStart
			e.begin(stepEdit)
			e.replace(at, l.start, "")
			if at == len(e.Source()) {
				// The line ended the document: the empty line after it is
				// the one the caret leaves the quotation onto.
				e.replace(at, at, "\n")
				e.commit(at+1, at+1)
				return
			}
			e.commit(at, at)
			return
		}
		e.begin(stepEdit)
		e.replace(off, off, "\n")
		e.commit(off+1, off+1)
		return
	case kRule:
		// A rule has no line to type on: Enter opens one below it.
		e.setPhantom(&phantom{at: blockEnd(e.Source(), l), pre: "\n" + cont, conts: l.conts})
		return
	}
	// Text: a paragraph, a heading, an item.
	if l.pending {
		e.begin(stepEdit)
		e.replace(off, off, "\n"+cont)
		e.commit(off+1+len(cont), off+1+len(cont))
		return
	}
	// Shift+Enter is the same as Enter: the break the source takes is the
	// break the key makes, whatever the modifiers say (see docs/editor-model.md).
	_ = shift
	if l.kind == kPara && e.tableShortcut(l) {
		return
	}
	if l.opensItem() {
		e.enterItem(l, off)
		return
	}
	first, _ := e.edgeStop(l, false)
	last, _ := e.edgeStop(l, true)
	switch {
	case off >= last:
		// A new line below, inside the leaf's containers: Enter in the
		// source is one line break, and the markers of the containers come
		// with it, as they do in the file.
		at := blockEnd(e.Source(), l)
		ins := "\n" + cont
		e.begin(stepEdit)
		e.replace(at, at, ins)
		e.commit(at+len(ins), at+len(ins))
	case off <= first && strings.TrimSpace(e.Source()[l.start:l.end]) != "":
		// A new line above: the break goes in before the leaf's own line,
		// and the caret stays on the empty line it leaves.
		at := l.syntaxStart
		ins := "\n" + cont
		e.begin(stepEdit)
		e.replace(at, at, ins)
		e.commit(at, at)
	default:
		// Spaces at the break go with it.
		from, to := off, off
		for from > l.start && e.Source()[from-1] == ' ' {
			from--
		}
		for to < l.end && e.Source()[to] == ' ' {
			to++
		}
		e.begin(stepEdit)
		ins := "\n" + cont
		e.replace(from, to, ins)
		e.commit(from+len(ins), from+len(ins))
	}
}

// lineContentStart returns where the content of the line holding off
// starts in a leaf of lines.
func (e *Editor) lineContentStart(l *leaf, off int) int {
	for _, ln := range l.codeLines() {
		if off >= ln.off && off <= ln.off+len(ln.text) {
			return ln.off
		}
	}
	return lineStartAt(e.Source(), off)
}

// enterItem breaks an item at the caret: the text after it goes to a new
// item of the list, or on an empty item, the item leaves the list.
func (e *Editor) enterItem(l *leaf, off int) {
	it, _ := l.innerItem()
	if strings.TrimSpace(e.Source()[l.start:l.end]) == "" {
		e.leaveItem(l)
		return
	}
	plan, ok := mdedit.ParseSource(e.Source()).ContinueListAt(mdedit.ListContinuationFacts{
		Caret: mdtext.Offset(off), ItemStart: mdtext.Offset(it.start),
		ContentStart: mdtext.Offset(l.start), ItemEnd: mdtext.Offset(l.end),
		Marker: rune(it.marker), Ordered: it.ordered, Number: it.num,
		Task: it.task != syntax.IncrementalNoTask, Tight: it.tight,
	})
	if ok {
		e.applyMarkdownPlan(plan)
	}
}

// leaveItem takes an item out of its list: a nested item goes up a level,
// one at the top becomes a paragraph after the list.
func (e *Editor) leaveItem(l *leaf) {
	it, _ := l.innerItem()
	nested := 0
	for _, c := range l.conts {
		if c.kind == cItem {
			nested++
		}
	}
	lineStart := lineStartAt(e.Source(), it.start)
	lineEnd := lineEndAt(e.Source(), lineStart)
	plan, ok := mdedit.ParseSource(e.Source()).ExitList(mdedit.ListExitFacts{
		MarkerStart: mdtext.Offset(it.start), ContentStart: mdtext.Offset(l.start),
		LineStart: mdtext.Offset(lineStart), LineEnd: mdtext.Offset(lineEnd),
		ItemEnd: mdtext.Offset(l.end), Caret: mdtext.Offset(e.Caret()),
		ItemDepth: nested, HasPreviousLeaf: lineStart > 0 && e.leafBefore(l) != nil,
	})
	if !ok {
		return
	}
	if plan.Kind == mdedit.ListExitOutdent {
		e.outdentItem(l)
		return
	}
	outer := e.Source()[lineStart:it.start]
	e.applyMarkdownPlan(plan.Plan)
	if plan.Kind == mdedit.ListExitEmpty {
		prefix := blankMarkers(outer)
		blank := blankPrefix(prefix)
		var conts []cont
		for _, c := range l.conts {
			if c.kind != cItem {
				conts = append(conts, c)
			}
		}
		if plan.AtDocumentEnd {
			pre := "\n" + blank + "\n" + prefix
			if plan.Plan.Selection != nil && int(plan.Plan.Selection.Head) == 0 {
				pre = prefix
			}
			e.setPhantom(&phantom{at: int(plan.Plan.Selection.Head), pre: pre, conts: conts})
			return
		}
		pre := blank + "\n" + prefix
		if lineStart == 0 {
			pre = prefix
		}
		e.setPhantom(&phantom{at: int(plan.Plan.Selection.Head), pre: pre, post: "\n" + blank + "\n", before: true, conts: conts})
	}
}

func (e *Editor) applyMarkdownPlan(plan mdedit.Plan) {
	transaction := tablemodel.TransactionFromPlan(plan, e.Session.Selection())
	if len(transaction.Edits) == 0 {
		return
	}
	e.begin(stepEdit)
	for i := len(transaction.Edits) - 1; i >= 0; i-- {
		edit := transaction.Edits[i]
		e.replace(edit.Range.Start, edit.Range.End, edit.Text)
	}
	e.commit(transaction.After.Anchor, transaction.After.Head)
}

// leafBefore returns the leaf before l, or nil.
func (e *Editor) leafBefore(l *leaf) *leaf {
	if i := e.indexOf(l); i > 0 {
		return e.leaves[i-1]
	}
	return nil
}

// unquote takes a leaf out of its innermost quotation: its lines lose the
// quotation's marker.
func (e *Editor) unquote(l *leaf) {
	q := l.conts[len(l.conts)-1]
	depth := 0
	for _, c := range l.conts {
		if c.kind == cQuote {
			depth++
		}
	}
	plan, ok := paragraph.PlanUnquote(e.Source(), max(q.start, l.lineStart), blockEnd(e.Source(), l), depth, e.Caret())
	e.applyParagraphLinePlan(plan, ok)
}

// tableShortcut turns a paragraph of one line of cells between pipes into
// a table, at Enter: "| a | b |" makes a table of two columns with an empty
// row for the caret. It reports whether it did.
func (e *Editor) tableShortcut(l *leaf) bool {
	plan, ok := tablemodel.PlanShortcut(e.Source()[l.start:l.end], e.contPrefix(l))
	if !ok {
		return false
	}
	end := l.end
	e.begin(stepEdit)
	e.replace(end, end, plan.Insert)
	// The caret goes in the first cell of the new row.
	c := end + plan.Caret
	e.commit(c, c)
	return true
}

// ---------------------------------------------------------------------------
// Tables

// cellPos returns the table, row and column of the cell holding off.
func (e *Editor) cellPos(off int) (*leaf, int, int) {
	u := e.unitAt(off)
	if u == nil {
		return nil, -1, -1
	}
	t := e.tableOf(u)
	if t == nil {
		return nil, -1, -1
	}
	view, ok := (editorTableHost{editor: e}).TableView(t.lineStart)
	if !ok {
		return nil, -1, -1
	}
	cell, ok := tablemodel.CellAtSourceOffset(view, off, tablemodel.PreferNextCell)
	if !ok {
		return nil, -1, -1
	}
	return t, cell.Row, cell.Col
}

// moveCell moves the caret to the end of the cell dir cells away, through
// the rows; past the last, a new row.
func (e *Editor) moveCell(dir int) {
	t, r, c := e.cellPos(e.Caret())
	if t == nil {
		return
	}
	cols := t.tab.cols()
	idx := r*cols + c + dir
	if idx < 0 {
		return
	}
	nr, nc := idx/cols, idx%cols
	if nr >= len(t.tab.rows) {
		e.appendRow(t, 0)
		return
	}
	row := t.tab.rows[nr]
	if nc >= len(row) {
		nc = len(row) - 1
	}
	cell := row[nc]
	e.Select(cell.start, cell.end)
}

// nextRow moves the caret to the same column of the next row; past the
// last, a new row.
func (e *Editor) nextRow() {
	t, r, c := e.cellPos(e.Caret())
	if t == nil {
		return
	}
	if r+1 >= len(t.tab.rows) {
		e.appendRow(t, c)
		return
	}
	row := t.tab.rows[r+1]
	cell := row[min(c, len(row)-1)]
	e.setCaret(cell.end, false)
}

// appendRow adds an empty row at the end of a table and puts the caret in
// its column col.
func (e *Editor) appendRow(t *leaf, col int) {
	if t == nil || t.tab == nil || len(t.tab.rows) == 0 {
		return
	}
	_, _ = e.tableUI().Dispatch(tableui.Action{
		Kind: tableui.InsertRowAfter, LineStart: t.lineStart,
		First: len(t.tab.rows) - 1, Focus: tablemodel.Cell{Col: col},
	})
}

// ---------------------------------------------------------------------------
// Indentation

// tab indents (or with shift, outdents) the item at the caret, moves to the
// next cell of a table, or indents a line of code.
func (e *Editor) tab(shift bool) bool {
	e.layoutIfNeeded()
	if e.phantom != nil {
		return true
	}
	u := e.unitAt(e.Caret())
	if u == nil {
		return false
	}
	if e.isCell(u) {
		if shift {
			e.moveCell(-1)
		} else {
			e.moveCell(1)
		}
		return true
	}
	if u.kind == kCode {
		e.indentCode(u, shift)
		return true
	}
	if _, ok := u.innerItem(); ok {
		if shift {
			e.outdentItem(u)
		} else {
			e.indentItem(u)
		}
		return true
	}
	return true
}

// indentCode indents the lines of code the selection touches, or types
// spaces at the caret.
func (e *Editor) indentCode(l *leaf, shift bool) {
	unit := "    "
	if strings.EqualFold(l.language(), "go") || strings.EqualFold(l.language(), "makefile") {
		unit = "\t"
	}
	a, z := e.selection()
	if a == z && !shift {
		e.begin(stepTyping)
		e.replace(a, a, unit)
		e.commit(a+len(unit), a+len(unit))
		return
	}
	e.begin(stepEdit)
	anchor, caret := e.Session.Selection().Anchor, e.Caret()
	for i := len(l.codeLines()) - 1; i >= 0; i-- {
		ln := l.codeLines()[i]
		if ln.off > z || ln.off+len(ln.text) < a {
			continue
		}
		if shift {
			n := 0
			for n < len(ln.text) && n < len(unit) && (ln.text[n] == ' ' || ln.text[n] == '\t') {
				n++
				if ln.text[n-1] == '\t' {
					break
				}
			}
			e.replace(ln.off, ln.off+n, "")
		} else {
			e.replace(ln.off, ln.off, unit)
		}
	}
	_ = anchor
	_ = caret
	e.commit(e.Session.Selection().Anchor, e.Caret())
}

// itemRange returns the source range of the item a leaf is in, from the
// start of its first line to the end of its last.
func (e *Editor) itemRange(it cont) (int, int) {
	b := e.p.Parser().Block(it.id)
	end := lineEndAt(e.Source(), b.End)
	for c := it.id + 1; c < b.Next && c < e.p.Parser().Len(); c++ {
		end = max(end, lineEndAt(e.Source(), e.p.Parser().Block(c).End))
	}
	return lineStartAt(e.Source(), it.start), end
}

// itemSiblings returns the items of the list an item is in.
func (e *Editor) itemSiblings(it cont) []int {
	parent := e.p.Parser().Block(it.id).Parent
	var out []int
	for c := range e.p.Parser().Children(parent) {
		out = append(out, c)
	}
	return out
}

// indentItem makes the item at the caret a child of the item before it.
func (e *Editor) indentItem(l *leaf) {
	it, _ := l.innerItem()
	sibs := e.itemSiblings(it)
	k := -1
	for i, s := range sibs {
		if s == it.id {
			k = i
		}
	}
	if k <= 0 {
		return
	}
	prev := e.p.Parser().Block(sibs[k-1])
	// The item before's content column, past its marker.
	w := paragraph.MarkerWidth(e.Source(), prev.Start)
	pad := strings.Repeat(" ", w)
	start, end := e.itemRange(it)
	e.indentLines(start, end, pad, len(blankMarkers(e.Source()[lineStartAt(e.Source(), it.start):it.start])))
}

// outdentItem moves a nested item up a level, after its parent.
func (e *Editor) outdentItem(l *leaf) {
	var items []cont
	for _, c := range l.conts {
		if c.kind == cItem {
			items = append(items, c)
		}
	}
	if len(items) < 2 {
		e.leaveItem(l)
		return
	}
	it, parent := items[len(items)-1], items[len(items)-2]
	w := paragraph.MarkerWidth(e.Source(), parent.start)
	start, end := e.itemRange(it)
	e.outdentLines(start, end, w, len(blankMarkers(e.Source()[lineStartAt(e.Source(), it.start):it.start]))-w)
}

// indentLines puts pad at column col of every line from start to end.
func (e *Editor) indentLines(start, end int, pad string, col int) {
	plan, ok := paragraph.PlanIndentLines(e.Source(), start, end, pad, col, e.Session.Selection().Anchor, e.Caret())
	e.applyParagraphLinePlan(plan, ok)
}

// outdentLines takes up to n spaces from column col of every line from
// start to end.
func (e *Editor) outdentLines(start, end, n, col int) {
	plan, ok := paragraph.PlanOutdentLines(e.Source(), start, end, n, col, e.Session.Selection().Anchor, e.Caret())
	e.applyParagraphLinePlan(plan, ok)
}

func (e *Editor) applyParagraphLinePlan(plan paragraph.LinePlan, ok bool) {
	if !ok || len(plan.Edits) == 0 {
		return
	}
	e.begin(stepEdit)
	for _, edit := range plan.Edits {
		e.replace(edit.Start, edit.End, edit.Text)
	}
	e.commit(plan.Anchor, plan.Head)
}
