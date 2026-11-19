package editor

import (
	"path/filepath"
	"strings"

	"github.com/egoist/mygo/transfer"
	"github.com/egoist/mygo/ui/richtext"
	"github.com/wheregrassdreams/muse-editor/markdown/inline"
	"github.com/wheregrassdreams/muse-editor/markdown/paragraph"
	syntax "github.com/wheregrassdreams/muse-editor/markdown/syntax"
	tableui "muse/nativeui/workbench/features/editor/blocks/table"
)

// Inline formats: a command on a selection wraps it in the format's
// markers, or takes them away when the selection is in that format; on a
// caret, it formats the word at it, or types the markers with the caret
// between them.

// inlineAround returns the innermost inline of kind around [a, z] in the
// text unit holding the caret.
func (e *Editor) inlineAround(kind syntax.IncrementalInlineKind, a, z int) (syntax.IncrementalInline, bool) {
	u := e.unitAt(e.Caret())
	if u == nil || u.id < 0 || !(u.kind == kPara || u.kind == kHeading) {
		return syntax.IncrementalInline{}, false
	}
	var best syntax.IncrementalInline
	found := false
	for _, in := range e.p.Parser().Inlines(u.id) {
		if in.Kind == kind && in.Start <= a && z <= in.End {
			if !found || in.End-in.Start < best.End-best.Start {
				best, found = in, true
			}
		}
	}
	return best, found
}

// ToggleInline formats the selection with marker (as "**"), or takes the
// format of kind away from it.
func (e *Editor) ToggleInline(kind syntax.IncrementalInlineKind, open, close string) {
	e.layoutIfNeeded()
	a, z := e.selection()
	if in, ok := e.inlineAround(kind, a, z); ok && e.phantom == nil {
		// The inline model plans delimiter removal and selection mapping; the
		// host applies only the resulting source transaction.
		markerKind := inline.MarkerGeneric
		switch in.Kind {
		case syntax.IncrementalCode:
			markerKind = inline.MarkerCode
		case syntax.IncrementalEmphasis:
			markerKind = inline.MarkerEmphasis
		case syntax.IncrementalStrong:
			markerKind = inline.MarkerStrong
		case syntax.IncrementalStrikethrough:
			markerKind = inline.MarkerStrikethrough
		}
		plan, valid := inline.UnwrapSelection(
			e.Source(), inline.Range{Start: a, End: z}, inline.Range{Start: in.Start, End: in.End}, markerKind, open, close,
		)
		if !valid {
			return
		}
		e.applyInlinePlan(plan)
		return
	}
	e.wrapInline(open, close)
}

// wrapInline puts the selection between the markers of a format, or on a
// caret, the word at it, or else types them with the caret between them.
func (e *Editor) wrapInline(open, close string) {
	e.layoutIfNeeded()
	if e.phantom != nil {
		e.typeText(open + close)
		e.setCaret(e.Caret()-len(close), false)
		return
	}
	a, z := e.selection()
	word := inline.Range{}
	if a == z {
		wa, wz := e.wordRangeAt(a)
		word = inline.Range{Start: wa, End: wz}
	}
	plan, ok := inline.WrapAtCaret(e.Source(), inline.Range{Start: a, End: z}, word, open, close)
	if !ok {
		return
	}
	e.applyInlinePlan(plan)
}

func (e *Editor) applyInlinePlan(plan inline.FormatPlan) {
	e.begin(stepEdit)
	for index := len(plan.Edits) - 1; index >= 0; index-- {
		edit := plan.Edits[index]
		e.replace(edit.Range.Start, edit.Range.End, edit.Text)
	}
	e.commit(plan.After.Anchor, plan.After.Head)
}

// ToggleBold, ToggleItalic, ToggleStrike, ToggleCode and ToggleUnderline
// format the selection.
func (e *Editor) ToggleBold()   { e.ToggleInline(syntax.IncrementalStrong, "**", "**") }
func (e *Editor) ToggleItalic() { e.ToggleInline(syntax.IncrementalEmphasis, "*", "*") }
func (e *Editor) ToggleStrike() { e.ToggleInline(syntax.IncrementalStrikethrough, "~~", "~~") }
func (e *Editor) ToggleCode()   { e.ToggleInline(syntax.IncrementalCode, "`", "`") }

