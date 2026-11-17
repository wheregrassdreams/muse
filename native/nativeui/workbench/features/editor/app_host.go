// Package nativeui is Muse's fully native shell: MyGo draws the window, the
// icon rail, the context panel and the Markdown editor, with no webview and
// no frontend build.
//
// The layout is a 56px icon rail, one shared context panel (files / outline
// / search / AI) and the editor card, which shows the document as Typora
// does: one surface the user writes on, rendered as it reads.
package nativeui

func (a *App) editorContent(c *ui.Context) {
	if a.imageDocumentOpen() {
		a.renderFocusedPane(c)
		return
	}
	if a.root != "" && a.documentPath() == "" && a.documentSource() == "" {
		model := vaulthome.DefaultViewModel(componenticons.Edit(), componenticons.Search())
		vaulthome.Render(c, model, func(action vaulthome.Action) {
			switch action {
			case vaulthome.NewDocument:
				a.newFileIn(a.fileTreeState.CreateTarget(a.root, a.selectedPath, a.catalogIsDirectory(a.selectedPath)))
			case vaulthome.OpenFile:
				a.openFileDialog()
			}
		})
		return
	}
	ui.Column(c).Fill().Children(func() {
		opts := a.editorOptions()
		if !a.flat {
			a.editorHeader(c)
			a.configureActiveNote(opts.TopInset)
			a.renderEditorPane(c)
			return
		}
		opts.TopInset = a.headerH(c)
		a.configureActiveNote(opts.TopInset)
		a.renderEditorPane(c)
		var scrollY float32
		if editor := a.noteEditor(); editor != nil {
			scrollY = editor.NavigationCommands().ScrollY()
		}
		windowcontrols.RenderHeaderFade(c, &a.headerFadeCache, a.headerFade, a.headerH(c), scrollY, a.windowBG())
		a.editorHeader(c)
	})
}

func (a *App) renderEditorPane(c *ui.Context) {
	if a.windows == nil {
		return
	}
	pane := a.focusedPane()
	state := workspace.State{Version: 1}
	if a.workspaceSession != nil {
		state = a.workspaceSession.Snapshot()
	}
	if state.ActiveTab == "" || a.workspacePresenter == nil {
		a.windows.RenderPane(c, "main", pane)
		return
	}
	a.workspacePresenter.Render(c, state, "main", func(c *ui.Context, host window.HostID, renderedPane window.PaneID) {
		a.windows.RenderPane(c, host, renderedPane)
	}, func(action workspacepresenter.Action) {
		if a.workspaceSession == nil {
			return
		}
		switch action.Kind {
		case workspacepresenter.FocusPane:
			if err := a.syncWorkspaceDocument(); err != nil {
				a.notice(err.Error())
			}
			a.workspacePane = window.PaneID(action.PaneID)
			_ = a.workspaceSession.Focus(action.TabID, action.PaneID)
			a.editorPaneHandle = a.workspacePaneHandles[a.workspacePane]
			if !a.editorPaneHandle.IsZero() {
				a.windows.Focus(a.editorPaneHandle)
			}
		case workspacepresenter.SplitRatioChanged:
			if err := a.workspaceSession.SetSplitRatio(action.TabID, action.SplitPath, action.Ratio); err != nil {
				a.notice(err.Error())
			}
		}
	})
}

