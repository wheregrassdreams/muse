// Package nativeui is Muse's fully native shell: MyGo draws the window, the
// icon rail, the context panel and the Markdown editor, with no webview and
// no frontend build.
//
// The layout is a 56px icon rail, one shared context panel (files / outline
// / search / AI) and the editor card, which shows the document as Typora
// does: one surface the user writes on, rendered as it reads.
package nativeui

// The composition root adapts vault, image, and viewport capabilities to the
// link-preview feature. Hover and view state remain inside its controller.
func (a *App) linkPreviewPorts() linkpreview.Ports {
	return linkpreview.Ports{
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
					if resolved := a.resolveVaultImage(src, base, note.Root); resolved != "" {
						a.openImagePreview(resolved)
					}
				},
			}
		},
	}
}

func (a *App) hoverWikiLink(target string, x, y float32) {
	a.hoverWikiLinkAt(target, x, y, ui.Rect{})
}

func (a *App) hoverWikiLinkAt(target string, x, y float32, bounds ui.Rect) {
	if a == nil || a.linkPreview == nil {
		return
	}
	a.linkPreview.Hover(strings.TrimSpace(target), x, y, bounds)
	a.invalidate()
}

func (a *App) wikiLinkPreviewDialog(c *ui.Context) {
	if a != nil && a.linkPreview != nil {
		a.linkPreview.Render(c)
	}
}

func (a *App) resolveVaultImage(src, base, root string) string {
	if a.vault != nil {
		return markdownui.ResolveImageSourceWithLocalFilesAndLookup(src, base, root, a.imageLocalFiles, a.vault.FindImage)
	}
	// Avoid a fallback WalkDir during the first frame while the vault catalog
	// is loading. Local and root-relative image references still resolve.
	return markdownui.ResolveImageSourceWithLocalFilesAndLookup(src, base, root, a.imageLocalFiles, nil)
}

func (a *App) wikiSuggestionSnapshot() []vaultlinks.Suggestion {
	if a == nil || a.vault == nil || !a.vaultSnapshot.Ready {
		return nil
	}
	return a.vault.WikiSuggestions()
}

func (a *App) wikiLinkSuggestions(_ string) []string {
	suggestions := a.wikiSuggestionSnapshot()
	items := make([]string, len(suggestions))
	for i, suggestion := range suggestions {
		items[i] = suggestion.Target
	}
	return items
}

func (a *App) wikiLinkSuggestionDetails(_ string) []vaultlinks.Suggestion {
	return a.wikiSuggestionSnapshot()
}

func (a *App) clearWikiSuggestionCache() {
	if a != nil && a.vault != nil {
		a.vault.ClearWikiSuggestions()
	}
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
	path = markdownui.ResolveImageSourceWithLocalFilesAndLookup(path, "", a.vaultSnapshot.Root, a.imageLocalFiles, nil)
	if path == "" || a.imageLocalFiles == nil {
		return ""
	}
	info, err := a.imageLocalFiles.Stat(path)
	if err != nil || info == nil || info.IsDir() {
		return ""
	}
	return path
}

func (a *App) resolveWikiEmbed(currentPath, target string) (path, source string, ok bool) {
	if a.vault == nil || !a.vaultSnapshot.Ready {
		return "", "", false
	}
	current := ""
	if rel, err := filepath.Rel(a.vaultSnapshot.Root, currentPath); err == nil {
		current = filepath.ToSlash(rel)
	}
	location, ok := a.vault.ResolveLink(current, target)
	if !ok {
		return "", "", false
	}
	_, source, ok = a.vault.NoteContent(location.Path)
	if !ok {
		return "", "", false
	}
	return location.Path, source, true
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

func (a *App) wikiPreviewAnchor(c *ui.Context, preview linkpreview.Snapshot) ui.Rect {
	ox, oy := float32(0), float32(0)
	if !a.flat {
		ox = a.canvasLeft(c) + cardBorder + a.panelW
		oy = float32(canvasPadding+cardBorder) + a.headerH(c)
	}
	if preview.LinkBounds.W > 0 && preview.LinkBounds.H > 0 {
		return ui.Rect{X: ox + preview.LinkBounds.X, Y: oy + preview.LinkBounds.Y, W: preview.LinkBounds.W, H: preview.LinkBounds.H}
	}
	return ui.Rect{X: ox + preview.X, Y: oy + preview.Y, W: 1, H: 1}
}

func (a *App) resolveWikiLinkStyle(currentPath, target string) inlineui.LinkTargetState {
	if a.vault == nil || !a.vaultSnapshot.Ready {
		return inlineui.LinkTargetUnknown
	}
	current := ""
	if currentPath == "" {
		currentPath = a.documentPath()
	}
	if currentPath != "" {
		if rel, err := filepath.Rel(a.vaultSnapshot.Root, currentPath); err == nil {
			current = filepath.ToSlash(rel)
		}
	}
	if _, ok := a.vault.ResolveLink(current, target); ok {
		return inlineui.LinkTargetValid
	}
	if a.resolveWikiImageTargetFrom(currentPath, target) != "" {
		return inlineui.LinkTargetValid
	}
	return inlineui.LinkTargetInvalid
}

// openLink opens a link Cmd-clicked in the document: a heading of the
// document by its anchor, a document of the workspace by its path, or a
// page in the browser.
func (a *App) openLink(href string) {
	target := vaultlinks.ParseNavigationTarget(href)
	switch target.Kind {
	case vaultlinks.NavigationEmpty:
		return
	case vaultlinks.NavigationWiki:
		a.openWiki(target.Value)
		return
	case vaultlinks.NavigationHeading:
		if a.noteEditor() == nil {
			return
		}
		slug := target.Value
		for _, h := range a.noteEditor().NavigationCommands().Headings() {
			if markdownsyntax.HeadingSlug(h.Text) == strings.ToLower(slug) {
				a.noteEditor().NavigationCommands().NavigateToOffset(h.Offset)
				return
			}
		}
		return
	case vaultlinks.NavigationExternal:
		a.launchExternal(target.Value)
		return
	}
	if a.indexedLink(target.Value) {
		return
	}
	p := target.Value
	if !filepath.IsAbs(p) {
		p = filepath.Join(a.docDir(), filepath.FromSlash(p))
	}
	if kind, err := a.pathKind(p); err == nil && kind == vaultfiles.EntryRegular && doctypes.IsEditablePath(p) {
		a.requestOpen(p)
		return
	}
	a.launchExternal(p)
}

func (a *App) launchExternal(target string) {
	if a.openExternal == nil {
		return
	}
	go func() {
		if err := a.openExternal(target); err != nil {
			mygo.RunOnMain(func() { a.notice(err.Error()) })
		}
	}()
}

// openWiki opens a `[[wiki link]]` by its target (see resolveWiki).
func (a *App) openWiki(target string) {
	a.linkPreview.Clear()
	if imagePath := a.resolveWikiImageTarget(target); imagePath != "" {
		a.pendingWiki = ""
		a.openImagePreview(imagePath)
		return
	}
	// The vault index is the single authority for note path and anchor
	// resolution. In particular, do not fall back to a path-only resolver
	// after an anchor failed to resolve: that would open the note while
	// silently ignoring the requested heading or block.
	if a.vault == nil || !a.vaultSnapshot.Ready {
		a.pendingWiki = target
		return
	}
	a.pendingWiki = ""
	a.indexedLink(target)
}
