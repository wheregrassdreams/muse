package nativeui

import (
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	applicationvault "muse/application/vault"
	doctypes "muse/document/types"
	"muse/locales"
	documentdialogs "muse/nativeui/dialogs/document"
	dialogsui "muse/nativeui/dialogs/export"
	noteeditor "muse/nativeui/editor/surface"
	"muse/nativeui/explorer"
	searchui "muse/nativeui/search/workspace"
	"muse/nativeui/shell"
	"muse/nativeui/shell/windowcontrols"
	componenticons "muse/nativeui/ui/icons"
	"muse/nativeui/vaulthome"
	"muse/nativeui/workbench"
	workspaceui "muse/nativeui/workspace"
	"muse/vault/files"
	"muse/workspace"
)

// initializeWorkbench is the single composition point for feature surfaces.
// Workbench owns the shell frame and translates its UI actions; App supplies
// feature renderers and application capabilities.
func (a *App) initializeWorkbench() {
	if a == nil {
		return
	}
	a.workbench = workbench.NewHost(workbench.Composition{
		Frame: a.workbenchFrame,
		Surfaces: map[shell.Surface]workbench.SurfaceRenderer{
			shell.SurfaceFiles:   func(c *ui.Context) { a.renderPanelSurface(c, shell.SurfaceFiles, a.renderFilesSurface) },
			shell.SurfaceOutline: func(c *ui.Context) { a.renderPanelSurface(c, shell.SurfaceOutline, a.outlineView) },
			shell.SurfaceSearch:  func(c *ui.Context) { a.renderPanelSurface(c, shell.SurfaceSearch, a.renderWorkspaceSearch) },
			shell.SurfaceAgent: func(c *ui.Context) {
				a.renderPanelSurface(c, shell.SurfaceAgent, func(c *ui.Context) { a.assistant.Render(c) })
			},
		},
		RenderCanvas:    a.renderEditorCanvas,
		RenderInspector: func(c *ui.Context) { a.propertiesPanel.RenderSidebar(c) },
		RenderOverlays:  a.renderOverlays,
		PanelWidth:      func(width float32) { a.panelW = width },
		Actions: workbench.ActionPorts{
			SelectSurface: func(surface shell.Surface) { a.togglePanel(surface) },
			TogglePanel: func(open bool, width float32, surface shell.Surface) {
				a.panel, a.panelOpen, a.panelWidth = surface, open, width
				if open {
					a.panelW = width
				} else {
					a.panelW = 0
				}
				a.invalidate()
			},
			Geometry: func(open bool, width float32, surface shell.Surface) {
				a.panel, a.panelOpen, a.panelWidth, a.panelW = surface, open, width, width
				if !open {
					a.panelW = 0
				}
				a.invalidate()
			},
			OpenSettings: a.openSettings,
		},
	})
}

