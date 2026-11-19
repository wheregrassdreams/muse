package editor

import (
	"path/filepath"
	"strings"
	"testing"

	inlineui "muse/nativeui/editor/blocks/inline"
)

const embeddedNoteMarkdown = "# Embedded Heading\n\nA **bold** body paragraph with [a link](https://example.com).\n"

func configureWikiEmbed(h *harness, resolver func(currentPath, target string) (string, string, bool)) {
	opts := h.e.Options()
	opts.CurrentPath = filepath.Join("/vault", "Host.md")
	opts.AssetRoot = "/vault"
	opts.ResolveWikiEmbed = resolver
	opts.LinkRevision++
	h.e.SetOptions(opts)
	h.e.leavesLaid = false
	h.frame()
}

func findEmbeddedNoteObject(e *Editor) *object {
	e.layoutIfNeeded()
	for _, l := range e.leaves {
		if l.view == nil {
			continue
		}
		for _, p := range l.view.disp.pieces {
			if p.obj != nil && (p.obj.embed != nil || p.obj.embedBlocked != "") {
				return p.obj
			}
		}
	}
	return nil
}

func editorDisplayText(e *Editor) string {
	e.layoutIfNeeded()
	var b strings.Builder
	for _, l := range e.leaves {
		if l.view != nil {
			b.WriteString(l.view.disp.text)
		}
	}
	return b.String()
}

func TestWikiNoteEmbedRendersMarkdownAsReadOnlyVirtualAtom(t *testing.T) {
	const host = "Host paragraph.\n\n![[Projects/Target]]\n\nHost tail."
	h := newHarness(t, host)
	var gotCurrent, gotTarget string
	configureWikiEmbed(h, func(currentPath, target string) (string, string, bool) {
		gotCurrent, gotTarget = currentPath, target
		return "/vault/Projects/Target.md", embeddedNoteMarkdown, true
	})
	if h.e.Source() != host {
		t.Fatalf("rendering embed rewrote host source: %q", h.e.Source())
	}
	if gotCurrent != "/vault/Host.md" || gotTarget != "Projects/Target" {
		t.Fatalf("resolver context=(%q,%q)", gotCurrent, gotTarget)
	}
	obj := findEmbeddedNoteObject(h.e)
	if obj == nil || obj.embed == nil {
		t.Fatalf("note embed was not rendered as a virtual editor object: %#v", obj)
	}
	child := obj.embed
	if child.Source() != embeddedNoteMarkdown || child.focused || child.opts.OnChange != nil || child.opts.OnLink != nil {
		t.Fatalf("embedded editor must be complete and read-only: source=%q focused=%v opts=%+v", child.Source(), child.focused, child.opts)
	}
	child.layoutIfNeeded()
	if len(child.leaves) < 2 || child.leaves[0].kind != kHeading || child.leaves[0].view.disp.text != "Embedded Heading" {
		t.Fatalf("embedded Markdown heading was not parsed/rendered: leaves=%+v", child.leaves)
	}
	strong := false
	for _, leaf := range child.leaves {
		if leaf.view == nil {
			continue
		}
		for _, piece := range leaf.view.disp.pieces {
			if piece.text == "bold" && piece.span.Weight >= 600 {
				strong = true
			}
		}
	}
	if !strong {
		t.Fatal("embedded Markdown formatting did not survive the nested parser/layout")
	}
	start := strings.Index(host, "![[Projects/Target]]")
	end := start + len("![[Projects/Target]]")
	a, z, ok := h.e.objectAt(start)
	if !ok || a != start || z != end {
		t.Fatalf("virtual atom host source range=(%d,%d,%v), want (%d,%d,true)", a, z, ok, start, end)
	}
	if h.e.linkAt(start+2) != "" {
		t.Fatal("embedded wiki syntax should not behave as a navigation link")
	}
	h.e.Select(start, end)
	if h.e.SelectedText() != host[start:end] || h.e.Source() != host {
		t.Fatalf("selection escaped host source: selected=%q source=%q", h.e.SelectedText(), h.e.Source())
	}
	h.e.Replace(start, end, "host changed")
	if child.Source() != embeddedNoteMarkdown {
		t.Fatalf("editing the host embed syntax changed its target note: %q", child.Source())
	}
}

