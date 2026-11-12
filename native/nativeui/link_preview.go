package nativeui

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"muse/mdoc"
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
	imageAsset     *mdoc.ImageAsset
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

// wikiLinkSuggestions returns vault-relative Markdown paths without their
// extension. Paths remain unambiguous even when several notes share a title.
func (a *App) wikiLinkSuggestions(_ string) []string {
	if a.vault == nil || a.vault.index == nil {
		return nil
	}
	root, revision := filepath.Clean(a.vault.root), a.vault.linkRevision
	if a.wikiSuggestionReady && a.wikiSuggestionRoot == root && a.wikiSuggestionRevision == revision {
		return append([]string(nil), a.wikiSuggestionCache...)
	}
	docs := a.vault.index.Snapshot()
	items := make([]string, 0, len(docs))
	for _, doc := range docs {
		if !strings.EqualFold(filepath.Ext(doc.Path), ".md") {
			continue
		}
		items = append(items, strings.TrimSuffix(filepath.ToSlash(doc.Path), filepath.Ext(doc.Path)))
	}
	// Attachments are linkable files too. Always suggest vault-relative
	// paths so same-named images in different folders remain distinguishable.
	_ = filepath.WalkDir(a.vault.root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if entry.IsDir() {
			if path != a.vault.root && strings.HasPrefix(entry.Name(), ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if !isImageFile(path) {
			return nil
		}
		rel, err := filepath.Rel(a.vault.root, path)
		if err == nil {
			items = append(items, filepath.ToSlash(rel))
		}
		return nil
	})
	sort.Strings(items)
	a.wikiSuggestionRoot = root
	a.wikiSuggestionRevision = revision
	a.wikiSuggestionCache = append(a.wikiSuggestionCache[:0], items...)
	a.wikiSuggestionReady = true
	return append([]string(nil), a.wikiSuggestionCache...)
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
	resolved := mdoc.ResolveImageSource(target, base, root)
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
	if a.vault == nil || a.vault.index == nil {
		return "", "", false
	}
	current := ""
	if rel, err := filepath.Rel(a.vault.root, currentPath); err == nil {
		current = filepath.ToSlash(rel)
	}
	loc, ok := a.vault.index.Resolve(current, target)
	if !ok {
		return "", "", false
	}
	doc, ok := a.vault.index.Document(loc.Path)
	if !ok {
		return "", "", false
	}
	return loc.Path, doc.Source, true
}

func (a *App) wikiLinkPreviewDialog(c *ui.Context) {
	p := &a.linkPreview
	if p.target == "" || a.imagePreview.open {
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
	if a.vault == nil || a.vault.index == nil {
		return
	}
	current := ""
	if rel, err := filepath.Rel(a.vault.root, a.path); err == nil {
		current = filepath.ToSlash(rel)
	}
	anchor := a.wikiPreviewAnchor(c, p)
	vw, vh := c.Size()
	t := c.Theme()
	if imagePath := a.resolveWikiImageTarget(p.target); imagePath != "" {
		if p.imagePath != imagePath {
			p.imagePath = imagePath
			p.image = nil
			p.imageAsset = mdoc.LoadImageAsset(imagePath, "", a.invalidate)
		}
		if p.imageAsset != nil {
			if next, _ := p.imageAsset.Advance(c.Now()); next > 0 {
				c.After(next)
			}
			p.image = p.imageAsset.Bitmap()
		}
		boxRect := placeWikiPreview(ui.Rect{W: vw, H: vh}, anchor, 280, 220)
		box := ui.Box(c).Key("wiki-link-preview").Label("图片链接预览").Absolute().Left(boxRect.X).Top(boxRect.Y).Width(boxRect.W).Height(boxRect.H).
			Radius(10).Background(t.Background).Border(1, t.Border).
			Shadow(0, 10, 28, -5, ui.RGBA(0, 0, 0, 0.24)).Clip()
		if !a.updateWikiPreviewHover(c, p, box.Hovered()) {
			return
		}
		box.Children(func() {
			ui.Column(c).Fill().Padding(10).Gap(7).Children(func() {
				ui.Text(c, filepath.Base(imagePath)).FontSize(11).TextColor(t.TextMuted).SingleLine().Ellipsis("…")
				ui.Box(c).FillWidth().Height(1).Background(t.Border)
				if p.image == nil {
					ui.Text(c, "图片无法预览").Fill().Center().TextColor(t.TextMuted)
					return
				}
				bw, bh := ui.ImageSize(p.image)
				if bw <= 0 || bh <= 0 {
					ui.Text(c, "图片无法预览").Fill().Center().TextColor(t.TextMuted)
					return
				}
				fit := min(254/bw, 178/bh)
				if fit <= 0 {
					fit = 1
				}
				w, h := bw*fit, bh*fit
				ui.Box(c).Fill().Center().Children(func() {
					ui.Image(c, p.image).Key("wiki-image-hover-thumbnail").Size(w, h)
				})
			})
		})
		return
	}
	loc, ok := a.vault.index.Resolve(current, p.target)
	if !ok {
		return
	}
	doc, ok := a.vault.index.Document(loc.Path)
	if !ok {
		return
	}
	title := strings.TrimSuffix(filepath.Base(loc.Path), filepath.Ext(loc.Path))
	if strings.TrimSpace(doc.Title) != "" {
		title = doc.Title
	}
	preview := wikiPreviewMarkdown(doc.Source, loc.Offset)
	previewDoc := mdoc.Parse(preview)
	boxRect := placeWikiPreview(ui.Rect{W: vw, H: vh}, anchor, 380, 300)
	box := ui.Box(c).Key("wiki-link-preview").Label("链接预览").Absolute().Left(boxRect.X).Top(boxRect.Y).Width(boxRect.W).Height(boxRect.H).
		Radius(10).Background(t.Background).Border(1, t.Border).
		Shadow(0, 10, 28, -5, ui.RGBA(0, 0, 0, 0.24)).Clip()
	if !a.updateWikiPreviewHover(c, p, box.Hovered()) {
		return
	}
	box.Children(func() {
		ui.Column(c).Fill().Padding(14, 16).Gap(7).Children(func() {
			ui.Text(c, title).FontSize(15).FontWeight(650).SingleLine().Ellipsis("…")
			ui.Text(c, loc.Path).FontSize(10.5).TextColor(t.TextMuted).SingleLine().Ellipsis("…")
			ui.Box(c).FillWidth().Height(1).Background(t.Border)
			mdoc.View(c, previewDoc, mdoc.Options{
				BodyFontSize: 12,
				Padding:      0,
				ContentWidth: 346,
				BaseDir:      filepath.Join(a.vault.root, filepath.Dir(loc.Path)),
				AssetRoot:    a.vault.root,
				OnAsync:      a.invalidate,
				OnImagePreview: func(src string) {
					resolved := mdoc.ResolveImageSource(src, filepath.Join(a.vault.root, filepath.Dir(loc.Path)), a.vault.root)
					if resolved != "" {
						a.openImagePreview(resolved)
					}
				},
			}).Fill()
		})
	})
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

// placeWikiPreview positions the card on a vertical side of the hovered link.
// Keeping the link's y-range outside the card means the trigger remains visible
// and the pointer can move into the card without losing the hover target.
func placeWikiPreview(viewport, anchor ui.Rect, width, height float32) ui.Rect {
	const (
		edge = float32(8)
		gap  = float32(12)
	)
	width = min(width, max(viewport.W-2*edge, 1))
	height = min(height, max(viewport.H-2*edge, 1))
	x := max(viewport.X+edge, min(anchor.X, viewport.X+viewport.W-width-edge))
	below := anchor.Y + anchor.H + gap
	above := anchor.Y - height - gap
	bottom := viewport.Y + viewport.H - edge
	topEdge := viewport.Y + edge
	var y float32
	switch {
	case below+height <= bottom:
		y = below
	case above >= topEdge:
		y = above
	case bottom-below >= anchor.Y-topEdge:
		y = max(topEdge, min(below, bottom-height))
	default:
		y = max(topEdge, min(above, bottom-height))
	}
	return ui.Rect{X: x, Y: y, W: width, H: height}
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

func wikiPreviewExcerpt(source string, offset int) string {
	if source == "" {
		return "（空笔记）"
	}
	offset = max(0, min(offset, len(source)))
	start := strings.LastIndex(source[:offset], "\n") + 1
	end := min(len(source), start+1200)
	lines := strings.Split(source[start:end], "\n")
	var excerpt []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "---") || strings.HasPrefix(line, "#") {
			continue
		}
		excerpt = append(excerpt, line)
		if len(excerpt) == 5 {
			break
		}
	}
	if len(excerpt) == 0 {
		return "（没有可预览的正文）"
	}
	text := strings.Join(excerpt, "\n")
	if len([]rune(text)) > 230 {
		text = string([]rune(text)[:230]) + "…"
	}
	return text
}
