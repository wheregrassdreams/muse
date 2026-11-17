package paragraph

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