func (a *App) editorHeader(c *ui.Context) {
	t := c.Theme()
	// Windows and Linux put the view's own window buttons at the top right
	// (frame.go); the header keeps them clear. macOS keeps the system's
	// traffic lights at the left, and c.TitleBar says how much they take.
	bar := c.TitleBar()
	padRight := 18 + bar.Right
	if windowcontrols.Visible() {
		padRight += windowcontrols.RightPadding()
	}
	header := ui.Row(c).Height(a.headerH(c)).Shrink(0).Padding(0, padRight, 0, 6).Gap(2).
		DragWindow().AlignItems(ui.Center)
	if a.flat {
		// Over the document, which scrolls under it: placed by its own
		// column, in window DIPs, instead of taking a row of it.
		header.Absolute().Top(0).Left(0).Right(0)
	}
	header.Children(func() {
		// Room for the app's sidebar toggle, which stays at the content's
		// left edge whether the panel is open or folded: folding the panel
		// brings this header over to it, and the header keeps the seat.
		ui.Box(c).Width(max(float32(30), bar.Left+38-a.panelW)).Shrink(0)
		tint := iconTint(t)
		if a.canNav(-1) {
			a.smallIcon(c, componenticons.ChevronLeft(), locales.ToolbarBack(), func() { a.nav(-1) })
		} else {
			a.smallIconTint(c, componenticons.ChevronLeft(), locales.ToolbarBack(), nil, tint.Alpha(0.35))
		}
		if a.canNav(1) {
			a.smallIcon(c, componenticons.Chevron(), locales.ToolbarForward(), func() { a.nav(1) })
		} else {
			a.smallIconTint(c, componenticons.Chevron(), locales.ToolbarForward(), nil, tint.Alpha(0.35))
		}
		// The path sits in the middle of what is left, its end kept: the file
		// name is the part that says which document this is.
		ui.Row(c).Grow(1).MinWidth(0).Justify(ui.Center).Padding(8, 0).Children(func() {
			vw, _ := c.Size()
			avail := vw - railWidth - a.panelW - 220
			ui.Text(c, a.tailPath(c, a.docPath(), 12.5, avail)).
				FontSize(12.5).TextColor(t.TextMuted).SingleLine()
		})
		if a.imageDocumentOpen() {
			a.smallIcon(c, componenticons.Image(), locales.ToolbarGlobalPreview(), func() { a.openImagePreview(a.imageDocumentPath()) })
		}
		if label := a.noteEditor().ModalCommands().ModeLabel(); label != "" {
			ui.Text(c, label).FontSize(11).TextColor(t.TextMuted).Label(locales.DocumentMode())
		}
		if a.status != "" {
			ui.Text(c, a.status).FontSize(11).TextColor(t.TextMuted).SingleLine().Ellipsis("…")
		}
		if a.documentDirty() && !a.imageDocumentOpen() {
			a.smallIcon(c, componenticons.Save(), locales.MenuSave(), func() { a.save() })
		}
		if a.propertiesPanel.Enabled() {
			a.smallIconTint(c, componenticons.Info(), locales.DocumentInfo(), func() { a.propertiesPanel.Open() }, tint)
		} else {
			info := a.smallIconTint(c, componenticons.Info(), locales.DocumentInfo(), nil, tint)
			info.Menu(func(m *ui.Menu) {
				statistics := documentsave.AnalyzeText(a.documentSource())
				m.Item(locales.DocumentPathValue(a.docPath()))
				m.Item(locales.DocumentCharacterLineCount(statistics.Characters, statistics.Lines))
				if a.metadataReader != nil {
					if metadata, err := a.metadataReader.ReadMetadata(a.documentPath()); err == nil && !metadata.ModifiedAt.IsZero() {
						m.Item(locales.DocumentModifiedAt(metadata.ModifiedAt))
					}
				}
				m.Separator()
				if m.Item(locales.FilesInFinder()).Chosen() && a.documentPath() != "" {
					mygo.Shell.ShowItemInFolder(a.documentPath())
				}
			})
		}
		more := a.smallIconTint(c, componenticons.More(), locales.DocumentMore(), nil, tint)
		more.Menu(func(m *ui.Menu) {
			if !a.imageDocumentOpen() && doctypes.IsMarkdownPath(a.documentPath()) {
				a.exportMenu.AddMenu(m)
				m.Separator()
			}
			if m.Item(locales.DocumentHistoryMenu()).Chosen() {
				a.openHistory()
			}
			if m.Item(locales.DocumentRecoveryMenu()).Chosen() {
				a.openRecovery()
			}
			if m.Item(locales.DocumentLinksMenu()).Chosen() {
				a.openLinks()
			}
			if m.Item(locales.MenuSave()).Chosen() {
				a.save()
			}
			if m.Item(locales.DocumentCopyPathMenu()).Chosen() && a.documentPath() != "" {
				clipboardWrite(a.documentPath())
				a.notice(locales.DocumentCopiedPath())
			}
			if m.Item(locales.FilesSetWorkspace()).Chosen() && a.documentPath() != "" {
				a.setWorkspace(filepath.Dir(a.documentPath()))
			}
		})
	})
}

