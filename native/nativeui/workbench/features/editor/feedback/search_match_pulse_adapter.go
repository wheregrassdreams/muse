package feedback

import "time"

// SearchPulseChange describes a current-result transition without exposing
// editor-specific highlight or range types.
type SearchPulseChange struct {
	Previous, Next TextRange
	PreviousValid  bool
	NextValid      bool
	Enabled        bool
	Now            time.Time
}

// UpdateSearchPulse starts a pulse only when an enabled current result becomes
// visible or changes. It returns whether a new pulse was started.
func (h *Handle) UpdateSearchPulse(change SearchPulseChange) bool {
	if h == nil {
		return false
	}
	if !change.Enabled || !change.NextValid || change.PreviousValid && change.Previous == change.Next {
		return false
	}
	now := change.Now
	if now.IsZero() {
		now = time.Now()
	}
	h.StartSearchPulse(now)
	return true
}
