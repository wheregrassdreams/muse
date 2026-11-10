package searchprobe

import (
	"fmt"
	"strings"
)

// Synthetic returns count documents with exactly 2048 Body bytes each.
func Synthetic(count int) []Document {
	out := make([]Document, count)
	for n := range out {
		prefix := fmt.Sprintf("commonword 中文搜索 note%05d ", n)
		if n == count-1 {
			prefix += "rareword "
		}
		pad := strings.Repeat("plain body text. ", 128)
		body := prefix + pad
		body = body[:2048]
		out[n] = Document{ID: fmt.Sprint(n), Path: fmt.Sprintf("notes/%05d.md", n), Title: fmt.Sprintf("Note %05d", n), Body: body}
	}
	return out
}
