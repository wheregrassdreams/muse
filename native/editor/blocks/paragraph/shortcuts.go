package paragraph

import "strings"

// BlockShortcutInput contains source and parser facts needed to decide
// whether typed text turns the current paragraph into a fenced block.
type BlockShortcutInput struct {
	Source                         string
	Typed                          string
	LineStart, ContentStart, Caret int
	LineEnd                        int
	ContainerPrefix                string
	TextUnit, OpeningListItem      bool
}

// BlockShortcutPlan describes the replacement for a typed block opener and
// any trailing line whitespace.
type BlockShortcutPlan struct {
	Start, End int
	Text       string
	Caret      int
}

// PlanBlockShortcut applies the paragraph eligibility and source-edit policy
// for typing a fenced-code or math opener at the start of a paragraph.
func PlanBlockShortcut(in BlockShortcutInput) (BlockShortcutPlan, bool) {
	if !in.TextUnit || in.ContentStart < in.LineStart || in.Caret < in.ContentStart || in.LineEnd < in.Caret ||
		!validBoundary(in.Source, in.LineStart) || !validBoundary(in.Source, in.ContentStart) ||
		!validBoundary(in.Source, in.Caret) || !validBoundary(in.Source, in.LineEnd) {
		return BlockShortcutPlan{}, false
	}
	if strings.TrimSpace(in.Source[in.Caret:in.LineEnd]) != "" {
		return BlockShortcutPlan{}, false
	}
	if prefix := in.Source[in.LineStart:in.ContentStart]; strings.Trim(prefix, " \t>") != "" && !in.OpeningListItem {
		return BlockShortcutPlan{}, false
	}
	block, caret, ok := BlockShortcut(in.Source[in.ContentStart:in.Caret]+in.Typed, in.ContainerPrefix)
	if !ok {
		return BlockShortcutPlan{}, false
	}
	return BlockShortcutPlan{
		Start: in.ContentStart, End: in.LineEnd,
		Text: block, Caret: in.ContentStart + caret,
	}, true
}

// BlockShortcut returns the source and caret offset created by typing a
// block opener alone at the start of a paragraph. containerPrefix is copied
// onto generated lines so the block remains inside its quote/list container.
func BlockShortcut(text, containerPrefix string) (source string, caret int, ok bool) {
	switch text {
	case "```", "~~~":
		return text + "\n" + containerPrefix + "\n" + containerPrefix + text, len(text), true
	case "$$":
		return "$$\n" + containerPrefix + "\n" + containerPrefix + "$$", 3 + len(containerPrefix), true
	default:
		return "", 0, false
	}
}
