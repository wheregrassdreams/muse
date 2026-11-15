package nativeui

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	mdedit "muse/editor"
	"muse/markdown"
	linkpreview "muse/nativeui/components/linkpreview"
	markdownui "muse/nativeui/components/markdown"
	vaultindex "muse/vault/index"
)

const (
	wikiLinkPreviewDelay = 450 * time.Millisecond
	wikiLinkPreviewLeave = 220 * time.Millisecond
)

var wikiPreviewRef = regexp.MustCompile(`!?\[\[([^\]]+)\]\]`)

type wikiLinkPreviewState struct {
	target         string
	x, y           float32
	linkBounds     ui.Rect
	entered        time.Time
	leaveAt        time.Time
	overlayHovered bool
	imagePath      string
	imageAsset     *markdownui.ImageAsset
	image          *ui.Bitmap
}

// hoverWikiLink receives editor-local viewport coordinates. Keep the delay
// stable while the pointer moves across one link, and cancel it as soon as
// the pointer leaves or moves onto an embedded wiki link.
func (a *App) hoverWikiLink(target string, x, y float32) {
	a.hoverWikiLinkAt(target, x, y, ui.Rect{})
}

func (a *App) hoverWikiLinkAt(target string, x, y float32, linkBounds ui.Rect) {
	target = strings.TrimSpace(target)
	if target == "" {
		if a.linkPreview.target != "" && a.linkPreview.leaveAt.IsZero() {
			a.linkPreview.leaveAt = time.Now().Add(wikiLinkPreviewLeave)
		}
		a.invalidate()
		return
	}
	if a.linkPreview.target != target {
		a.linkPreview = wikiLinkPreviewState{target: target, entered: time.Now()}
	}
	a.linkPreview.x, a.linkPreview.y, a.linkPreview.linkBounds = x, y, linkBounds
	a.linkPreview.leaveAt = time.Time{}
	a.invalidate()
}

func (a *App) resolveVaultImage(src, base, root string) string {
	if a.vault != nil && a.vault.files != nil {
		return markdownui.ResolveImageSourceWithLookup(src, base, root, a.vault.files.FindImage)
	}
	// The catalog is built asynchronously when a vault opens. Keep the first
	// frame free of a fallback WalkDir; local/root-relative images still resolve.
	return markdownui.ResolveImageSourceWithLookup(src, base, root, nil)
}

// wikiLinkSuggestions returns vault-relative Markdown paths without their
// extension. Paths remain unambiguous even when several notes share a title.
func (a *App) wikiLinkSuggestions(_ string) []string {
	if a.vault == nil || a.vault.index == nil || a.vault.files == nil {
		return nil
	}
	root, revision, filesRevision := filepath.Clean(a.vault.root), a.vault.index.Revision(), a.vault.files.Revision()
	if a.wikiSuggestionReady && a.wikiSuggestionVault == a.vault && a.wikiSuggestionRoot == root && a.wikiSuggestionRevision == revision && a.wikiSuggestionFilesRevision == filesRevision {
		return append([]string(nil), a.wikiSuggestionCache...)
	}
	notes := a.vault.index.NotePaths()
	images := a.vault.files.ImagePaths()
	items := make([]string, 0, len(notes)+len(images))
	for _, path := range notes {
		items = append(items, strings.TrimSuffix(filepath.ToSlash(path), filepath.Ext(path)))
	}
	// Attachments are linkable files too. Always suggest vault-relative
	// paths so same-named images in different folders remain distinguishable.
	items = append(items, images...)
	sort.Strings(items)
	a.wikiSuggestionRoot = root
	a.wikiSuggestionVault = a.vault
	a.wikiSuggestionRevision = revision
	a.wikiSuggestionFilesRevision = filesRevision
	a.wikiSuggestionCache = append(a.wikiSuggestionCache[:0], items...)
	a.wikiSuggestionReady = true
	a.wikiSuggestionDetailsReady = false
	return append([]string(nil), a.wikiSuggestionCache...)
}

// wikiLinkSuggestionDetails projects titles and aliases from the shared note
// index. Targets remain vault-relative paths, which are stable for insertion
// and disambiguate notes that share a title.
func (a *App) wikiLinkSuggestionDetails(query string) []mdedit.WikiSuggestion {
	if a.vault == nil || a.vault.index == nil || a.vault.files == nil {
		return nil
	}
	targets := a.wikiLinkSuggestions(query)
	if a.wikiSuggestionDetailsReady && len(a.wikiSuggestionDetailsCache) == len(targets) {
		return append([]mdedit.WikiSuggestion(nil), a.wikiSuggestionDetailsCache...)
	}
	notesByTarget := make(map[string]vaultindex.Document)
	for _, path := range a.vault.index.NotePaths() {
		doc, ok := a.vault.index.Document(path)
		if ok {
			target := strings.TrimSuffix(filepath.ToSlash(path), filepath.Ext(path))
			notesByTarget[target] = doc
		}
	}
	details := make([]mdedit.WikiSuggestion, 0, len(targets))
	for _, target := range targets {
		if isImageFile(target) {
			details = append(details, mdedit.WikiSuggestion{Target: target, Title: filepath.Base(target)})
			continue
		}
		doc, ok := notesByTarget[target]
		if !ok {
			continue
		}
		details = append(details, mdedit.WikiSuggestion{Target: target, Title: doc.Title, Aliases: append([]string(nil), doc.Metadata().Aliases...)})
	}
	a.wikiSuggestionDetailsCache = details
	a.wikiSuggestionDetailsReady = true
	return append([]mdedit.WikiSuggestion(nil), details...)
}

