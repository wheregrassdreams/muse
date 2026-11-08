package mdoc

import (
	"encoding/xml"
	"image/color"
	"io"
	"math"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"github.com/egoist/mygo/ui"
	"github.com/tdewolff/canvas"
	"github.com/tdewolff/canvas/renderers/rasterizer"
	"tuios.dev/sketch"
)

// SketchTheme is the palette a diagram is drawn in: pele's tokens, in Muse's
// colors. A field left zero keeps pele's own.
type SketchTheme struct {
	Bg, Fg, Muted, Line, Surface, SurfaceAlt, Border, Accent ui.Color
	Radius                                                   float32
}

// SketchOptions are what Muse tells sketch about the page it draws for.
type SketchOptions struct {
	Theme *SketchTheme
	// Measure gives the width of a label. Without it pele measures in Arial's
	// advances, which is right for Latin text and wrong for Chinese and
	// Japanese: a label comes out narrower than it is drawn, and spills.
	Measure func(text string, size float64, bold bool) float64
}

// ParseSketchDiagram draws a Mermaid diagram with sketch — the Go port of
// pele, the renderer Obsidian uses — and lays it out for Muse: the shapes are
// rasterised, the labels are drawn as text, in the theme's colors and the
// page's own font.
func ParseSketchDiagram(code string, opts SketchOptions) (Diagram, bool) {
	out, err := sketch.Render(code, sketch.RenderOptions{
		Theme:    sketchTheme(opts.Theme),
		Measurer: sketchMeasurer(opts.Measure),
	})
	if err != nil {
		return Diagram{}, false
	}
	return newSketchDiagram(out), true
}

// sketchTheme maps Muse's palette onto pele's tokens.
func sketchTheme(t *SketchTheme) *sketch.Theme {
	if t == nil {
		return nil
	}
	th := &sketch.Theme{
		Bg:         stdColor(t.Bg),
		Fg:         stdColor(t.Fg),
		Muted:      stdColor(t.Muted),
		Line:       stdColor(t.Line),
		Surface:    stdColor(t.Surface),
		SurfaceAlt: stdColor(t.SurfaceAlt),
		Border:     stdColor(t.Border),
		Accent:     stdColor(t.Accent),
	}
	if t.Radius > 0 {
		th.Radius = float64(t.Radius)
	}
	return th
}

// stdColor is a Muse color as the standard library's.
func stdColor(c ui.Color) color.RGBA {
	return color.RGBA{R: c.R, G: c.G, B: c.B, A: c.A}
}

// measureFunc is Muse's text measurement as pele's TextMeasurer.
type measureFunc func(text string, size float64, bold bool) float64

// Width gives the width of text at size; pele's style is 0 regular, 1 bold,
// 2 mono, which Muse draws in one family.
func (f measureFunc) Width(text string, size float64, style int) float64 {
	return f(text, size, style == 1)
}

func sketchMeasurer(m func(text string, size float64, bold bool) float64) sketch.TextMeasurer {
	if m == nil {
		return nil
	}
	return measureFunc(m)
}

// svgText is a label of a diagram: where it is written, and how.
type svgText struct {
	x, y   float32 // the baseline's start, in the diagram's own units
	size   float32
	bold   bool
	color  ui.Color
	anchor int // 0 start, 1 middle, 2 end
	text   string
}

// sketchDiagram is a diagram sketch drew, kept as the shapes to rasterise and
// the labels to set.
type sketchDiagram struct {
	w, h   float32 // the diagram's own units
	shapes string  // the SVG with the labels taken out
	texts  []svgText

	mu    sync.Mutex
	drawn map[int]*ui.Bitmap // by the size it was rasterised at
}

// newSketchDiagram reads the SVG sketch draws into shapes and labels.
func newSketchDiagram(out sketch.SVG) Diagram {
	d := &sketchDiagram{
		w:     float32(out.Width),
		h:     float32(out.Height),
		drawn: map[int]*ui.Bitmap{},
	}
	if d.w <= 0 || d.h <= 0 {
		d.w, d.h = svgSize(out.Data)
	}
	resolved := resolveSVGVars(out.Data)
	d.shapes = svgShapes(resolved)
	d.texts = svgTexts(resolved)
	return Diagram{
		width:  func() float32 { return d.w },
		height: func() float32 { return d.h },
		draw:   d.draw,
	}
}

