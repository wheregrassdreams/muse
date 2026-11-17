package editor

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
	media "muse/nativeui/workbench/features/editor/blocks/media"
)

func TestImageResizeReferenceMarkdownIsUndoableAndDoesNotWriteImage(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sample file.png")
	var encoded bytes.Buffer
	img := image.NewRGBA(image.Rect(0, 0, 120, 60))
	img.Set(0, 0, color.RGBA{R: 12, G: 34, B: 56, A: 255})
	if err := png.Encode(&encoded, img); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, encoded.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	src := "![nested [alt]][pic]\n\n[pic]: sample%20file.png \"a title\"\n"
	h := newHarness(t, src)
	opts := h.e.Options()
	opts.BaseDir = dir
	h.e.SetOptions(opts)
	h.frame()
	var obj *object
	for _, l := range h.e.leaves {
		if l.view == nil {
			continue
		}
		for _, pc := range l.view.disp.pieces {
			if pc.obj != nil && pc.obj.imageSource.Syntax == media.ImageSyntaxMarkdown {
				obj = pc.obj
				break
			}
		}
	}
	if obj == nil {
		t.Fatalf("reference image was not parsed as an image: %#v", h.e.leaves)
	}
	if obj.imageDest != "sample%20file.png" || obj.imageTitle != "a title" || !strings.Contains(obj.alt, "alt") {
		t.Fatalf("parsed image metadata: dest=%q alt=%q title=%q", obj.imageDest, obj.alt, obj.imageTitle)
	}
	if obj.imagePath != path {
		t.Fatalf("resolved preview path = %q, want %q", obj.imagePath, path)
	}

	hit := imageHit{obj: obj}
	h.e.resizeImageSource(hit, 180)
	if !strings.Contains(h.e.Source(), `![nested [alt]|180][pic]`) || !strings.Contains(h.e.Source(), `[pic]: sample%20file.png "a title"`) {
		t.Fatalf("resized source did not preserve Markdown reference and title: %q", h.e.Source())
	}
	if !h.e.Undo() || h.e.Source() != src {
		t.Fatalf("undo did not restore original image reference: %q", h.e.Source())
	}
	if got, err := os.ReadFile(path); err != nil || !bytes.Equal(got, before) {
		t.Fatalf("resize changed image bytes: err=%v", err)
	}
}

func TestWikiImageResizeUsesWidthPipeAndUndo(t *testing.T) {
	const src = "![[assets/foo.png]]"
	e := New(src, Options{FontSize: 16})
	e.lk.size = 16
	e.lk.lh = 1.7
	e.layoutIfNeeded()
	var obj *object
	for _, l := range e.leaves {
		if l.view != nil {
			for _, pc := range l.view.disp.pieces {
				if pc.obj != nil && pc.obj.imageSource.Syntax == media.ImageSyntaxWiki {
					obj = pc.obj
				}
			}
		}
	}
	if obj == nil {
		t.Fatal("wiki image has no drawable object")
	}
	e.resizeImageSource(imageHit{obj: obj}, 240)
	if e.Source() != "![[assets/foo.png|240]]" {
		t.Fatalf("resized wiki source %q", e.Source())
	}
	if !e.Undo() || e.Source() != src {
		t.Fatalf("undo left wiki width: %q", e.Source())
	}
}

