package nativeui

import (
	"testing"
	"time"
)

func TestKeySequenceMatcherSingleAndCustomSequence(t *testing.T) {
	now := time.Unix(100, 0)
	m, err := NewKeySequenceMatcher(time.Second, func() time.Time { return now },
		KeyBinding{Sequence: []KeyStroke{{Key: "K", Mods: 1}}, ActionID: ActionNavigate, Values: map[string]any{"to": "next"}},
		KeyBinding{Sequence: []KeyStroke{{Key: "G"}, {Key: "G"}, {Key: "X", Mods: 2}}, ActionID: ActionShowHelp},
	)
	if err != nil {
		t.Fatal(err)
	}
	got := m.Match(KeyStroke{Key: "K", Mods: 1})
	if got.ActionID != ActionNavigate || got.Values["to"] != "next" {
		t.Fatalf("single stroke result = %+v", got)
	}
	if first := m.Match(KeyStroke{Key: "G"}); !first.Pending || !first.Handled {
		t.Fatal("custom sequence prefix was not pending")
	}
	if second := m.Match(KeyStroke{Key: "G"}); !second.Pending || !second.Handled {
		t.Fatal("second custom sequence prefix was not pending")
	}
	got = m.Match(KeyStroke{Key: "X", Mods: 2})
	if got.ActionID != ActionShowHelp {
		t.Fatalf("custom sequence result = %+v", got)
	}
}

func TestKeySequenceMatcherMultiPrefixAndOverlap(t *testing.T) {
	now := time.Unix(200, 0)
	m, err := NewKeySequenceMatcher(500*time.Millisecond, func() time.Time { return now },
		KeyBinding{Sequence: []KeyStroke{{Key: "G"}}, ActionID: ActionOpen},
		KeyBinding{Sequence: []KeyStroke{{Key: "G"}, {Key: "G"}}, ActionID: ActionNavigate},
		KeyBinding{Sequence: []KeyStroke{{Key: "G"}, {Key: "H"}}, ActionID: ActionShowHelp},
	)
	if err != nil {
		t.Fatal(err)
	}
	if got := m.Match(KeyStroke{Key: "G"}); !got.Pending || !got.Handled || got.ActionID != "" {
		t.Fatalf("overlapping one-stroke prefix = %+v", got)
	}
	if got := m.Match(KeyStroke{Key: "H"}); got.ActionID != ActionShowHelp {
		t.Fatalf("branch under shared prefix = %+v", got)
	}
	if got := m.Match(KeyStroke{Key: "G"}); !got.Pending {
		t.Fatalf("second prefix = %+v", got)
	}
	if got := m.Expire(); !got.Pending || got.ActionID != "" {
		t.Fatalf("early expire = %+v", got)
	}
	now = now.Add(500 * time.Millisecond)
	if got := m.Expire(); got.ActionID != ActionOpen {
		t.Fatalf("expired overlapping prefix = %+v", got)
	}
}

func TestKeySequenceMatcherTimeoutAndEscapeCancel(t *testing.T) {
	now := time.Unix(300, 0)
	m, err := NewKeySequenceMatcher(250*time.Millisecond, func() time.Time { return now },
		KeyBinding{Sequence: []KeyStroke{{Key: "A"}, {Key: "B"}}, ActionID: ActionRename},
	)
	if err != nil {
		t.Fatal(err)
	}
	if !m.Match(KeyStroke{Key: "A"}).Pending {
		t.Fatal("expected pending prefix")
	}
	if got := m.Match(KeyStroke{Key: "Escape"}); !got.Handled || !got.Cancelled || got.ActionID != "" {
		t.Fatalf("Escape result = %+v", got)
	}
	if got := m.Match(KeyStroke{Key: "B"}); got.ActionID != "" || got.Pending {
		t.Fatalf("cancel did not reset state: %+v", got)
	}
	if !m.Match(KeyStroke{Key: "A"}).Pending {
		t.Fatal("expected second pending prefix")
	}
	now = now.Add(time.Second)
	if got := m.Expire(); got.ActionID != "" || got.Pending {
		t.Fatalf("incomplete prefix timeout = %+v", got)
	}
}