// draw paints the diagram: its shapes, then its labels over them, in the
// font of the page.
func (d *sketchDiagram) draw(p *ui.Painter, r ui.Rect, t *ui.Theme) {
	if d.w <= 0 || d.h <= 0 || r.W <= 0 || r.H <= 0 {
		return
	}
	// The box a diagram is given is not its own shape: it is scaled to fit,
	// and centered in what is left.
	scale := min(r.W/d.w, r.H/d.h)
	if scale <= 0 {
		return
	}
	const density = 2.0
	x0 := r.X + (r.W-d.w*scale)/2
	y0 := r.Y + (r.H-d.h*scale)/2
	if bmp := d.shapeBitmap(int(d.w*scale*density+0.5), int(d.h*scale*density+0.5), scale*density); bmp != nil {
		p.Image(bmp, ui.Rect{X: x0, Y: y0, W: d.w * scale, H: d.h * scale}, ui.Contain)
	}
	for _, tx := range d.texts {
		color := tx.color
		if color == (ui.Color{}) {
			color = t.Text
		}
		sp := ui.Span{Text: tx.text, Size: tx.size * scale, Color: color}
		if tx.bold {
			sp.Weight = 700
		}
		w, _ := p.MeasureText(0, sp)
		x := x0 + tx.x*scale
		switch tx.anchor {
		case 1:
			x -= w / 2
		case 2:
			x -= w
		}
		// A diagram writes a baseline; text is set from its top.
		p.RichText(x, y0+tx.y*scale-sp.Size*0.8, 0, sp)
	}
}

// shapeBitmap rasterises the shapes at the size they are drawn, once.
func (d *sketchDiagram) shapeBitmap(w, h int, scale float32) *ui.Bitmap {
	key := w<<16 | h
	d.mu.Lock()
	defer d.mu.Unlock()
	if bmp, ok := d.drawn[key]; ok {
		return bmp
	}
	c, err := canvas.ParseSVG(strings.NewReader(d.shapes))
	if c == nil {
		return nil
	}
	_ = err // a part Muse cannot read is still most of the diagram
	img := rasterizer.Draw(c, canvas.DPMM(float64(scale)), canvas.DefaultColorSpace)
	if img == nil {
		return nil
	}
	bmp := ui.NewBitmap(img)
	if len(d.drawn) > 8 {
		d.drawn = map[int]*ui.Bitmap{}
	}
	d.drawn[key] = bmp
	return bmp
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
func svgColor(s string) (ui.Color, bool) {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "#") {
		return ui.Color{}, false
	}
	s = s[1:]
	if len(s) == 3 {
		s = string([]byte{s[0], s[0], s[1], s[1], s[2], s[2]})
	}
	if len(s) != 6 {
		return ui.Color{}, false
	}
	v, err := strconv.ParseUint(s, 16, 32)
	if err != nil {
		return ui.Color{}, false
	}
	return ui.RGBA(uint8(v>>16), uint8(v>>8), uint8(v), 1), true
}

// svgTexts reads the labels of a diagram and where the page puts them: the
// transforms of the groups around a label apply to it, and the font of a
// group is the font of the text in it unless the text says otherwise.
func svgTexts(svg string) []svgText {
	dec := xml.NewDecoder(strings.NewReader(svg))
	type frame struct {
		m     svgMatrix
		size  float32
		bold  bool
		fill  ui.Color
		text  bool // inside a <text>: character data is a label
		start svgText
		cur   svgText
		has   bool
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
			f := &frame{m: parent.m, size: parent.size, bold: parent.bold, fill: parent.fill}
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
			if v, ok := attrs["text-anchor"]; ok {
				f.start.anchor = anchorIndex(v)
			}
			if tok.Name.Local == "text" {
				f.text = true
				f.cur = svgText{
					x:      svgNum(attrs["x"], 0),
					y:      svgNum(attrs["y"], 0),
					size:   f.size,
					bold:   f.bold,
					color:  f.fill,
					anchor: anchorIndex(attrs["text-anchor"]),
				}
				f.has = true
			}
			stack = append(stack, f)
		case xml.CharData:
			f := top()
			if !f.text {
				continue
			}
			f.cur.text += string(tok)
		case xml.EndElement:
			f := top()
			if tok.Name.Local == "text" && f.has {
				t := f.cur
				t.text = strings.TrimSpace(t.text)
				if t.text != "" {
					t.x, t.y = f.m.apply(t.x, t.y)
					t.size = f.m.scale() * t.size
					out = append(out, t)
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
