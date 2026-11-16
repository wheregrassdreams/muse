package editor

import (
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/transfer"
	"muse/locales"
)

func TestCopyImageObjectPastesAsVaultRelativeInternalEmbed(t *testing.T) {
	h := newHarness(t, "before after")
	h.e.opts.AssetRoot = filepath.Join(t.TempDir(), "vault")
	path := filepath.Join(h.e.opts.AssetRoot, "Attachments", "Flow | draft].png")

	h.e.copyImageObject(path)
	md, err := h.clip.Read(transfer.Markdown)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(md), "![[Attachments/Flow \\| draft\\].png]]"; got != want {
		t.Fatalf("copied Markdown image object = %q, want %q", got, want)
	}
	plain, err := h.clip.Read(transfer.Text)
	if err != nil || string(plain) != string(md) {
		t.Fatalf("copied plain text = %q, err %v; want same internal embed", plain, err)
	}

	h.caretAt(len("before "))
	h.e.Paste(false)
	if got, want := h.e.Source(), "before ![[Attachments/Flow \\| draft\\].png]]after"; got != want {
		t.Fatalf("pasted image object = %q, want %q", got, want)
	}
}

func TestCopyImageObjectOutsideVaultUsesFilename(t *testing.T) {
	h := newHarness(t, "")
	h.e.opts.AssetRoot = filepath.Join(t.TempDir(), "vault")
	h.e.copyImageObject(filepath.Join(t.TempDir(), "outside.png"))
	md, err := h.clip.Read(transfer.Markdown)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(md), "![[outside.png]]"; got != want {
		t.Fatalf("copied external image object = %q, want %q", got, want)
	}
}

func TestInlineImageRightClickMenuOffersCopyAndPreview(t *testing.T) {
	root := t.TempDir()
	imagePath := filepath.Join(root, "Attachments", "diagram.png")
	if err := os.MkdirAll(filepath.Dir(imagePath), 0o755); err != nil {
		t.Fatal(err)
	}
	f, err := os.Create(imagePath)
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(f, image.NewRGBA(image.Rect(0, 0, 40, 30))); err != nil {
		_ = f.Close()
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	h := newHarness(t, "![[Attachments/diagram.png]]")
	opts := h.e.Options()
	opts.AssetRoot = root
	opts.BaseDir = root
	opts.WriteClipboard = func(data transfer.Data) { h.clip = data }
	opts.OnImagePreview = func(string) {}
	h.e.SetOptions(opts)
	h.frame()
	if len(h.e.imageHits) == 0 {
		t.Fatal("inline image has no hit box")
	}
	box := h.e.imageHits[0].box
	h.tt.RightClickAt(box.X+box.W/2, box.Y+box.H/2)
	h.frame()
	menu := h.tt.Menu()
	wantCopy, wantPreview := false, false
	for _, item := range menu {
		wantCopy = wantCopy || item == locales.CommonCopyAction()
		wantPreview = wantPreview || item == locales.ImagePreviewTitle()
	}
	if !wantCopy || !wantPreview {
		t.Fatalf("inline image context menu = %#v, want Copy and Preview", menu)
	}
	if err := h.tt.ChooseMenuItem(locales.CommonCopyAction()); err != nil {
		t.Fatalf("choose Copy from image context menu: %v", err)
	}
	data, err := h.clip.Read(transfer.Markdown)
	if err != nil || string(data) != "![[Attachments/diagram.png]]" {
		t.Fatalf("image menu copy = %q, err %v", data, err)
	}
}