func (a *App) snapshotDoc() (dirty bool, name, path, source, revision string) {
	a.onMain(func() {
		dirty, name, path, source = a.documentDirty(), a.displayName(), a.documentPath(), a.diskSource()
		if a.noteEditor() != nil {
			snapshot := a.noteEditor().Snapshot()
			dirty, path, source = snapshot.Dirty, snapshot.Path, snapshot.DiskSource
			revision = string(snapshot.Revision)
		}
	})
	return
}

func (a *App) openFolderDialog() {
	if a.busy {
		return
	}
	a.startBusy()
	go func() {
		paths, err := openDialog(mygo.OpenDialogOptions{
			Parent:    a.win,
			Title:     locales.CommonDialogFolderTitle(),
			Directory: true,
		})
		a.finish(func() {
			if err == nil && len(paths) > 0 {
				a.setWorkspace(paths[0])
			}
		})
	}()
}

func (a *App) openFileDialog() {
	if a.busy {
		return
	}
	imageExtensions := doctypes.ImageExtensions()
	for i := range imageExtensions {
		imageExtensions[i] = strings.TrimPrefix(imageExtensions[i], ".")
	}
	filters := append(doctypes.EditableExtensions(), imageExtensions...)
	a.startBusy()
	go func() {
		paths, err := openDialog(mygo.OpenDialogOptions{
			Parent: a.win,
			Title:  locales.CommonDialogFileTitle(),
			Filters: []mygo.FileFilter{
				{Name: locales.CommonDialogOpenFileFilter(), Extensions: filters},
			},
		})
		if err != nil || len(paths) == 0 {
			a.finish(nil)
			return
		}
		if doctypes.IsImagePath(paths[0]) {
			a.finish(func() { a.openImageDocument(paths[0], true) })
			return
		}
		if !a.confirmDiscardOffUI() {
			a.finish(nil)
			return
		}
		if a.noteEditor() == nil {
			a.finish(nil)
			return
		}
		note, err := a.readDocument(paths[0])
		data := []byte(note.Source)
		if err != nil {
			errorDialog(locales.DialogOpenFailed(), err.Error())
			a.finish(nil)
			return
		}
		if !doctypes.IsTextData(data) {
			errorDialog(locales.DialogCannotOpen(), locales.DialogNotTextFile())
			a.finish(nil)
			return
		}
		a.finish(func() { a.applyOpen(paths[0], string(data)) })
	}()
}

func (a *App) openPathNow(path string) {
	if doctypes.IsImagePath(path) {
		a.openImageDocument(path, true)
		return
	}
	if !doctypes.IsEditablePath(path) {
		a.notice(locales.FilesUnsupportedType(filepath.Base(path)))
		a.invalidate()
		return
	}
	if a.noteEditor() == nil {
		a.notice("document pane is not initialized")
		return
	}
	note, err := documentsave.OpenDocument(a.documentRepository, path)
	if err != nil {
		if errors.Is(err, documentsave.ErrNotText) {
			a.notice(locales.DocumentNotTextWithName(filepath.Base(path)))
		} else {
			a.notice(locales.CommonOpenFileFailed(err.Error()))
		}
		a.invalidate()
		return
	}
	a.applyOpen(path, note.Source)
	a.invalidate()
}

// requestOpen opens a path from the UI. A clean document opens at once; an
// edited one asks first, with the dialog off the UI thread.
func (a *App) requestOpen(path string) {
	if doctypes.IsImagePath(path) {
		a.openImageDocument(path, true)
		return
	}
	if !doctypes.IsEditablePath(path) {
		a.notice(locales.FilesUnsupportedType(filepath.Base(path)))
		a.invalidate()
		return
	}
	if !a.documentDirty() {
		a.openPathNow(path)
		return
	}
	if a.win == nil || a.busy {
		return
	}
	a.startBusy()
	go func() {
		if !a.confirmDiscardOffUI() {
			a.finish(func() { a.indexedPath = "" })
			return
		}
		if a.noteEditor() == nil {
			a.finish(nil)
			return
		}
		note, err := documentsave.OpenDocument(a.documentRepository, path)
		if err != nil {
			errorDialog(locales.DialogOpenFailed(), err.Error())
			a.finish(nil)
			return
		}
		a.finish(func() { a.applyOpen(path, note.Source) })
	}()
}

