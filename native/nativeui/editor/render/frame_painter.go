package render

import (
	"github.com/egoist/mygo/ui"
)

// Paint draws a detached visible frame. It does not retain the frame, painter,
// resource values, or any editor-owned objects after returning.
func Paint(p *ui.Painter, frame Frame, resources Resources) {
	if p == nil {
		return
	}
	paint := func() {
		for _, overlay := range frame.Behind {
			PaintOverlay(p, overlay)
		}
		for _, block := range frame.Blocks {
			PaintBlock(p, frame.Style, block, resources)
		}
		for _, overlay := range frame.InFront {
			PaintOverlay(p, overlay)
		}
	}
	if frame.Viewport.W > 0 && frame.Viewport.H > 0 {
		p.Clip(frame.Viewport, 0, paint)
	} else {
		paint()
	}
}

// PaintOverlay synchronously paints one resolved visual primitive.
func PaintOverlay(p *ui.Painter, overlay Overlay) {
	if p == nil {
		return
	}
	draw := func() {
		switch overlay.Kind {
		case FillOverlay:
			p.Fill(overlay.Bounds, overlay.Color, overlay.Radius)
		case StrokeOverlay:
			p.Stroke(overlay.Bounds, overlay.Color, overlay.Radius, overlay.Width)
		case TextOverlay:
			p.RichText(overlay.Bounds.X, overlay.Bounds.Y, overlay.MaxW, overlay.Text)
		case PolylineOverlay:
			if len(overlay.Points) >= 2 {
				p.StrokePolyline(overlay.Points, overlay.Width, overlay.Color)
			}
		case LayoutOverlay:
			if overlay.Layout != nil {
				overlay.Layout.Draw(p, overlay.X, overlay.Y, overlay.Color)
			}
		}
	}
	if overlay.Clip.W > 0 && overlay.Clip.H > 0 {
		p.Clip(overlay.Clip, overlay.ClipRadius, draw)
	} else {
		draw()
	}
}

// PaintBlock synchronously paints one detached visible block without creating
// a frame slice. It is useful when a host is already iterating its cached
// viewport blocks and must keep per-frame allocations bounded.
func PaintBlock(p *ui.Painter, style Style, block Block, resources Resources) {
	if p == nil {
		return
	}
	draw := func() { paintBlock(p, style, block, resources) }
	if block.Clip.W > 0 && block.Clip.H > 0 {
		p.Clip(block.Clip, block.ClipRadius, draw)
	} else {
		draw()
	}
}

func paintBlock(p *ui.Painter, style Style, block Block, resources Resources) {
	if block.Bounds.W <= 0 || block.Bounds.H <= 0 {
		return
	}
	if block.Panel {
		p.Fill(block.Bounds, style.Panel, 6)
	}
	switch block.Kind {
	case CalloutBlock:
		paintCallout(p, style, block)
	case ImageBlock:
		paintImage(p, style, block, resources.Images)
	case MathBlock:
		paintMath(p, style, block, resources.Math)
	case DiagramBlock:
		paintDiagram(p, style, block, resources.Diagrams)
	case EmbedBlock:
		p.Fill(block.Bounds, style.Panel, 7)
		p.Stroke(block.Bounds, style.Border.Alpha(.7), 7, 1)
		pad := style.FontSize * .55
		inner := ui.Rect{X: block.Bounds.X + pad, Y: block.Bounds.Y + pad, W: max(block.Bounds.W-2*pad, 1), H: max(block.Bounds.H-2*pad, 1)}
		p.Clip(inner, 4, func() {
			if block.Label != "" {
				p.RichText(inner.X, inner.Y, inner.W, ui.Span{Text: block.Label, Size: style.FontSize * .82, Color: style.Muted})
				return
			}
			for _, child := range block.Children {
				child.Bounds.X += inner.X
				child.Bounds.Y += inner.Y
				child.TextX += inner.X
				child.TextY += inner.Y
				paintBlock(p, style, child, resources)
			}
		})
	default:
		if block.Layout != nil {
			block.Layout.Draw(p, block.TextX, block.TextY, style.Text)
		}
		for _, run := range block.Runs {
			sp := run.Style
			sp.Text = run.Text
			p.RichText(block.TextX+run.X, block.TextY+run.Y, max(block.Bounds.W-run.X, 1), sp)
		}
	}
}

