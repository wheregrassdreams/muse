package editor

import (
	"testing"
	"time"
)

func TestSearchMatchPulseIsBriefAndOptIn(t *testing.T) {
	start := time.Unix(10, 0)
	e := New("alpha beta", Options{SearchMatchPulse: true})
	e.searchPulseStart = start
	e.searchPulseActive = true
	if got, active := e.searchMatchPulse(start.Add(searchMatchPulseDuration / 2)); !active || got < 0.99 {
		t.Fatalf("peak pulse=%v active=%v", got, active)
	}
	if got, active := e.searchMatchPulse(start.Add(searchMatchPulseDuration)); active || got != 0 {
		t.Fatalf("finished pulse=%v active=%v", got, active)
	}

	e.SetOptions(Options{})
	e.searchPulseActive = true
	if got, active := e.searchMatchPulse(start.Add(time.Millisecond)); active || got != 0 {
		t.Fatalf("disabled pulse=%v active=%v", got, active)
	}
}

func TestSearchMatchPulseStartsWhenCurrentHitChanges(t *testing.T) {
	e := New("alpha alpha", Options{SearchMatchPulse: true})
	e.SetHighlights([]Range{{Start: 0, End: 5}, {Start: 6, End: 11}}, 0)
	if !e.searchPulseActive {
		t.Fatal("choosing a current search match did not start the pulse")
	}
	start := e.searchPulseStart
	e.SetHighlights([]Range{{Start: 0, End: 5}, {Start: 6, End: 11}}, 0)
	if !e.searchPulseStart.Equal(start) {
		t.Fatal("repainting the same current hit restarted the pulse")
	}
	e.SetHighlights([]Range{{Start: 0, End: 5}, {Start: 6, End: 11}}, 1)
	if !e.searchPulseStart.After(start) {
		t.Fatal("selecting a different current hit did not restart the pulse")
	}
}

func TestSearchMatchPulseScreenshot(t *testing.T) {
	h := newHarness(t, "alpha alpha")
	opts := h.e.Options()
	opts.SearchMatchPulse = true
	h.e.SetOptions(opts)
	h.e.SetHighlights([]Range{{Start: 0, End: 5}, {Start: 6, End: 11}}, 1)
	h.tt.Advance(searchMatchPulseDuration / 2)
	h.shot("search-match-pulse")
}

func TestVimYankFlashIsOptInAndVimOnly(t *testing.T) {
	e := New("alpha beta", Options{VimYankFlash: true})
	e.SetKeymap("vim")
	e.Select(0, 0)
	modalKeys(e, "yy")
	if !e.vimYankActive || len(e.vimYankRanges) != 1 {
		t.Fatalf("Vim yy did not flash its yanked range: active=%v ranges=%v", e.vimYankActive, e.vimYankRanges)
	}
	if got := e.src[e.vimYankRanges[0].Start:e.vimYankRanges[0].End]; got != "alpha beta" {
		t.Fatalf("flashed range=%q", got)
	}

	e.SetKeymap("helix")
	e.Select(0, 0)
	modalKeys(e, "vly")
	if e.vimYankActive {
		t.Fatal("Vim yank flash leaked into Helix mode")
	}

	e.SetKeymap("vim")
	e.SetOptions(Options{})
	e.Select(0, 0)
	modalKeys(e, "yy")
	if e.vimYankActive {
		t.Fatal("disabled Vim yank flash was active")
	}
}

func TestVimYankFlashTracksVisualYankRange(t *testing.T) {
	e := New("alpha beta", Options{VimYankFlash: true})
	e.SetKeymap("vim")
	e.Select(0, 0)
	modalKeys(e, "vly")
	if !e.vimYankActive || len(e.vimYankRanges) != 1 {
		t.Fatalf("Visual yank did not flash its range: active=%v ranges=%v", e.vimYankActive, e.vimYankRanges)
	}
	r := e.vimYankRanges[0]
	if got := e.src[r.Start:r.End]; got != e.modal.register {
		t.Fatalf("flashed Visual range=%q, yanked register=%q", got, e.modal.register)
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
	h.tt.Advance(vimYankFlashDuration / 5)
	h.shot("vim-yank-flash")
}
