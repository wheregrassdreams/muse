package feedback

import (
	"testing"
	"time"
)

func TestHandleBeginsCursorFromEligibleHostSnapshot(t *testing.T) {
	start := time.Unix(1, 0)
	var handle Handle
	if !handle.BeginCursorMove(CursorMove{
		From: 4, To: 8, SelectionAnchor: 4, Enabled: true, Navigation: true,
		ModalNormal: true, Now: start,
	}) {
		t.Fatal("eligible modal navigation did not start a cursor transition")
	}
	state := handle.CursorState()
	if !state.Active || state.From != 4 || state.To != 8 || !state.Started.Equal(start) {
		t.Fatalf("cursor transition = %+v", state)
	}
	if handle.BeginCursorMove(CursorMove{
		From: 8, To: 9, SelectionAnchor: 8, Enabled: true, Navigation: true,
		Extending: true, ModalNormal: true, Now: start.Add(time.Millisecond),
	}) {
		t.Fatal("extending selection started an animation")
	}
	if state := handle.CursorState(); state.Active {
		t.Fatalf("rejected movement retained old transition: %+v", state)
	}
}

func TestHandleStartsSearchPulseOnlyWhenCurrentHitChanges(t *testing.T) {
	start := time.Unix(2, 0)
	var handle Handle
	current := TextRange{Start: 10, End: 15}
	if !handle.UpdateSearchPulse(SearchPulseChange{Next: current, NextValid: true, Enabled: true, Now: start}) {
		t.Fatal("first current hit did not start a pulse")
	}
	if handle.UpdateSearchPulse(SearchPulseChange{
		Previous: current, Next: current, PreviousValid: true, NextValid: true,
		Enabled: true, Now: start.Add(time.Millisecond),
	}) {
		t.Fatal("unchanged current hit restarted the pulse")
	}
	if got := handle.SearchState().Started; !got.Equal(start) {
		t.Fatalf("pulse restarted at %v, want %v", got, start)
	}
}

func TestHandleClampsAndGatesVimYankFlash(t *testing.T) {
	start := time.Unix(3, 0)
	var handle Handle
	if handle.StartVimYankFlash(YankFlashInput{
		Ranges: []TextRange{{Start: -4, End: 3}, {Start: 4, End: 20}}, SourceLength: 8,
		Enabled: true, VimKeymap: true, Now: start,
	}) != true {
		t.Fatal("enabled Vim yank did not start a flash")
	}
	got := handle.YankState()
	want := []TextRange{{Start: 0, End: 3}, {Start: 4, End: 8}}
	if !got.Active || !got.Started.Equal(start) || len(got.Ranges) != len(want) || got.Ranges[0] != want[0] || got.Ranges[1] != want[1] {
		t.Fatalf("yank flash = %+v, want ranges %v", got, want)
	}
	if handle.StartVimYankFlash(YankFlashInput{Ranges: want, SourceLength: 8, Enabled: true, VimKeymap: false}) {
		t.Fatal("non-Vim keymap started a yank flash")
	}
}
