package editor

import (
	"github.com/egoist/mygo/ui"

	"muse/editor"
	"muse/nativeui/window"
)

// ProductionContent is a temporary migration bridge from the production
// editor's existing virtualized view to the generic pane lifecycle. It keeps
// existing behavior while toolkit input/render responsibilities move into
// this component; it is not the final editor boundary because Editor.View
// still owns MyGo work.
type ProductionContent struct {
	editor  *editor.Editor
	onClose func()
	closed  bool
}

// NewProductionContent hosts the existing production editor as pane content.
// This bridge should be removed when the migrated Session-backed component
// owns the production rendering and input path.
func NewProductionContent(value *editor.Editor, onClose func()) *ProductionContent {
	return &ProductionContent{editor: value, onClose: onClose}
}

func (c *ProductionContent) Render(ctx *ui.Context) {
	if c == nil || c.closed || c.editor == nil {
		return
	}
	c.editor.View(ctx)
}

func (c *ProductionContent) Close() {
	if c == nil || c.closed {
		return
	}
	c.closed = true
	if c.onClose != nil {
		c.onClose()
	}
}

var _ window.Content = (*ProductionContent)(nil)
