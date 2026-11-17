package navigation

// SourceBlockKind identifies source-level block forms whose navigation range
// extends beyond the parsed content end.
type SourceBlockKind uint8

const (
	SourceBlockCode SourceBlockKind = iota + 1
	SourceBlockMath
	SourceBlockHeading
)

// SourceBlock describes the parser facts needed to find a block's source end.
// Offsets are UTF-8 bytes into the original Markdown source.
type SourceBlock struct {
	LineStart  int
	ContentEnd int
	Kind       SourceBlockKind
	Fenced     bool
	Setext     bool
}

// BlockEnd returns the source end of a block, including a closing fence or a
// setext underline when the supplied parser facts indicate one is present.
func BlockEnd(source string, block SourceBlock) int {
	end := LineEnd(source, max(block.ContentEnd, block.LineStart))
	switch block.Kind {
	case SourceBlockCode, SourceBlockMath:
		if block.Kind == SourceBlockCode && !block.Fenced {
			return end
		}
		if end < len(source) {
			next := LineEnd(source, end+1)
			if IsClosingFence(source[end+1:next], block.Kind) {
				return next
			}
		}
	case SourceBlockHeading:
		if block.Setext && end < len(source) {
			return LineEnd(source, end+1)
		}
	}
	return end
}

// IsClosingFence reports whether line begins with a supported closing fence
// after Markdown container prefixes. Matching follows the editor's existing
// source-navigation contract for code and display-math blocks.
func IsClosingFence(line string, kind SourceBlockKind) bool {
	line = TrimBlockPrefix(line)
	if kind == SourceBlockMath {
		return len(line) >= 2 && (line[:2] == "$$" || line[:2] == `\]`)
	}
	return kind == SourceBlockCode && len(line) >= 3 && (line[:3] == "```" || line[:3] == "~~~")
}

// TrimBlockPrefix strips the ASCII spaces, tabs, and quote markers that can
// prefix a fenced block line inside Markdown containers.
func TrimBlockPrefix(line string) string {
	for len(line) > 0 && (line[0] == ' ' || line[0] == '\t' || line[0] == '>') {
		line = line[1:]
	}
	return line
}
