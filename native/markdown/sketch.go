package markdown

import (
	"image/color"
	"strings"
	"sync"

	"github.com/egoist/mygo/ui"
	"github.com/tdewolff/canvas"
	"github.com/tdewolff/canvas/renderers/rasterizer"
	"muse/markdown/diagrams"
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
	// advances, which is wrong for many system fonts.
	Measure func(text string, size float64, bold bool) float64
}

// ParseSketchDiagram asks the backend-neutral diagrams subsystem to parse
// Mermaid and adapts its shape/label result to the MyGo painter.
func ParseSketchDiagram(code string, opts SketchOptions) (Diagram, bool) {
	var theme *diagrams.Theme
	if opts.Theme != nil {
		t := opts.Theme
		theme = &diagrams.Theme{
			Bg: stdColor(t.Bg), Fg: stdColor(t.Fg), Muted: stdColor(t.Muted), Line: stdColor(t.Line),
			Surface: stdColor(t.Surface), SurfaceAlt: stdColor(t.SurfaceAlt), Border: stdColor(t.Border),
			Accent: stdColor(t.Accent), Radius: float64(t.Radius),
		}
	}
	result, ok := diagrams.Parse(code, diagrams.Options{Theme: theme, Measure: opts.Measure})
	if !ok {
		return Diagram{}, false
	}
	d := &sketchDiagram{result: result, drawn: map[int]*ui.Bitmap{}}
	return Diagram{width: func() float32 { return result.Width }, height: func() float32 { return result.Height }, draw: d.draw}, true
}

func stdColor(c ui.Color) color.RGBA {
	return color.RGBA{R: c.R, G: c.G, B: c.B, A: c.A}
}

type sketchDiagram struct {
	result diagrams.Result
	mu     sync.Mutex
	drawn  map[int]*ui.Bitmap
}

func (d *sketchDiagram) draw(p *ui.Painter, r ui.Rect, t *ui.Theme) {
	w, h := d.result.Width, d.result.Height
	if w <= 0 || h <= 0 || r.W <= 0 || r.H <= 0 {
		return
	}
	scale := min(r.W/w, r.H/h)
	if scale <= 0 {
		return
	}
	const density = 2.0
	x0 := r.X + (r.W-w*scale)/2
	y0 := r.Y + (r.H-h*scale)/2
	if bmp := d.shapeBitmap(int(w*scale*density+0.5), int(h*scale*density+0.5), scale*density); bmp != nil {
		p.Image(bmp, ui.Rect{X: x0, Y: y0, W: w * scale, H: h * scale}, ui.Contain)
	}
	for _, tx := range d.result.Labels {
		c := ui.Color{R: tx.Color.R, G: tx.Color.G, B: tx.Color.B, A: tx.Color.A}
		if c == (ui.Color{}) {
			c = t.Text
		}
		sp := ui.Span{Text: tx.Text, Size: tx.Size * scale, Color: c}
		if tx.Bold {
			sp.Weight = 700
		}
		labelX := x0 + tx.X*scale
		textWidth, _ := p.MeasureText(0, sp)
		switch tx.Anchor {
		case 1:
			labelX -= textWidth / 2
		case 2:
			labelX -= textWidth
		}
		p.RichText(labelX, y0+tx.Y*scale-sp.Size*0.8, 0, sp)
	}
}

func (d *sketchDiagram) shapeBitmap(w, h int, scale float32) *ui.Bitmap {
	key := w<<16 | h
	d.mu.Lock()
	defer d.mu.Unlock()
	if bmp, ok := d.drawn[key]; ok {
		return bmp
	}
	c, err := canvas.ParseSVG(strings.NewReader(d.result.Shapes))
	if c == nil {
		return nil
	}
	_ = err // a part the rasterizer cannot read is still omitted gracefully
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