// renderPanelSurface restores the shared workspace header and footer that
// frame every sidebar feature. Feature renderers own only the panel body.
func (a *App) renderPanelSurface(c *ui.Context, surface shell.Surface, render func(*ui.Context)) {
	if a == nil || c == nil {
		return
	}
	name := locales.DocumentUntitled()
	if a.root != "" {
		name = filepath.Base(a.root)
	} else if path := a.documentPath(); path != "" {
		name = filepath.Base(filepath.Dir(path))
	}
	ui.Column(c).Width(a.panelWidth).FillHeight().Children(func() {
		bar := c.TitleBar()
		ui.Row(c).Height(max(float32(44), bar.Height)).Shrink(0).Padding(0, 8, 0, max(float32(34), bar.Left+34)).DragWindow().Children(func() {
			if a.settingsPanel != nil && a.settingsPanel.Runtime().AIEnabled {
				a.panelIcon(c, componenticons.AI(), locales.AiAssistant(), func() { a.togglePanel(shell.SurfaceAgent) })
			}
		})
		ui.Row(c).Height(44).Shrink(0).Padding(0, 10).Gap(4).AlignItems(ui.Center).Children(func() {
			space := ui.ButtonBase(c).Height(28).Padding(0, 4).Radius(6).Label(name).Tooltip(locales.WorkspaceLabel())
			space.Menu(func(menu *ui.Menu) {
				if menu.Item(locales.FilesOpenFolderMenu()).Chosen() {
					a.openFolderDialog()
				}
				if menu.Item(locales.FilesOpenFileMenu()).Chosen() {
					a.openFileDialog()
				}
				menu.Separator()
				if menu.Item(locales.MenuOutline()).Chosen() {
					a.togglePanel(shell.SurfaceOutline)
				}
				if menu.Item(locales.MenuSearchWorkspace()).Chosen() {
					a.openWorkspaceSearch()
				}
				if a.settingsPanel != nil && a.settingsPanel.Runtime().AIEnabled && menu.Item(locales.AiAssistant()).Chosen() {
					a.togglePanel(shell.SurfaceAgent)
				}
			})
			space.Children(func() {
				ui.Row(c).Gap(3).AlignItems(ui.Center).Children(func() {
					ui.Text(c, name).FontSize(13).FontWeight(600).SingleLine().Ellipsis("…")
					ui.Icon(c, componenticons.ChevronDown()).Size(12, 12).TextColor(c.Theme().TextMuted)
				})
			})
			ui.Spacer(c)
			a.panelIcon(c, componenticons.Search(), locales.MenuSearchWorkspace(), a.openWorkspaceSearch)
			ui.Row(c).Height(28).Radius(8).Background(c.Theme().Surface).AlignItems(ui.Center).Children(func() {
				a.panelIcon(c, componenticons.Edit(), locales.DocumentNew(), func() {
					if a.root != "" {
						a.newFileIn(a.root)
					} else {
						a.newDoc()
					}
				})
				more := ui.ButtonBase(c).Size(20, 28).Padding(0).Label(locales.DocumentNewMenu()).Tooltip(locales.DocumentNewMenu())
				more.Menu(func(menu *ui.Menu) {
					if menu.Item(locales.FilesNewFolderMenu()).Chosen() {
						a.newFolderIn(a.root)
					}
				})
				more.Children(func() { ui.Icon(c, componenticons.ChevronDown()).Size(11, 11).TextColor(c.Theme().TextMuted) })
			})
		})
		if surface != shell.SurfaceFiles {
			ui.Row(c).Height(30).Shrink(0).Padding(0, 10).AlignItems(ui.Center).Children(func() {
				a.panelIcon(c, componenticons.ChevronLeft(), locales.FilesBackToTree(), func() { a.togglePanel(shell.SurfaceFiles) })
			})
		}
		ui.Column(c).Grow(1).FillWidth().MinHeight(0).Children(func() {
			if render != nil {
				render(c)
			}
		})
		ui.Row(c).Shrink(0).Padding(0, 8, 10, 8).Children(func() {
			a.panelIcon(c, componenticons.Settings(), locales.SettingsTitle(), a.openSettings)
		})
	})
}

func (a *App) panelIcon(c *ui.Context, icon *ui.SVG, label string, fn func()) {
	button := ui.ButtonBase(c).Size(26, 26).Radius(6).Padding(0).Justify(ui.Center).AlignItems(ui.Center).Label(label).Tooltip(label)
	if button.Hovered() && fn != nil {
		button.Background(c.Theme().SurfaceHover)
	}
	if fn != nil {
		button.OnClick(fn)
	}
	color := c.Theme().TextMuted
	if c.Theme().Dark {
		color = ui.RGBA(255, 255, 255, 0.92)
	}
	button.Children(func() { ui.Icon(c, icon).Size(16, 16).TextColor(color) })
}

func (a *App) renderWorkspaceSearch(c *ui.Context) {
	if a == nil || a.workspaceSearch == nil {
		return
	}
	availability := searchui.VaultRequired
	if a.vault != nil {
		availability = searchui.IndexBuilding
	}
	a.workspaceSearch.Render(c, searchui.View{
		Availability: availability, CanChangeMode: true, SearchIcon: componenticons.Search(),
	}, func(action searchui.Action) {
		a.workspaceSearch.Apply(action)
		if action.Kind == searchui.Dismiss {
			a.panelOpen, a.panelW = false, 0
		}
		a.invalidate()
	})
}