func TestImageHoverDragCancelCommitAndUndo(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "live.png")
	img := image.NewRGBA(image.Rect(0, 0, 160, 80))
	for y := 0; y < 80; y++ {
		for x := 0; x < 160; x++ {
			img.Set(x, y, color.RGBA{R: uint8(40 + x), G: uint8(80 + y), B: 200, A: 255})
		}
	}
	var data bytes.Buffer
	if err := png.Encode(&data, img); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	originalBytes, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	src := `![live](live.png)`
	h := newHarness(t, src)
	opts := h.e.Options()
	opts.BaseDir = dir
	h.e.SetOptions(opts)
	h.frame()
	if len(h.e.imageHits) == 0 {
		t.Fatal("paint did not publish the image hit region")
	}
	base := append([]byte(nil), h.tt.Image().Pix...)
	hit := h.e.imageHits[0]
	h.tt.Move(hit.box.X+hit.box.W/2, hit.box.Y+hit.box.H/2)
	h.frame()
	if bytes.Equal(base, h.tt.Image().Pix) {
		t.Fatal("hover did not draw image preview/source controls")
	}
	if capture := os.Getenv("MUSE_IMAGE_HOVER_CAPTURE"); capture != "" {
		f, err := os.Create(capture)
		if err != nil {
			t.Fatal(err)
		}
		if err := png.Encode(f, h.tt.Image()); err != nil {
			f.Close()
			t.Fatal(err)
		}
		if err := f.Close(); err != nil {
			t.Fatal(err)
		}
	}
	hit = h.e.imageHits[0]
	_, _, _, grip := media.ToolRects(hit.box, h.e.lk.size)
	x, y := grip.X+grip.W/2, grip.Y+grip.H/2
	startWidth := hit.obj.w
	h.e.input(ui.InputEvent{Kind: ui.InputPointerDown, Button: 0, X: x, Y: y})
	h.e.input(ui.InputEvent{Kind: ui.InputPointerMove, Button: 0, X: x + 40, Y: y + 20})
	h.frame()
	if h.e.Source() != src {
		t.Fatalf("source changed before pointer release: %q", h.e.Source())
	}
	if len(h.e.imageHits) == 0 || h.e.imageHits[0].obj.w <= startWidth {
		t.Fatalf("drag did not resize the live layout: start %.1f hits %+v", startWidth, h.e.imageHits)
	}
	h.e.input(ui.InputEvent{Kind: ui.InputKeyDown, Key: ui.KeyEscape})
	h.frame()
	if h.e.Source() != src || h.e.imageHits[0].obj.w != startWidth {
		t.Fatal("cancel did not restore the original image view")
	}
	hit = h.e.imageHits[0]
	_, _, _, grip = media.ToolRects(hit.box, h.e.lk.size)
	x, y = grip.X+grip.W/2, grip.Y+grip.H/2
	h.e.input(ui.InputEvent{Kind: ui.InputPointerDown, Button: 0, X: x, Y: y})
	h.e.input(ui.InputEvent{Kind: ui.InputPointerMove, Button: 0, X: x + 40, Y: y + 20})
	h.frame()
	wantWidth := h.e.imageDrag.gesture.PreviewWidth()
	h.e.input(ui.InputEvent{Kind: ui.InputPointerUp, Button: 0, X: x + 40, Y: y + 20})
	h.frame()
	if !strings.Contains(h.e.Source(), "|"+strconv.Itoa(int(wantWidth))+"](live.png)") {
		t.Fatalf("release did not persist Markdown width: %q", h.e.Source())
	}
	if !h.e.Undo() || h.e.Source() != src {
		t.Fatalf("resize was not one undo step: %q", h.e.Source())
	}
	if got, err := os.ReadFile(path); err != nil || !bytes.Equal(got, originalBytes) {
		t.Fatalf("drag resize changed source image bytes: err=%v", err)
	}
}

func TestImageResizePreservesImplicitReferenceLookup(t *testing.T) {
	for _, embed := range []string{"![photo][]", "![photo]"} {
		src := embed + "\n\n[photo]: absent.png \"caption\"\n"
		h := newHarness(t, src)
		if len(h.e.imageHits) == 0 {
			t.Fatalf("no image parsed for %q", embed)
		}
		h.e.resizeImageSource(h.e.imageHits[0], 220)
		h.frame()
		if !strings.Contains(h.e.Source(), "![photo|220][photo]") {
			t.Fatalf("reference lookup changed: %q", h.e.Source())
		}
		if len(h.e.imageHits) == 0 || h.e.imageHits[0].obj.imageDest != "absent.png" {
			t.Fatal("resized reference no longer resolves")
		}
		if !h.e.Undo() || h.e.Source() != src {
			t.Fatal("resize undo failed")
		}
	}
}

