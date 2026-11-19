package documentdialogs

import "github.com/egoist/mygo/ui"

// MigrationActions are the only application capabilities needed by the
// migration dialog controller. The composition root owns both use cases.
type MigrationActions struct {
	Apply func()
	Close func()
}

// MigrationController coordinates migration dialog state and user actions.
// Preview construction and applying a migration remain application concerns.
type MigrationController struct {
	handle  *Handle
	actions MigrationActions
}

func NewMigrationController(handle *Handle, actions MigrationActions) *MigrationController {
	return &MigrationController{handle: handle, actions: actions}
}

// Render shows the migration dialog and routes its typed actions to the
// callbacks supplied by the composition root.
func (c *MigrationController) Render(ctx *ui.Context, model MigrationModel) {
	if c == nil || c.handle == nil {
		return
	}
	RenderCoreMigration(ctx, c.handle, model, c.HandleAction)
}

// HandleAction is exported so non-UI command sources can use the same action
// routing and dialog lifecycle behavior.
func (c *MigrationController) HandleAction(action Action) {
	if c == nil || c.handle == nil {
		return
	}
	switch action.Kind {
	case ApplyCoreMigration:
		if c.actions.Apply != nil {
			c.actions.Apply()
		}
	case CloseCoreMigration:
		c.handle.CloseCoreMigration()
		if c.actions.Close != nil {
			c.actions.Close()
		}
	}
}
