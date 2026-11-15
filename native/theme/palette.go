// Package theme defines shared visual presets and runtime semantic colors.
package theme

import (
	"math"
	"strings"

	"github.com/egoist/mygo/ui"
)

// Preset is a named accent color that can be selected in application settings.
type Preset struct {
	ID    string
	Name  string
	Color ui.Color
}

// Presets are the built-in accent colors. Red is the default.
var Presets = []Preset{
	{ID: "red", Name: "红色", Color: ui.Hex("#dc4b50")},
	{ID: "orange", Name: "橙色", Color: ui.Hex("#df762f")},
	{ID: "yellow", Name: "黄色", Color: ui.Hex("#c99a18")},
	{ID: "green", Name: "绿色", Color: ui.Hex("#2e9b68")},
	{ID: "blue", Name: "蓝色", Color: ui.Hex("#3979d1")},
	{ID: "purple", Name: "紫色", Color: ui.Hex("#8960c8")},
	{ID: "pink", Name: "粉色", Color: ui.Hex("#d94f8b")},
}

// Normalize returns a known preset ID, using red for missing or unknown IDs.
func Normalize(id string) string {
	id = strings.ToLower(strings.TrimSpace(id))
	for _, preset := range Presets {
		if id == preset.ID {
			return id
		}
	}
	return Presets[0].ID
}

// Color returns a preset color, using red for missing or unknown IDs.
func Color(id string) ui.Color {
	id = Normalize(id)
	for _, preset := range Presets {
		if preset.ID == id {
			return preset.Color
		}
	}
	return Presets[0].Color
}

// Palette contains semantic colors derived from one accent for a theme.
type Palette struct {
	Accent, Hover, Pressed, OnAccent, Link, Selection, Soft, Ink ui.Color
}

// From derives an accent palette. The dark palette is tuned for a #202020
// surface; the light palette is tuned for white. Accent-derived text colors
// are adjusted as needed to retain readable contrast on those surfaces.
func From(accent ui.Color, dark bool) Palette {
	if accent == (ui.Color{}) {
		accent = Color("red")
	}
	h, s, l := rgbToHSL(accent)
	// Clamp rather than repeatedly scaling. Feeding a derived Accent back into
	// From therefore stays in the same color band instead of drifting each pass.
	s = clamp(s, 0.65, 0.78)
	if dark {
		l = clamp(l, 0.62, 0.70)
	} else {
		l = clamp(l, 0.49, 0.58)
	}
	base := hslColor(h, s, l)
	onAccent := ui.Hex("#ffffff")
	if contrast(onAccent, base) < 4.5 {
		onAccent = ui.Hex("#202020")
		if contrast(onAccent, base) < 4.5 {
			onAccent = ui.Hex("#000000")
		}
	}

	linkSaturation := clamp(s, 0.65, 0.78)
	inkSaturation := clamp(s*0.62, 0.35, 0.52)
	linkLightness := contrastedLightness(h, linkSaturation, 0.52, ui.Hex("#ffffff"), false)
	inkLightness := contrastedLightness(h, inkSaturation, 0.48, ui.Hex("#ffffff"), false)
	soft := hslColor(h, clamp(s*0.58, 0.34, 0.56), 0.91)
	selection := hslColor(h, clamp(s*0.68, 0.40, 0.64), 0.82)
	if dark {
		linkLightness = contrastedLightness(h, linkSaturation, 0.82, ui.Hex("#202020"), true)
		inkLightness = contrastedLightness(h, inkSaturation, 0.78, ui.Hex("#202020"), true)
		soft = hslColor(h, clamp(s*0.54, 0.32, 0.52), 0.32)
		selection = hslColor(h, clamp(s*0.64, 0.38, 0.62), 0.40)
	}
	return Palette{
		Accent:   base,
		Hover:    hslColor(h, s, clamp(l+map[bool]float64{true: 0.055, false: -0.055}[dark], 0.30, 0.78)),
		Pressed:  hslColor(h, s, clamp(l+map[bool]float64{true: 0.10, false: -0.10}[dark], 0.28, 0.82)),
		OnAccent: onAccent,
		Link:     hslColor(h, linkSaturation, linkLightness),
		Selection: ui.RGBA(selection.R, selection.G, selection.B,
			map[bool]float32{true: 0.50, false: 0.34}[dark]),
		Soft: ui.RGBA(soft.R, soft.G, soft.B, map[bool]float32{true: 0.28, false: 0.22}[dark]),
		Ink:  hslColor(h, inkSaturation, inkLightness),
	}
}

func contrastedLightness(h, s, start float64, bg ui.Color, lighter bool) float64 {
	step := 0.005
	if !lighter {
		step = -step
	}
	for l := start; l >= 0 && l <= 1; l += step {
		if contrast(hslColor(h, s, l), bg) >= 4.5 {
			return l
		}
	}
	if lighter {
		return 1
	}
	return 0
}

func contrast(a, b ui.Color) float64 {
	la, lb := luminance(a), luminance(b)
	if la < lb {
		la, lb = lb, la
	}
	return (la + 0.05) / (lb + 0.05)
}

func luminance(c ui.Color) float64 {
	channel := func(v uint8) float64 {
		x := float64(v) / 255
		if x <= 0.04045 {
			return x / 12.92
		}
		return pow((x+0.055)/1.055, 2.4)
	}
	return 0.2126*channel(c.R) + 0.7152*channel(c.G) + 0.0722*channel(c.B)
}

func rgbToHSL(c ui.Color) (h, s, l float64) {
	r, g, b := float64(c.R)/255, float64(c.G)/255, float64(c.B)/255
	maxc, minc := max3(r, g, b), min3(r, g, b)
	l = (maxc + minc) / 2
	d := maxc - minc
	if d == 0 {
		return 0, 0, l
	}
	if l > 0.5 {
		s = d / (2 - maxc - minc)
	} else {
		s = d / (maxc + minc)
	}
	switch maxc {
	case r:
		h = (g - b) / d
		if g < b {
			h += 6
		}
	case g:
		h = (b-r)/d + 2
	default:
		h = (r-g)/d + 4
	}
	return h / 6, s, l
}

func hslColor(h, s, l float64) ui.Color {
	var r, g, b float64
	if s == 0 {
		r, g, b = l, l, l
	} else {
		q := l * (1 + s)
		if l >= 0.5 {
			q = l + s - l*s
		}
		p := 2*l - q
		r = hueChannel(p, q, h+1.0/3)
		g = hueChannel(p, q, h)
		b = hueChannel(p, q, h-1.0/3)
	}
	return ui.RGB(byte(clamp(r*255+0.5, 0, 255)), byte(clamp(g*255+0.5, 0, 255)), byte(clamp(b*255+0.5, 0, 255)))
}

func hueChannel(p, q, t float64) float64 {
	if t < 0 {
		t++
	} else if t > 1 {
		t--
	}
	switch {
	case t < 1.0/6:
		return p + (q-p)*6*t
	case t < 1.0/2:
		return q
	case t < 2.0/3:
		return p + (q-p)*(2.0/3-t)*6
	default:
		return p
	}
}

func clamp(v, low, high float64) float64 { return min(high, max(low, v)) }
func max3(a, b, c float64) float64       { return max(a, max(b, c)) }
func min3(a, b, c float64) float64       { return min(a, min(b, c)) }

// pow is kept as a small wrapper so the color helpers read like the WCAG formula.
func pow(v, exponent float64) float64 { return math.Pow(v, exponent) }
