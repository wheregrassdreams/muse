package nativeui

import (
	"bytes"
	mdoc "muse/markdown"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	inlineui "muse/nativeui/workbench/features/editor/blocks/inline"
	linkpreview "muse/nativeui/workbench/features/linkpreview"
	vaultlinks "muse/vault/links"
)

func TestWikiLinkHoverPreviewDelayAndDismissal(t *testing.T) {
	a, tt, _ := runtimeFixtureWithNotes(t, map[string]string{"Projects/Atlas/Target.md": "# Preview section\n\nPreview body that should appear in the hover card."})
	waitDocument(t, tt, func() bool {
		_, ok := a.vault.Document("Projects/Atlas/Target.md")
		return ok
	})
	a.hoverWikiLink("Projects/Atlas/Target#Preview-section", 120, 80)
	tt.Frame()
	if tt.HasText("Preview body that should appear in the hover card.") {
		t.Fatal("wiki link preview appeared before the hover delay")
	}
	time.Sleep(linkpreview.DwellDelay + 30*time.Millisecond)
	tt.Frame()
	if !tt.HasText("Preview body that should appear in the hover card.") {
		t.Fatalf("wiki preview missing after delay; text=%q", tt.Texts())
	}
	runtimeShot(t, tt, "wiki-link-preview")
	a.hoverWikiLink("", 0, 0)
	time.Sleep(linkpreview.LeaveDelay + 30*time.Millisecond)
	tt.Frame()
	if tt.HasText("Preview body that should appear in the hover card.") {
		t.Fatal("wiki preview remained after pointer left the link")
	}
}

func TestWorkspaceSearchModalCoversWikiPreview(t *testing.T) {
	a, tt, _ := runtimeFixtureWithNotes(t, map[string]string{
		"Target.md": "# Preview target\n\nThis hover preview must stay below the search dialog.",
	})
	waitDocument(t, tt, func() bool {
		_, ok := a.vault.Document("Target.md")
		return ok
	})
	trigger := ui.Rect{X: 1000, Y: 700, W: 80, H: 20}
	a.hoverWikiLinkAt("Target", trigger.X, trigger.Y, trigger)
	time.Sleep(linkpreview.DwellDelay + 20*time.Millisecond)
	a.openWorkspaceSearch()
	tt.Frame()
	if !a.workspaceSearch.IsOpen() || !tt.HasText("搜索笔记库") {
		t.Fatal("workspace search did not open over the hover preview")
	}

	// The preview is positioned partly under the dialog. A pointer over that
	// overlap must belong to the modal, not activate the obscured preview.
	tt.Move(940, 600)
	tt.Frame()
	if tt.HasText("This hover preview must stay below the search dialog") || a.linkPreview.Snapshot().OverlayHovered {
		t.Fatal("hover preview stayed visible or received the pointer above the search dialog")
	}
	runtimeShot(t, tt, "workspace-search-over-link-preview")
}

func TestWikiPreviewClosesAfterScrolledExit(t *testing.T) {
	a, tt, _ := runtimeFixtureCopy(t, filepath.Join("..", "testdata", "vault-fixture"))
	waitDocument(t, tt, func() bool {
		_, ok := a.vault.Document("Projects/Atlas/Overview.md")
		return ok
	})
	trigger := ui.Rect{X: 300, Y: 80, W: 140, H: 20}
	a.hoverWikiLinkAt("Projects/Atlas/Overview", trigger.X, trigger.Y, trigger)
	time.Sleep(linkpreview.DwellDelay + 20*time.Millisecond)
	tt.Frame()
	card, ok := tt.Find("链接预览")
	if !ok {
		t.Fatalf("preview overlay did not appear after delay: %q", tt.Texts())
	}
	runtimeShot(t, tt, "wiki-preview-trigger-visible")

	tt.Move(card.X+card.W/2, card.Y+card.H/2)
	tt.Frame()
	tt.Scroll(card.X+card.W/2, card.Y+card.H/2, 0, 90)
	tt.Frame()
	if !tt.HasText("链接预览") || !a.linkPreview.Snapshot().LeaveAt.IsZero() || !a.linkPreview.Snapshot().OverlayHovered {
		t.Fatalf("scrolling inside preview lost its hover grace: state=%+v text=%q", a.linkPreview.Snapshot(), tt.Texts())
	}
	tt.Move(0, 0)
	tt.Frame()
	time.Sleep(linkpreview.LeaveDelay + time.Millisecond)
	tt.Frame()
	if tt.HasText("链接预览") || a.linkPreview.Snapshot().Target != "" {
		t.Fatalf("preview did not close after pointer left: state=%+v text=%q", a.linkPreview.Snapshot(), tt.Texts())
	}
}

