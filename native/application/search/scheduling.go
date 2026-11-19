package search

import "time"

const (
	balancedAfter = 8 * time.Millisecond
	slowAfter     = 24 * time.Millisecond
	balancedWait  = 16 * time.Millisecond
	lowLoadWait   = 40 * time.Millisecond
)

// CoalesceDelay adapts interactive query cadence to measured search latency.
// Fast searches remain immediate; slower or overlapping searches briefly
// coalesce keystrokes so repeated prefixes do not queue redundant work.
func CoalesceDelay(estimatedCost time.Duration, queryInFlight bool) time.Duration {
	if estimatedCost >= slowAfter {
		return lowLoadWait
	}
	if estimatedCost >= balancedAfter || queryInFlight {
		return balancedWait
	}
	return 0
}

// QueryRequest is the part of an interactive search request that determines
// whether a search has already been submitted. Literal mode is included
// because the same text can have different search semantics.
type QueryRequest struct {
	Text           string
	Literal        bool
	ActivePath     string
	ActiveRevision string
}

// QueryScheduler coalesces interactive search requests and remembers the
// last request submitted to the search executor. It owns no timers or UI
// callbacks: callers provide time and poll the due request from their own
// lifecycle.
type QueryScheduler struct {
	submitted    QueryRequest
	hasSubmitted bool
	pending      QueryRequest
	hasPending   bool
	forceNext    bool
	due          time.Time
}

// Schedule records the latest request and returns the time remaining before
// it can be submitted. A burst of edits updates the pending request but keeps
// the original deadline, so repeated keystrokes coalesce into one search.
// When the latest request is identical to the already submitted request, any
// obsolete pending request is cancelled.
func (s *QueryScheduler) Schedule(request QueryRequest, estimatedCost time.Duration, queryInFlight bool, now time.Time) time.Duration {
	if s == nil {
		return 0
	}
	if s.hasSubmitted && !s.forceNext && request == s.submitted {
		s.clearPending()
		return 0
	}
	if !s.hasPending {
		s.hasPending = true
		s.pending = request
		if s.due.IsZero() && !s.forceNext {
			s.due = now.Add(CoalesceDelay(estimatedCost, queryInFlight))
		}
	} else {
		s.pending = request
	}
	if remaining := s.due.Sub(now); remaining > 0 {
		return remaining
	}
	return 0
}

// Force invalidates the submitted request and schedules the next request even
// when its text and literal mode are unchanged. A non-positive wait makes it
// available on the next Take call.
func (s *QueryScheduler) Force(now time.Time, wait time.Duration) {
	if s == nil {
		return
	}
	s.forceNext = true
	if wait < 0 {
		wait = 0
	}
	s.due = now.Add(wait)
}

// Due reports the deadline for the currently pending request.
func (s *QueryScheduler) Due() (time.Time, bool) {
	if s == nil || !s.hasPending || s.due.IsZero() {
		return time.Time{}, false
	}
	return s.due, true
}

// Take returns the pending request when its deadline has arrived. Once
// returned, the request becomes the submitted baseline for future deduping.
func (s *QueryScheduler) Take(now time.Time) (QueryRequest, bool) {
	if s == nil || !s.hasPending || now.Before(s.due) {
		return QueryRequest{}, false
	}
	request := s.pending
	forced := s.forceNext
	s.clearPending()
	s.forceNext = false
	if !forced && s.hasSubmitted && request == s.submitted {
		return QueryRequest{}, false
	}
	s.submitted = request
	s.hasSubmitted = true
	return request, true
}

// Reset forgets both submitted and pending requests, including any forced
// refresh. This is useful when the search corpus or its owning session is
// replaced.
func (s *QueryScheduler) Reset() {
	if s == nil {
		return
	}
	*s = QueryScheduler{}
}

func (s *QueryScheduler) clearPending() {
	s.pending = QueryRequest{}
	s.hasPending = false
	s.due = time.Time{}
}
