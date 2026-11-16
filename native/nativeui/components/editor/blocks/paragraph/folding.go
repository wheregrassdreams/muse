package editor

import (
	"github.com/egoist/mygo/ui"

	"muse/editor/blocks/paragraph"
	paragraphui "muse/nativeui/components/editor/blocks/paragraph"
)

// foldSection describes the source range hidden by one heading fold.
type foldSection struct {
	heading *leaf
	model   paragraph.FoldSection
}

func (e *Editor) resetFolds() {
	e.folded = nil
	e.foldRevision++
	e.foldSectionsCache = nil
	e.foldCacheRevision = 0
	e.unitRev = 0
	e.unitList = nil
}

func (e *Editor) foldSections() []foldSection {
	e.sync()
	if e.foldCacheRevision == e.Session.Revision() && e.foldSectionsCache != nil {
		return e.foldSectionsCache
	}
	headings := make([]*leaf, 0, len(e.railLeaves))
	modelHeadings := make([]paragraph.HeadingRange, 0, len(e.railLeaves))
	for _, l := range e.leaves {
		if l.kind == kHeading {
			headings = append(headings, l)
			modelHeadings = append(modelHeadings, paragraph.HeadingRange{
				Start: l.lineStart, End: l.end, ContentStart: foldContentStart(e.Source(), l),
				Level: l.level, Pending: l.pending,
			})
		}
	}
	blocks := make([]paragraph.BlockRange, 0, len(e.leaves))
	for _, l := range e.leaves {
		blocks = append(blocks, paragraph.BlockRange{LineStart: l.lineStart, Structural: l.isLine()})
	}
	models := paragraph.BuildFoldSections(len(e.Source()), modelHeadings, blocks)
	byStart := make(map[int]*leaf, len(headings))
	for _, h := range headings {
		byStart[h.lineStart] = h
	}
	sections := make([]foldSection, 0, len(models))
	for _, model := range models {
		if h := byStart[model.HeadingStart]; h != nil {
			sections = append(sections, foldSection{heading: h, model: model})
		}
	}
	e.foldSectionsCache, e.foldCacheRevision = sections, e.Session.Revision()
	return sections
}

func foldContentStart(source string, heading *leaf) int {
	from := blockEnd(source, heading)
	if from < len(source) && source[from] == '\r' {
		from++
	}
	if from < len(source) && source[from] == '\n' {
		from++
	}
	return from
}

func (e *Editor) foldSectionAtHeading(h *leaf) (foldSection, bool) {
	if h == nil || h.kind != kHeading || h.pending {
		return foldSection{}, false
	}
	for _, section := range e.foldSections() {
		if section.model.HeadingStart == h.lineStart {
			return section, section.model.HasContent
		}
	}
	return foldSection{}, false
}

func (e *Editor) isFoldedHeading(h *leaf) bool {
	return h != nil && e.folded[h.lineStart]
}

func (e *Editor) foldHidden(l *leaf) bool {
	if l == nil || len(e.folded) == 0 {
		return false
	}
	for _, section := range e.foldSections() {
		if e.folded[section.heading.lineStart] && l != section.heading && l.lineStart >= section.model.From && l.lineStart < section.model.To {
			return true
		}
	}
	return false
}

// isFoldedSourceOffset reports whether a source offset belongs to hidden
// content. Source selection can remain intact while the presentation stays
// folded; only explicit navigation to the offset expands its parents.
func (e *Editor) isFoldedSourceOffset(off int) bool {
	sections := e.foldSections()
	models := make([]paragraph.FoldSection, len(sections))
	folded := make([]int, 0, len(e.folded))
	for i, section := range sections {
		models[i] = section.model
		if e.folded[section.heading.lineStart] {
			folded = append(folded, section.heading.lineStart)
		}
	}
	return paragraph.FoldedContains(models, folded, off)
}