func TestImageHoverPreviewPassesResolvedPathWithoutChangingSource(t *testing.T) {
	dir := t.TempDir()
	var got string
	src := "before\n\n![sample](sample%20file.png)\n"
	h := newHarness(t, src)
	opts := h.e.Options()
	opts.BaseDir = dir
	opts.OnImagePreview = func(path string) { got = path }
	h.e.SetOptions(opts)
	h.frame()
	if len(h.e.imageHits) == 0 {
		t.Fatal("no rendered image region")
	}
	hit := h.e.imageHits[0]
	preview, _, _, _ := media.ToolRects(hit.box, h.e.lk.size)
	h.e.input(ui.InputEvent{Kind: ui.InputPointerDown, Button: 0, X: preview.X + preview.W/2, Y: preview.Y + preview.H/2})
	if got != filepath.Join(dir, "sample file.png") {
		t.Fatalf("preview callback got %q", got)
	}
	if h.e.Source() != src {
		t.Fatal("opening preview changed Markdown source")
	}
}

func TestStandaloneImageAlignmentSurvivesSourceReveal(t *testing.T) {
	for _, tc := range []struct {
		name       string
		left       bool
		wantCenter bool
	}{
		{name: "centered", wantCenter: true},
		{name: "left aligned", left: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "sample.png")
			var encoded bytes.Buffer
			bitmap := image.NewRGBA(image.Rect(0, 0, 120, 60))
			for y := 0; y < 60; y++ {
				for x := 0; x < 120; x++ {
					bitmap.Set(x, y, color.RGBA{R: 80, G: 130, B: 190, A: 255})
				}
			}
			if err := png.Encode(&encoded, bitmap); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, encoded.Bytes(), 0600); err != nil {
				t.Fatal(err)
			}
			h := newHarness(t, "![sample](missing.png)")
			opts := h.e.Options()
			opts.BaseDir = dir
			h.e.SetOptions(opts)
			h.e.SetSource("![sample](sample.png)")
			h.frame()
			h.frame()
			if len(h.e.imageHits) == 0 {
				t.Fatal("standalone image has no hit box")
			}
			hit := h.e.imageHits[0]
			if tc.left {
				h.tt.Move(hit.box.X+hit.box.W/2, hit.box.Y+hit.box.H/2)
				h.frame()
				hit = h.e.imageHits[0]
				_, _, align, _ := media.ToolRects(hit.box, h.e.lk.size)
				h.tt.ClickAt(align.X+align.W/2, align.Y+align.H/2)
				h.frame()
				hit = h.e.imageHits[0]
			}
			before, ok := standaloneImageX(hit.leaf)
			if !ok {
				t.Fatal("image object has no layout rectangle")
			}
			h.e.toggleSource(hit.leaf)
			h.frame()
			hit = h.e.imageHits[0]
			h.shot("standalone-image-source-" + strings.ReplaceAll(tc.name, " ", "-"))
			after, ok := standaloneImageX(hit.leaf)
			if !ok {
				t.Fatal("revealed image object has no layout rectangle")
			}
			if after != before {
				t.Fatalf("source reveal moved image horizontally: before %.1f after %.1f", before, after)
			}
			if tc.wantCenter && before < h.e.colW/3 {
				t.Fatalf("centered image was not placed near the center: x=%.1f column=%.1f", before, h.e.colW)
			}
			if !tc.wantCenter && before > h.e.colW/4 {
				t.Fatalf("left-aligned image was not placed near the left edge: x=%.1f column=%.1f", before, h.e.colW)
			}
			if got := hit.obj.alignCenter; got != tc.wantCenter {
				t.Fatalf("alignment mode = %v, want centered=%v; source=%d alignments=%v", got, tc.wantCenter, hit.obj.imageSource.From, h.e.imageAlignments)
			}
			h.e.toggleSource(hit.leaf)
			h.frame()
			hit = h.e.imageHits[0]
			collapsed, ok := standaloneImageX(hit.leaf)
			if !ok || collapsed != before {
				t.Fatalf("source collapse moved image horizontally: before %.1f after %.1f (ok=%v)", before, collapsed, ok)
			}
			h.tt.Move(hit.box.X+hit.box.W/2, hit.box.Y+hit.box.H/2)
			h.frame()
			h.shot("standalone-image-controls-" + strings.ReplaceAll(tc.name, " ", "-"))
		})
	}
}

func standaloneImageX(l *leaf) (float32, bool) {
	if l == nil || l.view == nil || l.view.layout == nil {
		return 0, false
	}
	for _, obj := range l.view.layout.Objects() {
		return obj.Rect.X, true
	}
	return 0, false
}
