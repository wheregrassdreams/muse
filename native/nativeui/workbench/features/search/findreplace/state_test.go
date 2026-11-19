package findreplace

import "testing"

func TestSyncRebuildsDocumentMatchesAndClearsOutOfRangeSelection(t *testing.T) {
	state := NewHandle()
	state.Apply(Action{Kind: QueryChanged, Query: "muse"})
	if !state.Sync("Muse and muse") {
		t.Fatal("first Sync should build matches")
	}
	if len(state.Snapshot().Matches) != 2 {
		t.Fatalf("matches = %d, want 2", len(state.Snapshot().Matches))
	}
	state.Apply(Action{Kind: OpenMatch, Index: 1})
	if state.Snapshot().Selected != 1 {
		t.Fatalf("selected = %d, want 1", state.Snapshot().Selected)
	}
	if state.Sync("only Muse") != true {
		t.Fatal("Sync should notice source change")
	}
	snapshot := state.Snapshot()
	if len(snapshot.Matches) != 1 || snapshot.Selected != -1 {
		t.Fatalf("after source change: matches=%d selected=%d", len(snapshot.Matches), snapshot.Selected)
	}
	if state.Sync("only Muse") {
		t.Fatal("unchanged query and source should reuse the projection")
	}
}

func TestInvalidateRebuildsHighlightsForSameSource(t *testing.T) {
	state := NewHandle()
	state.Apply(Action{Kind: QueryChanged, Query: "note"})
	state.Sync("a note")
	state.Invalidate()
	if !state.Sync("a note") {
		t.Fatal("invalidated Sync should rebuild highlights")
	}
}

func TestSnapshotDoesNotExposeCachedMatches(t *testing.T) {
	handle := NewHandle()
	handle.Apply(Action{Kind: QueryChanged, Query: "note"})
	handle.Sync("a note")

	snapshot := handle.Snapshot()
	if len(snapshot.Matches) != 1 {
		t.Fatalf("matches = %d, want 1", len(snapshot.Matches))
	}
	snapshot.Matches[0].Start = 99
	if got := handle.Snapshot().Matches[0].Start; got == 99 {
		t.Fatal("snapshot exposed the panel-owned match slice")
	}
}
