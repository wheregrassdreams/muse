package editor

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func virtualImageHarness(t *testing.T) (*harness, int, string) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "tall.png")
	bmp := image.NewRGBA(image.Rect(0, 0, 120, 1000))
	for y := 0; y < 1000; y++ {
		for x := 0; x < 120; x++ {
			bmp.Set(x, y, color.RGBA{R: uint8(y % 255), G: 110, B: 180, A: 255})
		}
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(f, bmp); err != nil {
		f.Close()
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	src := "before\n\n![](tall.png)\n\nafter"
	h := newHarness(t, src)
	opts := h.e.Options()
	opts.BaseDir = dir
	opts.VimVirtualImageRows = true
	h.e.SetOptions(opts)
	h.e.SetKeymap("vim")
	h.frame()
	// Image assets decode on bounded background workers. Keep advancing the
	// harness until the real image has replaced its placeholder so geometry
	// assertions do not depend on scheduler timing.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		l := h.e.leafAt(strings.Index(src, "![]"))
		loaded := false
		if l != nil && l.view != nil {
			for _, piece := range l.view.disp.pieces {
				if piece.obj != nil && piece.obj.asset != nil {
					loaded = piece.obj.asset.Loaded()
					break
				}
			}
		}
		if loaded {
			h.frame()
			break
		}
		time.Sleep(time.Millisecond)
		h.frame()
	}
	return h, strings.Index(src, "![]"), src
}

func moveVimToImage(t *testing.T, h *harness, imageAt int) {
	t.Helper()
	h.caretAt(0)
	for i := 0; i < 8 && h.e.Caret() < imageAt; i++ {
		sendVim(h, "j")
	}
	if h.e.vimImageRow == nil || h.e.vimImageRow.lineStart != imageAt {
		t.Fatalf("Vim did not enter the tall image: caret=%d virtual=%+v leaves=%+v", h.e.Caret(), h.e.vimImageRow, h.e.leaves)
	}
}

func TestVimVirtualImageRowsTrackRenderedHeightWithoutChangingSourcePosition(t *testing.T) {
	h, imageAt, _ := virtualImageHarness(t)
	moveVimToImage(t, h, imageAt)
	initial := h.e.vimImageRow.row
	start, ok := h.e.caretRect(h.e.Caret())
	if !ok {
		t.Fatal("virtual image caret has no geometry")
	}
	h.shot("vim-virtual-image-row-top")
	sourceCaret := h.e.Caret()
	sendVim(h, "j")
	second, ok := h.e.caretRect(h.e.Caret())
	if !ok || h.e.Caret() != sourceCaret || h.e.vimImageRow == nil || h.e.vimImageRow.row != initial+1 {
		t.Fatalf("j changed the source position or failed to advance virtual row: caret=%d row=%+v", h.e.Caret(), h.e.vimImageRow)
	}
	lineHeight := h.e.lk.size * h.e.lk.lh
	if delta := second.Y - start.Y; delta < lineHeight-1 || delta > lineHeight+1 {
		t.Fatalf("virtual row caret moved by %.1f, want one visual line (%.1f): before=%+v after=%+v", delta, lineHeight, start, second)
	}
	h.shot("vim-virtual-image-row-next")
	sendVim(h, "k")
	back, _ := h.e.caretRect(h.e.Caret())
	if h.e.Caret() != sourceCaret || h.e.vimImageRow == nil || h.e.vimImageRow.row != initial || back.Y != start.Y {
		t.Fatalf("k did not return to the previous virtual row: caret=%d row=%+v rect=%+v want=%+v", h.e.Caret(), h.e.vimImageRow, back, start)
	}
	box, rows, ok := h.e.vimImageGeometry(h.e.leafAt(imageAt))
	if !ok || rows < 10 {
		t.Fatalf("tall image geometry = %+v / %d rows / %v", box, rows, ok)
	}
	for i := h.e.vimImageRow.row + 1; i < rows; i++ {
		sendVim(h, "j")
	}
	if h.e.vimImageRow == nil || h.e.vimImageRow.row != rows-1 {
		t.Fatalf("j stopped before the image's last virtual row: %+v, rows=%d", h.e.vimImageRow, rows)
	}
	h.shot("vim-virtual-image-row-bottom")
	last, _ := h.e.caretRect(h.e.Caret())
	if last.Y < box.Y+box.H-lineHeight-1 {
		t.Fatalf("last virtual caret row is not near the image bottom: box=%+v caret=%+v", box, last)
	}
	if h.e.scroll.Y <= 0 {
		t.Fatalf("moving through the tall image did not scroll the viewport with its virtual caret: scrollY=%v last=%+v", h.e.scroll.Y, last)
	}
	sendVim(h, "j")
	if h.e.vimImageRow != nil || h.e.Caret() <= imageAt {
		t.Fatalf("j did not leave the image at its lower edge: caret=%d virtual=%+v", h.e.Caret(), h.e.vimImageRow)
	}
	sendVim(h, "k")
	if h.e.vimImageRow == nil || h.e.vimImageRow.row != rows-1 {
		t.Fatalf("k entering from below did not land on the image's last row: %+v, rows=%d", h.e.vimImageRow, rows)
	}
}

func TestVimVirtualImageRowsAreOptIn(t *testing.T) {
	h, imageAt, _ := virtualImageHarness(t)
	opts := h.e.Options()
	opts.VimVirtualImageRows = false
	h.e.SetOptions(opts)
	moveVimToSourceLine(t, h, imageAt)
	if h.e.vimImageRow != nil {
		t.Fatalf("disabled setting created a virtual image row: %+v", h.e.vimImageRow)
	}
}

