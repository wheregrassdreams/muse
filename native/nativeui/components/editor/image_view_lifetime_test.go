package editor

import (
	"testing"

	"muse/markdown/images"
)

func TestVirtualLayoutForMediumImageNoteLoadsImagesOnDemand(t *testing.T) {
	dir := t.TempDir()
	src := stressImages(dir, 18)
	e := New(src, editorTestOptions(Options{FontSize: 16, BaseDir: dir}))
	if len(e.leaves) >= virtualLayoutMin {
		t.Fatalf("fixture has %d leaves, want a medium note below the general threshold", len(e.leaves))
	}
	if !e.virtualLayout() {
		t.Fatalf("%d-leaf image note did not use viewport layout", len(e.leaves))
	}

	tester := stressTester(e)
	tester.Frame()
	var imageLeaves []*leaf
	for _, l := range e.leaves {
		if leafMayContainImage(e.Source(), l) {
			imageLeaves = append(imageLeaves, l)
		}
	}
	if len(imageLeaves) != 18 {
		t.Fatalf("fixture has %d image leaves, want 18", len(imageLeaves))
	}
	if imageLeaves[0].view == nil || !imageLeaves[0].view.hasImageAsset() {
		t.Fatal("first visible image did not create its asset view")
	}
	last := imageLeaves[len(imageLeaves)-1]
	if last.y <= e.scroll.Y+e.viewH {
		t.Fatalf("last image at y=%v is not below viewport bottom %v", last.y, e.scroll.Y+e.viewH)
	}
	if last.view != nil && last.view.hasImageAsset() {
		t.Fatal("first frame created an asset view for the offscreen last image")
	}

	lastStart, lastY := last.start, last.y
	e.scroll.Y = max(e.docH-e.viewH, 0)
	e.layoutIfNeeded()
	var scrolledLast *leaf
	for _, l := range e.leaves {
		if l.start == lastStart && leafMayContainImage(e.Source(), l) {
			scrolledLast = l
			break
		}
	}
	if scrolledLast == nil || scrolledLast.view == nil || !scrolledLast.view.hasImageAsset() {
		t.Fatalf("scrolling to last image at y=%v (offset %d) did not create its asset view; scroll=%v docH=%v viewport=%v band=[%v,%v] target=%+v",
			lastY, lastStart, e.scroll.Y, e.docH, e.viewH, e.viewTop, e.viewBottom, scrolledLast)
	}
}

func TestVirtualLayoutDropsOffscreenImageViews(t *testing.T) {
	dir := t.TempDir()
	src := stressImages(dir, 80)
	e := New(src, editorTestOptions(Options{FontSize: 16, BaseDir: dir}))
	tester := stressTester(e)
	tester.Frame()
	if len(e.leaves) < virtualLayoutMin {
		t.Fatalf("fixture has %d leaves, want virtual layout", len(e.leaves))
	}
	first := e.leaves[0]
	if first.view == nil || !first.view.hasImageAsset() {
		t.Fatal("first visible image has no asset view")
	}
	for _, cached := range e.views {
		if cached.hasImageAsset() {
			t.Fatal("editor view cache retained an image asset")
		}
	}

	e.setSessionSelection(len(e.Source()), len(e.Source()))
	e.scroll.Y = max(e.docH-e.viewH, 0)
	tester.Frame()
	if first.view != nil && first.view.hasImageAsset() {
		t.Fatal("offscreen image leaf retained its decoded asset view")
	}
}

func TestVirtualLayoutDoesNotRetainPrefetchedImageViews(t *testing.T) {
	dir := t.TempDir()
	src := stressImages(dir, 80)
	e := New(src, editorTestOptions(Options{FontSize: 16, BaseDir: dir}))
	tester := stressTester(e)
	tester.Frame()
	if !e.virtualLayout() {
		t.Fatal("fixture did not enable virtual layout")
	}

	e.setSessionSelection(len(e.Source()), len(e.Source()))
	e.scroll.Y = max(e.docH-e.viewH, 0) / 2
	tester.Frame()
	viewportTop, viewportBottom := e.scroll.Y, e.scroll.Y+e.viewH
	for i, l := range e.leaves {
		if l.view == nil || !l.view.hasImageAsset() {
			continue
		}
		if l.y > viewportBottom || l.y+l.h < viewportTop {
			t.Fatalf("offscreen image leaf %d retained a decoded view at y=%v h=%v outside [%v, %v]", i, l.y, l.h, viewportTop, viewportBottom)
		}
	}
}

func TestLeafViewImageAssetDetection(t *testing.T) {
	if (*leafView)(nil).hasImageAsset() {
		t.Fatal("nil view reported an image asset")
	}
	view := &leafView{disp: display{pieces: []piece{{obj: &object{asset: nil}}}}}
	if view.hasImageAsset() {
		t.Fatal("placeholder without an asset reported an image asset")
	}
	view.disp.pieces[0].obj.asset = &images.Asset{}
	if !view.hasImageAsset() {
		t.Fatal("inline image asset was not detected")
	}
	view.block = &object{asset: &images.Asset{}}
	if !view.hasImageAsset() {
		t.Fatal("block image asset was not detected")
	}
}
