package theme

import "image/color"

// EditorPalette contains colors for editor-only text semantics. These stay
// separate from both the application UI palette and Markdown content colors.
type EditorPalette struct {
	Muted color.RGBA
}

// EditorColors returns editor-specific colors for virtual or secondary text,
// such as frontmatter source and other dimmed editor content.
func EditorColors(dark bool) EditorPalette {
	if dark {
		return EditorPalette{Muted: hexRGBA(0x858585)}
	}
	return EditorPalette{Muted: hexRGBA(0x777774)}
}
