package core

import (
	"testing"
	"time"
)

type testHistorySnapshot struct{ value int }

func (s testHistorySnapshot) Clone() HistorySnapshot { return s }

type testHistoryParticipant struct{ value int }

func (p *testHistoryParticipant) CaptureHistory() HistorySnapshot {
	return testHistorySnapshot{value: p.value}
}

func (p *testHistoryParticipant) RestoreHistory(snapshot HistorySnapshot) {
	p.value = snapshot.(testHistorySnapshot).value
}

func TestSessionChangeHookObservesCommittedStateOutsideLock(t *testing.T) {
	s := NewSession("abc")
	var changes []Change
	s.SetChangeHook(func(change Change) {
		// Reading through the public API from the hook must not deadlock and
		// must see the already-committed source revision. Ordinary changes carry
		// deltas, avoiding a full-document copy in the hot path.
		if got := s.Revision(); got < change.Revision {
			t.Errorf("hook observed revision %d, want committed revision at least %d", got, change.Revision)
		}
		if change.Kind == ChangeReplaceSource && s.Source() != change.AfterSource {
			t.Errorf("replacement hook observed source %q, want %q", s.Source(), change.AfterSource)
		}
		changes = append(changes, change)
	})

	if err := s.SetSelection(Selection{Anchor: 1, Head: 1}); err != nil {
		t.Fatal(err)
	}
	before := s.Selection()
	if _, err := s.ApplyTransaction(Transaction{
		Before: before,
		After:  Selection{Anchor: 2, Head: 2},
		Edits:  []TextEdit{{Range: TextRange{Start: 1, End: 1}, Text: "X"}},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Undo(); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Redo(); err != nil {
		t.Fatal(err)
	}
	s.ReplaceSource("next")

	wantKinds := []ChangeKind{ChangeSelection, ChangeEdit, ChangeUndo, ChangeRedo, ChangeReplaceSource}
	if len(changes) != len(wantKinds) {
		t.Fatalf("got %d changes, want %d: %+v", len(changes), len(wantKinds), changes)
	}
	for i, kind := range wantKinds {
		if changes[i].Kind != kind {
			t.Errorf("change %d kind = %v, want %v", i, changes[i].Kind, kind)
		}
	}
	if changes[1].BeforeSource != "" || changes[1].AfterSource != "" || changes[1].BeforeRevision != 0 || changes[1].Revision != 1 {
		t.Fatalf("edit change does not describe the committed transaction: %+v", changes[1])
	}
	if len(changes[1].Edits) != 1 || changes[1].Edits[0] != (TextEdit{Range: TextRange{Start: 1, End: 1}, Text: "X"}) {
		t.Fatalf("edit change has wrong source-coordinate delta: %+v", changes[1].Edits)
	}
	if len(changes[2].Edits) != 1 || changes[2].Edits[0] != (TextEdit{Range: TextRange{Start: 1, End: 2}}) {
		t.Fatalf("undo change has wrong inverse delta: %+v", changes[2].Edits)
	}
	if len(changes[3].Edits) != 1 || changes[3].Edits[0] != (TextEdit{Range: TextRange{Start: 1, End: 1}, Text: "X"}) {
		t.Fatalf("redo change has wrong reapplied delta: %+v", changes[3].Edits)
	}
	if s.Source() != "next" || s.CanUndo() || s.CanRedo() {
		t.Fatalf("ReplaceSource did not reset history/source: source=%q undo=%t redo=%t", s.Source(), s.CanUndo(), s.CanRedo())
	}
}

func TestSessionHistoryParticipantRestoresTypedSnapshots(t *testing.T) {
	s := NewSession("")
	participant := &testHistoryParticipant{}
	s.BindHistoryParticipant(participant)
	before := s.CaptureHistory()
	participant.value = 1
	after := s.CaptureHistory()
	if _, err := s.ApplyTransactionWithOptions(Transaction{
		Before: Selection{}, After: Selection{Anchor: 1, Head: 1},
		Edits: []TextEdit{{Range: TextRange{}, Text: "x"}},
	}, TransactionOptions{BeforeMetadata: before, AfterMetadata: after}); err != nil {
		t.Fatal(err)
	}
	participant.value = 7
	if _, err := s.Undo(); err != nil {
		t.Fatal(err)
	}
	if participant.value != 0 {
		t.Fatalf("undo restored metadata %d, want before snapshot 0", participant.value)
	}
	if _, err := s.Redo(); err != nil {
		t.Fatal(err)
	}
	if participant.value != 1 {
		t.Fatalf("redo restored metadata %d, want after snapshot 1", participant.value)
	}
}

func TestSessionTypingCoalescingTreatsWhitespaceLikeOtherText(t *testing.T) {
	s := NewSession("")
	start := time.Now()
	insert := func(text string, at int, now time.Time) {
		t.Helper()
		before := s.Selection()
		_, err := s.ApplyTransactionWithOptions(Transaction{
			Before: before, After: Selection{Anchor: at + len(text), Head: at + len(text)},
			Edits: []TextEdit{{Range: TextRange{Start: at, End: at}, Text: text}},
		}, TransactionOptions{Group: HistoryGroupTyping, At: now})
		if err != nil {
			t.Fatal(err)
		}
	}
	insert("a", 0, start)
	insert(" ", 1, start.Add(100*time.Millisecond))
	insert("b", 2, start.Add(200*time.Millisecond))
	if got := s.Source(); got != "a b" {
		t.Fatalf("source=%q, want %q", got, "a b")
	}
	if _, err := s.Undo(); err != nil {
		t.Fatal(err)
	}
	if got := s.Source(); got != "" {
		t.Fatalf("one coalesced typing undo left source %q, want empty", got)
	}
}

func TestSessionDeleteCoalescing(t *testing.T) {
	s := NewSession("abc")
	if err := s.SetSelection(Selection{Anchor: 3, Head: 3}); err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	for _, span := range []TextRange{{Start: 2, End: 3}, {Start: 1, End: 2}} {
		before := s.Selection()
		if _, err := s.ApplyTransactionWithOptions(Transaction{
			Before: before, After: Selection{Anchor: span.Start, Head: span.Start},
			Edits: []TextEdit{{Range: span}},
		}, TransactionOptions{Group: HistoryGroupDelete, At: start}); err != nil {
			t.Fatal(err)
		}
		start = start.Add(100 * time.Millisecond)
	}
	if got := s.Source(); got != "a" {
		t.Fatalf("source=%q, want a", got)
	}
	if _, err := s.Undo(); err != nil {
		t.Fatal(err)
	}
	if got := s.Source(); got != "abc" || s.CanUndo() {
		t.Fatalf("one coalesced undo restored source=%q, canUndo=%t", got, s.CanUndo())
	}
}

func TestSessionEditBatchPublishesOnceAndUndoReplaysAllEdits(t *testing.T) {
	s := NewSession("abc")
	var changes []Change
	s.SetChangeHook(func(change Change) { changes = append(changes, change) })
	batch, err := s.BeginTransaction(TransactionOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.ApplyEdit(batch, TextEdit{Range: TextRange{Start: 0, End: 0}, Text: "x"}); err != nil {
		t.Fatal(err)
	}
	if got := s.Source(); got != "xabc" {
		t.Fatalf("first intermediate source=%q, want xabc", got)
	}
	if _, err := s.ApplyEdit(batch, TextEdit{Range: TextRange{Start: 4, End: 4}, Text: "!"}); err != nil {
		t.Fatal(err)
	}
	if got := s.Source(); got != "xabc!" {
		t.Fatalf("second intermediate source=%q, want xabc!", got)
	}
	if len(changes) != 0 {
		t.Fatalf("uncommitted batch emitted %d change notifications", len(changes))
	}
	if _, err := s.CommitTransaction(batch, Selection{Anchor: 5, Head: 5}); err != nil {
		t.Fatal(err)
	}
	if len(changes) != 1 || len(changes[0].Edits) != 2 {
		t.Fatalf("commit notifications=%+v, want one change with two sequential edits", changes)
	}
	if _, err := s.Undo(); err != nil {
		t.Fatal(err)
	}
	if got := s.Source(); got != "abc" {
		t.Fatalf("undo batch source=%q, want abc", got)
	}
	if _, err := s.Redo(); err != nil {
		t.Fatal(err)
	}
	if got := s.Source(); got != "xabc!" {
		t.Fatalf("redo batch source=%q, want xabc!", got)
	}
}

func TestSessionAbortBatchRestoresSourceAndSelection(t *testing.T) {
	s := NewSession("hello")
	if err := s.SetSelection(Selection{Anchor: 2, Head: 2}); err != nil {
		t.Fatal(err)
	}
	batch, err := s.BeginTransaction(TransactionOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.ApplyEdit(batch, TextEdit{Range: TextRange{Start: 1, End: 4}, Text: "i"}); err != nil {
		t.Fatal(err)
	}
	if err := s.AbortTransaction(batch); err != nil {
		t.Fatal(err)
	}
	if s.Source() != "hello" || s.Selection() != (Selection{Anchor: 2, Head: 2}) || s.CanUndo() {
		t.Fatalf("abort left source=%q selection=%+v undo=%t", s.Source(), s.Selection(), s.CanUndo())
	}
}

func TestSessionMergePreviousKeepsCommandOneUndoStep(t *testing.T) {
	s := NewSession("")
	for i, text := range []string{"x", "y"} {
		before := s.Selection()
		at := before.Head
		if _, err := s.ApplyTransactionWithOptions(Transaction{
			Before: before, After: Selection{Anchor: at + len(text), Head: at + len(text)},
			Edits: []TextEdit{{Range: TextRange{Start: at, End: at}, Text: text}},
		}, TransactionOptions{MergePrevious: i == 1}); err != nil {
			t.Fatal(err)
		}
	}
	if got := s.Source(); got != "xy" {
		t.Fatalf("source=%q, want xy", got)
	}
	if _, err := s.Undo(); err != nil {
		t.Fatal(err)
	}
	if got := s.Source(); got != "" || s.CanUndo() {
		t.Fatalf("merged command undo source=%q canUndo=%t", got, s.CanUndo())
	}
}

func TestSessionUpdateLastHistoryAfterCapturesLateBlockState(t *testing.T) {
	s := NewSession("")
	participant := &testHistoryParticipant{}
	s.BindHistoryParticipant(participant)
	before := s.CaptureHistory()
	participant.value = 1
	batch, err := s.BeginTransaction(TransactionOptions{BeforeMetadata: before})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.ApplyEdit(batch, TextEdit{Range: TextRange{}, Text: "x"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CommitTransaction(batch, Selection{Anchor: 1, Head: 1}); err != nil {
		t.Fatal(err)
	}
	participant.value = 2
	if !s.UpdateLastHistoryAfter() {
		t.Fatal("UpdateLastHistoryAfter reported no history step")
	}
	if _, err := s.Undo(); err != nil {
		t.Fatal(err)
	}
	if participant.value != 0 {
		t.Fatalf("undo restored metadata=%d, want 0", participant.value)
	}
	if _, err := s.Redo(); err != nil {
		t.Fatal(err)
	}
	if participant.value != 2 {
		t.Fatalf("redo restored late metadata=%d, want 2", participant.value)
	}
}