// ToggleUnderline wraps the selection in <u>, or takes it away.
func (e *Editor) ToggleUnderline() {
	e.ToggleInline(syntax.IncrementalHTML, "<u>", "</u>")
}

// InsertLink makes the selection a link, with the caret in its
// destination, or edits the link at the caret.
func (e *Editor) InsertLink() {
	e.layoutIfNeeded()
	a, z := e.selection()
	if in, ok := e.inlineAround(syntax.IncrementalLink, a, z); ok && !in.Auto {
		// Select the link's destination.
		if destination, valid := inline.LinkDestination(e.Source(), inline.Range{Start: in.Start, End: in.End}); valid {
			e.Select(destination.Start, destination.End)
			return
		}
	}
	if e.phantom != nil {
		e.typeText("[]()")
		e.setCaret(e.Caret()-3, false)
		return
	}
	plan, valid := inline.PlanLink(e.Source(), inline.Range{Start: a, End: z})
	if valid {
		e.applyInlinePlan(plan)
	}
}

// InsertImage types an image's Markdown at the caret: its alt text the
// selection, the caret in its destination.
func (e *Editor) InsertImage(path string) {
	text := "![](" + path + ")"
	e.typeText(text)
	if path == "" {
		e.setCaret(e.Caret()-1, false)
	}
}

// ---------------------------------------------------------------------------
// Paragraph formats

// lineLeaf returns the text leaf at the caret, which paragraph commands act
// on.
func (e *Editor) lineLeaf() *leaf {
	e.layoutIfNeeded()
	u := e.unitAt(e.Caret())
	if u == nil || e.isCell(u) {
		return nil
	}
	return u
}

// SetHeading makes the paragraph at the caret a heading of level, or a
// paragraph for level 0.
func (e *Editor) SetHeading(level int) {
	l := e.lineLeaf()
	if l == nil {
		return
	}
	if l.kind == kPhantom {
		if level > 0 {
			e.typeText(strings.Repeat("#", level) + " ")
		}
		return
	}
	if l.kind != kPara && l.kind != kHeading {
		return
	}
	input := paragraph.HeadingInput{Source: e.Source(), Caret: e.Caret(), Level: level}
	switch {
	case l.kind == kHeading && l.setext:
		input.Shape = paragraph.SetextHeading
		input.PrefixFrom = l.start
		input.SetextFrom = lineEndAt(e.Source(), l.end)
		input.SetextTo = blockEnd(e.Source(), l)
	case l.kind == kHeading && !l.pending:
		input.Shape = paragraph.ATXHeading
		input.PrefixFrom = l.syntaxStart
		input.PrefixTo = l.start
	default:
		// The line holding the caret becomes the heading.
		ls := max(lineStartAt(e.Source(), e.Caret()), l.start)
		if ls != l.start {
			ls = lineStartAt(e.Source(), e.Caret()) + len(e.contPrefix(l))
		}
		input.InsertAt = ls
	}
	plan, ok := paragraph.PlanHeading(input)
	if !ok || len(plan.Edits) == 0 {
		return
	}
	e.begin(stepEdit)
	for _, edit := range plan.Edits {
		e.replace(edit.Start, edit.End, edit.Text)
	}
	e.commit(plan.Caret, plan.Caret)
}

// headingLevel returns the level of the heading at the caret, 0 for a
// paragraph.
func (e *Editor) headingLevel() int {
	if l := e.lineLeaf(); l != nil && l.kind == kHeading && !l.pending {
		return l.level
	}
	return 0
}

// ShiftHeading raises (dir > 0) or lowers the heading level at the caret.
func (e *Editor) ShiftHeading(dir int) {
	level, changed := paragraph.ShiftHeadingLevel(e.headingLevel(), dir)
	if changed {
		e.SetHeading(level)
	}
}