func (a *App) resolveWikiImageTarget(target string) string {
	return a.resolveWikiImageTargetFrom(a.path, target)
}

func (a *App) resolveWikiImageTargetFrom(currentPath, target string) string {
	target = strings.TrimSpace(target)
	if i := strings.IndexAny(target, "#^"); i >= 0 {
		target = target[:i]
	}
	if !isImageFile(target) {
		return ""
	}
	root := a.root
	if a.vault != nil && a.vault.root != "" {
		root = a.vault.root
	}
	base := a.docDir()
	if currentPath != "" {
		base = filepath.Dir(currentPath)
	}
	resolved := a.resolveVaultImage(target, base, root)
	if resolved == "" {
		return ""
	}
	st, err := os.Stat(resolved)
	if err != nil || st.IsDir() {
		return ""
	}
	return resolved
}

// resolveWikiEmbed returns the indexed Markdown source for a note embed.
// The path and source both come from the same vault index used by link style,
// navigation, previews, and backlinks.
func (a *App) resolveWikiEmbed(currentPath, target string) (path, source string, ok bool) {
	if a.vault == nil || a.vault.index == nil || a.vault.links == nil {
		return "", "", false
	}
	current := ""
	if rel, err := filepath.Rel(a.vault.root, currentPath); err == nil {
		current = filepath.ToSlash(rel)
	}
	loc, ok := a.vault.links.Resolve(current, target)
	if !ok {
		return "", "", false
	}
	_, source, ok = a.vault.index.NoteContent(loc.Path)
	if !ok {
		return "", "", false
	}
	return loc.Path, source, true
}

func (a *App) wikiLinkPreviewDialog(c *ui.Context) {
	p := &a.linkPreview
	if p.target == "" || a.imagePreviewOpen() || a.workspaceSearch {
		return
	}
	if !p.leaveAt.IsZero() && !p.overlayHovered {
		if remaining := time.Until(p.leaveAt); remaining > 0 {
			c.After(min(remaining, 80*time.Millisecond))
		} else {
			p.target = ""
			return
		}
	}
	if remaining := time.Until(p.entered.Add(wikiLinkPreviewDelay)); remaining > 0 {
		c.After(remaining)
		return
	}
	if a.vault == nil || a.vault.index == nil || a.vault.links == nil {
		return
	}
	current := ""
	if rel, err := filepath.Rel(a.vault.root, a.path); err == nil {
		current = filepath.ToSlash(rel)
	}
	anchor := a.wikiPreviewAnchor(c, p)
	vw, vh := c.Size()
	if imagePath := a.resolveWikiImageTarget(p.target); imagePath != "" {
		if p.imagePath != imagePath {
			p.imagePath = imagePath
			p.image = nil
			p.imageAsset = markdownui.LoadImageAssetWithCacheStore(imagePath, "", a.imageCacheStore, a.invalidate)
		}
		if p.imageAsset != nil {
			if next, _ := p.imageAsset.Advance(c.Now()); next > 0 {
				c.After(next)
			}
			p.image = p.imageAsset.Bitmap()
		}
		keepOpen := true
		linkpreview.Render(c, linkpreview.ViewModel{
			Kind: linkpreview.Image, Title: filepath.Base(imagePath), Path: imagePath,
			Anchor: anchor, Viewport: ui.Rect{W: vw, H: vh}, Width: 280, Height: 220,
			OverlayHovered: p.overlayHovered, Bitmap: p.image,
		}, func(action linkpreview.Action) {
			if action.Kind == linkpreview.HoverChanged {
				keepOpen = a.updateWikiPreviewHover(c, p, action.Hovered)
			}
		})
		if !keepOpen {
			return
		}
		return
	}
	loc, ok := a.vault.links.Resolve(current, p.target)
	if !ok {
		return
	}
	titleText, source, ok := a.vault.index.NoteContent(loc.Path)
	if !ok {
		return
	}
	title := strings.TrimSuffix(filepath.Base(loc.Path), filepath.Ext(loc.Path))
	if strings.TrimSpace(titleText) != "" {
		title = titleText
	}
	preview := wikiPreviewMarkdown(source, loc.Offset)
	previewDoc := markdown.Parse(preview)
	keepOpen := true
	linkpreview.Render(c, linkpreview.ViewModel{
		Kind: linkpreview.Markdown, Title: title, Path: loc.Path,
		Anchor: anchor, Viewport: ui.Rect{W: vw, H: vh}, Width: 380, Height: 300,
		OverlayHovered: p.overlayHovered, Document: previewDoc,
		MarkdownOptions: markdownui.Options{
			BodyFontSize: 12, Padding: 0, ContentWidth: 346,
			BaseDir: filepath.Join(a.vault.root, filepath.Dir(loc.Path)), AssetRoot: a.vault.root,
			ImageCacheStore: a.imageCacheStore, ResolveImage: a.resolveVaultImage, OnAsync: a.invalidate,
			OnImagePreview: func(src string) {
				resolved := a.resolveVaultImage(src, filepath.Join(a.vault.root, filepath.Dir(loc.Path)), a.vault.root)
				if resolved != "" {
					a.openImagePreview(resolved)
				}
			},
		},
	}, func(action linkpreview.Action) {
		if action.Kind == linkpreview.HoverChanged {
			keepOpen = a.updateWikiPreviewHover(c, p, action.Hovered)
		}
	})
	if !keepOpen {
		return
	}
}

