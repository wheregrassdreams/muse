package commands

import (
	"errors"
	"testing"

	"muse/nativeui/workbench/context"
)

func TestActionDispatcherChecksContextAndDispatches(t *testing.T) {
	called := false
	d, err := NewActionDispatcher(PanelAction{
		ID: ActionRename,
		CanRun: func(ctx ActionContext) bool {
			return ctx.Panel == PanelIDFiles && ctx.Focus.Target == FocusTargetTree && ctx.Focus.HasSelection
		},
		Run: func(ctx ActionContext) error {
			called = true
			if ctx.Values["path"] != "note.md" {
				t.Fatalf("action context value = %v", ctx.Values["path"])
			}
			return nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	ctx := ActionContext{Panel: PanelIDFiles, Focus: FocusState{Target: FocusTargetTree, HasSelection: true}, Values: map[string]any{"path": "note.md"}}
	if !d.CanExecute(ActionRename, ctx) {
		t.Fatal("selected tree item should make rename executable")
	}
	if err := d.Dispatch(ActionRename, ctx); err != nil {
		t.Fatal(err)
	}
	if !called {
		t.Fatal("action handler was not called")
	}
}

func TestActionDispatcherRejectsUnavailableAndUnknownActions(t *testing.T) {
	d, err := NewActionDispatcher(PanelAction{
		ID:     ActionPaste,
		CanRun: func(ActionContext) bool { return false },
		Run:    func(ActionContext) error { t.Fatal("disabled action ran"); return nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	if d.CanExecute(ActionPaste, ActionContext{}) {
		t.Fatal("disabled action reported executable")
	}
	if err := d.Dispatch(ActionPaste, ActionContext{}); !errors.Is(err, ErrActionDisabled) {
		t.Fatalf("disabled dispatch error = %v", err)
	}
	if err := d.Dispatch(ActionID("editor.unknown"), ActionContext{}); !errors.Is(err, ErrUnknownAction) {
		t.Fatalf("unknown dispatch error = %v", err)
	}
}

func TestSemanticActionIDsAreStableAndDispatchable(t *testing.T) {
	ids := []ActionID{
		ActionNavigate, ActionOpen, ActionToggleExpanded, ActionSelectAll,
		ActionSelectRange, ActionRename, ActionCopy, ActionCut, ActionPaste,
		ActionTrash, ActionCreateFile, ActionCreateFolder, ActionCopyPath,
		ActionRevealInFinder, ActionRefreshPanel, ActionShowHelp, ActionCancel,
		ActionFocusPanel, ActionFocusEditor, ActionFocusNextPanel,
		ActionFocusPreviousPanel, ActionFocusDirection,
	}
	seen := make(map[ActionID]bool, len(ids))
	actions := make([]PanelAction, 0, len(ids))
	for _, id := range ids {
		if id == "" || seen[id] {
			t.Fatalf("invalid or duplicate semantic action ID %q", id)
		}
		seen[id] = true
		actions = append(actions, PanelAction{ID: id, Run: func(ActionContext) error { return nil }})
	}
	d, err := NewActionDispatcher(actions...)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range ids {
		if !d.CanExecute(id, ActionContext{}) {
			t.Errorf("semantic action %q is not executable", id)
		}
		if err := d.Dispatch(id, ActionContext{}); err != nil {
			t.Errorf("dispatch %q: %v", id, err)
		}
	}
}

func TestActionDispatcherAllowsUnconditionalActionsAndRejectsDuplicateIDs(t *testing.T) {
	action := PanelAction{ID: "app.help", Run: func(ActionContext) error { return nil }}
	d, err := NewActionDispatcher(action)
	if err != nil {
		t.Fatal(err)
	}
	if !d.CanExecute(action.ID, ActionContext{}) {
		t.Fatal("action without a predicate should be executable")
	}
	if err := d.Register(action); !errors.Is(err, ErrDuplicateAction) {
		t.Fatalf("duplicate registration error = %v", err)
	}
}

func TestActionDispatcherEvaluatesContextRuleForAvailabilityAndDispatch(t *testing.T) {
	vimMode := context.NewKey[bool]("editor.vim-mode")
	called := false
	d, err := NewActionDispatcher(PanelAction{
		ID:   ActionNavigate,
		When: context.Equals(vimMode, true),
		Run: func(ActionContext) error {
			called = true
			return nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx := ActionContext{Context: context.Compose(context.With(context.Scope{}, vimMode, false), context.Scope{}, context.Scope{})}
	if d.Evaluate(ActionNavigate, ctx) || d.CanExecute(ActionNavigate, ctx) {
		t.Fatal("action should be unavailable outside Vim mode")
	}
	if err := d.Dispatch(ActionNavigate, ctx); !errors.Is(err, ErrActionDisabled) {
		t.Fatalf("dispatch outside Vim mode error = %v", err)
	}
	if called {
		t.Fatal("unavailable action handler ran")
	}
	ctx.Context = context.Compose(context.With(context.Scope{}, vimMode, true), context.Scope{}, context.Scope{})
	if !d.Evaluate(ActionNavigate, ctx) || !d.CanExecute(ActionNavigate, ctx) {
		t.Fatal("action should be available in Vim mode")
	}
	if err := d.Dispatch(ActionNavigate, ctx); err != nil || !called {
		t.Fatalf("dispatch in Vim mode: err=%v called=%v", err, called)
	}
}

func TestActionDispatcherRequiresStableIDAndHandler(t *testing.T) {
	if _, err := NewActionDispatcher(PanelAction{Run: func(ActionContext) error { return nil }}); err == nil {
		t.Fatal("empty action ID was accepted")
	}
	if _, err := NewActionDispatcher(PanelAction{ID: "app.help"}); err == nil {
		t.Fatal("missing handler was accepted")
	}
}
