package commands

import (
	"errors"
	"fmt"

	"muse/nativeui/context"
)

// PanelID identifies the panel that owns the current interaction context.
// Values are stable strings so action IDs and diagnostics do not depend on
// panelView's internal numeric values.
type PanelID string

const (
	PanelIDFiles   PanelID = "files"
	PanelIDOutline PanelID = "outline"
	PanelIDAI      PanelID = "ai"
	PanelIDSearch  PanelID = "search"
	PanelIDEditor  PanelID = "editor"
)

// FocusTarget describes the semantic control that currently owns keyboard
// focus. It deliberately does not encode key bindings.
type FocusTarget string

const (
	FocusTargetNone       FocusTarget = "none"
	FocusTargetTree       FocusTarget = "tree"
	FocusTargetEditor     FocusTarget = "editor"
	FocusTargetSearch     FocusTarget = "search"
	FocusTargetTextInput  FocusTarget = "text-input"
	FocusTargetAIComposer FocusTarget = "ai-composer"
)

// FocusState is a snapshot of focus relevant to action availability.
type FocusState struct {
	Target        FocusTarget
	Editable      bool
	HasSelection  bool
	SelectionSize int
}

// ActionContext carries the state needed to decide whether an action can run.
// Values lets callers pass action-specific data without coupling the action
// registry to App or a particular panel implementation.
type ActionContext struct {
	Panel   PanelID
	Focus   FocusState
	Values  map[string]any
	Context context.Snapshot
}

// ActionID is a stable identifier such as "files.rename" or "editor.copy".
type ActionID string

// Stable semantic action IDs shared by panel and editor integrations.
// Callers bind these IDs to menus, shortcuts, or other UI independently.
const (
	ActionNavigate           ActionID = "navigation.navigate"
	ActionOpen               ActionID = "navigation.open"
	ActionToggleExpanded     ActionID = "navigation.toggle-expanded"
	ActionCloseNode          ActionID = "navigation.close-node"
	ActionCollapseAll        ActionID = "navigation.collapse-all"
	ActionSelectAll          ActionID = "selection.select-all"
	ActionSelectRange        ActionID = "selection.select-range"
	ActionToggleSelection    ActionID = "selection.toggle"
	ActionInvertSelection    ActionID = "selection.invert-visible"
	ActionRename             ActionID = "files.rename"
	ActionCopy               ActionID = "edit.copy"
	ActionCut                ActionID = "edit.cut"
	ActionPaste              ActionID = "edit.paste"
	ActionTrash              ActionID = "files.trash"
	ActionCreateFile         ActionID = "files.create-file"
	ActionCreateFolder       ActionID = "files.create-folder"
	ActionCopyPath           ActionID = "files.copy-path"
	ActionRevealInFinder     ActionID = "files.reveal-in-finder"
	ActionRefreshPanel       ActionID = "panel.refresh"
	ActionShowHelp           ActionID = "panel.help"
	ActionCancel             ActionID = "app.cancel"
	ActionFocusPanel         ActionID = "panel.focus"
	ActionFocusEditor        ActionID = "editor.focus"
	ActionFocusNextPanel     ActionID = "panel.focus-next"
	ActionFocusPreviousPanel ActionID = "panel.focus-previous"
	ActionFocusDirection     ActionID = "panel.focus-direction"
)

// PanelAction separates availability from execution so menus and other UI can
// query enabled state without invoking the action.
type PanelAction struct {
	ID     ActionID
	When   context.Rule
	CanRun func(ActionContext) bool
	Run    func(ActionContext) error
}

var (
	ErrDuplicateAction = errors.New("duplicate action ID")
	ErrUnknownAction   = errors.New("unknown action ID")
	ErrActionDisabled  = errors.New("action is not executable")
)

// ActionDispatcher stores actions by stable ID. It has no knowledge of
// shortcuts or UI routing; callers decide when to query or dispatch actions.
type ActionDispatcher struct {
	actions map[ActionID]PanelAction
}

func NewActionDispatcher(actions ...PanelAction) (*ActionDispatcher, error) {
	d := &ActionDispatcher{actions: make(map[ActionID]PanelAction, len(actions))}
	for _, action := range actions {
		if err := d.Register(action); err != nil {
			return nil, err
		}
	}
	return d, nil
}

func (d *ActionDispatcher) Register(action PanelAction) error {
	if action.ID == "" {
		return fmt.Errorf("action ID must not be empty")
	}
	if action.Run == nil {
		return fmt.Errorf("action %q has no handler", action.ID)
	}
	if d.actions == nil {
		d.actions = make(map[ActionID]PanelAction)
	}
	if _, exists := d.actions[action.ID]; exists {
		return fmt.Errorf("%w: %q", ErrDuplicateAction, action.ID)
	}
	d.actions[action.ID] = action
	return nil
}

func (d *ActionDispatcher) CanExecute(id ActionID, ctx ActionContext) bool {
	return d.Evaluate(id, ctx)
}

// Evaluate reports whether the registered action is available in ctx. The
// declarative context rule and legacy CanRun predicate are both applied here
// so menus and dispatch follow the same availability semantics.
func (d *ActionDispatcher) Evaluate(id ActionID, ctx ActionContext) bool {
	if d == nil {
		return false
	}
	action, exists := d.actions[id]
	if !exists {
		return false
	}
	return action.When.Matches(ctx.Context) && (action.CanRun == nil || action.CanRun(ctx))
}

func (d *ActionDispatcher) Dispatch(id ActionID, ctx ActionContext) error {
	if d == nil {
		return fmt.Errorf("%w: %q", ErrUnknownAction, id)
	}
	action, exists := d.actions[id]
	if !exists {
		return fmt.Errorf("%w: %q", ErrUnknownAction, id)
	}
	if !d.Evaluate(id, ctx) {
		return fmt.Errorf("%w: %q", ErrActionDisabled, id)
	}
	return action.Run(ctx)
}