func (a *App) wikiPreviewAnchor(c *ui.Context, p *wikiLinkPreviewState) ui.Rect {
	ox, oy := float32(0), float32(0)
	if !a.flat {
		ox = a.canvasLeft(c) + cardBorder + a.panelW
		oy = float32(canvasPadding+cardBorder) + a.headerH(c)
	}
	if p.linkBounds.W > 0 && p.linkBounds.H > 0 {
		return ui.Rect{
			X: ox + p.linkBounds.X, Y: oy + p.linkBounds.Y,
			W: p.linkBounds.W, H: p.linkBounds.H,
		}
	}
	return ui.Rect{X: ox + p.x, Y: oy + p.y, W: 1, H: 1}
}

func (a *App) updateWikiPreviewHover(c *ui.Context, p *wikiLinkPreviewState, hovered bool) bool {
	if hovered {
		p.overlayHovered = true
		p.leaveAt = time.Time{}
		return true
	}
	if p.overlayHovered {
		p.overlayHovered = false
		p.leaveAt = time.Now().Add(wikiLinkPreviewLeave)
	}
	if p.leaveAt.IsZero() {
		return true
	}
	if remaining := time.Until(p.leaveAt); remaining <= 0 {
		p.target = ""
		return false
	} else {
		c.After(min(remaining, 80*time.Millisecond))
	}
	return true
}

// wikiPreviewMarkdown keeps the normal Markdown source for the native renderer,
// strips frontmatter, and flattens wiki references to their visible labels.
// The hover card is read-only, so references do not become source syntax in it.
func wikiPreviewMarkdown(source string, offset int) string {
	if strings.HasPrefix(source, "---\n") || strings.HasPrefix(source, "---\r\n") {
		lines := strings.SplitAfter(source, "\n")
		end := -1
		for i := 1; i < len(lines); i++ {
			line := strings.TrimSpace(lines[i])
			if line == "---" || line == "..." {
				end = i + 1
				break
			}
		}
		if end > 0 {
			source = strings.Join(lines[end:], "")
			offset = max(0, offset-len(strings.Join(lines[:end], "")))
		}
	}
	if offset > 0 && offset < len(source) {
		// For a heading target, show that section from its heading through the
		// next heading at the same or a higher level.
		start := strings.LastIndex(source[:offset], "\n") + 1
		line := strings.TrimSpace(source[start:offset])
		level := 0
		for level < len(line) && line[level] == '#' {
			level++
		}
		if level > 0 && level <= 6 {
			lines := strings.Split(source[start:], "\n")
			end := len(lines)
			for i := 1; i < len(lines); i++ {
				candidate := strings.TrimSpace(lines[i])
				n := 0
				for n < len(candidate) && candidate[n] == '#' {
					n++
				}
				if n > 0 && n <= level && (len(candidate) == n || candidate[n] == ' ') {
					end = i
					break
				}
			}
			source = strings.Join(lines[:end], "\n")
		}
	}
	source = wikiPreviewRef.ReplaceAllStringFunc(source, func(ref string) string {
		parts := wikiPreviewRef.FindStringSubmatch(ref)
		if len(parts) != 2 {
			return ref
		}
		target, alias, _ := strings.Cut(parts[1], "|")
		label := strings.TrimSpace(alias)
		if label == "" {
			label = strings.TrimSpace(strings.SplitN(target, "#", 2)[0])
			label = strings.TrimSuffix(filepath.Base(filepath.FromSlash(label)), filepath.Ext(label))
		}
		if strings.HasPrefix(ref, "!") {
			return label
		}
		return "**" + label + "**"
	})
	return strings.TrimSpace(source)
}
