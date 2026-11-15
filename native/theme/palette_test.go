package theme

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestPresetCatalogAndFallback(t *testing.T) {
	if len(Presets) != 7 {
		t.Fatalf("got %d presets, want 7", len(Presets))
	}
	if Presets[0].ID != "red" || Presets[0].Color != ui.Hex("#dc4b50") {
		t.Fatalf("red default = %#v", Presets[0])
	}
	wantNames := []string{"红色", "橙色", "黄色", "绿色", "蓝色", "紫色", "粉色"}
	for i, want := range wantNames {
		if Presets[i].Name != want {
			t.Errorf("preset %q name = %q, want %q", Presets[i].ID, Presets[i].Name, want)
		}
	}
	seen := map[string]bool{}
	for _, p := range Presets {
		if p.ID == "" || p.Name == "" || p.Color.A != 255 || seen[p.ID] {
			t.Errorf("invalid or duplicate preset: %#v", p)
		}
		seen[p.ID] = true
		if Normalize(p.ID) != p.ID || Color(p.ID) != p.Color {
			t.Errorf("preset lookup failed for %q", p.ID)
		}
	}
	if got := Normalize(" BLUE "); got != "blue" {
		t.Errorf("Normalize(known) = %q", got)
	}
	if got := Normalize("not-a-preset"); got != "red" {
		t.Errorf("Normalize(invalid) = %q", got)
	}
	if got := Color(""); got != Presets[0].Color {
		t.Errorf("Color(empty) = %#v, want default red", got)
	}
}

func TestPaletteContrastAcrossPresetsAndThemes(t *testing.T) {
	white := ui.Hex("#ffffff")
	darkSurface := ui.Hex("#202020")
	for _, preset := range Presets {
		for _, dark := range []bool{false, true} {
			p := From(preset.Color, dark)
			bg := white
			if dark {
				bg = darkSurface
			}
			if ratio := contrast(p.Link, bg); ratio < 4.5 {
				t.Errorf("%s dark=%t Link contrast %.2f:1, want >= 4.5:1", preset.ID, dark, ratio)
			}
			_, linkSaturation, _ := rgbToHSL(p.Link)
			if !dark && (linkSaturation < 0.645 || linkSaturation > 0.785) {
				t.Errorf("%s light Link saturation %.2f, want 0.65–0.78", preset.ID, linkSaturation)
			}
			if ratio := contrast(p.OnAccent, p.Accent); ratio < 4.5 {
				t.Errorf("%s dark=%t OnAccent contrast %.2f:1, want >= 4.5:1", preset.ID, dark, ratio)
			}
			if p.Selection.A == 255 || p.Selection.A == 0 || p.Soft.A == 255 || p.Soft.A == 0 {
				t.Errorf("%s dark=%t should have translucent Selection and Soft: %#v", preset.ID, dark, p)
			}
			if p.Ink.A != 255 {
				t.Errorf("%s dark=%t inline code Ink should be opaque", preset.ID, dark)
			}
		}
	}
}

func TestPaletteUsesAccentHueAndThemeTuning(t *testing.T) {
	accent := Color("red")
	light := From(accent, false)
	dark := From(accent, true)
	if light.Accent == dark.Accent {
		t.Fatal("light and dark accents should use theme-specific saturation/lightness")
	}
	if got := From(light.Accent, false).Accent; got != light.Accent {
		t.Errorf("re-deriving light Accent drifted: got %#v, want %#v", got, light.Accent)
	}
	if got := From(dark.Accent, true).Accent; got != dark.Accent {
		t.Errorf("re-deriving dark Accent drifted: got %#v, want %#v", got, dark.Accent)
	}
	if light.Link == From(Color("blue"), false).Link {
		t.Fatal("derived link should follow the accent hue")
	}
	if light.Selection.A == dark.Selection.A || light.Soft.A == dark.Soft.A {
		t.Fatal("soft surfaces should use theme-specific alpha")
	}
	if got := From(ui.Color{}, false).Accent; got != light.Accent {
		t.Errorf("zero accent fallback = %#v, want red-derived %#v", got, light.Accent)
	}
}
