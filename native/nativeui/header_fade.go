package nativeui

import (
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/shader"
)

// The header's fade: in the flat style the header floats over the document,
// and the page scrolls under it, dissolving into it rather than being cut
// off at its edge, as Craft's pages do.
//
// One strip draws it, in two blurs and one veil. The strip reads the pixels
// the window has already painted under it — the document — and lays two
// copies of them over each other: the fine one, blurred by two DIPs, which
// reaches to the strip's foot, and the coarse one, blurred by ten, which
// covers the header's own height and opens up a third of the way down. The
// coarse copy fades into the fine one, and the fine one, being all but the
// document itself, fades into the document at the foot without an edge. The
// page's color veils the top, where the header's controls are.
//
// Two blurs, not a radius that widens continuously, and not one blur fading
// out. A blur that widens with height is closer to what a lens does, but a
// line of text crosses from "letters" to "a smudge" all at once, at the
// height where the radius passes a glyph's stroke, and that crossing runs
// across the page as a seam. One blur fading out has to be seen at a
// fraction of its strength, which is the same letters, ghosted, over the
// document's own. Two steps are far enough apart to read as "out of focus"
// then "not", and the last of them is a blur small enough to be no blur.

// headerFadeReach is how far the fade reaches below the header, in DIPs.
const headerFadeReach = 22

// The fade's two blurs, in DIPs: the coarse one under the header, and the
// fine one that takes over from it and hands the document back. Two DIPs is
// about where a blur stops being one — the copy is nearly the document, so
// it can be laid over it and taken away again without an edge.
const (
	headerFadeBlur = 4
	headerFadeSeam = 2
)

// headerFadeVeil is how much of the page's color the header is. A little:
// enough to take the edge off the copy as it goes, not enough to paint the
// page over what is under the header. Invisible over the page, a light wash
// over anything else — over a callout's blue it leaves the blue, where a
// veil near 1 would leave a white band with a straight edge. The blur does
// the work; the veil only softens the hand-off.
const headerFadeVeil = 0.3

// headerFadeRamp is how far the document scrolls before the header's fade
// is as strong as it gets, in DIPs.
const headerFadeRamp = 48

// The fade's layout, as shares of headerFadeReach below the header, where
// 0 is the header's bottom and 1 the strip's:
//
//	0 ── 0.25 ─────── 0.70 ───── 0.80 ────── 1
//	coarse  ███████████▓▓▓▓▓▓▓▓▓▓░░░░░░░░░░░░
//	fine    ██████████████████████████████▓▓▓
//	veil    ███████████▓▓▓▓▓▓▓░░░░░░░░░░░░░░░
//
// The coarse copy holds for the header's own third and hands over to the
// fine one across the next forty-five hundredths — 4 DIPs to 2, twice apart,
// which is as close as two blurs may be and still tell apart. The veil then
// covers that hand-over and is gone by 0.75, before the fine copy goes: the
// last hand-over, fine to the document, is between two images that are all
// but the same, and a tint there would draw the eye to the line it is meant
// to hide. The fine copy holds a fifth of the way before it opens, so the eye
// sees a slightly soft image before it sees the document.
const (
	fadeCoarseHold = 0.25
	fadeCoarseEnd  = 0.70
	fadeSeamHold   = 0.80
	fadeVeilHold   = 0.25
	fadeVeilGone   = 0.75
)

// fadeLayers are the fade's copies: each covers the strip from its top
// until Hold, as a share of the strip's fade, and opens up by End.
var fadeLayers = []struct{ blur, hold, end float32 }{
	{headerFadeBlur, fadeCoarseHold, fadeCoarseEnd},
	{headerFadeSeam, fadeSeamHold, 1},
}

// headerBlurShader lays one blurred copy of the strip's backdrop over it,
// and takes it away towards the strip's foot: Solid is the header's height
// in DIPs, Band the strip's, Hold where the copy starts to open and End
// where it is gone, Cover how much of the fade a scroll has brought in.
var headerBlurShader = shader.MustCompile(`package main

var Band float
var Solid float
var Hold float
var End float
var Cover float

func Fragment(dst vec4, pos vec2) vec4 {
	t := clamp((pos.y-Solid)/max(Band-Solid, 1.0), 0.0, 1.0)
	k := (1.0 - smoothstep(Hold, End, t)) * Cover
	c := backdropSample(pos)
	return vec4(c.rgb*k, c.a*k)
}
`)

// headerVeilShader lays the page's color over the strip, opaque where the
// header is and open where the strip's fade ends. The veil is a shader
// rather than a gradient over the strip: a gradient holds a curve in
// straight steps, and the step where it leaves the header shows as a faint
// line across the page — a Mach band — the very edge this fade is for. It
// reads no backdrop, so it costs a fill and no more.
var headerVeilShader = shader.MustCompile(`package main

var Band float
var Solid float
var Tint vec3
var Hold float
var From float
var Gone float
var Cover float

func Fragment(dst vec4, pos vec2) vec4 {
	t := clamp((pos.y-Solid)/max(Band-Solid, 1.0), 0.0, 1.0)
	a := Hold * (1.0 - smoothstep(From, Gone, t)) * Cover
	return vec4(Tint*a, a)
}
`)

var (
	blurBandU  = headerBlurShader.Uniform("Band")
	blurSolidU = headerBlurShader.Uniform("Solid")
	blurHoldU  = headerBlurShader.Uniform("Hold")
	blurEndU   = headerBlurShader.Uniform("End")
	blurCoverU = headerBlurShader.Uniform("Cover")
	veilBandU  = headerVeilShader.Uniform("Band")
	veilSolidU = headerVeilShader.Uniform("Solid")
	veilTintU  = headerVeilShader.Uniform("Tint")
	veilHoldU  = headerVeilShader.Uniform("Hold")
	veilFromU  = headerVeilShader.Uniform("From")
	veilGoneU  = headerVeilShader.Uniform("Gone")
	veilCoverU = headerVeilShader.Uniform("Cover")
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
	if p.Preferences().ReduceTransparency {
		// Opaque surfaces: the header's own color, opened up where the
		// document comes through, and nothing of it showing through the
		// header.
		p.FillGradient(r, ui.LinearGradient{Angle: 180, Stops: veilStops(tint, cover, 1, h, band)}, 0)
		return
	}
	if a.blurValues == nil {
		a.blurValues, a.veilValues = headerBlurShader.NewValues(), headerVeilShader.NewValues()
	}
	for _, l := range fadeLayers {
		a.blurValues.
			Float(blurBandU, band).
			Float(blurSolidU, h).
			Float(blurHoldU, l.hold).
			Float(blurEndU, l.end).
			Float(blurCoverU, cover)
		headerBlurShader.Draw(p, r, shader.Options{Values: a.blurValues, BackdropBlur: l.blur})
	}
	if headerFadeVeil <= 0 {
		return
	}
	a.veilValues.
		Float(veilBandU, band).
		Float(veilSolidU, h).
		Vec3(veilTintU, float32(tint.R)/255, float32(tint.G)/255, float32(tint.B)/255).
		Float(veilHoldU, headerFadeVeil).
		Float(veilFromU, fadeVeilHold).
		Float(veilGoneU, fadeVeilGone).
		Float(veilCoverU, cover)
	headerVeilShader.Draw(p, r, shader.Options{Values: a.veilValues})
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
