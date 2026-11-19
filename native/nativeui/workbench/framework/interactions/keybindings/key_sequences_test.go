package keybindings

import (
	"testing"
	"time"

	commands "muse/nativeui/workbench/framework/interactions/commands"
	"muse/nativeui/workbench/framework/interactions/context"
)

func TestKeySequenceMatcherSingleAndCustomSequence(t *testing.T) {
	now := time.Unix(100, 0)
	m, err := NewKeySequenceMatcher(time.Second, func() time.Time { return now },
		KeyBinding{Sequence: []KeyStroke{{Key: "K", Mods: 1}}, ActionID: commands.ActionNavigate, Values: map[string]any{"to": "next"}},
		KeyBinding{Sequence: []KeyStroke{{Key: "G"}, {Key: "G"}, {Key: "X", Mods: 2}}, ActionID: commands.ActionShowHelp},
	)
	if err != nil {
		t.Fatal(err)
	}
	got := m.Match(KeyStroke{Key: "K", Mods: 1})
	if got.ActionID != commands.ActionNavigate || got.Values["to"] != "next" {
		t.Fatalf("single stroke result = %+v", got)
	}
	if first := m.Match(KeyStroke{Key: "G"}); !first.Pending || !first.Handled {
		t.Fatal("custom sequence prefix was not pending")
	}
	if second := m.Match(KeyStroke{Key: "G"}); !second.Pending || !second.Handled {
		t.Fatal("second custom sequence prefix was not pending")
	}
	got = m.Match(KeyStroke{Key: "X", Mods: 2})
	if got.ActionID != commands.ActionShowHelp {
		t.Fatalf("custom sequence result = %+v", got)
	}
}

func TestKeySequenceMatcherMultiPrefixAndOverlap(t *testing.T) {
	now := time.Unix(200, 0)
	m, err := NewKeySequenceMatcher(500*time.Millisecond, func() time.Time { return now },
		KeyBinding{Sequence: []KeyStroke{{Key: "G"}}, ActionID: commands.ActionOpen},
		KeyBinding{Sequence: []KeyStroke{{Key: "G"}, {Key: "G"}}, ActionID: commands.ActionNavigate},
		KeyBinding{Sequence: []KeyStroke{{Key: "G"}, {Key: "H"}}, ActionID: commands.ActionShowHelp},
	)
	if err != nil {
		t.Fatal(err)
	}
	if got := m.Match(KeyStroke{Key: "G"}); !got.Pending || !got.Handled || got.ActionID != "" {
		t.Fatalf("overlapping one-stroke prefix = %+v", got)
	}
	if got := m.Match(KeyStroke{Key: "H"}); got.ActionID != commands.ActionShowHelp {
		t.Fatalf("branch under shared prefix = %+v", got)
	}
	if got := m.Match(KeyStroke{Key: "G"}); !got.Pending {
		t.Fatalf("second prefix = %+v", got)
	}
	if got := m.Expire(); !got.Pending || got.ActionID != "" {
		t.Fatalf("early expire = %+v", got)
	}
	now = now.Add(500 * time.Millisecond)
	if got := m.Expire(); got.ActionID != commands.ActionOpen {
		t.Fatalf("expired overlapping prefix = %+v", got)
	}
}

func TestKeySequenceMatcherTimeoutAndEscapeCancel(t *testing.T) {
	now := time.Unix(300, 0)
	m, err := NewKeySequenceMatcher(250*time.Millisecond, func() time.Time { return now },
		KeyBinding{Sequence: []KeyStroke{{Key: "A"}, {Key: "B"}}, ActionID: commands.ActionRename},
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
		KeyBinding{Sequence: []KeyStroke{{Key: "A"}, {Key: "C"}}, ActionID: commands.ActionRename},
		KeyBinding{Sequence: []KeyStroke{{Key: "B"}}, ActionID: commands.ActionCopy},
	)
	if err != nil {
		t.Fatal(err)
	}
	if !m.Match(KeyStroke{Key: "A"}).Pending {
		t.Fatal("expected prefix pending")
	}
	if got := m.Match(KeyStroke{Key: "B"}); got.ActionID != commands.ActionCopy || !got.Handled {
		t.Fatalf("mismatch stroke was not rematched from root: %+v", got)
	}
	if got := m.Match(KeyStroke{Key: "Z"}); got.Handled || got.Pending || got.ActionID != "" {
		t.Fatalf("unknown root stroke should remain available to another consumer: %+v", got)
	}
}

