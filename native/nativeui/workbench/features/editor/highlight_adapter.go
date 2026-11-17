package editor

import inlineblock "muse/editor/blocks/inline"

// ToggleHighlight applies the source-level highlight plan selected from the
// current parser range. Delimiter recognition stays in the inline domain;
// this adapter owns only the live Editor transaction and selection update.
func (e *Editor) ToggleHighlight() {
	e.layoutIfNeeded()
	a, z := e.selection()
	if unit := e.unitAt(e.Caret()); unit != nil && unit.id >= 0 && unit.isText() {
		inlines := e.p.Parser().Inlines(unit.id)
		best := inlineblock.HighlightMark{From: -1}
		for _, mark := range inlineblock.AllHighlightRanges(e.Source(), inlines, 0, len(inlines)) {
			if mark.Open && mark.From <= a && z <= mark.To &&
				(best.From < 0 || mark.To-mark.From < best.To-best.From) {
				best = mark
			}
		}
		if best.From >= 0 {
			e.begin(stepEdit)
			e.replace(best.To-2, best.To, "")
			e.replace(best.From, best.From+2, "")
			e.commit(max(a-2, best.From), max(min(z, best.To-2)-2, best.From))
			return
		}
	}
	e.wrapInline("==", "==")
}
