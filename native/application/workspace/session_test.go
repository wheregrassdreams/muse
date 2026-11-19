package workspace

import (
	"math"
	"path/filepath"
	"reflect"
	"testing"

	domain "muse/workspace"
)

func TestSwitchVaultClearsPreviousDocumentsAndPreservesSameVault(t *testing.T) {
	root := t.TempDir()
	initial := workspaceInRoot(root)
	session, err := NewSession(nil, initial)
	if err != nil {
		t.Fatal(err)
	}
	activePath := filepath.Join(root, "left.md")
	same, err := session.SwitchVault(filepath.Join(root, "child", ".."), activePath)
	if err != nil {
		t.Fatal(err)
	}
	if same.VaultChanged || same.ResetWorkspace || same.Cause != VaultResetNone || same.ActiveDocumentPath != activePath || len(same.State.Tabs) != len(initial.Tabs) {
		t.Fatalf("same-vault plan = %+v", same)
	}

	newRoot := t.TempDir()
	plan, err := session.SwitchVault(newRoot, filepath.Join(newRoot, "new.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !plan.VaultChanged || !plan.ResetWorkspace || plan.Cause != VaultResetRootChanged || plan.PreviousVaultPath != root || plan.VaultPath != newRoot || plan.DefaultTab.Decision != CreateBlankTab || plan.DefaultTab.TabID != DefaultTabID || plan.DefaultTab.PaneID != DefaultPaneID {
		t.Fatalf("vault switch plan = %+v", plan)
	}
	want := domain.State{Version: 1, VaultPath: newRoot}
	if got := session.Snapshot(); !reflect.DeepEqual(got, want) {
		t.Fatalf("switch snapshot = %+v, want %+v", got, want)
	}
	if got := session.FocusHistory("main"); len(got) != 0 {
		t.Fatalf("vault switch retained focus history: %v", got)
	}
}

func TestSwitchVaultResetsSameRootWhenActiveDocumentIsExternal(t *testing.T) {
	root := t.TempDir()
	session, err := NewSession(nil, workspaceInRoot(root))
	if err != nil {
		t.Fatal(err)
	}
	externalPath := filepath.Join(t.TempDir(), "outside.md")
	plan, err := session.SwitchVault(filepath.Join(root, "child", ".."), externalPath)
	if err != nil {
		t.Fatal(err)
	}
	if plan.VaultChanged || !plan.ResetWorkspace || plan.Cause != VaultResetDocumentOutsideTarget || plan.ActiveDocumentPath != externalPath || len(plan.State.Tabs) != 0 {
		t.Fatalf("same-root external document plan=%+v", plan)
	}
}

func TestSwitchVaultDistinguishesRootChangeFromExternalDocument(t *testing.T) {
	oldRoot := t.TempDir()
	newRoot := t.TempDir()
	session, err := NewSession(nil, workspaceInRoot(oldRoot))
	if err != nil {
		t.Fatal(err)
	}
	externalPath := filepath.Join(t.TempDir(), "outside.md")
	plan, err := session.SwitchVault(newRoot, externalPath)
	if err != nil {
		t.Fatal(err)
	}
	if !plan.VaultChanged || !plan.ResetWorkspace || plan.Cause != VaultResetRootAndDocumentOutsideTarget || plan.ActiveDocumentPath != externalPath {
		t.Fatalf("cross-root external document plan=%+v", plan)
	}
}

func TestSessionRestoreReturnsOrderedPanePlan(t *testing.T) {
	root := t.TempDir()
	state := workspaceInRoot(root)
	session, err := NewSession(nil, domain.State{Version: 1, VaultPath: root})
	if err != nil {
		t.Fatal(err)
	}
	plan, err := session.Restore(state)
	if err != nil {
		t.Fatal(err)
	}
	if plan.ActiveTab != "main" || plan.ActivePane != "left" || len(plan.Documents) != 2 {
		t.Fatalf("restore plan = %+v", plan)
	}
	if plan.Documents[0].PaneID != "left" || !plan.Documents[0].Focused || plan.Documents[1].PaneID != "right" || plan.Documents[1].Focused {
		t.Fatalf("pane order/focus plan = %+v", plan.Documents)
	}
	if got := session.Snapshot(); !reflect.DeepEqual(got, plan.State) {
		t.Fatalf("session snapshot = %+v, want restored state %+v", got, plan.State)
	}
}

func TestSessionRejectsOutOfVaultDocumentTransitions(t *testing.T) {
	root := t.TempDir()
	state := workspaceInRoot(root)
	session, err := NewSession(nil, state)
	if err != nil {
		t.Fatal(err)
	}
	before := session.Snapshot()
	path := filepath.Join(t.TempDir(), "outside.md")
	if err := session.UpdateDocument("main", "left", domain.Document{Path: path}); err != ErrDocumentOutsideVault {
		t.Fatalf("UpdateDocument error=%v, want outside-vault", err)
	}
	if got := session.Snapshot(); !reflect.DeepEqual(got, before) {
		t.Fatalf("rejected document transition changed state: %+v", got)
	}
}

func TestRecordActiveDocumentCreatesDefaultTabAndRecordsCursorAndFocus(t *testing.T) {
	root := t.TempDir()
	session, err := NewSession(nil, domain.State{Version: 1, VaultPath: root})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "note.md")
	plan, err := session.RecordActiveDocument(ActiveDocumentSnapshot{Path: path, CursorOffset: 13})
	if err != nil {
		t.Fatal(err)
	}
	if !plan.CreatedDefaultTab || plan.TabID != DefaultTabID || plan.PaneID != DefaultPaneID {
		t.Fatalf("default document plan=%+v", plan)
	}
	doc, ok := plan.State.ActiveDocument()
	if !ok || doc.Path != path || doc.CursorOffset != 13 || plan.State.ActiveTab != DefaultTabID || plan.State.Tabs[0].ActivePaneID != DefaultPaneID {
		t.Fatalf("recorded default document state=%+v", plan.State)
	}
	if got := session.FocusHistory(DefaultTabID); len(got) != 1 || got[0] != DefaultPaneID {
		t.Fatalf("default focus history=%v", got)
	}
}