func (a *App) applyOpen(path, data string) {
	if a.indexedPath == path && a.indexedRevision != "" && string(document.RevisionOf(data)) != a.indexedRevision && a.vault != nil {
		rel, _ := filepath.Rel(a.activeVaultRoot(), path)
		hit := vaultsearch.Hit{Path: filepath.ToSlash(rel), Revision: a.indexedRevision}
		a.indexedPath, a.indexedRevision = "", ""
		a.notice(locales.WorkspaceSearchMoved())
		a.navigateSearchHit(hit)
		return
	}
	a.pushNav(path)
	a.closeImageDocument()
	a.selectedPath = path
	if a.vault != nil {
		_ = a.vault.WatchFile(path)
		if doctypes.IsMarkdownPath(path) {
			a.queueLinkIndexSnapshot()
		}
	}
	a.loadDocumentAt(path, data)
	a.updateTitle()
	a.addRecent(path, false)
	a.noteEditor().FocusCommands().Focus()
	if a.indexedPath == path {
		a.noteEditor().NavigationCommands().ScrollToOffset(editorsource.OffsetInNormalizedSource(data, a.indexedOffset))
		a.indexedPath = ""
		a.indexedRevision = ""
	}
}

func (a *App) newDoc() {
	if !a.documentDirty() {
		a.resetDoc()
		return
	}
	if a.busy {
		return
	}
	a.startBusy()
	go func() {
		if !a.confirmDiscardOffUI() {
			a.finish(nil)
			return
		}
		a.finish(a.resetDoc)
	}()
}

func (a *App) resetDoc() {
	a.closeImageDocument()
	a.selectedPath = ""
	a.loadDocumentAt("", "")
	a.updateTitle()
	a.noteEditor().FocusCommands().Focus()
}

func (a *App) save() {
	a.flushManagedTimestamp()
	if a.documentPath() == "" {
		a.saveAs()
		return
	}
	if !doctypes.IsEditablePath(a.documentPath()) {
		a.notice(locales.FilesUnsupportedType(filepath.Base(a.documentPath())))
		a.invalidate()
		return
	}
	var revision document.Revision
	if a.noteEditor() != nil {
		revision = a.noteEditor().Snapshot().Revision
	}
	if err := a.writeDocument(a.documentPath(), a.diskSource(), revision); err != nil {
		if a.noteEditor() != nil && errors.Is(err, document.ErrConflict) {
			a.noteEditor().SetConflict(a.documentPath())
			if a.documents != nil {
				a.documents.Apply(documentdialogs.Action{Kind: documentdialogs.OpenConflict})
			}
		}
		a.notice(locales.CommonSaveError(err.Error()))
	} else {
		if a.noteEditor() != nil {
			_ = a.noteEditor().DeleteDraft(a.documentPath())
		}
		a.notice(locales.DocumentSaved())
	}
	a.updateTitle()
	a.invalidate()
}