// nextVisibleSourceOffset advances from a hidden source offset to the next
// visible logical block in dir, without changing the document or fold state.
func (e *Editor) nextVisibleSourceOffset(off, dir int) int {
	if dir == 0 || !e.isFoldedSourceOffset(off) {
		return off
	}
	for _, l := range e.leaves {
		if e.foldHidden(l) {
			continue
		}
		if dir > 0 && l.lineStart > off {
			if s, ok := e.edgeStop(l, false); ok {
				return s
			}
		}
	}
	for i := len(e.leaves) - 1; i >= 0; i-- {
		l := e.leaves[i]
		if e.foldHidden(l) {
			continue
		}
		if dir < 0 && l.lineStart < off {
			if s, ok := e.edgeStop(l, true); ok {
				return s
			}
		}
	}
	for _, section := range e.foldSections() {
		if e.folded[section.heading.lineStart] && off >= section.model.From && off < section.model.To {
			return section.heading.end
		}
	}
	return off
}

func (e *Editor) toggleFold(h *leaf) {
	_, ok := e.foldSectionAtHeading(h)
	if !ok {
		return
	}
	sections, folded := e.foldModels()
	result := paragraph.ApplyFold(sections, folded, paragraph.FoldToggle, h.lineStart, e.selectionFocus())
	e.applyFoldResult(result)
	if result.Changed {
		for _, start := range result.Folded {
			if start == h.lineStart {
				e.clearTableSelection()
				break
			}
		}
	}
}

func (e *Editor) expandFoldsAt(off int) bool {
	sections, folded := e.foldModels()
	result := paragraph.ExpandFoldsAt(sections, folded, off)
	e.applyFoldResult(result)
	return result.Changed
}

func (e *Editor) expandFoldsForRange(start, end int) {
	sections, folded := e.foldModels()
	e.applyFoldResult(paragraph.ExpandFoldsForRange(sections, folded, start, end))
}

func (e *Editor) foldChevronRect(h *leaf) (x, y, size float32, ok bool) {
	if _, ok := e.foldSectionAtHeading(h); !ok || e.foldHidden(h) || h.view == nil || h.view.layout == nil {
		return 0, 0, 0, false
	}
	lines := h.view.layout.Lines()
	if len(lines) == 0 {
		return 0, 0, 0, false
	}
	chevron := paragraphui.ChevronBounds(h.x, h.y+h.view.ty, lines[0].Rect.Y, lines[0].Rect.H, e.lk.size)
	return chevron.X, chevron.Y, chevron.W, true
}

// foldHeadingAt returns the visible foldable heading under the pointer. The
// heading's laid out text lines and the adjacent left-side chevron corridor
// form one continuous hover target.
func (e *Editor) foldHeadingAt(x, y float32) *leaf {
	e.layoutIfNeeded()
	py := y
	for i := len(e.leaves) - 1; i >= 0; i-- {
		h := e.leaves[i]
		if h.kind != kHeading || e.foldHidden(h) || h.view == nil || h.view.layout == nil {
			continue
		}
		cx, cy, size, ok := e.foldChevronRect(h)
		if !ok {
			continue
		}
		lines := h.view.layout.Lines()
		chevron := ui.Rect{X: cx, Y: cy, W: size, H: size}
		for lineIndex, line := range lines {
			target := ui.Rect{
				X: h.x + h.view.tx + line.Rect.X,
				Y: h.y + h.view.ty + line.Rect.Y,
				W: line.Rect.W,
				H: line.Rect.H,
			}
			if paragraphui.HeadingLineHit(target, lineIndex == 0, chevron, x, py) {
				return h
			}
		}
		if paragraphui.ChevronCorridorHit(chevron, x, py) {
			return h
		}
	}
	return nil
}

func (e *Editor) foldChevronAt(x, y float32) *leaf {
	e.layoutIfNeeded()
	py := y
	for i := len(e.leaves) - 1; i >= 0; i-- {
		h := e.leaves[i]
		if h.kind != kHeading || e.foldHidden(h) || e.foldHover != h.lineStart || e.foldHeadingAt(x, y) != h {
			continue
		}
		cx, cy, size, ok := e.foldChevronRect(h)
		if ok && paragraphui.ChevronHit(ui.Rect{X: cx, Y: cy, W: size, H: size}, x, py) {
			return h
		}
	}
	return nil
}
