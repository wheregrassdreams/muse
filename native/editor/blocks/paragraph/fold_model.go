package paragraph

import "sort"

// HeadingContentStart advances a parser-projected heading block end past its
// optional CR and LF line-ending bytes. The returned value is a source byte
// offset suitable as FoldSection.ContentStart.
func HeadingContentStart(source string, blockEnd int) int {
	blockEnd = min(max(blockEnd, 0), len(source))
	if blockEnd < len(source) && source[blockEnd] == '\r' {
		blockEnd++
	}
	if blockEnd < len(source) && source[blockEnd] == '\n' {
		blockEnd++
	}
	return blockEnd
}

// HeadingRange is the parser-derived source geometry required to compute a
// heading's fold section. ContentStart is the first byte after the heading
// line and its line ending.
type HeadingRange struct {
	Start, End   int
	ContentStart int
	Level        int
	Pending      bool
}

// BlockRange is the source location of a parsed block. Structural line
// markers such as blank lines do not count as foldable section content.
type BlockRange struct {
	LineStart  int
	Structural bool
}

// FoldSection is the source interval hidden by folding one heading.
type FoldSection struct {
	HeadingStart int
	HeadingEnd   int
	From, To     int
	HasContent   bool
}

// Hides reports whether a folded section hides the given source offset.
func (section FoldSection) Hides(folded bool, offset int) bool {
	return folded && offset >= section.From && offset < section.To
}

// FoldOperation is a paragraph-owned folding intent. Input keymaps may
// produce these values, but section and folded-state policy stays here.
type FoldOperation uint8

const (
	FoldToggle FoldOperation = iota + 1
	FoldClose
	FoldOpen
	FoldToggleRecursive
	FoldCloseRecursive
	FoldOpenRecursive
	FoldReveal
	FoldCloseAll
	FoldOpenAll
)

// FoldResult is the detached state transition produced by one folding intent.
// The caller owns installing Folded and applying Caret when MoveCaret is set.
type FoldResult struct {
	Folded    []int
	Handled   bool
	Changed   bool
	MoveCaret bool
	Caret     int
}

// BuildFoldSections computes foldable sections from parser-owned heading and
// block ranges. Inputs must be ordered by source position. The package does
// not retain slices or depend on editor layout objects.
func BuildFoldSections(sourceLength int, headings []HeadingRange, blocks []BlockRange) []FoldSection {
	sections := make([]FoldSection, 0, len(headings))
	for i, heading := range headings {
		if heading.Pending || heading.Level <= 0 || heading.Start < 0 || heading.End < heading.Start || heading.ContentStart < heading.End {
			continue
		}
		from := min(max(heading.ContentStart, 0), sourceLength)
		to := sourceLength
		for _, next := range headings[i+1:] {
			if !next.Pending && next.Level > 0 && next.Level <= heading.Level {
				to = min(max(next.Start, from), sourceLength)
				break
			}
		}
		hasContent := false
		for _, block := range blocks {
			if !block.Structural && block.LineStart >= from && block.LineStart < to {
				hasContent = true
				break
			}
		}
		sections = append(sections, FoldSection{
			HeadingStart: heading.Start,
			HeadingEnd:   heading.End,
			From:         from, To: to, HasContent: hasContent,
		})
	}
	return sections
}

// FoldedContains reports whether a source offset belongs to a folded section.
func FoldedContains(sections []FoldSection, foldedHeadingStarts []int, offset int) bool {
	for _, section := range sections {
		if containsOffset(foldedHeadingStarts, section.HeadingStart) && section.Hides(true, offset) {
			return true
		}
	}
	return false
}

// FoldSectionAtOffset returns the innermost foldable section containing a
// source offset. The offset may be inside the heading itself or its content.
func FoldSectionAtOffset(sections []FoldSection, offset int) (FoldSection, bool) {
	var selected FoldSection
	found := false
	for _, section := range sections {
		if !section.HasContent || offset < section.HeadingStart || offset >= section.To {
			continue
		}
		if !found || section.HeadingStart > selected.HeadingStart {
			selected, found = section, true
		}
	}
	return selected, found
}

