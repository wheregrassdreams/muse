package feedback

import "time"

// TextRange is a half-open source interval used by transient editor feedback.
// It is intentionally independent of the editor's selection model.
type TextRange struct {
	Start int
	End   int
}

// State owns short-lived visual feedback state for one editor surface.
// Callers provide source offsets and renderer-space rectangles as values;
// this package never retains an editor, widget, or renderer object.
type State struct {
	cursor CursorTransition
	search SearchPulse
	yank   YankFlash
}

type CursorTransition struct {
	From, To    int
	Started     time.Time
	Active      bool
	FromRect    Rect
	ToRect      Rect
	FromRectSet bool
	RectsSet    bool
}

type SearchPulse struct {
	Started time.Time
	Active  bool
}

type YankFlash struct {
	Started time.Time
	Ranges  []TextRange
	Active  bool
}

func (s *State) Reset() {
	if s == nil {
		return
	}
	s.cursor = CursorTransition{}
	s.search = SearchPulse{}
	s.yank = YankFlash{}
}

// CursorState returns a value snapshot of the current visual cursor transition.
func (s *State) CursorState() CursorTransition {
	if s == nil {
		return CursorTransition{}
	}
	return s.cursor
}

// SearchState returns a value snapshot of the current match pulse.
func (s *State) SearchState() SearchPulse {
	if s == nil {
		return SearchPulse{}
	}
	return s.search
}

// YankState returns a detached snapshot of the active yank feedback.
func (s *State) YankState() YankFlash {
	if s == nil {
		return YankFlash{}
	}
	state := s.yank
	state.Ranges = append([]TextRange(nil), state.Ranges...)
	return state
}

// YankRanges returns the current ranges for synchronous rendering. The slice
// is borrowed and must not be retained or modified; it remains valid until the
// next state mutation.
func (s *State) YankRanges() []TextRange {
	if s == nil || !s.yank.Active {
		return nil
	}
	return s.yank.Ranges
}

func (s *State) CancelCursor() {
	if s != nil {
		s.cursor = CursorTransition{}
	}
}

func (s *State) CancelSearchPulse() {
	if s != nil {
		s.search = SearchPulse{}
	}
}

func (s *State) CancelYankFlash() {
	if s != nil {
		s.yank = YankFlash{}
	}
}

func (s *State) BeginCursor(from, to int, now time.Time) {
	if s == nil {
		return
	}
	var rebased Rect
	rebasedSet := false
	if s.cursor.Active && s.cursor.RectsSet {
		if current, active := InterpolateSmoothCursor(s.cursor.Started, now, s.cursor.FromRect, s.cursor.ToRect); active {
			rebased = current
			rebasedSet = true
		}
	}
	s.cursor = CursorTransition{
		From: from, To: to, Started: now, Active: from != to,
		FromRect: rebased, FromRectSet: rebasedSet,
	}
}

// CursorRects returns interpolated cursor geometry while the transition is
// active. The final rectangle is returned with active=false when it expires.
func (s *State) CursorRects(now time.Time) (Rect, bool) {
	if s == nil || !s.cursor.Active || !s.cursor.RectsSet {
		return Rect{}, false
	}
	rect, active := InterpolateSmoothCursor(s.cursor.Started, now, s.cursor.FromRect, s.cursor.ToRect)
	if !active {
		s.cursor.Active = false
	}
	return rect, active
}

func (s *State) SetCursorRects(from, to Rect) {
	if s == nil {
		return
	}
	s.cursor.FromRect, s.cursor.ToRect = from, to
	s.cursor.FromRectSet, s.cursor.RectsSet = true, true
}

func (s *State) SetCursorFromRect(from Rect) {
	if s == nil {
		return
	}
	s.cursor.FromRect = from
	s.cursor.FromRectSet = true
}

func (s *State) StartSearchPulse(now time.Time) {
	if s != nil {
		s.search = SearchPulse{Started: now, Active: true}
	}
}

func (s *State) SearchPulseOpacity(now time.Time) (float32, bool) {
	if s == nil || !s.search.Active {
		return 0, false
	}
	alpha, active := SearchMatchPulse(now.Sub(s.search.Started))
	if !active {
		s.search.Active = false
	}
	return alpha, active
}

func (s *State) StartYankFlash(ranges []TextRange, now time.Time) {
	if s == nil {
		return
	}
	valid := make([]TextRange, 0, len(ranges))
	for _, r := range ranges {
		if r.End > r.Start {
			valid = append(valid, r)
		}
	}
	s.yank = YankFlash{Started: now, Ranges: valid, Active: len(valid) > 0}
}

func (s *State) YankFlashOpacity(now time.Time) (float32, bool) {
	if s == nil || !s.yank.Active {
		return 0, false
	}
	opacity, active := VimYankFlashOpacity(now.Sub(s.yank.Started))
	if !active {
		s.yank.Active = false
		s.yank.Ranges = nil
		return 0, false
	}
	return opacity, true
}