func TestWikiHoverPreviewRendersMarkdownAndRelativeImage(t *testing.T) {
	a, tt, _ := runtimeFixtureCopy(t, filepath.Join("..", "testdata", "vault-fixture"))
	path := filepath.Join(a.root, "Projects", "Atlas", "Overview.md")
	source := "---\naliases: [Preview]\n---\n# Atlas Overview\n\n## Architecture\n\nThe **rendered section** has a [link](https://example.test).\n\n![A diagram](../../Attachments/diagram.png)\n\n![Bare vault attachment](landscape.webp)\n"
	if err := os.WriteFile(path, []byte(source), 0644); err != nil {
		t.Fatal(err)
	}
	waitDocument(t, tt, func() bool {
		doc, ok := a.vault.Document("Projects/Atlas/Overview.md")
		return ok && doc.Source == source
	})
	preview := linkpreview.PreviewSource(source, strings.Index(source, "Architecture"))
	doc := mdoc.Parse(preview)
	if len(doc.Blocks) < 3 || doc.Blocks[0].Kind != mdoc.Heading || doc.Blocks[0].Level != 2 {
		t.Fatalf("hover markdown heading not preserved: %#v", doc.Blocks)
	}
	var strong, link bool
	for _, block := range doc.Blocks {
		for _, span := range block.Inlines {
			strong = strong || span.Bold && span.Text == "rendered section"
			link = link || span.Link == "https://example.test"
		}
	}
	if !strong || !link {
		t.Fatalf("hover markdown lost emphasis or link: %#v", doc.Blocks)
	}
	a.hoverWikiLink("Projects/Atlas/Overview#Architecture", 150, 90)
	time.Sleep(linkpreview.DwellDelay + 30*time.Millisecond)
	tt.Frame()
	if !tt.HasText("Architecture") || !tt.HasText("rendered section") || !tt.HasText("link") {
		t.Fatalf("rendered Markdown overlay missing expected content: %q", tt.Texts())
	}
	if tt.HasText("找不到“landscape.webp”") {
		t.Fatalf("bare vault attachment failed to resolve in preview: %q", tt.Texts())
	}
	if len(doc.Blocks) < 3 || doc.Blocks[2].Kind != mdoc.Image || doc.Blocks[2].Src != "../../Attachments/diagram.png" {
		t.Fatalf("relative image block did not reach Markdown renderer: %#v", doc.Blocks)
	}
	runtimeShot(t, tt, "wiki-link-preview-rendered-markdown")
	a.hoverWikiLink("", 0, 0)
	time.Sleep(linkpreview.LeaveDelay + 30*time.Millisecond)
	tt.Frame()
	if tt.HasText("rendered section") {
		t.Fatal("rendered preview did not close after pointer left")
	}
}

func TestWikiPreviewUsesActualHoveredLinkBounds(t *testing.T) {
	a, tt, _ := runtimeFixtureCopy(t, filepath.Join("..", "testdata", "vault-fixture"))
	waitDocument(t, tt, func() bool {
		_, ok := a.vault.Document("Projects/Atlas/Overview.md")
		return ok && a.noteEditor().Source() != ""
	})
	// runtimeFixtureCopy's 1200×800 canvas and compact property header place
	// the visible Home link around this point; target the rendered label rather
	// than the hidden source range.
	x, y := float32(540), float32(405)
	runtimeShot(t, tt, "wiki-preview-actual-link-before")
	tt.Move(x, y)
	tt.Frame()
	preview := a.linkPreview.Snapshot()
	if preview.Target != "Projects/Atlas/Overview" || preview.LinkBounds.W <= 0 || !preview.LinkBounds.Contains(preview.X, preview.Y) {
		t.Fatalf("actual pointer hover did not report the visible link bounds: preview=%+v moved=(%.1f,%.1f) source=%q texts=%q", preview, x, y, a.noteEditor().Source(), tt.Texts())
	}
	time.Sleep(linkpreview.DwellDelay + 30*time.Millisecond)
	tt.Frame()
	if _, ok := tt.Find("链接预览"); !ok {
		t.Fatalf("preview overlay did not render for the actual hovered link: %q", tt.Texts())
	}
	runtimeShot(t, tt, "wiki-preview-actual-link-hover")
}

