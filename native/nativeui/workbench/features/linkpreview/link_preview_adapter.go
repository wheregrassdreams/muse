package nativeui

import (
	"path/filepath"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	linkpreview "muse/nativeui/workbench/features/linkpreview"
	markdownui "muse/nativeui/workbench/features/markdown"
	vaultlinks "muse/vault/links"
)

// hoverWikiLink receives editor-local viewport coordinates. Keep the delay
// stable while the pointer moves across one link, and cancel it as soon as
// the pointer leaves or moves onto an embedded wiki link.
func (a *App) hoverWikiLink(target string, x, y float32) {
	a.hoverWikiLinkAt(target, x, y, ui.Rect{})
}

func (a *App) hoverWikiLinkAt(target string, x, y float32, linkBounds ui.Rect) {
	a.linkPreviewHandle.Hover(strings.TrimSpace(target), x, y, linkBounds, time.Now())
	a.invalidate()
}

func (a *App) resolveVaultImage(src, base, root string) string {
	if a.vault != nil {
		return markdownui.ResolveImageSourceWithLocalFilesAndLookup(src, base, root, a.imageLocalFiles, a.vault.FindImage)
	}
	// The catalog is built asynchronously when a vault opens. Keep the first
	// frame free of a fallback WalkDir; local/root-relative images still resolve.
	return markdownui.ResolveImageSourceWithLocalFilesAndLookup(src, base, root, a.imageLocalFiles, nil)
}

// wikiLinkSuggestions returns vault-relative Markdown paths without their
// extension. Paths remain unambiguous even when several notes share a title.
func (a *App) wikiLinkSuggestions(_ string) []string {
	suggestions := a.wikiSuggestionSnapshot()
	if suggestions == nil {
		return nil
	}
	items := make([]string, len(suggestions))
	for i, suggestion := range suggestions {
		items[i] = suggestion.Target
	}
	return items
}

func (a *App) wikiSuggestionSnapshot() []vaultlinks.Suggestion {
	if a.vault == nil || !a.vaultSnapshot.Ready {
		return nil
	}
	return a.vault.WikiSuggestions()
}

func (a *App) clearWikiSuggestionCache() {
	if a != nil && a.vault != nil {
		a.vault.ClearWikiSuggestions()
	}
}

// wikiLinkSuggestionDetails projects titles and aliases from the shared note
// index. Targets remain vault-relative paths, which are stable for insertion
// and disambiguate notes that share a title.
func (a *App) wikiLinkSuggestionDetails(_ string) []vaultlinks.Suggestion {
	return a.wikiSuggestionSnapshot()
}

func (a *App) resolveWikiImageTarget(target string) string {
	return a.resolveWikiImageTargetFrom(a.documentPath(), target)
}

func (a *App) resolveWikiImageTargetFrom(currentPath, target string) string {
	if a.vault == nil || !a.vaultSnapshot.Ready || a.vaultSnapshot.Root == "" {
		return ""
	}
	path, ok := a.vault.ResolveImageTarget(currentPath, target)
	if !ok {
		return ""
	}
	root := a.vaultSnapshot.Root
	path = markdownui.ResolveImageSourceWithLocalFilesAndLookup(path, "", root, a.imageLocalFiles, nil)
	if path == "" || a.imageLocalFiles == nil {
		return ""
	}
	info, err := a.imageLocalFiles.Stat(path)
	if err != nil || info == nil || info.IsDir() {
		return ""
	}
	return path
}

// resolveWikiEmbed returns the indexed Markdown source for a note embed.
// The path and source both come from the same vault index used by link style,
// navigation, previews, and backlinks.
func (a *App) resolveWikiEmbed(currentPath, target string) (path, source string, ok bool) {
	if a.vault == nil || !a.vaultSnapshot.Ready {
		return "", "", false
	}
	current := ""
	if rel, err := filepath.Rel(a.vaultSnapshot.Root, currentPath); err == nil {
		current = filepath.ToSlash(rel)
	}
	loc, ok := a.vault.ResolveLink(current, target)
	if !ok {
		return "", "", false
	}
	_, source, ok = a.vault.NoteContent(loc.Path)
	if !ok {
		return "", "", false
	}
	return loc.Path, source, true
}

func (a *App) wikiLinkPreviewDialog(c *ui.Context) {
	a.linkPreviewHandle.RenderActive(c, linkpreview.Ports{
		Suppressed: func() bool { return a.imagePreviewOpen() || a.workspaceSearch.IsOpen() },
		ResolveImage: func(target string) (string, bool) {
			path := a.resolveWikiImageTarget(target)
			return path, path != ""
		},
		ResolveNote: a.resolveWikiPreviewNote,
		LoadImage: func(path string) *markdownui.ImageAsset {
			return markdownui.LoadImageAssetWithLocalFiles(path, "", a.imageLocalFiles, a.imageCacheStore, nil, a.invalidate)
		},
		Anchor: func(c *ui.Context, state linkpreview.Snapshot) ui.Rect { return a.wikiPreviewAnchor(c, state) },
		MarkdownOptions: func(note linkpreview.NoteTarget) markdownui.Options {
			return markdownui.Options{
				BodyFontSize: 12, Padding: 0, ContentWidth: 346,
				ImageCacheStore: a.imageCacheStore, LocalFiles: a.imageLocalFiles,
				ResolveImage: a.resolveVaultImage, OnAsync: a.invalidate,
				OnImagePreview: func(src string) {
					base := filepath.Join(note.Root, filepath.Dir(note.Path))
					resolved := a.resolveVaultImage(src, base, note.Root)
					if resolved != "" {
						a.openImagePreview(resolved)
					}
				},
			}
		},
	})
}

func (a *App) resolveWikiPreviewNote(target string) (linkpreview.NoteTarget, bool) {
	if a.vault == nil || !a.vaultSnapshot.Ready || a.vaultSnapshot.Root == "" {
		return linkpreview.NoteTarget{}, false
	}
	current := ""
	if rel, err := filepath.Rel(a.vaultSnapshot.Root, a.documentPath()); err == nil {
		current = filepath.ToSlash(rel)
	}
	location, ok := a.vault.ResolveLink(current, target)
	if !ok {
		return linkpreview.NoteTarget{}, false
	}
	title, source, ok := a.vault.NoteContent(location.Path)
	if !ok {
		return linkpreview.NoteTarget{}, false
	}
	return linkpreview.NoteTarget{
		Title: title, Path: location.Path, Source: source,
		Offset: location.Offset, Root: a.vaultSnapshot.Root,
	}, true
}

func (a *App) wikiPreviewAnchor(c *ui.Context, p linkpreview.Snapshot) ui.Rect {
	ox, oy := float32(0), float32(0)
	if !a.flat {
		ox = a.canvasLeft(c) + cardBorder + a.panelW
		oy = float32(canvasPadding+cardBorder) + a.headerH(c)
	}
	if p.LinkBounds.W > 0 && p.LinkBounds.H > 0 {
		return ui.Rect{
			X: ox + p.LinkBounds.X, Y: oy + p.LinkBounds.Y,
			W: p.LinkBounds.W, H: p.LinkBounds.H,
		}
	}
	return ui.Rect{X: ox + p.X, Y: oy + p.Y, W: 1, H: 1}
}