// View is the live MyGo window surface. It composes the Explorer, workspace,
// inspector, and transient feature overlays through the workbench host.
func (a *App) View(c *ui.Context) {
	if a == nil || a.workbench == nil {
		return
	}
	a.mainMu.Lock()
	defer a.mainMu.Unlock()
	if a.windows != nil && a.windows.HasModal() && c.Shortcut(0, ui.KeyEscape) {
		if a.windows.HandleEscape() {
			a.invalidate()
		}
	}
	if a.navIconPicker != nil && a.navIconPicker.IsOpen() && c.Shortcut(0, ui.KeyEscape) {
		a.navIconPicker.Close()
	}
	if a.propertiesPanel != nil && c.Shortcut(0, ui.KeyEscape) {
		switch {
		case a.propertiesPanel.EditingKey() != "":
			a.propertiesPanel.CancelEdit()
			a.invalidate()
		case a.propertiesPanel.IsSourceMode():
			a.propertiesPanel.ReturnToProperties()
			a.invalidate()
		}
	}
	if !a.hasModalKeyboardOwner() && c.Shortcut(ui.Cmd, ui.KeyO) {
		a.openWorkspaceSearch()
	}
	a.ensureVaultSession()
	if a.vault != nil {
		a.vaultSnapshot = a.vault.Snapshot()
		if !a.vaultSnapshot.Ready {
			c.After(50 * time.Millisecond)
		}
	}
	a.linkIndexTick(c)
	root := c.Root()
	rootBounds := root.Bounds()
	root.ObserveInput(func(ev ui.InputEvent) {
		if a.fileTreeState != nil {
			if ev.Kind == ui.InputPointerDown {
				a.fileTreeState.SetPointerReleased(false)
			}
			if ev.Kind == ui.InputPointerUp && ev.Canceled {
				a.fileTreeState.SetPointerReleased(false)
				drag := a.fileTreeState.DragPreview()
				drag.Pointer = ui.Point{}
				a.fileTreeState.SetDragPreview(drag)
				a.fileTreeState.ClearDropHover()
				a.fileTreeState.ClearDropTargetRange()
				return
			}
			drag := a.fileTreeState.DragPreview()
			if !drag.StartedAt.IsZero() && (ev.Kind == ui.InputPointerMove || ev.Kind == ui.InputPointerUp) {
				drag.Pointer = ui.Point{X: rootBounds.X + ev.X, Y: rootBounds.Y + ev.Y}
				a.fileTreeState.SetDragPreview(drag)
				if ev.Kind == ui.InputPointerUp {
					a.fileTreeState.SetPointerReleased(true)
				}
			}
		}
		if !a.editorOptions().HideMouseCursor {
			return
		}
		editor := a.noteEditor()
		if editor == nil || editor.FocusCommands() == nil || !editor.FocusCommands().Focused() {
			return
		}
		keymap := ""
		if modal := editor.ModalCommands(); modal != nil {
			keymap = modal.Keymap()
		}
		if (keymap == "vim" || keymap == "helix") && ev.Kind == ui.InputKeyDown || ev.Kind == ui.InputText || ev.Kind == ui.InputCompose {
			a.mouseCursorHidden = true
			a.mouseCursorHiddenUntil = ev.Time.Add(1500 * time.Millisecond)
		}
	})
	if a.mouseCursorHidden && !c.Now().Before(a.mouseCursorHiddenUntil) {
		a.mouseCursorHidden = false
	}
	a.workbench.Render(c)
}

func (a *App) hasModalKeyboardOwner() bool {
	return a != nil && ((a.windows != nil && a.windows.HasModal()) ||
		(a.propertiesPanel != nil && (a.propertiesPanel.EditingKey() != "" || a.propertiesPanel.IsSourceMode())) ||
		(a.navIconPicker != nil && a.navIconPicker.IsOpen()))
}