func moveVimToSourceLine(t *testing.T, h *harness, imageAt int) {
	t.Helper()
	h.caretAt(0)
	for i := 0; i < 8 && h.e.Caret() < imageAt; i++ {
		sendVim(h, "j")
	}
	if lineStartAt(h.e.Source(), h.e.Caret()) != imageAt {
		t.Fatalf("Vim did not reach image source line: caret=%d source line=%d image line=%d", h.e.Caret(), lineStartAt(h.e.Source(), h.e.Caret()), imageAt)
	}
}

func TestVimVirtualImageLineOperationsKeepTheWholeImageRow(t *testing.T) {
	t.Run("dd", func(t *testing.T) {
		h, imageAt, src := virtualImageHarness(t)
		moveVimToImage(t, h, imageAt)
		sendVim(h, "jdd")
		want := "before\n\n\nafter"
		if got := h.e.Source(); got != want {
			t.Fatalf("dd on a virtual image row produced %q, want %q (source was %q)", got, want, src)
		}
		if !h.e.Undo() || h.e.Source() != src {
			t.Fatalf("undo did not restore the complete image row: %q", h.e.Source())
		}
	})
	t.Run("visual yank", func(t *testing.T) {
		h, imageAt, src := virtualImageHarness(t)
		moveVimToImage(t, h, imageAt)
		sendVim(h, "vj")
		if h.e.vimImageRow == nil || h.e.vimImageRow.row != 1 {
			t.Fatalf("visual selection lost the virtual image caret: %+v", h.e.vimImageRow)
		}
		visible := false
		for _, hit := range h.e.imageHits {
			visible = visible || hit.leaf != nil && hit.leaf.lineStart == imageAt
		}
		if !visible {
			l := h.e.leafAt(imageAt)
			t.Fatalf("visual selection hid its image: leaf=%+v view=%+v", l, l.view)
		}
		h.shot("vim-virtual-image-visual-selection")
		if got, want := h.e.SelectedText(), "![](tall.png)\n"; got != want {
			t.Fatalf("visual selection across virtual image rows = %q, want image source row %q (source %q)", got, want, src)
		}
		sendVim(h, "y")
		if !h.e.vimController().View().RegisterLine || h.e.vimController().View().Register != "![](tall.png)\n" {
			t.Fatalf("yank register did not preserve the whole image row: line=%v register=%q", h.e.vimController().View().RegisterLine, h.e.vimController().View().Register)
		}
	})
	t.Run("x deletes the mapped image row", func(t *testing.T) {
		h, imageAt, _ := virtualImageHarness(t)
		moveVimToImage(t, h, imageAt)
		sendVim(h, "jx")
		if strings.Contains(h.e.Source(), "tall.png") || !h.e.vimController().View().RegisterLine || h.e.vimController().View().Register != "![](tall.png)\n" {
			t.Fatalf("x did not operate on the image source row: source=%q register=%q line=%v", h.e.Source(), h.e.vimController().View().Register, h.e.vimController().View().RegisterLine)
		}
	})
	t.Run("cc changes the mapped image row as one line", func(t *testing.T) {
		h, imageAt, _ := virtualImageHarness(t)
		moveVimToImage(t, h, imageAt)
		sendVim(h, "jcc")
		if strings.Contains(h.e.Source(), "tall.png") || h.e.vimController().Mode() != "Insert" {
			t.Fatalf("cc did not change the entire image source row: mode=%q source=%q", h.e.vimController().Mode(), h.e.Source())
		}
		sendVim(h, "replacement")
		sendVim(h, "Escape")
		if strings.Contains(h.e.Source(), "tall.png") || !strings.Contains(h.e.Source(), "replacement") {
			t.Fatalf("cc replacement did not stay within the mapped image row: %q", h.e.Source())
		}
	})
	t.Run("V selects the mapped image row", func(t *testing.T) {
		h, imageAt, _ := virtualImageHarness(t)
		moveVimToImage(t, h, imageAt)
		sendVim(h, "jV")
		if got, want := h.e.SelectedText(), "![](tall.png)\n"; got != want {
			t.Fatalf("V on a virtual image row selected %q, want %q", got, want)
		}
	})
	t.Run("yy and P use the mapped image row", func(t *testing.T) {
		h, imageAt, _ := virtualImageHarness(t)
		moveVimToImage(t, h, imageAt)
		sendVim(h, "jyyP")
		if got := strings.Count(h.e.Source(), "![](tall.png)"); got != 2 {
			t.Fatalf("line yank and put did not duplicate one image row: %q", h.e.Source())
		}
		if !h.e.vimController().View().RegisterLine || h.e.vimController().View().Register != "![](tall.png)\n" {
			t.Fatalf("yy did not capture the whole image row: register=%q line=%v", h.e.vimController().View().Register, h.e.vimController().View().RegisterLine)
		}
	})
	t.Run("o and O", func(t *testing.T) {
		for _, key := range []string{"o", "O"} {
			t.Run(key, func(t *testing.T) {
				h, imageAt, src := virtualImageHarness(t)
				moveVimToImage(t, h, imageAt)
				sendVim(h, key+"x")
				got := h.e.Source()
				if !strings.Contains(got, "![](tall.png)") || !strings.Contains(got, "x") || got == src {
					t.Fatalf("%s on a virtual image row did not open an adjacent editable line: %q", key, got)
				}
			})
		}
	})
}