func TestWikiNoteEmbedCacheTracksSourcePathAndWidth(t *testing.T) {
	const host = "Host.\n\n![[Target]]\n"
	h := newHarness(t, host)
	targetSource := embeddedNoteMarkdown
	resolver := func(currentPath, target string) (string, string, bool) {
		if currentPath != "/vault/Host.md" || target != "Target" {
			t.Fatalf("resolver context=(%q,%q)", currentPath, target)
		}
		return "Projects/Target.md", targetSource, true
	}
	configureWikiEmbed(h, resolver)
	first := findEmbeddedNoteObject(h.e)
	if first == nil || first.embed == nil {
		t.Fatalf("first embed missing: %#v", first)
	}
	if first.embed.opts.CurrentPath != "/vault/Projects/Target.md" {
		t.Fatalf("relative path was not rooted for child context: %q", first.embed.opts.CurrentPath)
	}
	firstH, firstW := first.h, first.embed.colW
	if first.embed.docH <= 0 || firstH <= first.embed.docH {
		t.Fatalf("object height %v does not include measured child height %v", firstH, first.embed.docH)
	}
	targetSource = "# Updated heading\n\nA much longer paragraph for a reflow at the narrower width.\n"
	opts := h.e.Options()
	opts.LinkRevision++
	h.e.SetOptions(opts)
	h.frame()
	updated := findEmbeddedNoteObject(h.e)
	if updated == nil || updated.embed == nil || !strings.Contains(updated.embed.Source(), "Updated heading") {
		t.Fatalf("link revision did not refresh cached target source: %#v", updated)
	}
	h.tt.SetSize(560, 700)
	h.frame()
	narrow := findEmbeddedNoteObject(h.e)
	if narrow == nil || narrow.embed == nil || narrow.w >= first.w || narrow.embed.colW >= firstW {
		t.Fatalf("embed did not reflow at the new width: old=(%v,%v) new=(%v,%v)", first.w, firstW, narrow.w, narrow.embed.colW)
	}
}

func TestWikiNoteEmbedInvalidTargetFallsBackToLinkLabel(t *testing.T) {
	const host = "Before ![[Missing note]] after."
	h := newHarness(t, host)
	configureWikiEmbed(h, func(_, _ string) (string, string, bool) { return "", "", false })
	if obj := findEmbeddedNoteObject(h.e); obj != nil {
		t.Fatalf("invalid target produced an embedded object: %+v", obj)
	}
	if h.e.Source() != host {
		t.Fatalf("invalid target changed source: %q", h.e.Source())
	}
	if !strings.Contains(editorDisplayText(h.e), "Missing note") {
		t.Fatalf("invalid embed lost its visible target label: display=%q", editorDisplayText(h.e))
	}
}

func TestWikiNoteEmbedResolvesNestedLinksFromEmbeddedPath(t *testing.T) {
	h := newHarness(t, "Host.\n\n![[Target]]\n")
	var nestedLinkPath string
	opts := h.e.Options()
	opts.CurrentPath = "/vault/Host.md"
	opts.AssetRoot = "/vault"
	opts.ResolveWikiEmbed = func(_, _ string) (string, string, bool) {
		return "Projects/Target.md", "Nested [[Reference]]", true
	}
	opts.ResolveWikiLink = func(currentPath, target string) inlineui.LinkTargetState {
		if target == "Reference" {
			nestedLinkPath = currentPath
		}
		if currentPath == "/vault/Projects/Target.md" && target == "Reference" {
			return inlineui.LinkTargetValid
		}
		return inlineui.LinkTargetInvalid
	}
	h.e.SetOptions(opts)
	h.frame()
	embed := findEmbeddedNoteObject(h.e)
	if embed == nil || embed.embed == nil {
		t.Fatalf("embedded target missing: %#v", embed)
	}
	if embed.embed.opts.CurrentPath != "/vault/Projects/Target.md" || nestedLinkPath != embed.embed.opts.CurrentPath {
		t.Fatalf("nested link resolver path=%q child path=%q", nestedLinkPath, embed.embed.opts.CurrentPath)
	}
	if got := linkStyleAt(t, embed.embed, strings.Index(embed.embed.Source(), "Reference")); got.DottedUnderline {
		t.Fatalf("nested wiki link was resolved against the host path: %+v", got)
	}
}

