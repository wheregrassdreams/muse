package nativeui

import "github.com/egoist/mygo/ui"

// These dimensions are part of the existing canvas design. Keep them in the
// shell geometry while the app and editor surface are composed by separate
// packages.
const (
	canvasPadding float32 = 8
	cardBorder    float32 = 1
	editorHeaderH float32 = 44
)

// editorHeader is retained as the narrow callback used by hosted image
// documents. The implementation is the same header rendered by the note
// canvas, so both content types keep the same controls and dimensions.
func (a *App) editorHeader(c *ui.Context) { a.renderEditorHeader(c, false) }

func (a *App) headerH(c *ui.Context) float32 {
	height := float32(editorHeaderH)
	if a != nil && a.flat && c != nil && c.TitleBar().Height > height {
		height = c.TitleBar().Height
	}
	return height
}

// canvasLeft follows the card as the context panel opens and closes, while
// reserving the traffic-light inset when the panel is folded.
func (a *App) canvasLeft(c *ui.Context) float32 {
	if a == nil || a.flat {
		return 0
	}
	left := float32(canvasPadding)
	if a.panelWidth <= 0 || c == nil {
		return left
	}
	openness := a.panelW / a.panelWidth
	if openness > 1 {
		openness = 1
	}
	if openness < 0 {
		openness = 0
	}
	if extra := c.TitleBar().Left - 56; extra > 0 {
		left += extra * (1 - openness)
	}
	return left
}