// ToggleQuote puts the block at the caret in a quotation, or takes it out
// of one.
func (e *Editor) ToggleQuote() {
	l := e.lineLeaf()
	if l == nil {
		return
	}
	if n := len(l.conts); n > 0 && l.conts[n-1].kind == cQuote {
		e.unquote(l)
		return
	}
	if l.kind == kPhantom {
		e.typeText("> ")
		return
	}
	e.prefixLines(l, "> ")
}

// prefixLines puts p at the content start of every line of a leaf.
func (e *Editor) prefixLines(l *leaf, p string) {
	cont := len(e.contPrefix(l))
	start, end := l.lineStart, blockEnd(e.Source(), l)
	e.indentLines(start, end, p, cont)
}

// ToggleList makes the paragraph at the caret an item of a list of kind
// ("-", "1." or "- [ ]"), or takes it out of its list.
func (e *Editor) ToggleList(marker string) {
	l := e.lineLeaf()
	if l == nil {
		return
	}
	if l.kind == kPhantom {
		e.typeText(marker + " ")
		return
	}
	if l.opensItem() {
		it, _ := l.innerItem()
		task := it.task != syntax.IncrementalNoTask
		plan, ok := paragraph.PlanListToggle(paragraph.ListInput{
			Source: e.Source(), Caret: e.Caret(), Marker: marker, Item: true,
			Ordered: it.ordered, Task: task, MarkerStart: it.start, ContentStart: l.start,
		})
		if !ok {
			return
		}
		if plan.Action == paragraph.ListLeave {
			e.leaveItem(l)
			return
		}
		if plan.Action != paragraph.ListEdit {
			return
		}
		e.begin(stepEdit)
		e.replace(plan.Edit.Start, plan.Edit.End, plan.Edit.Text)
		e.commit(plan.Caret, plan.Caret)
		return
	}
	if l.kind != kPara && l.kind != kHeading {
		return
	}
	plan, ok := paragraph.PlanListToggle(paragraph.ListInput{
		Source: e.Source(), Caret: e.Caret(), Marker: marker,
		EligibleText: true, InsertAt: l.syntaxStart,
	})
	if !ok || plan.Action != paragraph.ListEdit {
		return
	}
	e.begin(stepEdit)
	e.replace(plan.Edit.Start, plan.Edit.End, plan.Edit.Text)
	e.commit(plan.Caret, plan.Caret)
}

// InsertCodeBlock puts the selected lines in a fenced block of code, or
// opens an empty one below the block at the caret.
func (e *Editor) InsertCodeBlock() { e.insertFenced("```", "```") }

// InsertMathBlock opens a block of math.
func (e *Editor) InsertMathBlock() { e.insertFenced("$$", "$$") }

func (e *Editor) insertFenced(open, close string) {
	l := e.lineLeaf()
	if l == nil {
		return
	}
	if l.kind == kPhantom {
		p := e.phantom
		e.begin(stepEdit)
		e.phantom = nil
		cont := phantomPrefix(p.conts)
		text := open + "\n" + cont + "\n" + cont + close
		e.replace(p.at, p.at, p.pre+text+p.post)
		c := p.at + len(p.pre) + len(open)
		if open == "$$" {
			c += 1 + len(cont)
		}
		e.commit(c, c)
		return
	}
	cont := e.contPrefix(l)
	if l.kind == kPara && strings.TrimSpace(e.Source()[l.start:l.end]) != "" {
		// The paragraph's text goes in the block.
		body := e.Source()[l.start:l.end]
		e.begin(stepEdit)
		text := open + "\n" + cont + body + "\n" + cont + close
		e.replace(l.start, l.end, text)
		c := l.start + len(open)
		if open == "$$" {
			c += 1 + len(cont)
		}
		e.commit(c, c)
		return
	}
	end := blockEnd(e.Source(), l)
	blank := blankPrefix(cont)
	text := "\n" + blank + "\n" + cont + open + "\n" + cont + "\n" + cont + close
	e.begin(stepEdit)
	e.replace(end, end, text)
	c := end + len("\n"+blank+"\n"+cont+open)
	if open == "$$" {
		c += 1 + len(cont)
	}
	e.commit(c, c)
}