// ensureVaultSession restores the vault catalog and watcher that back the
// file tree, link navigation, and workspace search. The session owns its own
// asynchronous startup; View only refreshes its detached snapshot while it
// becomes ready.
func (a *App) ensureVaultSession() {
	if a == nil {
		return
	}
	root := a.root
	if root == "" {
		if path := a.documentPath(); path != "" {
			root = filepath.Dir(path)
		}
	}
	if root == "" || (a.vault != nil && filepath.Clean(a.vaultSnapshot.Root) == filepath.Clean(root)) {
		return
	}
	if a.vault != nil {
		old := a.vault
		a.vault = nil
		go old.Close()
	}
	session, err := applicationvault.NewSession(root, applicationvault.SessionDependencies{
		NewIndex: a.newVaultIndex, NewCatalog: a.newVaultCatalog,
		NewWatcher: a.newWatcher, SearchReconciler: a.vaultSearchReconciler,
	})
	if err != nil {
		a.notice(err.Error())
		return
	}
	a.vault = session
	a.vaultSnapshot = session.Snapshot()
	if a.directoryCatalog != nil {
		a.directoryCatalog.Invalidate()
	}
	if a.quickOpen != nil {
		a.quickOpen.SetVault(session)
	}
}

func (a *App) workbenchFrame() shell.Frame {
	if a == nil {
		return shell.Frame{}
	}
	panelWidth := a.panelWidth
	if panelWidth <= 0 {
		panelWidth = 268
	}
	mode, accent := uint8(0), "red"
	if a.settingsPanel != nil {
		appearance := a.settingsPanel.Runtime().Appearance
		mode, accent = uint8(appearance.Theme), appearance.AccentID
	}
	toggleIcon, toggleLabel := componenticons.PanelLeftOpen(), locales.AiSidebarShow()
	if a.panelOpen {
		toggleIcon, toggleLabel = componenticons.PanelLeftClose(), locales.AiSidebarHide()
	}
	return shell.Frame{
		PanelOpen: a.panelOpen, PanelWidth: panelWidth, PanelMinWidth: 224,
		PanelMaxWidth: 440, PanelFoldAt: 200, PanelOpenAt: 36,
		FlatCanvas: a.flat, Surface: a.shellSurface(),
		Window: a.win, HideMouseCursor: a.mouseCursorHidden,
		CursorUntil:      a.mouseCursorHiddenUntil,
		PanelToggleLabel: toggleLabel, PanelToggleIcon: toggleIcon,
		WindowButtonsTop: 18, ThemeMode: mode, AccentID: accent,
	}
}

func (a *App) shellSurface() shell.Surface {
	if a == nil {
		return shell.SurfaceFiles
	}
	return a.panel
}

func (a *App) renderWorkspace(c *ui.Context, _ bool) {
	if a.workspaceHost == nil || a.workspaceSession == nil {
		return
	}
	a.workspaceHost.RenderSession(c, a.workspaceSession, a.workspaceState,
		func(pane workspace.PaneID) { a.activePane = pane },
		func(state workspace.State) { a.workspaceState = state; a.invalidate() },
	)
}

func (a *App) renderEditorCanvas(c *ui.Context, flat bool) {
	if a.imageDocumentOpen() {
		a.renderWorkspace(c, flat)
		return
	}
	if a.root != "" && a.documentPath() == "" && a.documentSource() == "" {
		vaulthome.Render(c, vaulthome.DefaultViewModel(componenticons.New(), componenticons.OpenFile()), func(action vaulthome.Action) {
			switch action {
			case vaulthome.NewDocument:
				a.newFileIn(a.root)
			case vaulthome.OpenFile:
				a.openFileDialog()
			}
		})
		return
	}
	if !flat {
		a.renderEditorHeader(c, false)
		a.renderWorkspace(c, false)
		return
	}
	ui.Column(c).Fill().Children(func() {
		a.renderWorkspace(c, true)
		var scrollY float32
		if editor := a.noteEditor(); editor != nil && editor.NavigationCommands() != nil {
			scrollY = editor.NavigationCommands().ScrollY()
		}
		windowcontrols.RenderHeaderFade(c, &a.headerFadeCache, a.headerFadeEnabled(),
			max(float32(44), c.TitleBar().Height), scrollY, c.Theme().Background)
		a.renderEditorHeader(c, true)
	})
}