func TestKeySequenceMatcherTimedOutMatchCanReprocessCurrentStroke(t *testing.T) {
	now := time.Unix(400, 0)
	m, err := NewKeySequenceMatcher(100*time.Millisecond, func() time.Time { return now },
		KeyBinding{Sequence: []KeyStroke{{Key: "G"}}, ActionID: commands.ActionOpen},
		KeyBinding{Sequence: []KeyStroke{{Key: "G"}, {Key: "G"}}, ActionID: commands.ActionNavigate},
	)
	if err != nil {
		t.Fatal(err)
	}
	if got := m.Match(KeyStroke{Key: "G"}); !got.Pending || !got.Handled {
		t.Fatalf("prefix result = %+v", got)
	}
	now = now.Add(100 * time.Millisecond)
	result := m.Match(KeyStroke{Key: "G"})
	if result.ActionID != commands.ActionOpen || !result.Handled || !result.Reprocess {
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
		KeyBinding{Sequence: []KeyStroke{{Key: "G"}}, ActionID: commands.ActionOpen},
		KeyBinding{Sequence: []KeyStroke{{Key: "G"}, {Key: "G"}}, ActionID: commands.ActionNavigate},
		KeyBinding{Sequence: []KeyStroke{{Key: "G"}, {Key: "A"}}, ActionID: commands.ActionShowHelp},
		KeyBinding{Sequence: []KeyStroke{{Key: "X", Mods: 1}}, ActionID: commands.ActionCopy},
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

func TestKeySequenceMatcherFiltersBindingsByContext(t *testing.T) {
	vimMode := context.NewKey[bool]("editor.vim-mode")
	vimSnapshot := context.Compose(context.With(context.Scope{}, vimMode, true), context.Scope{}, context.Scope{})
	otherSnapshot := context.Compose(context.With(context.Scope{}, vimMode, false), context.Scope{}, context.Scope{})
	m, err := NewKeySequenceMatcher(time.Second, nil,
		KeyBinding{Sequence: []KeyStroke{{Key: "G"}, {Key: "G"}}, ActionID: commands.ActionNavigate, When: context.Equals(vimMode, true)},
		KeyBinding{Sequence: []KeyStroke{{Key: "G"}, {Key: "H"}}, ActionID: commands.ActionShowHelp},
	)
	if err != nil {
		t.Fatal(err)
	}
	if got := m.MatchWithContext(KeyStroke{Key: "G"}, otherSnapshot); !got.Pending || !got.Handled {
		t.Fatalf("unconditional branch should remain available: %+v", got)
	}
	if got := m.MatchWithContext(KeyStroke{Key: "H"}, otherSnapshot); got.ActionID != commands.ActionShowHelp {
		t.Fatalf("context-disabled branch blocked the unconditional sibling: %+v", got)
	}
	if got := m.MatchWithContext(KeyStroke{Key: "G"}, vimSnapshot); !got.Pending {
		t.Fatalf("enabled prefix was not pending: %+v", got)
	}
	if got := m.MatchWithContext(KeyStroke{Key: "G"}, vimSnapshot); got.ActionID != commands.ActionNavigate {
		t.Fatalf("enabled binding did not execute: %+v", got)
	}
}

func TestKeySequenceMatcherReevaluatesContextDuringSequenceAndExpiration(t *testing.T) {
	vimMode := context.NewKey[bool]("editor.vim-mode")
	vimSnapshot := context.Compose(context.With(context.Scope{}, vimMode, true), context.Scope{}, context.Scope{})
	otherSnapshot := context.Compose(context.With(context.Scope{}, vimMode, false), context.Scope{}, context.Scope{})
	now := time.Unix(500, 0)
	m, err := NewKeySequenceMatcher(time.Second, func() time.Time { return now },
		KeyBinding{Sequence: []KeyStroke{{Key: "A"}, {Key: "B"}}, ActionID: commands.ActionRename, When: context.Equals(vimMode, true)},
	)
	if err != nil {
		t.Fatal(err)
	}
	if got := m.MatchWithContext(KeyStroke{Key: "A"}, vimSnapshot); !got.Pending {
		t.Fatalf("enabled prefix was not pending: %+v", got)
	}
	if got := m.MatchWithContext(KeyStroke{Key: "B"}, otherSnapshot); got.Handled || got.ActionID != "" {
		t.Fatalf("binding remained active after context changed: %+v", got)
	}

	m, err = NewKeySequenceMatcher(time.Second, func() time.Time { return now },
		KeyBinding{Sequence: []KeyStroke{{Key: "A"}}, ActionID: commands.ActionOpen, When: context.Equals(vimMode, true)},
		KeyBinding{Sequence: []KeyStroke{{Key: "A"}, {Key: "B"}}, ActionID: commands.ActionCopy},
	)
	if err != nil {
		t.Fatal(err)
	}
	if got := m.MatchWithContext(KeyStroke{Key: "A"}, vimSnapshot); !got.Pending {
		t.Fatalf("overlapping prefix was not pending: %+v", got)
	}
	now = now.Add(time.Second)
	if got := m.ExpireWithContext(otherSnapshot); got.ActionID != "" || got.Pending {
		t.Fatalf("disabled pending binding executed at expiration: %+v", got)
	}
}

func TestWorkbenchFocusBindingsAreOwnedByKeybindings(t *testing.T) {
	standard := StandardPanelBindings()
	if len(standard) != 1 {
		t.Fatalf("standard panel bindings = %+v, want one", standard)
	}
	if got := standard[0]; got.ActionID != commands.ActionFocusPanel || got.Values["panel"] != commands.PanelIDFiles ||
		len(got.Sequence) != 1 || got.Sequence[0] != (KeyStroke{Key: "E", Mods: ModPrimary | ModShift}) {
		t.Fatalf("standard focus binding = %+v", got)
	}

	vim := VimPanelBindings()
	if len(vim) != 6 {
		t.Fatalf("Vim panel bindings = %+v, want six", vim)
	}
	for i, direction := range []int{-1, -1, 1, 1} {
		binding := vim[i]
		if binding.ActionID != commands.ActionFocusDirection || binding.Values["direction"] != direction ||
			len(binding.Sequence) != 2 || binding.Sequence[0] != (KeyStroke{Key: "W", Mods: ModCtrl}) {
			t.Fatalf("Vim focus direction binding[%d] = %+v", i, binding)
		}
	}
	if vim[4].ActionID != commands.ActionFocusNextPanel || vim[5].ActionID != commands.ActionFocusPreviousPanel {
		t.Fatalf("Vim focus cycle bindings = %+v, %+v", vim[4], vim[5])
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
