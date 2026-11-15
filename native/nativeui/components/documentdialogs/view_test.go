package documentdialogs

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"muse/document"
	"muse/locales"
)

func TestRecoveryAndHistoryDialogsEmitTypedRestoreActions(t *testing.T) {
	state := &State{}
	state.OpenRecovery([]document.Draft{{Key: "draft-key"}})
	var actions []Action
	tester := ui.NewTester(func(c *ui.Context) {
		Render(c, state, func(action Action) { actions = append(actions, action) })
	}, 800, 600)
	tester.Frame()
	if err := tester.Click("draft-key"); err != nil {
		t.Fatal(err)
	}
	if len(actions) == 0 || actions[len(actions)-1] != (Action{Kind: RestoreDraft, Key: "draft-key"}) {
		t.Fatalf("recovery action = %+v", actions)
	}

	state.RecoveryOpen = false
	state.OpenHistory("note.md", []document.HistoryEntry{{ID: "entry", Revision: "1234567890", CreatedAt: time.Now()}})
	actions = nil
	tester.Frame()
	if err := tester.Click(locales.DocumentHistoryVersionLabel(state.History[0].CreatedAt.Local(), "12345678")); err != nil {
		t.Fatal(err)
	}
	if len(actions) == 0 || actions[len(actions)-1] != (Action{Kind: RestoreHistory, ID: "entry"}) {
		t.Fatalf("history action = %+v", actions)
	}
}

func TestConflictDialogEmitsMergeAction(t *testing.T) {
	state := &State{ConflictOpen: true}
	var actions []Action
	tester := ui.NewTester(func(c *ui.Context) {
		Render(c, state, func(action Action) { actions = append(actions, action) })
	}, 800, 600)
	tester.Frame()
	if err := tester.Click(locales.CommonTryMerge()); err != nil {
		t.Fatal(err)
	}
	if len(actions) == 0 || actions[len(actions)-1] != (Action{Kind: TryMerge}) {
		t.Fatalf("conflict action = %+v", actions)
	}
}
