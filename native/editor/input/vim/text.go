package vim

import (
	"strings"

	"github.com/go-text/typesetting/segmenter"
)

// FirstGrapheme returns the first extended grapheme in a Vim character
// command's text input.
func FirstGrapheme(text string) string {
	var seg segmenter.Segmenter
	_ = seg.InitWithBytes([]byte(text))
	it := seg.GraphemeIterator()
	if !it.Next() {
		return ""
	}
	g := it.Grapheme()
	return text[g.OffsetInBytes : g.OffsetInBytes+g.LengthInBytes]
}

// TrimLastGrapheme removes the final extended grapheme, as Vim's search
// prompt does when Backspace is pressed.
func TrimLastGrapheme(text string) string {
	if text == "" {
		return ""
	}
	var seg segmenter.Segmenter
	_ = seg.InitWithBytes([]byte(text))
	it := seg.GraphemeIterator()
	end := 0
	for it.Next() {
		end = it.Grapheme().OffsetInBytes
	}
	return text[:end]
}

// LinewiseRegisterPayload repeats linewise register content without carrying
// a trailing newline from the stored register value.
func LinewiseRegisterPayload(text string, count int) string {
	lines := strings.Split(strings.TrimSuffix(text, "\n"), "\n")
	count = max(1, count)
	var repeated []string
	for range count {
		repeated = append(repeated, lines...)
	}
	return strings.Join(repeated, "\n")
}