// InsertTable puts a table of rows and cols below the block at the caret,
// the caret in its first header cell.
func (e *Editor) InsertTable(rows, cols int) {
	l := e.lineLeaf()
	cont := ""
	at := len(e.Source())
	pre := "\n\n"
	if l != nil {
		cont = e.contPrefix(l)
		at = blockEnd(e.Source(), l)
		pre = "\n" + blankPrefix(cont) + "\n" + cont
	}
	if p := e.phantom; p != nil {
		at, pre = p.at, p.pre
		cont = phantomPrefix(p.conts)
	}
	var b strings.Builder
	b.WriteString("|")
	for c := 0; c < cols; c++ {
		b.WriteString("   |")
	}
	b.WriteString("\n" + cont + "|")
	for c := 0; c < cols; c++ {
		b.WriteString(" --- |")
	}
	for r := 1; r < rows; r++ {
		b.WriteString("\n" + cont + "|")
		for c := 0; c < cols; c++ {
			b.WriteString("   |")
		}
	}
	post := ""
	if p := e.phantom; p != nil {
		post = p.post
		e.phantom = nil
	}
	e.begin(stepEdit)
	e.replace(at, at, pre+b.String()+post)
	c := at + len(pre) + 2
	e.commit(c, c)
}

// InsertRule puts a thematic break below the block at the caret.
func (e *Editor) InsertRule() {
	l := e.lineLeaf()
	if l == nil {
		return
	}
	if p := e.phantom; p != nil {
		e.begin(stepEdit)
		e.phantom = nil
		e.replace(p.at, p.at, p.pre+"---"+p.post)
		e.commit(p.at+len(p.pre)+3, p.at+len(p.pre)+3)
		e.setPhantom(&phantom{at: e.Caret(), pre: "\n\n", conts: p.conts})
		return
	}
	cont := e.contPrefix(l)
	end := blockEnd(e.Source(), l)
	text := "\n" + blankPrefix(cont) + "\n" + cont + "---"
	e.begin(stepEdit)
	e.replace(end, end, text)
	e.commit(end+len(text), end+len(text))
	e.setPhantom(&phantom{at: e.Caret(), pre: "\n" + blankPrefix(cont) + "\n" + cont, conts: l.conts})
}

// ToggleTask checks or unchecks the task item the caret is in.
func (e *Editor) ToggleTask() {
	l := e.lineLeaf()
	if l == nil {
		return
	}
	if it, ok := l.innerItem(); ok && it.taskAt >= 0 {
		e.toggleTaskAt(it)
	}
}

// toggleTaskAt checks or unchecks an item's box.
func (e *Editor) toggleTaskAt(it cont) {
	at := it.taskAt
	if at < 0 || at+3 > len(e.Source()) {
		return
	}
	mark := "x"
	if it.task == syntax.IncrementalChecked {
		mark = " "
	}
	anchor, caret := e.Session.Selection().Anchor, e.Caret()
	e.actCaretStill(func() {
		e.begin(stepEdit)
		e.replace(at+1, at+2, mark)
		e.commit(anchor, caret)
	})
}

// ---------------------------------------------------------------------------
// Tables

// tableLines returns the source ranges of a table's lines: the header, the
// delimiter row and the body rows.
func (e *Editor) tableLines(t *leaf) [][2]int {
	var out [][2]int
	end := blockEnd(e.Source(), t)
	for off := t.lineStart; off <= end; {
		le := lineEndAt(e.Source(), off)
		out = append(out, [2]int{off, le})
		if le >= end || le >= len(e.Source()) {
			break
		}
		off = le + 1
	}
	return out
}

// splitRow splits a table line into its prefix and the offsets of its
// pipes.
func splitRow(src string, ln [2]int) (pipes []int) {
	for i := ln[0]; i < ln[1]; i++ {
		switch src[i] {
		case '\\':
			i++
		case '|':
			pipes = append(pipes, i)
		}
	}
	return pipes
}

