package doctypes

import "unicode/utf8"

// IsTextData rejects NUL-containing or invalid UTF-8 content before an
// extension-selected text parser receives it. The NUL scan is bounded to the
// same leading sample used by the native file-open policy.
func IsTextData(data []byte) bool {
	if len(data) == 0 {
		return true
	}
	sample := data
	if len(sample) > 8192 {
		sample = sample[:8192]
	}
	for _, b := range sample {
		if b == 0 {
			return false
		}
	}
	return utf8.Valid(data)
}