func TestWikiNoteEmbedInheritsFrontmatterVisibility(t *testing.T) {
	h := newHarness(t, "Host.\n\n![[Target]]")
	opts := h.e.Options()
	opts.HideFrontmatter = true
	opts.CurrentPath = "/vault/Host.md"
	opts.AssetRoot = "/vault"
	opts.ResolveWikiEmbed = func(_, _ string) (string, string, bool) {
		return "Target.md", "---\ntitle: Hidden metadata\n---\n\nVisible body.\n", true
	}
	h.e.SetOptions(opts)
	h.frame()
	obj := findEmbeddedNoteObject(h.e)
	if obj == nil || obj.embed == nil {
		t.Fatalf("embedded target missing: %#v", obj)
	}
	got := editorDisplayText(obj.embed)
	if strings.Contains(got, "Hidden metadata") || !strings.Contains(got, "Visible body") {
		t.Fatalf("embedded frontmatter visibility did not match host: %q", got)
	}
}

func TestWikiNoteEmbedCycleIsStoppedAtRepeatedAbsolutePath(t *testing.T) {
	const host = "Host.\n\n![[A]]\n"
	h := newHarness(t, host)
	resolver := func(currentPath, target string) (string, string, bool) {
		switch {
		case currentPath == "/vault/A.md" && target == "B":
			return "B.md", "# B\n\n![[A]]\n", true
		case currentPath == "/vault/Host.md" && target == "A":
			return "A.md", "# A\n\n![[B]]\n", true
		case currentPath == "/vault/B.md" && target == "A":
			return "A.md", "# A\n\n![[B]]\n", true
		default:
			return "", "", false
		}
	}
	configureWikiEmbed(h, resolver)
	outer := findEmbeddedNoteObject(h.e)
	if outer == nil || outer.embed == nil {
		t.Fatalf("outer A embed missing: %#v", outer)
	}
	blocked := findEmbeddedNoteObject(outer.embed)
	if blocked == nil || blocked.embed == nil {
		t.Fatalf("nested B embed missing: %#v", blocked)
	}
	blocked = findEmbeddedNoteObject(blocked.embed)
	if blocked == nil || blocked.embed != nil || !strings.Contains(blocked.embedBlocked, "循环") {
		t.Fatalf("A→B→A cycle was not stopped with a visible blocked state: %#v", blocked)
	}
}

func TestWikiNoteEmbedScreenshot(t *testing.T) {
	h := newHarness(t, "Host note.\n\n![[Target]]\n\nTail.")
	opts := h.e.Options()
	opts.CurrentPath = "/vault/Host.md"
	opts.AssetRoot = "/vault"
	opts.ResolveWikiEmbed = func(_, _ string) (string, string, bool) {
		return "Target.md", "# Embedded Heading\n\nA **bold** body with [[Nested target]].\n", true
	}
	opts.ResolveWikiLink = func(currentPath, target string) inlineui.LinkTargetState {
		if currentPath == "/vault/Target.md" && target == "Nested target" {
			return inlineui.LinkTargetValid
		}
		return inlineui.LinkTargetUnknown
	}
	h.e.SetOptions(opts)
	h.frame()
	h.shot("wiki-note-embed")
	if findEmbeddedNoteObject(h.e) == nil {
		t.Fatal("screenshot did not contain a parsed note embed")
	}
}