// ---------------------------------------------------------------------------
// The clipboard

// Copy puts the selection on the clipboard: its Markdown as text and as
// Markdown, and as HTML, which pastes with its formatting elsewhere.
func (e *Editor) Copy() bool {
	md := e.SelectedText()
	if md == "" {
		return false
	}
	html := syntax.ParseIncrementalSource(md).HTML()
	parts := e.selectionParts()
	var htmlParts []string
	hasStructure := false
	for _, part := range parts {
		if part.structure == nil {
			htmlParts = append(htmlParts, syntax.ParseIncrementalSource(e.Source()[part.source.Start:part.source.End]).HTML())
			continue
		}
		t := e.tableByLineStart(part.structure.lineStart)
		reg := e.tableRegisterForSelection(t, part.structure)
		if reg == nil || !reg.Region() || part.structure.axis == tableWhole {
			htmlParts = append(htmlParts, syntax.ParseIncrementalSource(e.Source()[part.source.Start:part.source.End]).HTML())
			continue
		}
		// Markdown uses the rectangle's first row as its header. HTML keeps
		// source table header semantics through the register's sourceHeader.
		htmlParts = append(htmlParts, tableui.ClipboardHTML(*reg))
		hasStructure = true
	}
	if hasStructure {
		html = strings.Join(htmlParts, "\n")
	}
	reps := []transfer.Representation{
		transfer.Bytes(transfer.Text, []byte(md)),
		transfer.Bytes(transfer.Markdown, []byte(md)),
		transfer.Bytes(transfer.HTML, []byte(html)),
	}
	if e.block != nil {
		reps = append(reps, transfer.Bytes(rectangleClipboardFormat, encodeRectangleClipboard(e.copyBlockRows())))
	}
	data := transfer.New(transfer.NewItem(reps...))
	if e.opts.WriteClipboard != nil {
		e.opts.WriteClipboard(data)
	} else {
		e.services.WriteClipboardData(data)
	}
	return true
}

// Cut copies the selection and deletes it.
func (e *Editor) Cut() bool {
	if !e.Copy() {
		return false
	}
	e.deleteSelection()
	return true
}

// Paste inserts the clipboard at the selection: Markdown as it is, an
// image saved next to the document, HTML turned into Markdown, or text;
// plain pastes text only.
func (e *Editor) Paste(plain bool) {
	var data transfer.Data
	if e.opts.ReadClipboard != nil {
		data = e.opts.ReadClipboard()
	} else {
		data = e.services.ReadClipboardData(transfer.Markdown, transfer.PNG, transfer.FileList, transfer.HTML, transfer.Text)
	}
	if !plain {
		if b, err := data.Read(rectangleClipboardFormat); err == nil {
			if rows, ok := decodeRectangleClipboard(b); ok {
				e.pasteRectangle(rows)
				return
			}
		}
		if b, err := data.Read(transfer.Markdown); err == nil && len(b) > 0 {
			e.insertText(string(b))
			return
		}
		if e.opts.SaveImage != nil {
			if b, err := data.Read(transfer.PNG); err == nil && len(b) > 0 {
				if path, err := e.opts.SaveImage(b, ".png"); err == nil {
					e.typeText("![](" + linkDest(path) + ")")
					return
				}
			}
			if files, err := data.Files(); err == nil && len(files) > 0 {
				var parts []string
				for _, f := range files {
					if !isImagePath(f) {
						continue
					}
					if e.opts.ImageLocalFiles == nil {
						continue
					}
					b, err := e.opts.ImageLocalFiles.ReadFile(f)
					if err != nil {
						continue
					}
					if path, err := e.opts.SaveImage(b, strings.ToLower(filepath.Ext(f))); err == nil {
						parts = append(parts, "!["+strings.TrimSuffix(filepath.Base(f), filepath.Ext(f))+"]("+linkDest(path)+")")
					}
				}
				if len(parts) > 0 {
					e.insertText(strings.Join(parts, "\n\n"))
					return
				}
			}
		}
		if b, err := data.Read(transfer.HTML); err == nil && richHTML(string(b)) {
			if md := strings.TrimSpace(richtext.ParseHTML(string(b)).Markdown()); md != "" {
				e.insertText(md)
				return
			}
		}
	}
	if b, err := data.Read(transfer.Text); err == nil && len(b) > 0 {
		text := strings.ReplaceAll(string(b), "\r\n", "\n")
		if e.blockInput != nil {
			e.typeBlockInput(text)
		} else {
			e.insertText(text)
		}
	}
}

