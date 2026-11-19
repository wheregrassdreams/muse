package paragraphui

// BreakProjection is a detached view policy for a parsed source break. The
// host maps its parser facts to this value; no Markdown package is required.
type BreakProjection struct {
	SoftBreak          bool
	CollapseSoftBreaks bool
	PreserveLine       bool
	EditingLine        bool
	StructuralBreak    bool
	RevealSource       bool
}

func (p BreakProjection) Newline() bool {
	return !(p.SoftBreak && p.CollapseSoftBreaks && !p.PreserveLine &&
		!p.EditingLine && !p.StructuralBreak && !p.RevealSource)
}

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
func ProjectBreak(input BreakProjectionInput) BreakProjection {
	return BreakProjection{
		SoftBreak:          input.SoftBreak,
		CollapseSoftBreaks: input.CollapseSoft,
		PreserveLine:       input.PreserveLine,
		EditingLine:        input.EditedLine,
		StructuralBreak:    input.StructuralBreak,
		RevealSource:       input.RevealSource,
	}
}
