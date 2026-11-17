package editor

import (
	"math"
	"os"
	"testing"

	"github.com/egoist/mygo/ui"
	"muse/locales"
)

func TestMissingImageMessageUsesLocalizedSourceLabel(t *testing.T) {
	for _, tc := range []struct {
		source string
		want   string
	}{
		{"", locales.DocumentMissingImage()},
		{"  data:image/png;base64,abc  ", locales.DocumentMissingImage()},
		{"folder\\image.png", locales.DocumentMissingImageNamed("folder/image.png")},
		{" folder/image.png ", locales.DocumentMissingImageNamed("folder/image.png")},
	} {
		if got := missingImageMessage(tc.source); got != tc.want {
			t.Errorf("missingImageMessage(%q) = %q, want %q", tc.source, got, tc.want)
		}
	}
}

func TestMissingImageFallbackIsFullWidthAndClickable(t *testing.T) {
	dir := t.TempDir()
	const src = "前 ![内联示例](inline-missing.png) 后。\n\n![独占一段的图片](block-missing.png)\n\n<img src=\"html-missing.png\" alt=\"HTML 图片\">"
	h := newHarness(t, src)
	opts := h.e.Options()
	opts.BaseDir = dir
	var previewed string
	opts.OnImagePreview = func(path string) { previewed = path }
	h.e.SetOptions(opts)
	h.caretAt(len(src))
	h.frame()

	bySource := map[string]imageHit{}
	for _, hit := range h.e.imageHits {
		if hit.obj == nil || hit.obj.bmp != nil {
			continue
		}
		bySource[hit.obj.imageDest] = hit
		if hit.leaf != nil && math.Abs(float64(hit.box.W-hit.leaf.w)) > 1.5 {
			t.Errorf("missing image %q occupies width %.1f, want paragraph width %.1f", hit.obj.imageDest, hit.box.W, hit.leaf.w)
		}
	}
	for _, source := range []string{"inline-missing.png", "block-missing.png", "html-missing.png"} {
		if _, ok := bySource[source]; !ok {
			t.Errorf("missing-image object for %q not rendered; got %v", source, imageSources(bySource))
		}
	}
	inline, ok := bySource["inline-missing.png"]
	if !ok {
		t.Fatal("inline missing image has no hit region")
	}
	// A click in the body of the fallback, away from its floating controls,
	// opens the same image preview as the explicit preview control.
	h.e.input(ui.InputEvent{
		Kind: ui.InputPointerDown, Button: 0,
		X: inline.box.X + inline.box.W/2,
		Y: inline.box.Y + inline.box.H/2,
	})
	if previewed != inline.obj.imagePath {
		t.Fatalf("fallback click opened %q, want %q", previewed, inline.obj.imagePath)
	}
	if dir := os.Getenv("MDEDIT_SHOTS"); dir != "" {
		h.tt.Move(0, 0)
		h.frame()
		h.shot("missing-images-editor")
	}
}

func imageSources(hits map[string]imageHit) []string {
	var out []string
	for source := range hits {
		out = append(out, source)
	}
	return out
}
