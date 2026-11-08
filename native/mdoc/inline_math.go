package mdoc

import (
	"strings"
	"unicode"

	"github.com/egoist/mygo/ui"
)

// inlineRun is one laid-out word or inline formula.
type inlineRun struct {
	span  ui.Span
	bmp   *ui.Bitmap
	w, h  float32
	pad   float32
	space bool
}

type inlineLine struct {
	runs  []inlineRun
	width float32
	h     float32
}

// renderInlineMath lays out paragraphs that contain inline TeX with real
// typeset images, wrapping at the content width. Plain paragraphs keep using
// ui.RichText.
func renderInlineMath(c *ui.Context, spans []Span, s rstyle, size float32, weight int, lineHeight float32) {
	layout := layoutInlineMath(c, spans, s, size, weight, lineHeight)
	ui.Box(c).FillWidth().Height(layout.height).Draw(func(p *ui.Painter, r ui.Rect) {
		y := r.Y
		for _, line := range layout.lines {
			x := r.X
			for _, run := range line.runs {
				if run.bmp != nil {
					h := run.h
					p.Image(run.bmp, ui.Rect{X: x + run.pad, Y: y + (line.h-h)/2, W: run.w - 2*run.pad, H: h}, ui.Contain)
				} else {
					_, th := p.MeasureText(0, run.span)
					p.RichText(x, y+(line.h-th)/2, 0, run.span)
				}
				x += run.w
			}
			y += line.h
		}
	})
}

func layoutInlineMath(c *ui.Context, spans []Span, s rstyle, size float32, weight int, lineHeight float32) *inlineLayout {
	maxW := s.inlineWidth()
	body := size
	if body <= 0 {
		body = 16
	}
	if lineHeight <= 0 {
		lineHeight = 1.9
	}
	lineH := body * lineHeight
	var lines []inlineLine
	var cur inlineLine
	flush := func() {
		for len(cur.runs) > 0 && cur.runs[len(cur.runs)-1].space {
			cur.runs = cur.runs[:len(cur.runs)-1]
		}
		if len(cur.runs) > 0 {
			if cur.h < lineH {
				cur.h = lineH
			}
			lines = append(lines, cur)
		}
		cur = inlineLine{}
	}
	add := func(run inlineRun) {
		if run.space && len(cur.runs) == 0 {
			return
		}
		if cur.width+run.w > maxW && cur.width > 0 {
			flush()
		}
		cur.runs = append(cur.runs, run)
		cur.width += run.w
		if run.h > cur.h {
			cur.h = run.h
		}
	}
	for _, sp := range spans {
		if sp.Math {
			if bmp := mathBitmap(sp.Text, float64(body), c.Theme().Dark, false); bmp != nil {
				w, h := bmp.Size()
				iw := float32(w) * 72 / mathDPI
				add(inlineRun{bmp: bmp, w: iw + 3, h: float32(h) * 72 / mathDPI, pad: 1.5})
				continue
			}
			sp.Text = texToUnicode(sp.Text)
		}
		u := uiSpans(c, []Span{sp}, s)[0]
		if u.Size == 0 {
			u.Size = body
		}
		if weight > 0 && u.Weight == 0 {
			u.Weight = weight
		}
		for _, tok := range tokenizeInline(u.Text) {
			u.Text = tok
			w, _ := c.MeasureText(0, u)
			add(inlineRun{span: u, w: w, space: tok == " "})
		}
	}
	flush()

	height := float32(0)
	for _, line := range lines {
		height += line.h
	}
	return &inlineLayout{lines: lines, height: height}
}

type inlineLayout struct {
	lines  []inlineLine
	height float32
}

func tokenizeInline(text string) []string {
	var out []string
	var word strings.Builder
	flush := func() {
		if word.Len() > 0 {
			out = append(out, word.String())
			word.Reset()
		}
	}
	for _, r := range text {
		switch {
		case unicode.IsSpace(r):
			flush()
			out = append(out, " ")
		case unicode.Is(unicode.Han, r) || unicode.Is(unicode.Hiragana, r) ||
			unicode.Is(unicode.Katakana, r) || unicode.Is(unicode.Hangul, r):
			flush()
			out = append(out, string(r))
		default:
			word.WriteRune(r)
		}
	}
	flush()
	return out
}

func spansHaveMath(spans []Span) bool {
	for _, sp := range spans {
		if sp.Math {
			return true
		}
	}
	return false
}