func (a *App) headerFadeEnabled() bool {
	if a == nil || a.settingsPanel == nil {
		return false
	}
	return a.settingsPanel.Runtime().Appearance.HeaderFade
}

func (a *App) renderEditorHeader(c *ui.Context, flat bool) {
	if a != nil {
		noteeditor.RenderHeader(c, a.editorHeaderModel(c, flat), a.dispatchEditorHeader)
	}
}

func (a *App) editorHeaderModel(c *ui.Context, flat bool) noteeditor.HeaderModel {
	snapshot := a.documentSnapshot()
	path := snapshot.Path
	if a.imageDocumentOpen() {
		path = a.imageDocumentPath()
	}
	label := path
	if path == "" {
		label = locales.DocumentUntitled()
	} else if a.root != "" {
		if relative, err := filepath.Rel(a.root, path); err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			label = filepath.ToSlash(relative)
		}
	}
	mode := ""
	if commands := a.noteEditor(); commands != nil && commands.ModalCommands() != nil {
		mode = commands.ModalCommands().ModeLabel()
	}
	items := []string{locales.DocumentPathValue(label)}
	chars := utf8.RuneCountInString(snapshot.Source)
	lines := strings.Count(snapshot.Source, "\n") + 1
	items = append(items, locales.DocumentCharacterLineCount(chars, lines))
	if info, err := os.Stat(path); err == nil {
		items = append(items, locales.DocumentModifiedAt(info.ModTime()))
	}
	return noteeditor.HeaderModel{
		Flat: flat, PanelWidth: a.panelW, HeaderHeight: max(float32(44), c.TitleBar().Height),
		Path: label, HasPrevious: a.navigationHistory != nil && a.navigationHistory.CanMove(-1),
		HasNext:       a.navigationHistory != nil && a.navigationHistory.CanMove(1),
		ImageDocument: a.imageDocumentOpen(), Mode: mode, Status: a.status,
		Dirty: snapshot.Dirty, Properties: a.propertiesPanel != nil,
		CanExport: path != "" && !a.imageDocumentOpen() && doctypes.IsMarkdownPath(path),
		InfoItems: items, HasFile: path != "",
	}
}

func (a *App) dispatchEditorHeader(action noteeditor.HeaderAction) {
	switch action.Kind {
	case noteeditor.HeaderBack:
		a.navigateDocumentHistory(-1)
	case noteeditor.HeaderForward:
		a.navigateDocumentHistory(1)
	case noteeditor.HeaderOpenImagePreview:
		a.openImagePreview(a.imageDocumentPath())
	case noteeditor.HeaderSave:
		if a.documentSave != nil {
			a.documentSave.Save()
		}
	case noteeditor.HeaderOpenProperties:
		if a.propertiesPanel != nil {
			a.propertiesPanel.Open()
			a.invalidate()
		}
	case noteeditor.HeaderRevealFile:
		if path := a.documentPath(); path != "" {
			mygo.Shell.ShowItemInFolder(path)
		}
	case noteeditor.HeaderExportMarkdown:
		if a.exportHost != nil {
			a.exportHost.Select(dialogsui.ExportMarkdown)
		}
	case noteeditor.HeaderExportHTML:
		if a.exportHost != nil {
			a.exportHost.Select(dialogsui.ExportHTML)
		}
	case noteeditor.HeaderExportPDF:
		if a.exportHost != nil {
			a.exportHost.Select(dialogsui.ExportPDF)
		}
	case noteeditor.HeaderOpenHistory:
		if commands := a.noteEditor(); commands != nil {
			if entries, err := commands.ListHistory(a.documentPath()); err == nil {
				a.documents.OpenHistory(a.documentPath(), entries)
			}
		}
	case noteeditor.HeaderOpenRecovery:
		if commands := a.noteEditor(); commands != nil {
			if drafts, err := commands.ListDrafts(); err == nil {
				a.documents.OpenRecovery(drafts)
			}
		}
	case noteeditor.HeaderOpenBacklinks:
		a.openLinks()
	case noteeditor.HeaderSetWorkspace:
		if path := a.documentPath(); path != "" {
			a.setWorkspace(filepath.Dir(path))
		}
	case noteeditor.HeaderCopyPath:
		if path := a.documentPath(); path != "" {
			mygo.Clipboard.WriteText(path)
			a.notice(locales.DocumentCopiedPath())
		}
	}
	a.invalidate()
}

