package editor

import (
	"testing"
	"time"

	"muse/nativeui/editor/feedback"
)

func TestSearchMatchPulseIsBriefAndOptIn(t *testing.T) {
	start := time.Unix(10, 0)
	e := New("alpha beta", Options{SearchMatchPulse: true})
	e.feedbackState.StartSearchPulse(start)
	if got, active := e.searchMatchPulse(start.Add(feedback.SearchMatchPulseDuration / 2)); !active || got < 0.99 {
		t.Fatalf("peak pulse=%v active=%v", got, active)
	}
	if got, active := e.searchMatchPulse(start.Add(feedback.SearchMatchPulseDuration)); active || got != 0 {
		t.Fatalf("finished pulse=%v active=%v", got, active)
	}

	e.SetOptions(Options{})
	e.feedbackState.StartSearchPulse(start)
	if got, active := e.searchMatchPulse(start.Add(time.Millisecond)); active || got != 0 {
		t.Fatalf("disabled pulse=%v active=%v", got, active)
	}
}

func TestSearchMatchPulseStartsWhenCurrentHitChanges(t *testing.T) {
	e := New("alpha alpha", Options{SearchMatchPulse: true})
	e.SetHighlights([]Range{{Start: 0, End: 5}, {Start: 6, End: 11}}, 0)
	if !e.feedbackState.SearchState().Active {
		t.Fatal("choosing a current search match did not start the pulse")
	}
	start := e.feedbackState.SearchState().Started
	e.SetHighlights([]Range{{Start: 0, End: 5}, {Start: 6, End: 11}}, 0)
	if !e.feedbackState.SearchState().Started.Equal(start) {
		t.Fatal("repainting the same current hit restarted the pulse")
	}
	e.SetHighlights([]Range{{Start: 0, End: 5}, {Start: 6, End: 11}}, 1)
	if !e.feedbackState.SearchState().Started.After(start) {
		t.Fatal("selecting a different current hit did not restart the pulse")
	}
}

func TestSearchMatchPulseScreenshot(t *testing.T) {
	h := newHarness(t, "alpha alpha")
	opts := h.e.Options()
	opts.SearchMatchPulse = true
	h.e.SetOptions(opts)
	h.e.SetHighlights([]Range{{Start: 0, End: 5}, {Start: 6, End: 11}}, 1)
	h.tt.Advance(feedback.SearchMatchPulseDuration / 2)
	h.shot("search-match-pulse")
}

func TestVimYankFlashIsOptInAndVimOnly(t *testing.T) {
	e := New("alpha beta", Options{VimYankFlash: true})
	e.SetKeymap("vim")
	e.Select(0, 0)
	modalKeys(e, "yy")
	yank := e.feedbackState.YankState()
	if !yank.Active || len(yank.Ranges) != 1 {
		t.Fatalf("Vim yy did not flash its yanked range: active=%v ranges=%v", yank.Active, yank.Ranges)
	}
	if got := e.Source()[yank.Ranges[0].Start:yank.Ranges[0].End]; got != "alpha beta" {
		t.Fatalf("flashed range=%q", got)
	}

	e.SetKeymap("helix")
	e.Select(0, 0)
	modalKeys(e, "vly")
	if e.feedbackState.YankState().Active {
		t.Fatal("Vim yank flash leaked into Helix mode")
	}

	e.SetKeymap("vim")
	e.SetOptions(Options{})
	e.Select(0, 0)
	modalKeys(e, "yy")
	if e.feedbackState.YankState().Active {
		t.Fatal("disabled Vim yank flash was active")
	}
}

func TestVimYankFlashTracksVisualYankRange(t *testing.T) {
	e := New("alpha beta", Options{VimYankFlash: true})
	e.SetKeymap("vim")
	e.Select(0, 0)
	modalKeys(e, "vly")
	yank := e.feedbackState.YankState()
	if !yank.Active || len(yank.Ranges) != 1 {
		t.Fatalf("Visual yank did not flash its range: active=%v ranges=%v", yank.Active, yank.Ranges)
	}
	r := yank.Ranges[0]
	if got, register := e.Source()[r.Start:r.End], e.vimController().View().Register; got != register {
		t.Fatalf("flashed Visual range=%q, yanked register=%q", got, register)
	}
}

func TestVimYankFlashScreenshot(t *testing.T) {
	h := newHarness(t, "alpha beta")
	opts := h.e.Options()
	opts.VimYankFlash = true
	h.e.SetOptions(opts)
	h.e.SetKeymap("vim")
	h.caretAt(0)
	modalKeys(h.e, "yy")
	h.tt.Advance(feedback.VimYankFlashDuration / 5)
	h.shot("vim-yank-flash")
}
