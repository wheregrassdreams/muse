// Package render owns backend-neutral projections of Markdown syntax for
// inline presentation. UI-specific painting remains in the root facade.
package render

import (
	"strings"

	texmath "muse/markdown/math"
)

// InlineRuns splits one inline Markdown source line into visible runs. It
// covers the common inline forms used by the editor: **bold**, *italic*,
// ~~strike~~, `code`, [text](url) and $math$. Unknown syntax stays literal.
func InlineRuns(src string) []InlineRun {
	return inlineRuns(src, 0, InlineRun{})
}

func inlineRuns(src string, base int, style InlineRun) []InlineRun {
	var out []InlineRun
	plainStart := 0
	flush := func(end int) {
		if end > plainStart {
			run := style
			run.Text = src[plainStart:end]
			run.SourceStart = base + plainStart
			run.SourceEnd = base + end
			out = append(out, run)
		}
	}
	for i := 0; i < len(src); {
		before := i
		if src[i] == '\\' && i+1 < len(src) {
			i += 2
			continue
		}
		// Code spans are literal: no nested inline parsing.
		if src[i] == '`' {
			if end := strings.IndexByte(src[i+1:], '`'); end >= 0 {
				flush(i)
				run := style
				run.Code = true
				run.Text = src[i+1 : i+1+end]
				run.SourceStart = base + i + 1
				run.SourceEnd = base + i + 1 + end
				out = append(out, run)
				i += end + 2
				plainStart = i
				continue
			}
		}
		if src[i] == '$' {
			if end := strings.IndexByte(src[i+1:], '$'); end >= 0 {
				flush(i)
				run := style
				run.Math = true
				run.Text = src[i+1 : i+1+end]
				run.Display = texmath.TeXToUnicode(run.Text)
				run.SourceStart = base + i + 1
				run.SourceEnd = base + i + 1 + end
				out = append(out, run)
				i += end + 2
				plainStart = i
				continue
			}
		}
		if src[i] == '[' {
			if labelEnd := strings.IndexByte(src[i+1:], ']'); labelEnd >= 0 {
				urlStart := i + 1 + labelEnd + 1
				if urlStart < len(src) && src[urlStart] == '(' {
					if urlEnd := strings.IndexByte(src[urlStart+1:], ')'); urlEnd >= 0 {
						flush(i)
						label := src[i+1 : i+1+labelEnd]
						url := src[urlStart+1 : urlStart+1+urlEnd]
						for _, r := range inlineRuns(label, base+i+1, style) {
							r.Link = url
							out = append(out, r)
						}
						i = urlStart + 1 + urlEnd + 1
						plainStart = i
						continue
					}
				}
			}
		}
		// Emphasis markers: only open when a closer exists.
		for _, m := range []struct {
			marker string
			apply  func(InlineRun) InlineRun
		}{
			{"**", func(r InlineRun) InlineRun { r.Bold = true; return r }},
			{"__", func(r InlineRun) InlineRun { r.Bold = true; return r }},
			{"~~", func(r InlineRun) InlineRun { r.Strike = true; return r }},
			{"*", func(r InlineRun) InlineRun { r.Italic = true; return r }},
			{"_", func(r InlineRun) InlineRun { r.Italic = true; return r }},
		} {
			if strings.HasPrefix(src[i:], m.marker) {
				if end := strings.Index(src[i+len(m.marker):], m.marker); end > 0 {
					flush(i)
					inner := src[i+len(m.marker) : i+len(m.marker)+end]
					innerStyle := m.apply(style)
					for _, r := range inlineRuns(inner, base+i+len(m.marker), innerStyle) {
						out = append(out, r)
					}
					i += len(m.marker) + end + len(m.marker)
					plainStart = i
					break
				}
			}
		}
		if i == before {
			i++
		}
	}
	flush(len(src))
	return out
}
