package document

import "time"

// NewNoteSource returns the initial Markdown source for a newly created note.
// The caller supplies the clock so creation remains deterministic in tests.
func NewNoteSource(now time.Time) (string, error) {
	source, _, err := EnsureCreatedAt("", now)
	return source, err
}