// richHTML reports whether HTML has formatting worth keeping, rather than
// the colored spans an editor of code copies.
func richHTML(html string) bool {
	low := strings.ToLower(html)
	for _, tag := range []string{"<h1", "<h2", "<h3", "<h4", "<ul", "<ol", "<table", "<a ", "<strong", "<b>", "<em>", "<i>", "<blockquote", "<img"} {
		if strings.Contains(low, tag) {
			return true
		}
	}
	return false
}

// dropFiles takes files dropped on the editor: images go in at the caret,
// saved next to the document, and the others go to the app.
func (e *Editor) dropFiles(paths []string) {
	var images, others []string
	for _, p := range paths {
		if isImagePath(p) {
			images = append(images, p)
		} else {
			others = append(others, p)
		}
	}
	var parts []string
	for _, f := range images {
		path := filepath.ToSlash(f)
		if e.opts.SaveImage != nil && e.opts.ImageLocalFiles != nil {
			if b, err := e.opts.ImageLocalFiles.ReadFile(f); err == nil {
				if p, err := e.opts.SaveImage(b, strings.ToLower(filepath.Ext(f))); err == nil {
					path = p
				}
			}
		}
		parts = append(parts, "!["+strings.TrimSuffix(filepath.Base(f), filepath.Ext(f))+"]("+linkDest(path)+")")
	}
	if len(parts) > 0 {
		e.insertText(strings.Join(parts, "\n\n"))
	}
	if len(others) > 0 && e.opts.OnDropFiles != nil {
		e.opts.OnDropFiles(others)
	}
}

// linkDest writes a path as a link's destination: in angle brackets when
// it has spaces.
func linkDest(p string) string {
	if strings.ContainsAny(p, " ()") {
		return "<" + p + ">"
	}
	return p
}

func isImagePath(p string) bool {
	switch strings.ToLower(filepath.Ext(p)) {
	case ".png", ".jpg", ".jpeg", ".gif", ".webp", ".bmp", ".svg", ".avif":
		return true
	}
	return false
}

// insertText inserts pasted text at the selection, its lines continuing
// the containers the caret is in, as one step of undo.
func (e *Editor) insertText(text string) {
	if text == "" {
		return
	}
	e.layoutIfNeeded()
	if e.phantom != nil && e.Session.Selection().Anchor != e.Caret() {
		e.begin(stepEdit)
		e.dropPhantom()
	}
	e.trackBlankLineInput(text, stepEdit)
	if p := e.phantom; p != nil {
		cont := phantomPrefix(p.conts)
		text = strings.ReplaceAll(text, "\n", "\n"+cont)
		e.begin(stepEdit)
		e.phantom = nil
		e.replace(p.at, p.at, p.pre+text+p.post)
		e.rememberInsertedBreaks(p.at, p.pre+text+p.post)
		c := p.at + len(p.pre) + len(text)
		e.commit(c, c)
		return
	}
	a, z := e.selection()
	if u := e.unitAt(e.Caret()); u != nil && e.tableSelection == nil {
		if e.isCell(u) {
			text = strings.ReplaceAll(strings.ReplaceAll(text, "|", `\|`), "\n", " ")
		} else if cont := e.contPrefix(u); cont != "" {
			text = strings.ReplaceAll(text, "\n", "\n"+cont)
		}
	}
	e.begin(stepEdit)
	e.replace(a, z, text)
	e.commit(a+len(text), a+len(text))
}