// ApplyFold applies one action to headingStart and returns a new sorted folded
// heading snapshot. Closing a section moves the caret to its heading end when
// the current focus would become hidden.
func ApplyFold(sections []FoldSection, folded []int, operation FoldOperation, headingStart, selectionFocus int) FoldResult {
	state := canonicalFolded(folded)
	result := FoldResult{Folded: state}
	var target FoldSection
	for _, section := range sections {
		if section.HeadingStart == headingStart && section.HasContent {
			target = section
			result.Handled = true
			break
		}
	}
	if !result.Handled {
		return result
	}
	closed := hasFold(state, headingStart)
	close := false
	switch operation {
	case FoldToggle:
		close = !closed
	case FoldClose:
		close = true
	case FoldOpen:
		close = false
	case FoldToggleRecursive:
		close = !closed
	case FoldCloseRecursive:
		close = true
	case FoldOpenRecursive:
		close = false
	default:
		return FoldResult{Folded: state}
	}
	if operation == FoldToggle || operation == FoldClose || operation == FoldOpen {
		state = setFold(state, headingStart, close)
	} else {
		for _, section := range sections {
			if section.HasContent && section.HeadingStart >= target.HeadingStart && section.HeadingStart < target.To {
				state = setFold(state, section.HeadingStart, close)
			}
		}
	}
	result.Folded = state
	result.Changed = !sameFolded(folded, state)
	if close && FoldedContains(sections, state, selectionFocus) {
		result.MoveCaret, result.Caret = true, target.HeadingEnd
	}
	return result
}

// ApplyFoldAtOffset resolves the deepest foldable heading covering offset and
// applies its action. FoldReveal expands every closed section hiding offset.
func ApplyFoldAtOffset(sections []FoldSection, folded []int, operation FoldOperation, offset, selectionFocus int) FoldResult {
	if operation == FoldReveal {
		return ExpandFoldsAt(sections, folded, offset)
	}
	target, ok := FoldSectionAtOffset(sections, offset)
	if !ok {
		return FoldResult{Folded: canonicalFolded(folded)}
	}
	return ApplyFold(sections, folded, operation, target.HeadingStart, selectionFocus)
}

// ApplyFoldAll closes every foldable section or opens all sections.
func ApplyFoldAll(sections []FoldSection, folded []int, operation FoldOperation) FoldResult {
	state := canonicalFolded(folded)
	var next []int
	switch operation {
	case FoldCloseAll:
		for _, section := range sections {
			if section.HasContent {
				next = append(next, section.HeadingStart)
			}
		}
	case FoldOpenAll:
		next = nil
	default:
		return FoldResult{Folded: state}
	}
	next = canonicalFolded(next)
	return FoldResult{Folded: next, Handled: true, Changed: !sameFolded(state, next)}
}

// ExpandFoldsAt opens all closed sections containing offset.
func ExpandFoldsAt(sections []FoldSection, folded []int, offset int) FoldResult {
	state := canonicalFolded(folded)
	next := make([]int, 0, len(state))
	for _, start := range state {
		section, ok := findFoldSection(sections, start)
		if ok && offset >= section.From && offset < section.To {
			continue
		}
		next = append(next, start)
	}
	changed := !sameFolded(state, next)
	return FoldResult{Folded: next, Handled: true, Changed: changed}
}

// ExpandFoldsForRange opens each folded section overlapped by [start,end].
func ExpandFoldsForRange(sections []FoldSection, folded []int, start, end int) FoldResult {
	state := canonicalFolded(folded)
	if end < start {
		start, end = end, start
	}
	next := make([]int, 0, len(state))
	for _, headingStart := range state {
		section, ok := findFoldSection(sections, headingStart)
		if ok && start < section.To && end >= section.From {
			continue
		}
		next = append(next, headingStart)
	}
	changed := !sameFolded(state, next)
	return FoldResult{Folded: next, Handled: true, Changed: changed}
}

func findFoldSection(sections []FoldSection, headingStart int) (FoldSection, bool) {
	i := sort.Search(len(sections), func(i int) bool { return sections[i].HeadingStart >= headingStart })
	if i < len(sections) && sections[i].HeadingStart == headingStart {
		return sections[i], true
	}
	return FoldSection{}, false
}

func canonicalFolded(starts []int) []int {
	if len(starts) == 0 {
		return nil
	}
	result := append([]int(nil), starts...)
	sort.Ints(result)
	write := 1
	for read := 1; read < len(result); read++ {
		if result[read] != result[write-1] {
			result[write] = result[read]
			write++
		}
	}
	return result[:write]
}

func hasFold(starts []int, headingStart int) bool {
	i := sort.SearchInts(starts, headingStart)
	return i < len(starts) && starts[i] == headingStart
}

func setFold(starts []int, headingStart int, folded bool) []int {
	i := sort.SearchInts(starts, headingStart)
	if folded {
		if i < len(starts) && starts[i] == headingStart {
			return starts
		}
		starts = append(starts, 0)
		copy(starts[i+1:], starts[i:])
		starts[i] = headingStart
		return starts
	}
	if i < len(starts) && starts[i] == headingStart {
		return append(starts[:i], starts[i+1:]...)
	}
	return starts
}

func sameFolded(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func containsOffset(sorted []int, value int) bool {
	for _, candidate := range sorted {
		if candidate == value {
			return true
		}
	}
	return false
}