// confirmDiscardOffUI asks about an edited document without blocking the UI
// thread. It runs in a goroutine; every state change hops back to the main
// thread. It returns true when it is safe to replace the document.
func (a *App) confirmDiscardOffUI() bool {
	dirty, name, path, _, _ := a.snapshotDoc()
	if !dirty {
		return true
	}
	res, err := messageDialog(mygo.MessageOptions{
		Parent:        a.win,
		Type:          mygo.MessageWarning,
		Message:       locales.DocumentSaveChanges(name),
		Detail:        locales.DocumentDiscardWarning(),
		Buttons:       []string{locales.DocumentSaveButton(), locales.DocumentDontSaveButton(), locales.CommonCancel()},
		DefaultButton: 0,
		CancelButton:  2,
	})
	if err != nil {
		return false
	}
	decision := documentsave.DirtyTransitionCancel
	savePath := ""
	switch res.Button {
	case 1:
		decision = documentsave.DirtyTransitionDiscard
	case 0:
		decision = documentsave.DirtyTransitionSave
		savePath = path
		if savePath == "" {
			defaultName := name
			if defaultName == "" || defaultName == locales.DocumentUntitled() {
				defaultName = "Untitled.md"
			} else if !strings.HasSuffix(strings.ToLower(defaultName), ".md") {
				defaultName += ".md"
			}
			savePath, err = saveDialog(mygo.SaveDialogOptions{
				Parent:      a.win,
				Title:       locales.CommonSaveFile(),
				DefaultPath: defaultName,
				Filters:     []mygo.FileFilter{{Name: locales.CommonTextFiles(), Extensions: doctypes.EditableExtensions()}},
			})
			if err != nil || savePath == "" {
				return false
			}
		}
		if !doctypes.IsEditablePath(savePath) {
			errorDialog(locales.DialogCannotSave(), locales.DialogChooseTextExtension())
			return false
		}
	default:
		return false
	}
	outcome, err := a.documentWorkflows().ResolveDirtyTransition(decision, savePath)
	if err != nil {
		errorDialog(locales.DialogSaveFailed(), err.Error())
		return false
	}
	if outcome.Saved {
		a.onMain(func() {
			a.selectedPath = outcome.Path
			a.updateTitle()
		})
	}
	return outcome.Allowed
}

func (a *App) saveAs() {
	if a.busy {
		return
	}
	a.flushManagedTimestamp()
	var name, source, originalPath string
	a.onMain(func() {
		name = locales.DocumentUntitled() + ".md"
		if a.documentPath() != "" {
			name = filepath.Base(a.documentPath())
		}
		source = a.diskSource()
		originalPath = a.documentPath()
	})
	a.startBusy()
	go func() {
		path, err := saveDialog(mygo.SaveDialogOptions{
			Parent:      a.win,
			Title:       locales.CommonSaveFile(),
			DefaultPath: name,
			Filters:     []mygo.FileFilter{{Name: locales.CommonTextFiles(), Extensions: doctypes.EditableExtensions()}},
		})
		if err != nil || path == "" {
			a.finish(nil)
			return
		}
		if !doctypes.IsEditablePath(path) {
			errorDialog(locales.DialogCannotSave(), locales.DialogChooseTextExtension())
			a.finish(nil)
			return
		}
		createdAt := ""
		if originalPath == "" && doctypes.IsMarkdownPath(path) {
			source, createdAt, err = documentsave.EnsureCreatedAt(source, time.Now())
			if err != nil {
				errorDialog(locales.DialogSaveFailed(), err.Error())
				a.finish(nil)
				return
			}
		}
		if createdAt != "" && a.noteEditor() != nil {
			var applyErr error
			a.onMain(func() {
				applyErr = a.noteEditor().ReplaceSystem(strings.ReplaceAll(source, "\r\n", "\n"))
			})
			if applyErr != nil {
				errorDialog(locales.DialogSaveFailed(), applyErr.Error())
				a.finish(nil)
				return
			}
		}
		err = a.writeDocument(path, source, "")
		if err != nil {
			errorDialog(locales.DialogSaveFailed(), err.Error())
			a.finish(nil)
			return
		}
		a.finish(func() {
			a.selectedPath = path
			a.updateTitle()
			a.clearTreeCache()
		})
	}()
}

// docDir is the folder images and links of the document resolve in.
func (a *App) docDir() string {
	if a.documentPath() != "" {
		return filepath.Dir(a.documentPath())
	}
	return a.root
}

