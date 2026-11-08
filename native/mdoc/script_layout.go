package mdoc

import (
	"image"
	"image/draw"
	"strings"
	"unicode/utf8"
)

// go-latex parses the AST for sup/sub but does not lay out every case, as
// the limits of \int. When it fails on an expression containing ^ or _,
// Muse splits the expression at the first top-level script and composes the
// base, its scripts and the text after them itself, on one baseline.
func typesetScripts(expr string, size float64, display bool) (typeset, error) {
	base, sup, sub, rest, ok := splitScript(expr)
	if !ok || strings.TrimSpace(base) == "" {
		return typeset{}, errNoScripts
	}
	b, err := typesetMath(base, size, display)
	if err != nil {
		return typeset{}, err
	}
	em := size * mathDPI / 72
	var parts []placed
	parts = append(parts, placed{ts: b})
	baseW := b.img.Bounds().Dx()
	ascent := b.baseline
	depth := b.img.Bounds().Dy() - b.baseline
	// A displayed formula puts the limits of a big operator above and below
	// it, as TeX does; a formula in a line of text puts them beside it.
	limits := display && isBigOperator(base)
	advance := baseW
	if sup != "" || sub != "" {
		up, dn := typeset{}, typeset{}
		if sup != "" {
			s, err := typesetMath(sup, size*0.7, false)
			if err != nil {
				return typeset{}, err
			}
			up = s
		}
		if sub != "" {
			s, err := typesetMath(sub, size*0.7, false)
			if err != nil {
				return typeset{}, err
			}
			dn = s
		}
		gap := int(em * 0.18)
		switch {
		case limits:
			widest := 0
			if sup != "" {
				w := up.img.Bounds().Dx()
				rise := up.img.Bounds().Dy() - up.baseline + ascent + gap
				parts = append(parts, placed{ts: up, x: max((baseW-w)/2, 0), rise: rise})
				widest = max(widest, w)
			}
			if sub != "" {
				w := dn.img.Bounds().Dx()
				rise := -(dn.baseline + depth + gap)
				parts = append(parts, placed{ts: dn, x: max((baseW-w)/2, 0), rise: rise})
				widest = max(widest, w)
			}
			advance = max(baseW, widest)
		default:
			scriptW := 0
			if sup != "" {
				// Raised by at least half an em, more for a tall base, as
				// the integral sign.
				shift := max(int(em*0.42), ascent-up.baseline*2/3)
				parts = append(parts, placed{ts: up, x: baseW + 1, rise: shift})
				scriptW = max(scriptW, up.img.Bounds().Dx()+1)
			}
			if sub != "" {
				// Lowered by a fifth of an em, more under a deep base.
				shift := max(int(em*0.22), depth-(dn.img.Bounds().Dy()-dn.baseline)/2)
				parts = append(parts, placed{ts: dn, x: baseW + 1, rise: -shift})
				scriptW = max(scriptW, dn.img.Bounds().Dx()+1)
			}
			advance += scriptW
		}
	}
	x := advance
	if trimmed := strings.TrimLeft(rest, " "); strings.TrimSpace(trimmed) != "" {
		r, err := typesetMath(trimmed, size, display)
		if err != nil {
			return typeset{}, err
		}
		if len(trimmed) < len(rest) || startsWithOperator(trimmed) {
			// The space before a binary operator or a relation is the one
			// TeX puts there: go-latex would have set it, but the formula
			// is composed in parts here.
			x += int(em * 0.2)
		}
		parts = append(parts, placed{ts: r, x: x + int(em*0.1)})
	}
	return compose(parts), nil
}

// bigOperators are the signs whose limits a displayed formula sets above and
// below them.
const bigOperators = "∫∑∏∐⋃⋂⨁⨂⨀⋁⋀∬∭"

// bigOperatorMacros are the same, written as macros.
var bigOperatorMacros = []string{
	`\int`, `\iint`, `\iiint`, `\sum`, `\prod`, `\coprod`,
	`\bigcup`, `\bigcap`, `\bigoplus`, `\bigotimes`, `\bigodot`,
	`\bigvee`, `\bigwedge`,
}

// isBigOperator reports whether a base is one of the signs a displayed
// formula sets its limits above and below.
func isBigOperator(base string) bool {
	base = strings.TrimSpace(base)
	for _, m := range bigOperatorMacros {
		if base == m {
			return true
		}
	}
	return strings.ContainsAny(base, bigOperators) && len([]rune(base)) == 1
}

// startsWithOperator reports whether a run begins with a binary operator or
// a relation, which the space before it goes with.
func startsWithOperator(s string) bool {
	r, _ := utf8.DecodeRuneInString(s)
	return strings.ContainsRune("+−-±∓×·=<>≤≥≠≈≡∼→←↔⇒⇔∈∉⊂⊃∪∩", r)
}

// placed is a part of a formula composed by Muse: its typesetting, where its
// starts across, and how far its baseline is raised above the formula's.
type placed struct {
	ts   typeset
	x    int
	rise int
}

// compose draws parts on one baseline, into an image as large as all of
// them.
func compose(parts []placed) typeset {
	top, bottom, right := 0, 0, 0
	for _, p := range parts {
		b := p.ts.img.Bounds()
		// Relative to the common baseline at 0: the part's top and bottom.
		top = min(top, -p.rise-p.ts.baseline)
		bottom = max(bottom, -p.rise-p.ts.baseline+b.Dy())
		right = max(right, p.x+b.Dx())
	}
	dst := image.NewRGBA(image.Rect(0, 0, right, bottom-top))
	for _, p := range parts {
		b := p.ts.img.Bounds()
		y := -p.rise - p.ts.baseline - top
		draw.Draw(dst, image.Rect(p.x, y, p.x+b.Dx(), y+b.Dy()), p.ts.img, b.Min, draw.Over)
	}
	return typeset{img: dst, baseline: -top}
}

func splitScript(expr string) (base, sup, sub, rest string, ok bool) {
	depth := 0
	for i := 0; i < len(expr); i++ {
		switch expr[i] {
		case '{':
			depth++
		case '}':
			if depth > 0 {
				depth--
			}
		case '^', '_':
			if depth != 0 {
				continue
			}
			base = expr[:i]
			j := i
			for j < len(expr) && (expr[j] == '^' || expr[j] == '_') {
				marker := expr[j]
				content, next := scriptContent(expr, j+1)
				if content == "" {
					return "", "", "", "", false
				}
				if marker == '^' {
					sup = content
				} else {
					sub = content
				}
				j = next
			}
			rest = expr[j:]
			return base, sup, sub, rest, true
		}
	}
	return "", "", "", "", false
}

func scriptContent(expr string, i int) (content string, next int) {
	if i >= len(expr) {
		return "", i
	}
	if expr[i] == '{' {
		depth := 0
		for j := i; j < len(expr); j++ {
			switch expr[j] {
			case '{':
				depth++
			case '}':
				depth--
				if depth == 0 {
					return expr[i+1 : j], j + 1
				}
			}
		}
		return "", i
	}
	return string(expr[i]), i + 1
}

type noScriptsError struct{}

func (noScriptsError) Error() string { return "no top-level scripts" }

var errNoScripts = noScriptsError{}