func (a *App) navigateDocumentHistory(delta int) {
	if a == nil || a.navigationHistory == nil {
		return
	}
	target, ok := a.navigationHistory.Plan(delta)
	if !ok {
		return
	}
	a.navigationHistory.Commit(target)
	if doctypes.IsImagePath(target.Path) {
		a.openImageDocument(target.Path, false)
	} else {
		a.requestOpen(target.Path)
	}
}

func (a *App) dispatchDocumentAction(action documentdialogs.Action) {
	if a == nil || a.documents == nil {
		return
	}
	a.documents.Apply(action)
	workflow := a.documentWorkflows()
	var err error
	switch action.Kind {
	case documentdialogs.RestoreDraft:
		err = workflow.RestoreDraft(action.Key)
	case documentdialogs.RestoreHistory:
		err = workflow.RestoreHistory(a.documents.HistoryPath(), action.ID)
	}
	if err != nil {
		a.notice(err.Error())
	}
	if action.Kind == documentdialogs.RestoreDraft || action.Kind == documentdialogs.RestoreHistory {
		a.configureActiveNote(0)
	}
	a.invalidate()
}

func (a *App) renderFilesSurface(c *ui.Context) {
	if a.fileTreeState == nil {
		return
	}
	useCases := a.vaultFileUseCases()
	explorer.RenderSurface(c, explorer.SurfacePorts{CatalogPanelPorts: explorer.CatalogPanelPorts{
		Root: a.root, Controller: a.fileTreeState,
		Focus: &a.treeFocusHandle, OnAction: a.routeExplorerAction,
		Children: func(directory string) []explorer.CatalogEntry {
			entries := useCases.CatalogChildren(a.vault, a.directoryCatalog, directory)
			result := make([]explorer.CatalogEntry, len(entries))
			for i, entry := range entries {
				kind := explorer.CatalogFile
				if entry.Kind == files.EntryDirectory {
					kind = explorer.CatalogDirectory
				}
				result[i] = explorer.CatalogEntry{Path: entry.Path, Kind: kind}
			}
			return result
		},
		VisibleFile: doctypes.IsVisibleWorkspacePath,
		IsDirectory: func(path string) bool { return useCases.IsCatalogDirectory(a.vault, path) },
		Icon: func(path string, directory bool) *ui.SVG {
			if a.navIconPicker != nil {
				return a.navIconPicker.IconForPath(path, directory)
			}
			return nil
		},
	}, OnNewDocument: a.newDoc, OnOpenFile: a.openFileDialog, OnOpenFolder: a.openFolderDialog})
}

func (a *App) renderOverlays(c *ui.Context) {
	if a == nil {
		return
	}
	if a.commandPalette != nil {
		a.commandPalette.Render(c, a.dispatchCommandPaletteAction)
	}
	if a.migrationHost != nil {
		a.migrationHost.Render(c)
	}
	if a.settingsPanel != nil && a.settingsPanel.IsOpen() {
		a.settingsPanel.RenderDialog(c)
	}
	if a.windows != nil {
		a.windows.Render(c, workspaceui.MainHost, workspaceui.Overlay)
		a.windows.Render(c, workspaceui.MainHost, workspaceui.ModalOverlay)
	}
	if a.linkPreview != nil {
		a.linkPreview.Render(c)
	}
}
