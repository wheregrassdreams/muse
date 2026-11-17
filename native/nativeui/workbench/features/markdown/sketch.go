package markdown

import (
	"strings"
	"sync"

	"github.com/egoist/mygo/ui"
	"github.com/tdewolff/canvas"
	"github.com/tdewolff/canvas/renderers/rasterizer"
	"muse/markdown/diagrams"
)

// ParseSketchDiagram asks the backend-neutral diagrams subsystem to parse
// Mermaid and adapts its shape/label result to the MyGo painter.
func ParseSketchDiagram(code string, opts diagrams.Options) (Diagram, bool) {
	result, ok := diagrams.Parse(code, opts)
	if !ok {
		return Diagram{}, false
	}
	return DiagramFromResult(result), true
}

// DiagramFromResult adapts an immutable, backend-neutral diagram projection
// into MyGo drawing primitives. It defensively copies label storage so a
// surface cannot retain mutable parser or editor-owned slices.
func DiagramFromResult(result diagrams.Result) Diagram {
	result.Labels = append([]diagrams.Label(nil), result.Labels...)
	d := &sketchDiagram{result: result, drawn: map[int]*ui.Bitmap{}}
	return Diagram{width: func() float32 { return result.Width }, height: func() float32 { return result.Height }, draw: d.draw}
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