func TestKeySequenceMatcherUnknownStrokeResetsThenRematches(t *testing.T) {
	m, err := NewKeySequenceMatcher(time.Second, nil,
		KeyBinding{Sequence: []KeyStroke{{Key: "A"}, {Key: "C"}}, ActionID: ActionRename},
		KeyBinding{Sequence: []KeyStroke{{Key: "B"}}, ActionID: ActionCopy},
	)
	if err != nil {
		t.Fatal(err)
	}
	if !m.Match(KeyStroke{Key: "A"}).Pending {
		t.Fatal("expected prefix pending")
	}
	if got := m.Match(KeyStroke{Key: "B"}); got.ActionID != ActionCopy || !got.Handled {
		t.Fatalf("mismatch stroke was not rematched from root: %+v", got)
	}
	if got := m.Match(KeyStroke{Key: "Z"}); got.Handled || got.Pending || got.ActionID != "" {
		t.Fatalf("unknown root stroke should remain available to another consumer: %+v", got)
	}
}

func TestKeySequenceMatcherTimedOutMatchCanReprocessCurrentStroke(t *testing.T) {
	now := time.Unix(400, 0)
	m, err := NewKeySequenceMatcher(100*time.Millisecond, func() time.Time { return now },
		KeyBinding{Sequence: []KeyStroke{{Key: "G"}}, ActionID: ActionOpen},
		KeyBinding{Sequence: []KeyStroke{{Key: "G"}, {Key: "G"}}, ActionID: ActionNavigate},
	)
	if err != nil {
		t.Fatal(err)
	}
	if got := m.Match(KeyStroke{Key: "G"}); !got.Pending || !got.Handled {
		t.Fatalf("prefix result = %+v", got)
	}
	now = now.Add(100 * time.Millisecond)
	result := m.Match(KeyStroke{Key: "G"})
	if result.ActionID != ActionOpen || !result.Handled || !result.Reprocess {
		t.Fatalf("timed out match result = %+v", result)
	}
	// The consumer dispatches ActionOpen, then retries the same stroke as
	// requested; it must now begin a fresh prefix rather than disappear.
	retry := m.Match(KeyStroke{Key: "G"})
	if !retry.Pending || !retry.Handled || retry.ActionID != "" {
		t.Fatalf("reprocessed stroke was lost: %+v", retry)
	}
}

func TestKeySequenceMatcherCandidatesAtRootAndPendingPrefix(t *testing.T) {
	m, err := NewKeySequenceMatcher(time.Second, nil,
		KeyBinding{Sequence: []KeyStroke{{Key: "G"}}, ActionID: ActionOpen},
		KeyBinding{Sequence: []KeyStroke{{Key: "G"}, {Key: "G"}}, ActionID: ActionNavigate},
		KeyBinding{Sequence: []KeyStroke{{Key: "G"}, {Key: "A"}}, ActionID: ActionShowHelp},
		KeyBinding{Sequence: []KeyStroke{{Key: "X", Mods: 1}}, ActionID: ActionCopy},
	)
	if err != nil {
		t.Fatal(err)
	}
	wantRoot := []KeyStroke{{Key: "G"}, {Key: "X", Mods: 1}}
	if got := m.Candidates(); !keyStrokesEqual(got, wantRoot) || m.Pending() {
		t.Fatalf("root candidates = %v pending=%v, want %v", got, m.Pending(), wantRoot)
	}

	if result := m.Match(KeyStroke{Key: "G"}); !result.Pending {
		t.Fatalf("prefix result = %+v", result)
	}
	wantPending := []KeyStroke{{Key: "A"}, {Key: "G"}, {Key: "X", Mods: 1}}
	if got := m.Candidates(); !keyStrokesEqual(got, wantPending) || !m.Pending() {
		t.Fatalf("pending candidates = %v pending=%v, want %v", got, m.Pending(), wantPending)
	}
}

func keyStrokesEqual(a, b []KeyStroke) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