func TestRecordActiveDocumentUpdatesExistingActiveTabAndRequestedPane(t *testing.T) {
	root := t.TempDir()
	state := workspaceInRoot(root)
	session, err := NewSession(nil, state)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "active.md")
	plan, err := session.RecordActiveDocument(ActiveDocumentSnapshot{Path: path, CursorOffset: 21, PaneID: "right"})
	if err != nil {
		t.Fatal(err)
	}
	if plan.CreatedDefaultTab || plan.TabID != "main" || plan.PaneID != "right" || plan.State.Tabs[0].ActivePaneID != "right" {
		t.Fatalf("existing document plan=%+v", plan)
	}
	var got domain.Document
	for _, pane := range plan.State.Tabs[0].Panes() {
		if pane.ID == "right" {
			got = pane.Document
		}
	}
	if got.Path != path || got.CursorOffset != 21 {
		t.Fatalf("updated active document=%+v", got)
	}
	if history := session.FocusHistory("main"); len(history) == 0 || history[0] != "right" {
		t.Fatalf("recorded active document did not update focus history: %v", history)
	}
}

func TestRecordActiveDocumentRejectsInvalidSnapshotWithoutMutation(t *testing.T) {
	root := t.TempDir()
	session, err := NewSession(nil, workspaceInRoot(root))
	if err != nil {
		t.Fatal(err)
	}
	before := session.Snapshot()
	for _, input := range []ActiveDocumentSnapshot{
		{Path: filepath.Join(t.TempDir(), "outside.md"), CursorOffset: 1},
		{Path: filepath.Join(root, "note.md"), CursorOffset: -1},
		{Path: filepath.Join(root, "note.md"), CursorOffset: 1, PaneID: "missing"},
	} {
		if _, err := session.RecordActiveDocument(input); err == nil {
			t.Fatalf("RecordActiveDocument(%+v) succeeded", input)
		}
		if after := session.Snapshot(); !reflect.DeepEqual(after, before) {
			t.Fatalf("rejected snapshot changed state: %+v", after)
		}
	}
}

func TestSessionPersistValidatesAndPreservesWorkspaceOrder(t *testing.T) {
	root := t.TempDir()
	state := workspaceInRoot(root)
	state.Tabs = append(state.Tabs, domain.Tab{
		ID: "second", ActivePaneID: "second-pane",
		Layout: domain.Leaf(domain.Pane{ID: "second-pane", Document: domain.Document{Path: filepath.Join(root, "second.md")}}),
	})
	repository := &memoryWorkspaceRepository{}
	session, err := NewSession(repository, state)
	if err != nil {
		t.Fatal(err)
	}
	if err := session.Persist(); err != nil {
		t.Fatal(err)
	}
	if len(repository.state.Tabs) != 2 || repository.state.Tabs[0].ID != "main" || repository.state.Tabs[1].ID != "second" {
		t.Fatalf("persisted tab ordering = %+v", repository.state.Tabs)
	}
}