// editorOptions are the editor's options, from the settings and the
// document being edited.
func (a *App) editorOptions() editorui.Options {
	var header func(*ui.Context)
	headerHeight := float32(0)
	if a.propertiesPanel.Enabled() {
		header = a.propertiesPanel.RenderSummary
		headerHeight = 160
	}
	return editorui.Options{
		DisableSmartTypography:  !a.smartTypography,
		HeadingMarkerReveal:     a.headingMarkerReveal,
		SoftBreaksAsSpaces:      false,
		PreserveTypedLineBreaks: true,
		VimVirtualImageRows:     a.vimVirtualImageRows,
		CursorFade:              a.cursorFade,
		SmoothCursor:            a.smoothCursor,
		SmoothPageJumps:         a.smoothPageJumps,
		SearchMatchPulse:        a.searchMatchPulse,
		VimYankFlash:            a.vimYankFlash,
		HideMouseCursor:         a.mouseCursorHidden,
		Header:                  header,
		HeaderHeight:            headerHeight,
		HideFrontmatter:         a.propertiesPanel.Enabled() && !a.propertiesPanel.IsSourceMode() && a.propertiesPanel.Document().Err == nil,
		FontSize:                float32(a.fontSize),
		LineHeight:              float32(a.lineHeight),
		MaxWidth:                800,
		PadX:                    56,
		BaseDir:                 a.docDir(),
		Placeholder:             locales.DocumentStartWriting(),
		Label:                   locales.DocumentDefaultLabel(),
		Code:                    editorui.CodeLanguageFor(a.documentPath()),
		Invalidate:              a.invalidate,
		OnLink:                  a.openLink,
		OnWikiLinkHover: func(target string, x, y float32, bounds ui.Rect) {
			a.hoverWikiLinkAt(target, x, y, bounds)
		},
		WikiLinkSuggestions:       a.wikiLinkSuggestions,
		WikiLinkSuggestionDetails: a.wikiLinkSuggestionDetails,
		ResolveWikiEmbed:          a.resolveWikiEmbed,
		CurrentPath:               a.documentPath(),
		ResolveWikiLink:           a.resolveWikiLinkStyle,
		LinkRevision:              a.editorLinkRevision(),
		LinkContext:               a.editorLinkContext(),
		AssetRoot:                 a.root,
		ImageCacheStore:           a.imageCacheStore,
		ImageLocalFiles:           a.imageLocalFiles,
		ResolveImage:              a.resolveVaultImage,
		OnImagePreview:            a.openImagePreview,
		OnDiagramPreview: func(source string, model diagrams.Result) {
			content := mermaidpreview.NewContent(source, model, func() { a.mermaidPreviewHandle = window.Handle{} })
			handle, err := a.windows.Open(content, window.Placement{Host: "main", Layer: window.ModalOverlay})
			if err != nil {
				a.notice(err.Error())
				return
			}
			a.mermaidPreviewHandle = handle
			a.invalidate()
		},
		SaveImage:           a.saveImage,
		OnDropFiles:         a.openDropped,
		OnVaultItemDragOver: func() { a.setTreeDragOperationDisplay(filetree.DragOperationInsertIntoEditor, "") },
	}
}

func (a *App) configureActiveNote(topInset float32) {
	if a.noteEditor() == nil {
		return
	}
	opts := a.editorOptions()
	opts.TopInset = topInset
	_ = a.noteEditor().Configure(noteeditor.PresentationSettings{
		Header:   opts.Header,
		FontSize: opts.FontSize, LineHeight: opts.LineHeight,
		DisableSmartTypography: opts.DisableSmartTypography,
		HeadingMarkerReveal:    opts.HeadingMarkerReveal,
		VimVirtualImageRows:    opts.VimVirtualImageRows,
		CursorFade:             opts.CursorFade, SmoothCursor: opts.SmoothCursor,
		HideMouseCursor: opts.HideMouseCursor, SmoothPageJumps: opts.SmoothPageJumps,
		SearchMatchPulse: opts.SearchMatchPulse, VimYankFlash: opts.VimYankFlash,
		TopInset: opts.TopInset, HeaderHeight: opts.HeaderHeight,
		HideFrontmatter: opts.HideFrontmatter, BaseDir: opts.BaseDir,
		AssetRoot: opts.AssetRoot, CurrentPath: opts.CurrentPath, Code: opts.Code,
		LinkRevision: opts.LinkRevision, LinkContext: opts.LinkContext,
	})
}

