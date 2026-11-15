// Package input owns toolkit-neutral state for composing multi-key editor
// commands. The editor translates toolkit events into this sequence model.
package input

const maxCount = 9999

// Sequence is the pending prefix and count of a modal command. It contains no
// editor or toolkit state, so parsing and display behavior can be tested alone.
type Sequence struct {
	Count         int
	OperatorCount int
	Pending       string
	Linewise      bool
}

// FeedDigit appends one decimal digit to the motion count. A leading zero is
// a motion command, not a count, and therefore returns false.
func (s *Sequence) FeedDigit(digit rune) bool {
	if digit < '0' || digit > '9' || digit == '0' && s.Count == 0 {
		return false
	}
	s.Count = min(maxCount, s.Count*10+int(digit-'0'))
	return true
}

// ConsumeCounts returns the defaulted motion/operator counts and clears only
// the motion count. Pending operators retain their count until resolved.
func (s *Sequence) ConsumeCounts() (motion, operator int) {
	motion, operator = max(1, s.Count), max(1, s.OperatorCount)
	s.Count = 0
	return motion, operator
}

// BeginOperator starts a pending operator sequence with the count entered
// before it. A later count is multiplied by this when the operator resolves.
func (s *Sequence) BeginOperator(operator string, count int) {
	s.Pending = operator
	s.OperatorCount = max(1, count)
}

// Reset discards all prefix, count, and linewise state.
func (s *Sequence) Reset() { *s = Sequence{} }
