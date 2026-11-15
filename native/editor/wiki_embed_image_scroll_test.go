package editor

import (
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestWikiEmbedImageLoadReflowsHostAndRendersAfterScroll(t *testing.T) {
	dir := t.TempDir()
	childImage := stressImagesSized(dir, 1, 160, 1200)
	childSource := "# Embedded image\n\n" + childImage + "\nTail.\n"

	var host strings.Builder
	host.WriteString("![[Target]]\n\n")
	for i := 0; i < 100; i++ {
		host.WriteString("Host paragraph after the image embed keeps the document virtualized.\n\n")
	}
	opts := editorTestOptions(Options{
		FontSize:        16,
		BaseDir:         dir,
		AssetRoot:       dir,
		ImageLocalFiles: editorTestLocalFiles{},
		CurrentPath:     filepath.Join(dir, "Host.md"),
		ResolveWikiEmbed: func(_, _ string) (string, string, bool) {
			return filepath.Join(dir, "Target.md"), childSource, true
		},
	})
	e := New(host.String(), opts)
	tt := stressTester(e)
	tt.Frame()

	outer := findEmbeddedNoteObject(e)
	if outer == nil || outer.embed == nil {
		t.Fatal("host did not build the wiki embed")
	}
	asset := editorImageAsset(outer.embed)
	if asset == nil {
		t.Fatal("embedded image did not start loading")
	}
	deadline := time.Now().Add(5 * time.Second)
	for !asset.Loaded() && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if !asset.Loaded() {
		t.Fatal("embedded image did not finish loading")
	}

	// The child's asynchronous asset callback must change the host layout key;
	// invalidating only the child leaves the cached embed at placeholder height.
	if e.assetGen.Load() == 0 {
		t.Fatal("embedded image completion did not invalidate the host layout")
	}
	tt.Frame()
	outer = findEmbeddedNoteObject(e)
	if outer == nil || outer.embed == nil {
		t.Fatal("host lost its wiki embed after image completion")
	}
	outer.embed.layoutIfNeeded()
	wantHeight := outer.embed.docH + 2*e.lk.size*0.55
	if outer.h+1 < wantHeight {
		t.Fatalf("host embed height %.1f clips loaded child height %.1f", outer.h, wantHeight)
	}

	var outerLeaf *leaf
	for _, leaf := range e.visibleLeaves {
		if leaf.view == nil {
			continue
		}
		for _, piece := range leaf.view.disp.pieces {
			if piece.obj == outer {
				outerLeaf = leaf
				break
			}
		}
	}
	if outerLeaf == nil {
		t.Fatal("host embed leaf disappeared after reflow")
	}
	targetScroll := max(outerLeaf.y+outer.h-e.viewH/2, 0)
	for cycle := 0; cycle < 2; cycle++ {
		if cycle > 0 {
			// Move well past the image, then return to the same image region.
			tt.Scroll(600, 400, 0, e.docH)
			tt.Frame()
			tt.Scroll(600, 400, 0, -e.docH)
			tt.Frame()
		}
		tt.Scroll(600, 400, 0, targetScroll)
		tt.Frame()
		if e.scroll.Y < targetScroll-1 {
			t.Fatalf("mouse scroll stopped at %.1f before the image target %.1f", e.scroll.Y, targetScroll)
		}
		img := tt.Image()
		blueImagePixel := false
		for y := 40; y < img.Bounds().Dy()-20 && !blueImagePixel; y++ {
			for x := 20; x < img.Bounds().Dx()-20; x++ {
				_, _, b, _ := img.At(x, y).RGBA()
				if b > 27000 {
					blueImagePixel = true
					break
				}
			}
		}
		if !blueImagePixel {
			t.Fatalf("mouse scroll cycle %d returned to the embed without rendering image pixels", cycle+1)
		}
	}
}