func TestWikiLinkClickUsesVaultIndexAndHeadingAnchor(t *testing.T) {
	a, tt, note := runtimeFixtureWithNotes(t, map[string]string{
		"Projects/Source.md":       "[[Atlas/Target#Second heading]]",
		"Projects/Atlas/Target.md": "# First heading\n\nfirst\n\n## Second heading\n\nsecond",
	})
	root := filepath.Dir(note)
	source := filepath.Join(root, "Projects", "Source.md")
	target := filepath.Join(root, "Projects", "Atlas", "Target.md")
	waitDocument(t, tt, func() bool {
		_, sourceIndexed := a.vault.Document("Projects/Source.md")
		_, targetIndexed := a.vault.Document("Projects/Atlas/Target.md")
		return sourceIndexed && targetIndexed
	})
	a.openPathNow(source)
	a.openLink("wiki:Atlas/Target#Second heading")
	waitDocument(t, tt, func() bool { return a.documentPath() == target })
	if got, want := a.noteEditor().NavigationCommands().Caret(), strings.Index(a.documentSource(), "Second heading")-len("## "); got != want {
		t.Fatalf("wiki click caret = %d, want heading source offset %d", got, want)
	}
	current := a.documentPath()
	a.openWiki("note#Missing heading")
	if a.documentPath() != current {
		t.Fatalf("unresolved heading fell back to opening note %q", a.documentPath())
	}
}

func TestBacklinkListDisplaysAndNavigatesToNestedSource(t *testing.T) {
	a, tt, target := runtimeFixtureWithNotes(t, map[string]string{"Projects/Atlas/Source.md": "This note links to [[note]]."})
	root := filepath.Dir(target)
	source := filepath.Join(root, "Projects", "Atlas", "Source.md")
	waitDocument(t, tt, func() bool {
		got := a.vault.Backlinks("note.md")
		return len(got) == 1 && got[0].Source == "Projects/Atlas/Source.md"
	})
	a.openLinks()
	waitDocument(t, tt, func() bool { return !a.vaultSnapshot.LinksBusy && len(a.vaultSnapshot.Incoming) == 1 })
	if !tt.HasText("Projects/Atlas/Source.md") {
		t.Fatalf("backlink source is not visible in the backlinks dialog: %q", tt.Texts())
	}
	if err := tt.Click("Projects/Atlas/Source.md"); err != nil {
		t.Fatalf("click backlink source: %v", err)
	}
	waitDocument(t, tt, func() bool { return filepath.Clean(a.documentPath()) == filepath.Clean(source) })
}

func TestFixtureBacklinksDisplayAndNavigateNestedVaultSource(t *testing.T) {
	a, tt, _ := runtimeFixtureCopy(t, filepath.Join("..", "testdata", "vault-fixture"))
	waitDocument(t, tt, func() bool {
		_, ok := a.vault.Document("Projects/Atlas/Overview.md")
		return ok
	})
	target := filepath.Join(a.root, "Projects", "Atlas", "Overview.md")
	a.openPathNow(target)
	a.openLinks()
	waitDocument(t, tt, func() bool {
		if a.vaultSnapshot.LinksBusy {
			return false
		}
		seen := map[string]bool{}
		for _, link := range a.vaultSnapshot.Incoming {
			seen[link.Source] = true
		}
		return seen["Home.md"] && seen["Journal/2026-11-12.md"] && seen["Projects/Atlas/Design Notes.md"] && seen["Projects/Atlas/Rendering Notes.md"]
	})
	for _, path := range []string{"Home.md", "Journal/2026-11-12.md", "Projects/Atlas/Design Notes.md", "Projects/Atlas/Rendering Notes.md"} {
		if !tt.HasText(path) {
			t.Errorf("fixture backlinks UI omitted %q: %q", path, tt.Texts())
		}
		if got := strings.Count(strings.Join(tt.Texts(), "\n"), "反向链接："+path); got != 1 {
			t.Errorf("fixture backlink %q rendered %d backlink rows, want one grouped row; UI text=%q", path, got, tt.Texts())
		}
	}
	runtimeShot(t, tt, "fixture-backlinks-overview")
	if err := tt.Click("Journal/2026-11-12.md（2 处）"); err != nil {
		t.Fatalf("click nested fixture backlink: %v", err)
	}
	waitDocument(t, tt, func() bool { return a.documentPath() == filepath.Join(a.root, "Journal", "2026-11-12.md") })
}

