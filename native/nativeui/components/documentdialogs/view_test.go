package documentdialogs

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"muse/document"
	"muse/locales"
)

func TestRecoveryAndHistoryDialogsEmitTypedRestoreActions(t *testing.T) {
	handle := NewHandle()
	handle.OpenRecovery([]document.Draft{{Key: "draft-key"}})
	var actions []Action
	tester := ui.NewTester(func(c *ui.Context) {
		Render(c, handle, func(action Action) { actions = append(actions, action) })
	}, 800, 600)
	tester.Frame()
	if err := tester.Click("draft-key"); err != nil {
		t.Fatal(err)
	}
	if len(actions) == 0 || actions[len(actions)-1] != (Action{Kind: RestoreDraft, Key: "draft-key"}) {
		t.Fatalf("recovery action = %+v", actions)
	}

	handle.Apply(Action{Kind: CloseRecovery})
	handle.OpenHistory("note.md", []document.HistoryEntry{{ID: "entry", Revision: "1234567890", CreatedAt: time.Now()}})
	actions = nil
	tester.Frame()
	if err := tester.Click(locales.DocumentHistoryVersionLabel(handle.HistoryEntries()[0].CreatedAt.Local(), "12345678")); err != nil {
		t.Fatal(err)
	}
	if len(actions) == 0 || actions[len(actions)-1] != (Action{Kind: RestoreHistory, ID: "entry"}) {
		t.Fatalf("history action = %+v", actions)
	}
}

func TestConflictDialogEmitsMergeAction(t *testing.T) {
	handle := NewHandle()
	handle.Apply(Action{Kind: OpenConflict})
	var actions []Action
	tester := ui.NewTester(func(c *ui.Context) {
		Render(c, handle, func(action Action) { actions = append(actions, action) })
	}, 800, 600)
	tester.Frame()
	if err := tester.Click(locales.CommonTryMerge()); err != nil {
		t.Fatal(err)
	}
	if len(actions) == 0 || actions[len(actions)-1] != (Action{Kind: TryMerge}) {
		t.Fatalf("conflict action = %+v", actions)
	}
}

func TestHandleExposesOnlyDetachedWorkflowSnapshot(t *testing.T) {
	handle := NewHandle()
	handle.OpenHistory("note.md", []document.HistoryEntry{{ID: "entry", Revision: "revision"}})
	if got := handle.Snapshot(); got.HistoryPath != "note.md" || !handle.ModalOpen() || handle.IsConflictOpen() {
		t.Fatalf("snapshot = %+v", got)
	}

	entries := handle.HistoryEntries()
	entries[0].ID = "mutated"
	if got := handle.HistoryEntries()[0].ID; got != "entry" {
		t.Fatalf("history entries were not copied: ID = %q", got)
	}

	handle.Close()
	if handle.ModalOpen() || handle.HistoryPath() != "" || len(handle.HistoryEntries()) != 0 {
		t.Fatalf("handle did not close and release dialog data: %+v", handle.Snapshot())
	}
}