func TestSetSplitRatioRejectsInvalidValuesWithoutChangingState(t *testing.T) {
	session, err := NewSession(nil, splitWorkspace())
	if err != nil {
		t.Fatal(err)
	}
	before := session.Snapshot()

	for _, ratio := range []float64{0, 1, math.NaN(), math.Inf(1), math.Inf(-1)} {
		if err := session.SetSplitRatio("main", "root", ratio); err == nil {
			t.Fatalf("SetSplitRatio(%v) unexpectedly succeeded", ratio)
		}
		if after := session.Snapshot(); !reflect.DeepEqual(after, before) {
			t.Fatalf("invalid ratio %v changed state: got %+v, want %+v", ratio, after, before)
		}
	}
}

func TestUpdateDocumentRejectsEmptyPathWithoutChangingState(t *testing.T) {
	session, err := NewSession(nil, splitWorkspace())
	if err != nil {
		t.Fatal(err)
	}
	before := session.Snapshot()
	if err := session.UpdateDocument("main", "left", domain.Document{Path: "  ", CursorOffset: 12}); err == nil {
		t.Fatal("UpdateDocument accepted an empty path")
	}
	if after := session.Snapshot(); !reflect.DeepEqual(after, before) {
		t.Fatalf("invalid document changed state: got %+v, want %+v", after, before)
	}
}

func TestTabsActivateMoveAndCloseRestoreRecentTab(t *testing.T) {
	session, err := NewSession(nil, splitWorkspace())
	if err != nil {
		t.Fatal(err)
	}
	second := domain.Tab{
		ID: "second", ActivePaneID: "second-pane",
		Layout: domain.Leaf(domain.Pane{ID: "second-pane", Document: domain.Document{Path: "/second.md"}}),
	}
	if err := session.CreateTab(second, true); err != nil {
		t.Fatal(err)
	}
	if got := session.Snapshot().ActiveTab; got != "second" {
		t.Fatalf("CreateTab active tab = %q, want second", got)
	}
	if _, err := session.ActivateTab("main"); err != nil {
		t.Fatal(err)
	}
	if err := session.MoveTab("second", 0); err != nil {
		t.Fatal(err)
	}
	state := session.Snapshot()
	if state.Tabs[0].ID != "second" || state.ActiveTab != "main" {
		t.Fatalf("MoveTab result = order [%q, %q], active %q", state.Tabs[0].ID, state.Tabs[1].ID, state.ActiveTab)
	}
	active, err := session.CloseTab("main")
	if err != nil {
		t.Fatal(err)
	}
	if active != "second" || session.Snapshot().ActiveTab != "second" {
		t.Fatalf("CloseTab selected %q, snapshot active %q; want second", active, session.Snapshot().ActiveTab)
	}
}

func TestClosePaneRestoresMostRecentlyFocusedSurvivor(t *testing.T) {
	session, err := NewSession(nil, splitWorkspace())
	if err != nil {
		t.Fatal(err)
	}
	if err := session.Focus("main", "right"); err != nil {
		t.Fatal(err)
	}
	if err := session.Focus("main", "left"); err != nil {
		t.Fatal(err)
	}
	focused, err := session.ClosePane("main", "left")
	if err != nil {
		t.Fatal(err)
	}
	if focused != "right" {
		t.Fatalf("ClosePane focus = %q, want right", focused)
	}
	state := session.Snapshot()
	if state.Tabs[0].ActivePaneID != "right" || state.Tabs[0].Layout.Pane == nil || state.Tabs[0].Layout.Pane.ID != "right" {
		t.Fatalf("ClosePane did not collapse tree and preserve focus: %+v", state.Tabs[0])
	}
}

func splitWorkspace() domain.State {
	return domain.State{
		Version: 1, ActiveTab: "main",
		Tabs: []domain.Tab{{
			ID: "main", ActivePaneID: "left",
			Layout: domain.Branch(domain.Horizontal, 0.5,
				domain.Leaf(domain.Pane{ID: "left", Document: domain.Document{Path: "/left.md"}}),
				domain.Leaf(domain.Pane{ID: "right", Document: domain.Document{Path: "/right.md"}}),
			),
		}},
	}
}

func workspaceInRoot(root string) domain.State {
	state := splitWorkspace()
	state.VaultPath = root
	state.Tabs[0].Layout = domain.Branch(domain.Horizontal, 0.5,
		domain.Leaf(domain.Pane{ID: "left", Document: domain.Document{Path: filepath.Join(root, "left.md")}}),
		domain.Leaf(domain.Pane{ID: "right", Document: domain.Document{Path: filepath.Join(root, "right.md")}}),
	)
	return state
}