func TestWikiLinkSuggestionsUseVaultRelativePaths(t *testing.T) {
	a, tt, _ := runtimeFixtureWithNotes(t, map[string]string{"Projects/Atlas/Note.md": "---\naliases: [Quick Tag]\n---\n# Display title\n\nnested"})
	waitDocument(t, tt, func() bool {
		_, ok := a.vault.Document("Projects/Atlas/Note.md")
		return ok && a.vaultSnapshot.Ready
	})
	want := []string{"Projects/Atlas/Note", "note"}
	got := a.wikiLinkSuggestions("Not")
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("wiki suggestion targets = %#v, want %#v", got, want)
	}
	details := a.wikiLinkSuggestionDetails("Quick Tag")
	var alias *vaultlinks.Suggestion
	for i := range details {
		if details[i].Target == "Projects/Atlas/Note" {
			alias = &details[i]
			break
		}
	}
	if alias == nil || alias.Title != "Display title" || len(alias.Aliases) != 1 || alias.Aliases[0] != "Quick Tag" {
		t.Fatalf("wiki suggestion details omitted the vault-relative alias projection: %#v", details)
	}
}

func TestResolveWikiEmbedUsesIndexedVaultSource(t *testing.T) {
	a, tt, _ := runtimeFixtureCopy(t, filepath.Join("..", "testdata", "vault-fixture"))
	waitDocument(t, tt, func() bool {
		_, ok := a.vault.Document("Projects/Atlas/Overview.md")
		return ok
	})
	path, source, ok := a.resolveWikiEmbed(a.documentPath(), "Atlas Overview")
	if !ok || path != "Projects/Atlas/Overview.md" || !strings.Contains(source, "vaultfixturedeepsearchunique") {
		t.Fatalf("resolved embedded note = %q, %v, %q", path, ok, source)
	}
	if path, source, ok := a.resolveWikiEmbed(a.documentPath(), "Missing note"); ok || path != "" || source != "" {
		t.Fatalf("unresolved embedded note = %q, %v, %q", path, ok, source)
	}
}

func TestFixtureEmbeddedNoteRendersReadOnlyWithoutLinkPreview(t *testing.T) {
	a, tt, _ := runtimeFixtureCopy(t, filepath.Join("..", "testdata", "vault-fixture"))
	waitDocument(t, tt, func() bool {
		_, ok := a.vault.Document("Projects/Atlas/Overview.md")
		return ok
	})
	a.loadDoc("![[Projects/Atlas/Overview]]")
	hostSource := a.noteEditor().Source()
	tt.Frame()
	if path, source, ok := a.resolveWikiEmbed(a.documentPath(), "Projects/Atlas/Overview"); !ok || path != "Projects/Atlas/Overview.md" || !strings.Contains(source, "vaultfixturedeepsearchunique") {
		t.Fatalf("embedded note resolver returned path=%q ok=%v source=%q", path, ok, source)
	}
	runtimeShot(t, tt, "wiki-note-embed")
	tt.Move(430, 536)
	tt.Frame()
	time.Sleep(linkpreview.DwellDelay + 30*time.Millisecond)
	tt.Frame()
	if a.noteEditor().Source() != hostSource || a.documentDirty() {
		t.Fatalf("read-only embedded content changed host source or dirty state: dirty=%v", a.documentDirty())
	}
	if a.linkPreview.Snapshot().Target != "" {
		t.Fatalf("embedded note opened a wiki hover preview for %q", a.linkPreview.Snapshot().Target)
	}
}

func TestWikiImageLinksSuggestResolveAndOpenImagePreview(t *testing.T) {
	a, tt, _ := runtimeFixtureCopy(t, filepath.Join("..", "testdata", "vault-fixture"))
	waitDocument(t, tt, func() bool {
		_, ok := a.vault.Document("Home.md")
		return ok
	})
	suggestions := a.wikiLinkSuggestions("land")
	for _, target := range []string{
		"Attachments/landscape.webp",
		"Attachments/diagram.png",
		"Attachments/photo.jpg",
		"Attachments/animation.gif",
		"Assets/diagram.png",
	} {
		if !containsString(suggestions, target) {
			t.Errorf("wiki suggestions omitted attachment %q", target)
		}
	}
	root := a.vaultSnapshot.Root
	resolved := a.resolveWikiImageTarget("Attachments/diagram.png")
	if resolved != filepath.Join(root, "Attachments", "diagram.png") {
		t.Fatalf("vault-relative image target resolved to %q", resolved)
	}
	setDocumentPathForTest(a, filepath.Join(root, "Projects", "Atlas", "Design Notes.md"))
	resolved = a.resolveWikiImageTarget("../../Attachments/landscape.webp")
	if resolved != filepath.Join(root, "Attachments", "landscape.webp") {
		t.Fatalf("note-relative image target resolved to %q", resolved)
	}
	setDocumentPathForTest(a, filepath.Join(root, "Home.md"))
	if got := a.resolveWikiLinkStyle(a.documentPath(), "Attachments/diagram.png"); got != inlineui.LinkTargetValid {
		t.Fatalf("attachment link style state = %v, want valid", got)
	}
	a.openLink("wiki:Attachments/diagram.png")
	if !a.imagePreviewOpen() {
		t.Fatal("wiki image click did not open the managed image preview surface")
	}
}

