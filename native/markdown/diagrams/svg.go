package diagrams

import (
	"encoding/xml"
	"image/color"
	"io"
	"math"
	"regexp"
	"strconv"
	"strings"
)

type svgText struct {
	x, y   float32
	size   float32
	bold   bool
	color  color.RGBA
	anchor int
	text   string
}

var (
	reAttr      = regexp.MustCompile(`([a-zA-Z-]+)="([^"]*)"`)
	reVarDef    = regexp.MustCompile(`--([a-z0-9_-]+)\s*:\s*([^;"]+)`)
	reVar       = regexp.MustCompile(`var\(--([a-z0-9_-]+)\s*(?:,\s*([^)]*))?\)`)
	reStyleRx   = regexp.MustCompile(`style="rx:([^;"]+)"`)
	reSVGText   = regexp.MustCompile(`(?s)<text\b[^>]*>.*?</text>`)
	reTransform = regexp.MustCompile(`(translate|scale|matrix|rotate)\s*\(([^)]*)\)`)
	reNum       = regexp.MustCompile(`-?[0-9.]+`)
)

// svgShapes is the SVG with its labels taken out and its CSS variables
// resolved: what is left is the picture, which the rasteriser draws.
func svgShapes(svg string) string {
	svg = reSVGText.ReplaceAllString(svg, "")
	// A corner radius written as a CSS property is not read by the parser.
	svg = reStyleRx.ReplaceAllStringFunc(svg, func(m string) string {
		g := reStyleRx.FindStringSubmatch(m)
		return `rx="` + strings.TrimSpace(g[1]) + `"`
	})
	return svg
}

// resolveSVGVars replaces var(--name, fallback) with its fallback, or with
// what the page sets that name to. pele writes a color per token with the
// color it means as the fallback, so a resolved page is the page's palette.
func resolveSVGVars(svg string) string {
	defs := map[string]string{}
	for _, m := range reVarDef.FindAllStringSubmatch(svg, -1) {
		defs[m[1]] = strings.TrimSpace(m[2])
	}
	for i := 0; i < 8; i++ {
		out := reVar.ReplaceAllStringFunc(svg, func(m string) string {
			g := reVar.FindStringSubmatch(m)
			if v, ok := defs[g[1]]; ok {
				return v
			}
			if g[2] != "" {
				return strings.TrimSpace(g[2])
			}
			return "none"
		})
		if out == svg {
			break
		}
		svg = out
	}
	return svg
}

// svgSize reads the width and height of an SVG.
func svgSize(svg string) (float32, float32) {
	root := svg
	if i := strings.Index(svg, ">"); i > 0 {
		root = svg[:i]
	}
	var w, h float32
	for _, m := range reAttr.FindAllStringSubmatch(root, -1) {
		v := svgNum(m[2], 0)
		switch m[1] {
		case "width":
			w = v
		case "height":
			h = v
		}
	}
	return w, h
}

// svgNum reads the first number of a value.
func svgNum(s string, def float32) float32 {
	m := reNum.FindString(s)
	if m == "" {
		return def
	}
	v, err := strconv.ParseFloat(m, 64)
	if err != nil {
		return def
	}
	return float32(v)
}

// svgColor reads a paint: a color the page wrote out.
func svgColor(s string) (color.RGBA, bool) {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "#") {
		return color.RGBA{}, false
	}
	s = s[1:]
	if len(s) == 3 {
		s = string([]byte{s[0], s[0], s[1], s[1], s[2], s[2]})
	}
	if len(s) != 6 {
		return color.RGBA{}, false
	}
	v, err := strconv.ParseUint(s, 16, 32)
	if err != nil {
		return color.RGBA{}, false
	}
	return color.RGBA{R: uint8(v >> 16), G: uint8(v >> 8), B: uint8(v), A: 255}, true
}

