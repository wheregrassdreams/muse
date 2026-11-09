package nativeui

import (
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/shader"
)

// The header's fade: in the flat style the header floats over the document,
// and the page scrolls under it, dissolving into it rather than being cut
// off at its edge, as Craft's pages do.
//
// One strip draws it, in one pass. The strip reads the pixels the window has
// already painted under it — the document — blurred by the renderer with one
// radius, and fades that copy out over most of its height: under the header
// the document shows as a wash behind the page's color, and by the strip's
// foot it is the document, sharp, again.
//
// One radius, not a radius that widens towards the header. A blur that gets
// stronger upwards is closer to what a lens does, but a strip of text
// crosses from "letters" to "a smudge" all at once, at the height where the
// radius passes a glyph's stroke — and that crossing runs across the page as
// a seam, the very edge this fade is for. One radius never crosses it: the
// whole strip is about equally out of focus, and only its strength falls.
//
// Nothing paints twice, and the window is read once. The strip's backdrop is
// its own rectangle of the window, which the renderer copies into a texture
// an eighth the size and blurs along rows and along columns — a few pixels
// tall — and the shader then reads one pixel of it for each of the strip's.
// A keystroke repaints the strip with the text it shows, as it repaints any
// other pixel of the document.

// headerFadeReach is how far the fade reaches below the header, in DIPs.
const headerFadeReach = 44

// headerFadeBlur is the standard deviation of the strip's blur, in DIPs:
// enough that text under the header reads as light rather than as letters.
const headerFadeBlur = 12

// headerFadeVeil is how much of the page's color the header is: the rest
// is the document, blurred, showing through as a wash.
const headerFadeVeil = 0.84

// headerFadeRamp is how far the document scrolls before the header's fade
// is as strong as it gets, in DIPs.
const headerFadeRamp = 48

// headerFadeShader blurs the strip's backdrop, veils it with the page's
// color and opens it up towards the strip's foot: Band is the strip's
// height in DIPs, Solid the header's, Tint the page's color, Hold how much
// of it the header is and Cover how much of the fade a scroll has brought
// in.
//
// The veil is the shader's rather than a gradient over the strip: a
// gradient holds a curve in straight steps, and the step where it leaves
// the header shows as a faint line across the page — a Mach band — the very
// edge this fade is for.
var headerFadeShader = shader.MustCompile(`package main

var Band float
var Solid float
var Tint vec3
var Hold float
var Cover float

func Fragment(dst vec4, pos vec2) vec4 {
	t := clamp((pos.y-Solid)/max(Band-Solid, 1.0), 0.0, 1.0)
	k := (1.0 - smoothstep(0.22, 1.0, t)) * Cover
	c := mix(backdropSample(pos), vec4(Tint, 1.0), Hold*(1.0-smoothstep(0.0, 1.0, t)))
	return vec4(c.rgb*k, c.a*k)
}
`)

var (
	headerFadeBandU  = headerFadeShader.Uniform("Band")
	headerFadeSolidU = headerFadeShader.Uniform("Solid")
	headerFadeTintU  = headerFadeShader.Uniform("Tint")
	headerFadeHoldU  = headerFadeShader.Uniform("Hold")
	headerFadeCoverU = headerFadeShader.Uniform("Cover")
)

// headerVeil builds the strip the header floats on, in the flat style. It
// is pass-through: clicks reach the document under it.
func (a *App) headerVeil(c *ui.Context) {
	h := a.headerH(c)
	if !a.headerFade {
		// Off: the header keeps its own solid color, as a window's title
		// bar does, and the document starts sharp under it.
		ui.Box(c).Absolute().Top(0).Left(0).Right(0).Height(h).PassThrough().
			Background(a.windowBG())
		return
	}
	ui.Box(c).Absolute().Top(0).Left(0).Right(0).Height(h + headerFadeReach).PassThrough().
		Draw(func(p *ui.Painter, r ui.Rect) { a.paintHeaderVeil(p, r, h) })
}

// paintHeaderVeil paints the fade over r, a strip h DIPs of header and
// headerFadeReach of fade.
func (a *App) paintHeaderVeil(p *ui.Painter, r ui.Rect, h float32) {
	band := r.H
	tint := a.windowBG()
	// The fade is a document scrolled: with the first line at the top, the
	// strip is the page's own color and nothing of the document is under
	// the header, so the veil ramps in over the first DIPs of a scroll, and
	// the document's first line never sits veiled at rest.
	cover := min(max(a.ed.ScrollY()/headerFadeRamp, 0), 1)
	// With the document at its top, the page shows under the header and
	// none of the document does: nothing to draw, and nothing to pay for.
	if cover <= 0 {
		return
	}
	if !p.Preferences().ReduceTransparency {
		if a.fadeValues == nil {
			a.fadeValues = headerFadeShader.NewValues()
		}
		a.fadeValues.
			Float(headerFadeBandU, band).
			Float(headerFadeSolidU, h).
			Vec3(headerFadeTintU, float32(tint.R)/255, float32(tint.G)/255, float32(tint.B)/255).
			Float(headerFadeHoldU, headerFadeVeil*cover).
			Float(headerFadeCoverU, cover)
		headerFadeShader.Draw(p, r, shader.Options{Values: a.fadeValues, BackdropBlur: headerFadeBlur})
		return
	}
	// Opaque surfaces: the header's own color, opened up where the document
	// comes through, and nothing of the document showing through the header.
	p.FillGradient(r, ui.LinearGradient{Angle: 180, Stops: veilStops(tint, cover, 1, h, band)}, 0)
}

// veilStops places the veil's color along the strip: strength opaque where
// the header is, nothing where the fade ends, easing out as smoothstep
// does. The gradient's stops hold the curve in a few steps; it is the
// fallback for opaque surfaces, which do not read the backdrop at all.
func veilStops(tint ui.Color, cover, strength, solid, band float32) []ui.GradientStop {
	const n = 12
	stops := make([]ui.GradientStop, 0, n+1)
	for i := 0; i <= n; i++ {
		t := float32(i) / n
		e := 1 - t*t*(3-2*t)
		stops = append(stops, ui.GradientStop{
			Color: tint.Alpha(e * strength * cover),
			At:    min((solid+(band-solid)*t)/band, 1),
		})
	}
	return stops
}
