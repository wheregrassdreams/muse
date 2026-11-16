package editor

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestRealHomeMouseWheelRoundTripKeepsImagesAndTextShaped follows the Home.md
// fixture path with the animated mouse-wheel event used by the app, rather
// than changing Editor.scroll directly or using touchpad-style Scroll events.
func TestRealHomeMouseWheelRoundTripKeepsImagesAndTextShaped(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", "testdata", "vault-fixture"))
	if err != nil {
		t.Fatal(err)
	}
	homePath := filepath.Join(root, "Home.md")
	source, err := os.ReadFile(homePath)
	if err != nil {
		t.Fatal(err)
	}
	resolveEmbed := func(_, target string) (string, string, bool) {
		path := filepath.Join(root, filepath.FromSlash(target)+".md")
		b, err := os.ReadFile(path)
		if err != nil {
			return "", "", false
		}
		return path, string(b), true
	}
	e := New(string(source), Options{
		FontSize:         16,
		BaseDir:          root,
		AssetRoot:        root,
		CurrentPath:      homePath,
		ResolveWikiEmbed: resolveEmbed,
	})
	tt := stressTester(e)
	tt.Frame()
	tt.Frame()
	if !e.virtualLayout() {
		t.Fatalf("Home.md fixture did not enter the same virtual layout path: leaves=%d", len(e.leaves))
	}
	assertVisibleImagesShaped := func(step string) {
		t.Helper()
		for _, l := range e.paintVisibleSpan(e.scroll.Y, e.scroll.Y+e.viewH) {
			if !leafMayContainImage(e.Source(), l) || l.y+l.h <= e.scroll.Y || l.y >= e.scroll.Y+e.viewH {
				continue
			}
			if l.view == nil {
				t.Fatalf("%s left a visible image leaf unshaped: scroll=[%.1f,%.1f] leaf=(%d,%d y=%.1f h=%.1f) source=%q coverage=[%.1f,%.1f]",
					step, e.scroll.Y, e.scroll.Y+e.viewH, l.start, l.end, l.y, l.h, strings.TrimSpace(e.Source()[l.start:l.end]), e.viewTop, e.viewBottom)
			}
		}
		for _, l := range e.paintVisibleSpan(e.scroll.Y, e.scroll.Y+e.viewH) {
			if l.view == nil {
				continue
			}
			for _, piece := range l.view.disp.pieces {
				if piece.obj == nil || piece.obj.embed == nil {
					continue
				}
				child := piece.obj.embed
				for _, nested := range child.visibleLeaves {
					if leafMayContainImage(child.Source(), nested) && nested.view == nil {
						t.Fatalf("%s left an embedded image unshaped: host=%q child=(%d,%d) source=%q",
							step, strings.TrimSpace(e.Source()[l.start:l.end]), nested.start, nested.end, strings.TrimSpace(child.Source()[nested.start:nested.end]))
					}
				}
			}
		}
	}
	assertVisibleTextShaped := func(step string) {
		t.Helper()
		for _, l := range e.paintVisibleSpan(e.scroll.Y, e.scroll.Y+e.viewH) {
			if (l.kind == kHeading || l.kind == kPara) && l.h > 0 && l.y+l.h > e.scroll.Y && l.y < e.scroll.Y+e.viewH && l.view == nil {
				t.Fatalf("%s left visible text unshaped: scroll=[%.1f,%.1f] leaf=(%d,%d y=%.1f h=%.1f kind=%d) source=%q coverage=[%.1f,%.1f]",
					step, e.scroll.Y, e.scroll.Y+e.viewH, l.start, l.end, l.y, l.h, l.kind, strings.TrimSpace(e.Source()[l.start:l.end]), e.viewTop, e.viewBottom)
			}
		}
	}
	for i := 0; i < 30; i++ {
		tt.Wheel(900, 420, 0, 120)
		tt.Advance(180 * time.Millisecond)
		tt.Frame()
		assertVisibleImagesShaped("forward mouse wheel")
	}
	if e.scroll.Y < 1000 {
		t.Fatalf("mouse wheel did not reach the Wiki embeds section: scroll=%v", e.scroll.Y)
	}
	stable := 0
	for i := 0; i < 60 && stable < 3; i++ {
		previous := e.scroll.Y
		tt.Wheel(900, 420, 0, 120)
		tt.Advance(180 * time.Millisecond)
		tt.Frame()
		if previous == e.scroll.Y {
			stable++
		} else {
			stable = 0
		}
		assertVisibleImagesShaped("mouse wheel at document end")
	}
	if stable < 3 {
		t.Fatalf("mouse wheel did not reach document end after 60 more notches: scroll=%.1f docH=%.1f", e.scroll.Y, e.docH)
	}
	// Keep the editor alive long enough for its periodic stale-view pruning to
	// run, then return through the same animated wheel path used by the user.
	for i := 0; i < 300; i++ {
		tt.Frame()
	}
	stable = 0
	for i := 0; i < 80 && stable < 3; i++ {
		previous := e.scroll.Y
		tt.Wheel(900, 420, 0, -120)
		tt.Advance(180 * time.Millisecond)
		tt.Frame()
		assertVisibleImagesShaped("reverse mouse wheel")
		assertVisibleTextShaped("reverse mouse wheel")
		if previous == e.scroll.Y {
			stable++
		} else {
			stable = 0
		}
	}
	if e.scroll.Y != 0 {
		t.Fatalf("mouse wheel did not return to the top after 80 reverse notches: scroll=%.1f", e.scroll.Y)
	}
}
