package windowcontrols

import (
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/shader"
)

// FadeCache holds reusable shader values for one window's header fade.
// Callers retain only this opaque cache; the component owns its contents.
type FadeCache struct {
	blurValues *shader.Values
	veilValues *shader.Values
}

// FadeReach is how far the fade reaches below the header, in DIPs.
const FadeReach = 20

const (
	headerFadeVeil  = 0.2
	headerFadeRamp  = 48
	headerFadeSolid = 0.5
	headerFadeDecay = 4
)

const (
	fadeVeilHold = 0.0
	fadeVeilGone = 0.85
)

var fadeLayers = []struct{ blur, hold, end float32 }{
	{8, 0.10, 0.50},
	{3, 0.35, 0.78},
	{1, 0.65, 1},
}

var headerBlurShader = shader.MustCompile(`package main

var Band float
var Solid float
var Hold float
var End float
var Cover float
var Decay float

func Fragment(dst vec4, pos vec2) vec4 {
	t := clamp((pos.y-Solid)/max(Band-Solid, 1.0), 0.0, 1.0)
	x := clamp((t-Hold)/max(End-Hold, 0.001), 0.0, 1.0)
	k := (exp(-Decay*x) - exp(-Decay)) / (1.0 - exp(-Decay)) * Cover
	c := backdropSample(pos)
	return vec4(c.rgb*k, c.a*k)
}
`)

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
	blurDecayU = headerBlurShader.Uniform("Decay")
	veilBandU  = headerVeilShader.Uniform("Band")
	veilSolidU = headerVeilShader.Uniform("Solid")
	veilTintU  = headerVeilShader.Uniform("Tint")
	veilHoldU  = headerVeilShader.Uniform("Hold")
	veilFromU  = headerVeilShader.Uniform("From")
	veilGoneU  = headerVeilShader.Uniform("Gone")
	veilCoverU = headerVeilShader.Uniform("Cover")
)

// RenderHeaderFade draws a pass-through header strip and its scroll fade.
// The renderer depends only on UI primitives and values supplied by its host.
func RenderHeaderFade(c *ui.Context, cache *FadeCache, enabled bool, headerHeight, scrollY float32, tint ui.Color) {
	if !enabled {
		ui.Box(c).Absolute().Top(0).Left(0).Right(0).Height(headerHeight).PassThrough().
			Background(tint)
		return
	}
	ui.Box(c).Absolute().Top(0).Left(0).Right(0).Height(headerHeight + FadeReach).PassThrough().
		Draw(func(p *ui.Painter, r ui.Rect) {
			paintHeaderFade(p, cache, r, headerHeight, scrollY, tint)
		})
}

func paintHeaderFade(p *ui.Painter, cache *FadeCache, r ui.Rect, headerHeight, scrollY float32, tint ui.Color) {
	cover := min(max(scrollY/headerFadeRamp, 0), 1)
	if cover <= 0 {
		return
	}
	band := r.H
	if p.Preferences().ReduceTransparency {
		p.FillGradient(r, ui.LinearGradient{Angle: 180, Stops: veilStops(tint, cover, 1, headerHeight, band)}, 0)
		return
	}
	if cache == nil {
		cache = &FadeCache{}
	}
	if cache.blurValues == nil {
		cache.blurValues, cache.veilValues = headerBlurShader.NewValues(), headerVeilShader.NewValues()
	}
	for _, layer := range fadeLayers {
		cache.blurValues.
			Float(blurBandU, band).
			Float(blurSolidU, headerHeight*headerFadeSolid).
			Float(blurHoldU, layer.hold).
			Float(blurEndU, layer.end).
			Float(blurCoverU, cover).
			Float(blurDecayU, headerFadeDecay)
		headerBlurShader.Draw(p, r, shader.Options{Values: cache.blurValues, BackdropBlur: layer.blur})
	}
	if headerFadeVeil <= 0 {
		return
	}
	cache.veilValues.
		Float(veilBandU, band).
		Float(veilSolidU, headerHeight*headerFadeSolid).
		Vec3(veilTintU, float32(tint.R)/255, float32(tint.G)/255, float32(tint.B)/255).
		Float(veilHoldU, headerFadeVeil).
		Float(veilFromU, fadeVeilHold).
		Float(veilGoneU, fadeVeilGone).
		Float(veilCoverU, cover)
	headerVeilShader.Draw(p, r, shader.Options{Values: cache.veilValues})
}

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
