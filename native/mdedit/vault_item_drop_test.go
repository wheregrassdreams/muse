package mdedit

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestDropVaultItemInsertsWikiLinkAtCaret(t *testing.T) {
	h := newHarness(t, "before after")
	h.e.setCaret(len("before "), false)
	h.e.dropVaultItem(VaultItemDrag{Path: "/vault/Notes/Target.md", Name: "Target.md"})
	if got, want := h.e.Source(), "before [[Target]]after"; got != want {
		t.Fatalf("source after vault item drop = %q, want %q", got, want)
	}
	if !h.e.Undo() || h.e.Source() != "before after" {
		t.Fatalf("undo did not restore pre-drop source: %q", h.e.Source())
	}
}

func TestDropVaultItemFallsBackToPathAndEscapesWikiSyntax(t *testing.T) {
	h := newHarness(t, "")
	h.e.dropVaultItem(VaultItemDrag{Path: "/vault/A | B].md"})
	if got, want := h.e.Source(), "[[A \\| B\\]]]"; got != want {
		t.Fatalf("source after vault item fallback = %q, want %q", got, want)
	}
}

func TestDropVaultImageUsesVaultRelativeEmbedPath(t *testing.T) {
	h := newHarness(t, "before after")
	h.e.opts.AssetRoot = "/vault"
	h.e.setCaret(len("before "), false)
	h.e.dropVaultItem(VaultItemDrag{Path: "/vault/Attachments/Diagrams/flow chart.webp", Name: "flow chart.webp"})
	if got, want := h.e.Source(), "before ![[Attachments/Diagrams/flow chart.webp]]after"; got != want {
		t.Fatalf("image drop = %q, want %q", got, want)
	}
}

func TestDropNonMarkdownFileUsesDocumentRelativeMarkdownLink(t *testing.T) {
	h := newHarness(t, "")
	h.e.opts.AssetRoot = "/vault"
	h.e.opts.BaseDir = "/vault/Notes/Current"
	h.e.dropVaultItem(VaultItemDrag{Path: "/vault/Attachments/Reference Guide.pdf", Name: "Reference Guide.pdf"})
	if got, want := h.e.Source(), "[Reference Guide.pdf](<../../Attachments/Reference Guide.pdf>)"; got != want {
		t.Fatalf("non-Markdown file drop = %q, want %q", got, want)
	}
}

func TestDropMarkdownFileUsesVaultRelativeWikiTarget(t *testing.T) {
	h := newHarness(t, "")
	h.e.opts.AssetRoot = "/vault"
	h.e.dropVaultItem(VaultItemDrag{Path: "/vault/Projects/Atlas/Overview.md", Name: "Overview.md"})
	if got, want := h.e.Source(), "[[Projects/Atlas/Overview]]"; got != want {
		t.Fatalf("Markdown file drop = %q, want %q", got, want)
	}
}

func TestVaultItemDropIndicatorTracksAndClearsHitBoundary(t *testing.T) {
	h := newHarness(t, "first line\nsecond line")
	h.e.layoutIfNeeded()
	h.e.elemRect = ui.Rect{X: 31, Y: 47, W: 720, H: 600}
	off := len("first line")
	caret, ok := h.e.caretRect(off)
	if !ok {
		t.Fatal("no caret geometry at expected drop boundary")
	}
	indicator, ok := h.e.vaultItemInsertionRect(off)
	if !ok || indicator.X != h.e.elemRect.X+caret.X-1 || indicator.Y != h.e.elemRect.Y+caret.Y || indicator.H < caret.H {
		t.Fatalf("indicator geometry %+v does not match boundary %+v", indicator, caret)
	}
	seen := -1
	if got := vaultItemInsertionOffset(true, 41, 19, func(x, y float32) int {
		if x != 41 || y != 19 {
			t.Fatalf("hit test got pointer %.1f,%.1f", x, y)
		}
		return off
	}); got != off {
		t.Fatalf("preview offset = %d, want %d", got, off)
	} else {
		seen = got
	}
	if got := vaultItemInsertionOffset(false, 90, 90, func(float32, float32) int { return 99 }); got != -1 {
		t.Fatalf("indicator remained active after pointer left: %d", got)
	}
	if h.e.Source() != "first line\nsecond line" {
		t.Fatal("showing the drop indicator mutated the source")
	}
	h.e.setCaret(seen, false)
	h.e.dropVaultItem(VaultItemDrag{Name: "Target.md"})
	if got, want := h.e.Source(), "first line[[Target]]\nsecond line"; got != want {
		t.Fatalf("drop inserted at a different boundary: %q, want %q", got, want)
	}
}
