package paragraphui

import "muse/editor/blocks/paragraph"

// BreakProjectionInput is a detached snapshot of parser, view-policy, and
// editor-owned typed-break facts for one source break.
type BreakProjectionInput struct {
	Start, End      int
	SoftBreak       bool
	CollapseSoft    bool
	PreserveLine    bool
	StructuralBreak bool
	RevealSource    bool
	EditedLine      bool
}

// ProjectBreak maps detached source facts to paragraph render policy. It does
// not mutate anchors, source, selection, or history.
func ProjectBreak(input BreakProjectionInput) paragraph.BreakProjection {
	return paragraph.BreakProjection{
		SoftBreak:          input.SoftBreak,
		CollapseSoftBreaks: input.CollapseSoft,
		PreserveLine:       input.PreserveLine,
		EditingLine:        input.EditedLine,
		StructuralBreak:    input.StructuralBreak,
		RevealSource:       input.RevealSource,
	}
}