func TestWikiImageHoverShowsThumbnailBeforeFullPreviewClick(t *testing.T) {
	a, tt, _ := runtimeFixtureCopy(t, filepath.Join("..", "testdata", "vault-fixture"))
	waitDocument(t, tt, func() bool {
		_, ok := a.vault.Document("Home.md")
		return ok
	})
	a.hoverWikiLink("Attachments/diagram.png", 120, 80)
	time.Sleep(linkpreview.DwellDelay + 30*time.Millisecond)
	tt.Frame()
	if a.linkPreview.Snapshot().ImageBitmap == nil || !tt.HasText("diagram.png") {
		t.Fatalf("wiki image hover thumbnail missing: image=%v text=%q", a.linkPreview.Snapshot().ImageBitmap != nil, tt.Texts())
	}
	runtimeShot(t, tt, "wiki-image-hover-thumbnail")
	a.openLink("wiki:Attachments/diagram.png")
	if !a.imagePreviewOpen() || a.linkPreview.Snapshot().Target != "" {
		t.Fatalf("image click did not replace thumbnail with full preview: preview=%v hover=%+v", a.imagePreviewOpen(), a.linkPreview.Snapshot())
	}
	if a.linkPreview.Snapshot().ImagePath != "" || a.linkPreview.Snapshot().HasImageAsset || a.linkPreview.Snapshot().ImageBitmap != nil {
		t.Fatal("opening the full image preview retained hover thumbnail resources")
	}
}

func TestWikiAnimatedImageHoverAdvancesThumbnailFrames(t *testing.T) {
	a, tt, _ := runtimeFixtureCopy(t, filepath.Join("..", "testdata", "vault-fixture"))
	waitDocument(t, tt, func() bool {
		_, ok := a.vault.Document("Home.md")
		return ok
	})
	a.hoverWikiLink("Attachments/animation.gif", 120, 80)
	time.Sleep(linkpreview.DwellDelay + 30*time.Millisecond)
	deadline := time.Now().Add(2 * time.Second)
	for a.linkPreview.Snapshot().ImageBitmap == nil && time.Now().Before(deadline) {
		tt.Frame()
		time.Sleep(10 * time.Millisecond)
	}
	tt.Frame()
	if !a.linkPreview.Snapshot().HasImageAsset || a.linkPreview.Snapshot().ImageBitmap == nil {
		t.Fatalf("GIF thumbnail asset did not load: image=%v target=%q", a.linkPreview.Snapshot().ImageBitmap != nil, a.linkPreview.Snapshot().Target)
	}
	runtimeShot(t, tt, "wiki-gif-hover-thumbnail")
	first := append([]byte(nil), tt.Image().Pix...)
	// Advance through the controller's scheduled next-frame deadline and
	// verify the rendered thumbnail changed without exposing its asset handle.
	tt.Advance(750 * time.Millisecond)
	if bytes.Equal(first, tt.Image().Pix) {
		t.Fatal("hover GIF thumbnail did not repaint with a later frame")
	}
}

func TestVaultDocumentPathHeaderIsVaultRelative(t *testing.T) {
	a, tt, _ := runtimeFixtureCopy(t, filepath.Join("..", "testdata", "vault-fixture"))
	waitDocument(t, tt, func() bool { return a.vault != nil && a.vaultSnapshot.Ready })
	tt.Frame()
	if got := a.docPath(); got != "Home.md" {
		t.Fatalf("root note header path = %q, want Home.md", got)
	}
	if tt.HasText(filepath.Base(a.root) + "/Home.md") {
		t.Fatalf("header leaked vault root into path: %q", tt.Texts())
	}
	if !tt.HasText("Home.md") {
		t.Fatalf("root-relative title missing from header: %q", tt.Texts())
	}
	nested := filepath.Join(a.root, "Projects", "Atlas", "Overview.md")
	a.openPathNow(nested)
	tt.Frame()
	if got := a.docPath(); got != "Projects/Atlas/Overview.md" {
		t.Fatalf("nested note header path = %q, want vault-relative path", got)
	}
}

func containsString(haystack []string, needle string) bool {
	for _, value := range haystack {
		if value == needle {
			return true
		}
	}
	return false
}
