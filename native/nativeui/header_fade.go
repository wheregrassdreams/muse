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
// already painted under it — the document, sharp — and blurs them itself,
// with a radius that widens towards the header: none where the strip ends,
// headerFadeRadius DIPs at its top. The blur is a disc of taps the shader
// samples, so the radius changes from pixel to pixel, which one blur of the
// whole strip could not do: the document blurs *gradually* into the header,
// instead of fading out or dissolving into a blur of a single strength. The
// page's color veils the top, where the header's controls are.
//
// Nothing paints twice, and the window is read once. A strip's backdrop is
// its own rectangle of the window, which the renderer copies into a texture
// for the shader; the taps read that texture, a handful of samples a pixel,
// over the strip's pixels only. A keystroke repaints the strip with the
// text it shows, as it repaints any other pixel of the document.

// headerFadeReach is how far the fade reaches below the header, in DIPs.
const headerFadeReach = 60

// headerFadeBlur is the blur's radius at the strip's top, in DIPs: how far
// the document's pixels spread under the header.
const headerFadeBlur = 20

// headerFadeVeil is how much of the page's color the top of the strip is,
// and headerFadeSolid the share of the header's height it stays that way:
// the rest is the document, blurred, showing through as a wash. The veil
// reaches a little below the header; the blur goes further.
const (
	headerFadeVeil  = 0.86
	headerFadeSolid = 0.9
)

// headerFadeRamp is how far the document scrolls before the header's fade
// is as strong as it gets, in DIPs.
const headerFadeRamp = 48

// headerFadeBase is the blur the renderer makes of the backdrop before the
// shader's taps widen it, in DIPs: the taps would show as grain over the
// sharp pixels, and a smooth base lets a dozen of them read as one blur.
const headerFadeBase = 8

// headerFadeOpen is where the strip opens up again at its foot, in DIPs
// from its top: the taps have shrunk to nothing there, and what is left of
// the base blur fades out rather than ending on a line.
const headerFadeOpen = 22

// headerFadeShader blurs the strip's backdrop with a radius that widens
// towards its top, and opens up again at its foot: Band is the strip's
// height in DIPs, Radius the blur's radius at its top and Open where the
// strip starts to fade out.
var headerFadeShader = shader.MustCompile(`package main

var Band float
var Radius float
var Open float

func Fragment(dst vec4, pos vec2) vec4 {
	t := clamp(pos.y/Band, 0.0, 1.0)
	d := 1.0 - t
	r := Radius * d * sqrt(d)
	c := backdropSample(pos)
	for i := 0; i < 12; i++ {
		a := float(i) * 2.39996
		q := sqrt((float(i)+1.0)/12.0) * r
		c += backdropSample(pos + vec2(cos(a), sin(a))*q)
	}
	c /= 13.0
	a := 1.0 - smoothstep(Open, Band, pos.y)
	return vec4(c.rgb*a, c.a*a)
}
`)

var (
	headerFadeBandU   = headerFadeShader.Uniform("Band")
	headerFadeRadiusU = headerFadeShader.Uniform("Radius")
	headerFadeOpenU   = headerFadeShader.Uniform("Open")
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
	strength := float32(headerFadeVeil)
	if p.Preferences().ReduceTransparency {
		// Opaque surfaces: the veil is solid where the header is, with
		// nothing of the document showing through it.
		strength = 1
	} else {
		if a.fadeValues == nil {
			a.fadeValues = headerFadeShader.NewValues()
		}
		a.fadeValues.
			Float(headerFadeBandU, band).
			Float(headerFadeRadiusU, headerFadeBlur*cover).
			Float(headerFadeOpenU, band-headerFadeOpen)
		headerFadeShader.Draw(p, r, shader.Options{Values: a.fadeValues, BackdropBlur: headerFadeBase})
	}
	// The veil: the page's color, solid where the header's controls are,
	// easing out where the document comes through.
	p.FillGradient(r, ui.LinearGradient{Angle: 180, Stops: veilStops(tint, cover, strength, min(h*headerFadeSolid, band), band)}, 0)
}

// veilStops places the veil's color along the strip: strength opaque where
// the header is, nothing where the fade ends, easing out as smoothstep
// does. The gradient's stops hold the curve in a few steps.
func veilStops(tint ui.Color, cover, strength, solid, band float32) []ui.GradientStop {
	const n = 6
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
