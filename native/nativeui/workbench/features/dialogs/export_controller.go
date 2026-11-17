// Package dialogs coordinates dialog actions that cross into application use
// cases. It owns the action contract, while the composition root supplies the
// handler that performs the export.
package dialogs

import (
	"github.com/egoist/mygo/ui"

	documentdialogs "muse/nativeui/workbench/features/dialogs/document"
)

// ExportAction is the user's request to export the current document. The
// application composition root decides how to execute it.
type ExportAction struct {
	Format documentdialogs.ExportFormat
}

// ExportController translates the document dialog's menu choice into an
// application-facing action without depending on App or export use cases.
type ExportController struct {
	dispatch func(ExportAction)
}

func NewExportController(dispatch func(ExportAction)) *ExportController {
	return &ExportController{dispatch: dispatch}
}

// AddMenu adds the document export choices and forwards selections through
// the controller's narrow action callback.
func (c *ExportController) AddMenu(menu *ui.Menu) {
	if c == nil {
		return
	}
	documentdialogs.AddExportMenu(menu, func(format documentdialogs.ExportFormat) {
		c.Select(format)
	})
}

// Select dispatches an export choice. It is also useful to action sources
// other than the menu, such as commands and keyboard shortcuts.
func (c *ExportController) Select(format documentdialogs.ExportFormat) {
	if c != nil && c.dispatch != nil {
		c.dispatch(ExportAction{Format: format})
	}
}
