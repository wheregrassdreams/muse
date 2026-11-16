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
	Cursor CursorTransition
	Search SearchPulse
	Yank   YankFlash
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
	s.Cursor = CursorTransition{}
	s.Search = SearchPulse{}
	s.Yank = YankFlash{}
}

func (s *State) CancelCursor() {
	if s != nil {
		s.Cursor = CursorTransition{}
	}
}

func (s *State) CancelSearchPulse() {
	if s != nil {
		s.Search = SearchPulse{}
	}
}

func (s *State) CancelYankFlash() {
	if s != nil {
		s.Yank = YankFlash{}
	}
}

func (s *State) BeginCursor(from, to int, now time.Time) {
	if s == nil {
		return
	}
	var rebased Rect
	rebasedSet := false
	if s.Cursor.Active && s.Cursor.RectsSet {
		if current, active := InterpolateSmoothCursor(s.Cursor.Started, now, s.Cursor.FromRect, s.Cursor.ToRect); active {
			rebased = current
			rebasedSet = true
		}
	}
	s.Cursor = CursorTransition{
		From: from, To: to, Started: now, Active: from != to,
		FromRect: rebased, FromRectSet: rebasedSet,
	}
}

// CursorRects returns interpolated cursor geometry while the transition is
// active. The final rectangle is returned with active=false when it expires.
func (s *State) CursorRects(now time.Time) (Rect, bool) {
	if s == nil || !s.Cursor.Active || !s.Cursor.RectsSet {
		return Rect{}, false
	}
	rect, active := InterpolateSmoothCursor(s.Cursor.Started, now, s.Cursor.FromRect, s.Cursor.ToRect)
	if !active {
		s.Cursor.Active = false
	}
	return rect, active
}

func (s *State) SetCursorRects(from, to Rect) {
	if s == nil {
		return
	}
	s.Cursor.FromRect, s.Cursor.ToRect = from, to
	s.Cursor.FromRectSet, s.Cursor.RectsSet = true, true
}

func (s *State) SetCursorFromRect(from Rect) {
	if s == nil {
		return
	}
	s.Cursor.FromRect = from
	s.Cursor.FromRectSet = true
}

func (s *State) StartSearchPulse(now time.Time) {
	if s != nil {
		s.Search = SearchPulse{Started: now, Active: true}
	}
}

func (s *State) SearchPulseOpacity(now time.Time) (float32, bool) {
	if s == nil || !s.Search.Active {
		return 0, false
	}
	alpha, active := SearchMatchPulse(now.Sub(s.Search.Started))
	if !active {
		s.Search.Active = false
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
	s.Yank = YankFlash{Started: now, Ranges: valid, Active: len(valid) > 0}
}

func (s *State) YankFlashOpacity(now time.Time) (float32, []TextRange, bool) {
	if s == nil || !s.Yank.Active {
		return 0, nil, false
	}
	opacity, active := VimYankFlashOpacity(now.Sub(s.Yank.Started))
	if !active {
		s.Yank.Active = false
		s.Yank.Ranges = nil
		return 0, nil, false
	}
	return opacity, s.Yank.Ranges, true
}
