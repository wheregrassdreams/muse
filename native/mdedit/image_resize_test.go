package mdedit

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
			if pc.obj != nil && pc.obj.imageSyntax == imageSyntaxMarkdown {
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
				if pc.obj != nil && pc.obj.imageSyntax == imageSyntaxWiki {
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

func TestSetHTMLAttrsPreservesOtherAttributesAndClosingForm(t *testing.T) {
	for _, tc := range []struct{ tag, want string }{
		{`<img src="x.png" alt="x">`, `<img src="x.png" alt="x" width="200">`},
		{`<img data-width="x" src="x.png" width='40' height="20" />`, `<img data-width="x" src="x.png" width='200' height="20" />`},
	} {
		got := setHTMLAttrs(tc.tag, struct{ name, value string }{"width", "200"})
		if got != tc.want {
			t.Errorf("setHTMLAttrs(%q)=%q, want %q", tc.tag, got, tc.want)
		}
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
	_, _, grip := imageToolRects(hit.box, h.e.lk.size)
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
	_, _, grip = imageToolRects(hit.box, h.e.lk.size)
	x, y = grip.X+grip.W/2, grip.Y+grip.H/2
	h.e.input(ui.InputEvent{Kind: ui.InputPointerDown, Button: 0, X: x, Y: y})
	h.e.input(ui.InputEvent{Kind: ui.InputPointerMove, Button: 0, X: x + 40, Y: y + 20})
	h.frame()
	wantWidth := h.e.imageDrag.previewWidth
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
	preview, _, _ := imageToolRects(hit.box, h.e.lk.size)
	h.e.input(ui.InputEvent{Kind: ui.InputPointerDown, Button: 0, X: preview.X + preview.W/2, Y: preview.Y + preview.H/2})
	if got != filepath.Join(dir, "sample file.png") {
		t.Fatalf("preview callback got %q", got)
	}
	if h.e.Source() != src {
		t.Fatal("opening preview changed Markdown source")
	}
}