// svgTexts reads the labels of a diagram and where the page puts them: the
// transforms of the groups around a label apply to it, and the font of a
// group is the font of the text in it unless the text says otherwise.
func svgTexts(svg string) []svgText {
	dec := xml.NewDecoder(strings.NewReader(svg))
	type textRun struct {
		text svgText
		m    svgMatrix
	}
	type textGroup struct {
		direct svgText
		m      svgMatrix
		runs   []*textRun
	}
	type frame struct {
		m     svgMatrix
		size  float32
		bold  bool
		fill  color.RGBA
		group *textGroup
		run   *textRun
	}
	base := svgMatrix{a: 1, d: 1}
	stack := []*frame{{m: base, size: 16}}
	var out []svgText
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			break
		}
		top := func() *frame { return stack[len(stack)-1] }
		switch tok := tok.(type) {
		case xml.StartElement:
			parent := top()
			f := &frame{m: parent.m, size: parent.size, bold: parent.bold, fill: parent.fill, group: parent.group, run: parent.run}
			attrs := map[string]string{}
			for _, a := range tok.Attr {
				attrs[a.Name.Local] = a.Value
			}
			if v, ok := attrs["transform"]; ok {
				f.m = parent.m.then(svgTransform(v))
			}
			if v, ok := attrs["font-size"]; ok {
				f.size = svgNum(v, f.size)
			}
			if v, ok := attrs["font-weight"]; ok {
				f.bold = v == "bold" || svgNum(v, 400) >= 600
			}
			if v, ok := attrs["fill"]; ok {
				if c, ok := svgColor(v); ok {
					f.fill = c
				}
			}
			if tok.Name.Local == "text" {
				label := svgText{
					x:      svgNum(attrs["x"], 0),
					y:      svgNum(attrs["y"], 0),
					size:   f.size,
					bold:   f.bold,
					color:  f.fill,
					anchor: anchorIndex(attrs["text-anchor"]),
				}
				f.group = &textGroup{direct: label, m: f.m}
				f.run = nil
			} else if tok.Name.Local == "tspan" && f.group != nil {
				_, hasX := attrs["x"]
				_, hasY := attrs["y"]
				y := f.group.direct.y
				x := f.group.direct.x
				if len(f.group.runs) > 0 {
					last := f.group.runs[len(f.group.runs)-1]
					x, y = last.text.x, last.text.y
				}
				if hasX {
					x = svgNum(attrs["x"], x)
				}
				if hasY {
					y = svgNum(attrs["y"], y)
				}
				// Adjacent tspans without a new x position continue the current
				// SVG text run. A new baseline starts a separate rendered line.
				if !hasX && len(f.group.runs) > 0 {
					last := f.group.runs[len(f.group.runs)-1]
					if !hasY || y == last.text.y {
						f.run = last
						stack = append(stack, f)
						continue
					}
				}
				anchor := f.group.direct.anchor
				if v, ok := attrs["text-anchor"]; ok {
					anchor = anchorIndex(v)
				}
				run := &textRun{
					text: svgText{x: x, y: y, size: f.size, bold: f.bold, color: f.fill, anchor: anchor},
					m:    f.m,
				}
				f.group.runs = append(f.group.runs, run)
				f.run = run
			}
			stack = append(stack, f)
		case xml.CharData:
			f := top()
			if f.group == nil {
				continue
			}
			if f.run != nil {
				f.run.text.text += string(tok)
			} else {
				f.group.direct.text += string(tok)
			}
		case xml.EndElement:
			f := top()
			if tok.Name.Local == "text" && f.group != nil {
				if len(f.group.runs) == 0 {
					t := f.group.direct
					t.text = strings.TrimSpace(t.text)
					if t.text != "" {
						t.x, t.y = f.group.m.apply(t.x, t.y)
						t.size = f.group.m.scale() * t.size
						out = append(out, t)
					}
				} else {
					for _, run := range f.group.runs {
						if strings.TrimSpace(run.text.text) == "" {
							continue
						}
						t := run.text
						t.x, t.y = run.m.apply(t.x, t.y)
						t.size = run.m.scale() * t.size
						out = append(out, t)
					}
				}
			}
			if len(stack) > 1 {
				stack = stack[:len(stack)-1]
			}
		}
	}
	return out
}

func anchorIndex(s string) int {
	switch s {
	case "middle":
		return 1
	case "end":
		return 2
	}
	return 0
}

// svgMatrix is a 2×3 affine transform: x' = a*x + c*y + e, y' = b*x + d*y + f.
type svgMatrix struct{ a, b, c, d, e, f float32 }

// then applies m after the transform it is called on.
func (m svgMatrix) then(n svgMatrix) svgMatrix {
	return svgMatrix{
		a: m.a*n.a + m.c*n.b,
		b: m.b*n.a + m.d*n.b,
		c: m.a*n.c + m.c*n.d,
		d: m.b*n.c + m.d*n.d,
		e: m.a*n.e + m.c*n.f + m.e,
		f: m.b*n.e + m.d*n.f + m.f,
	}
}

// apply moves a point through the transform.
func (m svgMatrix) apply(x, y float32) (float32, float32) {
	return m.a*x + m.c*y + m.e, m.b*x + m.d*y + m.f
}

// scale is how much the transform grows text.
func (m svgMatrix) scale() float32 {
	s := (m.a*m.a + m.b*m.b)
	if s <= 0 {
		return 1
	}
	return float32(math.Sqrt(float64(s)))
}

// svgTransform reads a transform attribute.
func svgTransform(s string) svgMatrix {
	out := svgMatrix{a: 1, d: 1}
	for _, m := range reTransform.FindAllStringSubmatch(s, -1) {
		var n []float32
		for _, f := range strings.FieldsFunc(m[2], func(r rune) bool {
			return r == ',' || r == ' '
		}) {
			n = append(n, svgNum(f, 0))
		}
		switch m[1] {
		case "translate":
			if len(n) >= 2 {
				out = out.then(svgMatrix{a: 1, d: 1, e: n[0], f: n[1]})
			} else if len(n) == 1 {
				out = out.then(svgMatrix{a: 1, d: 1, e: n[0]})
			}
		case "scale":
			if len(n) >= 2 {
				out = out.then(svgMatrix{a: n[0], d: n[1]})
			} else if len(n) == 1 {
				out = out.then(svgMatrix{a: n[0], d: n[0]})
			}
		case "matrix":
			if len(n) >= 6 {
				out = out.then(svgMatrix{a: n[0], b: n[1], c: n[2], d: n[3], e: n[4], f: n[5]})
			}
		case "rotate":
			if len(n) >= 1 {
				rad := n[0] * 3.141592653589793 / 180
				s, c := float32(math.Sin(float64(rad))), float32(math.Cos(float64(rad)))
				out = out.then(svgMatrix{a: c, b: s, c: -s, d: c})
			}
		}
	}
	return out
}