func paintCallout(p *ui.Painter, style Style, block Block) {
	accent := calloutAccent(block.Alert, style.Accent)
	alpha := float32(.1)
	if style.Dark {
		alpha = .18
		switch block.Alert {
		case AlertWarning:
			accent = ui.RGB(240, 185, 75)
		case AlertCaution:
			accent = ui.RGB(255, 115, 120)
		case AlertTip:
			accent = ui.RGB(80, 205, 155)
		case AlertImportant:
			accent = ui.RGB(185, 145, 255)
		}
	}
	p.Fill(block.Bounds, accent.Alpha(alpha), 8)
	if block.Title == "" {
		return
	}
	size := max(style.FontSize, 12)
	paintCalloutIcon(p, block.Bounds.X+size*1.4, block.Bounds.Y+size*.5, size, accent, block.Alert == AlertWarning || block.Alert == AlertCaution)
	p.RichText(block.Bounds.X+size*2.8, block.Bounds.Y+size*.45, max(block.Bounds.W-size*4.2, 1), ui.Span{Text: block.Title, Color: accent, Size: size, Weight: 600})
}

func calloutAccent(alert AlertKind, accent ui.Color) ui.Color {
	switch alert {
	case AlertWarning:
		return ui.RGB(202, 135, 12)
	case AlertCaution:
		return ui.RGB(218, 65, 70)
	case AlertTip:
		return ui.RGB(36, 155, 113)
	case AlertImportant:
		return ui.RGB(139, 93, 211)
	default:
		return accent
	}
}

func paintCalloutIcon(p *ui.Painter, x, y, size float32, color ui.Color, warning bool) {
	line := func(a, b, c, d float32) {
		p.StrokePolyline([][2]float32{{x + a*size, y + b*size}, {x + c*size, y + d*size}}, 1.6, color)
	}
	if warning {
		line(.5, .08, .94, .88)
		line(.94, .88, .06, .88)
		line(.06, .88, .5, .08)
		line(.5, .36, .5, .59)
		line(.5, .72, .5, .74)
		return
	}
	line(.18, .68, .66, .2)
	line(.66, .2, .82, .36)
	line(.82, .36, .34, .84)
	line(.34, .84, .12, .9)
	line(.12, .9, .18, .68)
	line(.58, .28, .74, .44)
}

func paintImage(p *ui.Painter, style Style, block Block, images ImageResources) {
	if images != nil {
		if bitmap, ok := images.Image(block.Resource); ok && bitmap != nil {
			p.Image(bitmap, block.Bounds, ui.Contain)
			return
		}
	}
	p.Fill(block.Bounds, style.Panel, 6)
	label := block.Alt
	if label == "" {
		label = "Image unavailable"
	}
	p.Clip(block.Bounds, 6, func() {
		p.RichText(block.Bounds.X+style.FontSize*.8, block.Bounds.Y+style.FontSize*.8, max(block.Bounds.W-style.FontSize*1.6, 1), ui.Span{Text: label, Color: style.Muted, Size: style.FontSize * .8})
	})
}

func paintMath(p *ui.Painter, style Style, block Block, resources MathResources) {
	if resources == nil {
		return
	}
	if resources.DrawMath(p, block.Bounds, block.Resource, style.Dark, style.Font) {
		return
	}
	visual, ok := resources.Math(block.Resource, style.Dark, style.Font)
	if !ok || visual == nil || visual.Bitmap() == nil {
		return
	}
	bitmap := visual.Bitmap()
	bw, bh := bitmap.Size()
	if bw <= 0 || bh <= 0 || block.Bounds.W <= 0 || block.Bounds.H <= 0 {
		return
	}
	scale := min(block.Bounds.W/float32(bw), block.Bounds.H/float32(bh))
	if scale <= 0 {
		return
	}
	w, h := float32(bw)*scale, float32(bh)*scale
	x, y := block.Bounds.X+(block.Bounds.W-w)/2, block.Bounds.Y+(block.Bounds.H-h)/2
	natural := visual.NaturalScale()
	if natural <= 0 {
		return
	}
	extra := scale / natural
	p.Transform(ui.Scaling(extra, extra), x, y, func() {
		p.Image(bitmap, ui.Rect{X: x, Y: y, W: float32(bw) * natural, H: float32(bh) * natural}, ui.Contain)
		visual.DrawFallback(p, x, y, natural)
	})
}

func paintDiagram(p *ui.Painter, style Style, block Block, resources DiagramResources) {
	if resources != nil {
		if visual, ok := resources.Diagram(block.Resource); ok && visual != nil {
			visual.Draw(p, block.Bounds, p.Theme())
			return
		}
	}
	p.Fill(block.Bounds, style.Panel, 6)
	p.Stroke(block.Bounds, style.Border.Alpha(.7), 6, 1)
	p.RichText(block.Bounds.X+8, block.Bounds.Y+8, max(block.Bounds.W-16, 1), ui.Span{Text: "Diagram unavailable", Color: style.Muted, Size: style.FontSize * .8})
}