// createActiveNote creates the pane-owned document and editor session. The
// production renderer attaches to that same session through window.Content.
func (a *App) createActiveNote(note document.Note, cursorOffset int) error {
	pane := a.focusedPane()
	handle, err := a.createNotePane(pane, note, cursorOffset)
	if err != nil {
		return err
	}
	a.editorPaneHandle = handle
	if err := a.syncWorkspaceDocument(); err != nil {
		return err
	}
	return nil
}

func (a *App) createNotePane(pane window.PaneID, note document.Note, cursorOffset int) (window.Handle, error) {
	content, err := noteeditor.NewContent(
		workspace.PaneID(pane), note,
		cursorOffset,
		noteeditor.Dependencies{
			Documents: a.documentRepository,
			Recovery:  a.documentRecovery,
			History:   a.documentHistory,
			Events:    a.documentEvents,
		},
		func(session *editormodel.Session, callbacks noteeditor.SurfaceOptions) window.Content {
			opts := a.editorOptions()
			opts.OnChange = func() {
				callbacks.OnChange()
			}
			view := editorui.NewFromSession(session, opts)
			view.SetKeymap(a.loadEditorSettings().Keymap)
			return noteeditor.NewEditorSurface(view)
		},
		func(action noteeditor.Action) {
			if action.Kind == noteeditor.DocumentChanged {
				a.queueLinkIndexSnapshot()
				a.refreshSearch()
				a.updateTitle()
				a.invalidate()
			}
		},
	)
	if err != nil {
		return window.Handle{}, err
	}
	if a.windows == nil {
		content.Close()
		return window.Handle{}, errors.New("window manager is not configured")
	}
	handle, err := a.windows.Open(content, window.Placement{Host: "main", Pane: pane, Layer: window.PaneContent})
	if err != nil {
		content.Close()
		return window.Handle{}, err
	}
	if a.workspacePaneHandles == nil {
		a.workspacePaneHandles = make(map[window.PaneID]window.Handle)
	}
	if previous := a.workspacePaneHandles[pane]; !previous.IsZero() && previous != handle {
		_ = a.windows.Close(previous)
	}
	a.workspacePaneHandles[pane] = handle
	return handle, nil
}

func (a *App) editorLinkRevision() uint64 {
	if a.vault == nil || !a.vaultSnapshot.Ready {
		return 0
	}
	return a.vaultSnapshot.IndexRevision
}

func (a *App) editorLinkContext() string {
	root := ""
	if a.vault != nil {
		root = a.vaultSnapshot.Root
	}
	return filepath.Clean(root) + "\x00" + filepath.Clean(a.documentPath())
}

// loadDoc makes src the document, as opening a file does: the history of
// undo starts over. Windows line endings are kept as \n while editing and
// written back as they were.
func (a *App) loadDoc(src string) {
	a.loadDocumentAt(a.documentPath(), src)
}

func (a *App) loadDocumentAt(path, src string) {
	a.linkPreview.Clear()
	a.propertiesPanel.SetOpenedAt(time.Now())
	a.propertiesPanel.CancelEdit()
	if a.noteEditor() != nil {
		_ = a.noteEditor().Open(document.NewNote(path, src, document.RevisionOf(src)), 0)
	}
	if path != "" {
		if err := a.syncWorkspaceDocument(); err != nil {
			a.notice(err.Error())
		}
	}
	a.configureActiveNote(0)
	a.findReplace.Invalidate()
}

// editDoc replaces the document with src as an edit the user can undo, as
// the AI's tools change it.
func (a *App) editDoc(src string) {
	_ = a.noteEditor().ReplaceUser(src)
	a.invalidate()
}

func (a *App) documentSnapshot() noteeditor.Snapshot {
	if a == nil || a.noteEditor() == nil {
		return noteeditor.Snapshot{}
	}
	return a.noteEditor().Snapshot()
}

func (a *App) documentPath() string   { return a.documentSnapshot().Path }

func (a *App) documentSource() string { return a.documentSnapshot().Source }

func (a *App) documentDirty() bool    { return a.documentSnapshot().Dirty }

func (a *App) diskSource() string     { return a.documentSnapshot().DiskSource }
