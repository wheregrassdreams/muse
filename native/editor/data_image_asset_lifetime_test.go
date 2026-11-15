package mdedit

import (
	"strings"
	"testing"
	"time"

	"muse/mdoc"
)

func TestLargeDataImageAssetSurvivesAssetInvalidationWithoutRedecode(t *testing.T) {
	loaded := make(chan struct{}, 1)
	src := "![large](data:image/png;base64," + strings.Repeat("A", (1<<20)+32) + ")\n"
	e := New(src, Options{
		FontSize: 16,
		Invalidate: func() {
			select {
			case loaded <- struct{}{}:
			default:
			}
		},
	})
	tester := stressTester(e)
	tester.Frame()
	first := editorImageAsset(e)
	if first == nil {
		t.Fatal("first frame did not create a data image asset")
	}
	select {
	case <-loaded:
	case <-time.After(5 * time.Second):
		t.Fatal("asynchronous data image load did not invalidate the editor")
	}
	if !first.Loaded() {
		t.Fatal("image invalidation ran before its asset finished loading")
	}

	// Asset loading changes the view key and forces a rebuild. The editor must
	// reuse its loaded non-cacheable data asset instead of decoding the entire
	// Base64 source again.
	tester.Frame()
	if got := editorImageAsset(e); got != first {
		t.Fatal("asset invalidation replaced the loaded data image asset")
	}

	e.SetSource("# next note")
	tester.Frame()
	if len(e.dataImageAssets) != 0 {
		t.Fatalf("switching notes retained %d old data image assets", len(e.dataImageAssets))
	}
}

func editorImageAsset(e *Editor) *mdoc.ImageAsset {
	for _, leaf := range e.leaves {
		if leaf.view == nil {
			continue
		}
		if leaf.view.block != nil && leaf.view.block.asset != nil {
			return leaf.view.block.asset
		}
		for _, piece := range leaf.view.disp.pieces {
			if piece.obj != nil && piece.obj.asset != nil {
				return piece.obj.asset
			}
		}
	}
	return nil
}
