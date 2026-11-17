// Package nativeui is Muse's fully native shell: MyGo draws the window, the
// icon rail, the context panel and the Markdown editor, with no webview and
// no frontend build.
//
// The layout is a 56px icon rail, one shared context panel (files / outline
// / search / AI) and the editor card, which shows the document as Typora
// does: one surface the user writes on, rendered as it reads.
package nativeui

func (a *App) View(c *ui.Context) {
	diagnostics.Log(a.memoryDiagnosticsSnapshot)
	a.mainMu.Lock()
	defer a.mainMu.Unlock()
	closedManagedSurface := false
	if a.windows != nil && a.windows.HasModal() && c.Shortcut(0, ui.KeyEscape) && a.windows.HandleEscape() {
		closedManagedSurface = true
		a.invalidate()
	}
	if a.navIconPicker.IsOpen() && c.Shortcut(0, ui.KeyEscape) {
		a.navIconPicker.Close()
	}
	if (a.propertiesPanel.EditingKey() != "" || a.propertiesPanel.IsSourceMode()) && c.Shortcut(0, ui.KeyEscape) {
		switch {
		case a.propertiesPanel.EditingKey() != "":
			a.propertiesPanel.CancelEdit()
		case a.propertiesPanel.IsSourceMode():
			a.propertiesPanel.ReturnToProperties()
		}
		a.invalidate()
	}
	if !closedManagedSurface && !a.anyModalOwnsKeyboard() {
		a.handlePanelShortcuts(c)
	} else {
		a.cancelPanelSequences()
		a.cancelTreeSequences()
	}
	if !closedManagedSurface && !a.anyModalOwnsKeyboard() && c.Shortcut(ui.Cmd, ui.KeyO) {
		a.openWorkspaceSearch()
	}
	a.vaultTick(c)
	a.documentTick(c)
	a.applyTheme(c)
	row := ui.Row(c).Fill()
	rootElement := c.Root()
	rootBounds := rootElement.Bounds()
	rootElement.ObserveInput(func(ev ui.InputEvent) {
		if ev.Kind == ui.InputPointerDown {
			a.fileTreeState.SetPointerReleased(false)
		}
		if ev.Kind == ui.InputPointerUp && ev.Canceled {
			a.fileTreeState.SetPointerReleased(false)
			dragState := a.fileTreeState.DragPreview()
			dragState.Pointer = ui.Point{}
			a.fileTreeState.SetDragPreview(dragState)
			a.fileTreeState.ClearDropHover()
			a.fileTreeState.ClearDropTargetRange()
			return
		}
		dragState := a.fileTreeState.DragPreview()
		if !dragState.StartedAt.IsZero() && (ev.Kind == ui.InputPointerMove || ev.Kind == ui.InputPointerUp) {
			dragState.Pointer = ui.Point{X: rootBounds.X + ev.X, Y: rootBounds.Y + ev.Y}
			a.fileTreeState.SetDragPreview(dragState)
			if ev.Kind == ui.InputPointerUp {
				a.fileTreeState.SetPointerReleased(true)
			}
		}
	})
	row.ObserveInput(func(ev ui.InputEvent) {
		if !a.hideMouseCursor || !a.noteEditor().FocusCommands().Focused() {
			return
		}
		if (a.noteEditor().ModalCommands().Keymap() == "vim" || a.noteEditor().ModalCommands().Keymap() == "helix") && ev.Kind == ui.InputKeyDown {
			a.mouseCursorHidden = true
			a.mouseCursorHiddenUntil = ev.Time.Add(1500 * time.Millisecond)
		} else if ev.Kind == ui.InputText || ev.Kind == ui.InputCompose {
			a.mouseCursorHidden = true
			a.mouseCursorHiddenUntil = ev.Time.Add(1500 * time.Millisecond)
		}
	})
	if a.mouseCursorHidden && !c.Now().Before(a.mouseCursorHiddenUntil) {
		a.mouseCursorHidden = false
	}
	if a.mouseCursorHidden {
		row.Cursor(ui.CursorNone)
		c.After(a.mouseCursorHiddenUntil.Sub(c.Now()))
	}
	row.HandleInput(func(ev ui.InputEvent) bool {
		if ev.Kind == ui.InputPointerDown && a.mouseCursorHidden {
			a.mouseCursorHidden = false
		}
		return false
	})
	if c.Vibrancy() {
		// 亚克力在视图没画背景的地方透出来：轨道与面板后面是材料，
		// 编辑卡片仍是不透明的表面。材料只在支持它的窗口上有，其余
		// 平台画不透明的底色。
		c.Root().Background(ui.Transparent)
	} else {
		row.Background(a.windowBG())
	}
	row.Children(func() {
		a.contextPanel(c)
		a.canvas(c)
		if a.propertiesPanel.IsOpen() {
			a.propertiesPanel.RenderSidebar(c)
		}
	})
	if a.noteEditor() != nil && a.noteEditor().FocusCommands().Focused() && !a.treeFocusRequested && !a.treeFocusHandle.Focused(c) {
		a.setFocusedPanel(commands.PanelIDEditor)
	}

	a.sidebarToggle(c)
	if win := a.win; win != nil && windowcontrols.Visible() {
		windowcontrols.Render(c, windowcontrols.Model{
			Top:       canvasPadding + cardBorder + (editorHeaderH-26)/2,
			Maximized: win.IsMaximized(),
			Minimize:  win.Minimize,
			Maximize:  win.Maximize,
			Restore:   win.Unmaximize,
			Close:     win.Close,
		})
	}
	// Keyboard ownership controls input routing, not whether an open dialog
	// gets rendered. Each dialog checks its own active state; keeping this
	// render path unconditional avoids a modal hiding itself from the frame.
	a.settingsDialog(c)
	a.renameDialog(c)
	a.documentDialogs(c)
	a.coreMetadataMigrationDialog(c)
	a.commandPaletteDialog(c)
	a.workspaceSearchDialog(c)
	if a.windows != nil {
		a.windows.Render(c, "main", window.Overlay)
		a.windows.Render(c, "main", window.ModalOverlay)
	}
	a.wikiLinkPreviewDialog(c)
	if previewID := a.fileTreeState.PreviewID(); previewID != "" {
		a.fileTreeState.SetPreviewPopupHovered(filetree.RenderImagePreview(c, filetree.ImagePreview{
			Path: previewID, Anchor: a.fileTreeState.PreviewBounds(),
			CacheStore: a.imageCacheStore, LocalFiles: a.imageLocalFiles, OnLoad: a.invalidate,
		}))
	}
}

// windowBG is a single flat color, not a wash: the light side is a neutral
// canvas shared with the editor, the dark side a near-black canvas.
func (a *App) windowBG() ui.Color {
	if a.dark {
		return ui.Hex("#1a1a1a")
	}
	return ui.Hex("#ffffff")
}

func (a *App) applyTheme(c *ui.Context) {
	systemDark := c.Theme().Dark
	a.dark = a.themeMode == themeDark || (a.themeMode == themeSystem && systemDark)
	a.syncAppearance()
	t := *c.Theme()
	if a.dark {
		t.Dark = true
		t.Background = ui.Hex("#232328")
		t.Surface = ui.Hex("#2a2a30")
		t.SurfaceHover = ui.RGBA(255, 255, 255, 0.07)
		t.SurfacePressed = ui.RGBA(255, 255, 255, 0.12)
		t.Border = ui.RGBA(255, 255, 255, 0.07)
		t.Text = ui.Hex("#ececf0")
		t.TextMuted = ui.Hex("#a9a9b4")
	} else {
		t.Dark = false
		t.Background = ui.Hex("#ffffff")
		t.Surface = ui.Hex("#f3f3f6")
		t.SurfaceHover = ui.RGBA(0, 0, 0, 0.045)
		t.SurfacePressed = ui.RGBA(0, 0, 0, 0.08)
		t.Border = ui.RGBA(0, 0, 0, 0.07)
		t.Text = ui.Hex("#1d1d1f")
		t.TextMuted = ui.Hex("#6e6e78")
	}
	palette := appearance.From(appearance.Color(a.accentID), a.dark)
	t.Accent, t.AccentHover, t.AccentPressed = palette.Accent, palette.Hover, palette.Pressed
	t.AccentText, t.Selection, t.Focus = palette.OnAccent, palette.Selection, palette.Accent
	t.Radius = 7
	t.FontSize = 13
	c.SetTheme(&t)
}

// syncAppearance hands the theme to the system so its own surfaces follow:
// the acrylic behind the rail and the panel, and the title bar, are drawn
// by the desktop and would keep the desktop's appearance while the page
// switches to the other theme.
func (a *App) syncAppearance() {
	if a.themeApplied == a.themeMode {
		return
	}
	a.themeApplied = a.themeMode
	switch a.themeMode {
	case themeDark:
		setAppAppearance(mygo.ThemeDark)
	case themeSystem:
		setAppAppearance(mygo.ThemeSystem)
	default:
		setAppAppearance(mygo.ThemeLight)
	}
}

func sidebarColor(t *ui.Theme) ui.Color { return t.Background.Alpha(0.8) }

// canvas is the floating editor card: 8px away from the window's top, right
// and bottom, and 8px from the context panel. The rail and the panel paint
// nothing, so the window color — or the acrylic material — shows behind them.
func (a *App) canvas(c *ui.Context) {
	t := c.Theme()
	padRight, padBottom := canvasPadding, canvasPadding
	if a.flat {
		// Flat: the page is the window, one sheet with no card around it, and
		// its first line starts at the window's top so that the controls on
		// it centre on the traffic lights'.
		padRight, padBottom = 0, 0
	}
	ui.Row(c).Grow(1).FillHeight().MinWidth(0).Padding(padRight, padBottom, padBottom, a.canvasLeft(c)).Children(func() {
		if a.flat {
			card := ui.Column(c).Fill().Clip().Background(a.windowBG())
			card.Children(func() {
				a.editorContent(c)
				a.panelResizeHandle(c)
			})
			return
		}
		card := ui.Column(c).Fill().Radius(14).Clip().Background(t.Background)
		if t.Dark {
			card.Border(1, ui.RGBA(236, 236, 240, 0.12)).
				Shadow(0, 1, 1, 0, ui.RGBA(0, 0, 0, 0.2)).
				Shadow(0, 4, 6, -3, ui.RGBA(0, 0, 0, 0.24))
		} else {
			card.Border(1, ui.RGBA(0, 0, 0, 0.06)).
				Shadow(0, 1, 2, 0, ui.RGBA(0, 0, 0, 0.07)).
				Shadow(0, 1, 6, 0, ui.RGBA(0, 0, 0, 0.035))
		}
		card.Children(func() {
			a.editorContent(c)
		})
	})
}

// sidebarToggle is the one control that folds the sidebar away and brings it
// back. It does not move: it sits at the content's left edge, over the panel
// header while the panel is open, and the editor header slides under it and
// keeps room for it when the panel is folded — so folding the panel reads as
// the button staying put and the page coming to it, not as a second button
// appearing.
func (a *App) sidebarToggle(c *ui.Context) {
	x := float32(6)
	// The window's own buttons come first on macOS: the control starts after
	// the traffic lights, above everything the panel and the editor draw.
	if bar := c.TitleBar(); bar.Left+6 > x {
		x = bar.Left + 6
	}
	tooltip := locales.AiSidebarShow()
	icon := componenticons.PanelLeftClose()
	if a.panelOpen {
		tooltip = locales.AiSidebarHide()
		icon = componenticons.PanelLeftOpen()
	}
	btn := ui.ButtonBase(c).Key("sidebar-toggle").Absolute().Left(x).
		Top(a.topLine(c) - 13).Width(26).Height(26).Radius(6).Padding(0).
		Justify(ui.Center).AlignItems(ui.Center).Label(locales.SidebarLabel()).Tooltip(tooltip)
	if btn.Hovered() {
		btn.Background(c.Theme().SurfaceHover)
	}
	btn.OnClick(func() {
		if a.panelOpen {
			a.collapsePanel()
		} else {
			a.showPanel(a.panel)
		}
	})
	btn.Children(func() { ui.Icon(c, icon).Size(16, 16).TextColor(iconTint(c.Theme())) })
}

// rail is the 56px icon-only column. Its background is the window color; the active
// view is a raised card, as the demo's chosen sidebar row is.
func (a *App) rail(c *ui.Context) {
	bar := c.TitleBar()
	ui.Column(c).Width(railWidth).FillHeight().Background(sidebarColor(c.Theme())).Padding(0, railInset).GapY(railGapY).Children(func() {
		// The room above the first button: macOS's traffic lights when the
		// system reports them, else just enough to centre the first button
		// on the window's one top line.
		ui.Box(c).Height(max(bar.Height, topLineCenter-railButtonSize/2-railGapY)).DragWindow()
		// 最左侧只放模块切换；新建/打开都是文件模块的内容（面板顶部与菜单）。
		a.railButton(c, componenticons.Files(), locales.MenuFile(), a.panelOpen && a.panel == panelFiles, func() {
			a.showPanel(panelFiles)
		})
		a.railButton(c, componenticons.Outline(), locales.MenuOutline(), a.panelOpen && a.panel == panelOutline, func() {
			a.showPanel(panelOutline)
		})
		a.railButton(c, componenticons.Search(), locales.MenuSearchWorkspace(), false, func() {
			a.openWorkspaceSearch()
		})
		if a.aiEnabled {
			a.railButton(c, componenticons.AI(), locales.AiAssistant(), a.panelOpen && a.panel == panelAI, func() {
				a.showPanel(panelAI)
			})
		}
		ui.Spacer(c)
		a.railButton(c, componenticons.Settings(), locales.SettingsTitle(), a.settingsPanel.IsOpen(), a.openSettings)
		ui.Box(c).Height(8)
	})
}

// iconTint is the colour of the shell's line icons. In the light theme
// they keep the muted grey the labels use; on the dark shell that grey
// nearly vanishes, because the thin strokes antialias into the near-black
// surface, so they draw as a bright white there instead.
func iconTint(t *ui.Theme) ui.Color {
	if t.Dark {
		return ui.RGBA(255, 255, 255, 0.92)
	}
	return t.TextMuted
}

func (a *App) railButton(c *ui.Context, icon *ui.SVG, label string, active bool, fn func()) {
	t := c.Theme()
	b := ui.ButtonBase(c).Width(railButtonSize).Height(railButtonSize).Radius(8).Padding(0).
		Justify(ui.Center).AlignItems(ui.Center).Label(label).Description(label)
	switch {
	case active:
		if t.Dark {
			b.Background(ui.RGBA(255, 255, 255, 0.08))
		} else {
			b.Background(t.Background)
			b.Shadow(0, 0.5, 2, 0, ui.RGBA(0, 0, 0, 0.06))
		}
	case b.Hovered():
		b.Background(t.SurfaceHover)
	}
	b.OnClick(fn)
	b.Children(func() {
		el := ui.Icon(c, icon).Size(16, 16)
		if active {
			el.TextColor(t.Accent)
		} else {
			el.TextColor(iconTint(t))
		}
	})
	a.railTooltip(c, b, label)
}

// railTooltip shows the label to the right of the rail, where it does not
// cover the context panel's header or the rail's own icons.
func (a *App) railTooltip(c *ui.Context, anchor ui.Element, label string) {
	t := c.Theme()
	fill, text := t.Inverse, t.InverseText
	if fill == ui.Transparent {
		fill = t.Text
	}
	if text == ui.Transparent {
		text = t.Background
	}
	ui.TooltipBase(c, anchor, func(tip ui.Element) {
		tip.MaxWidth(220).Padding(5, 8).Radius(6).Background(fill).TextColor(text).
			FontSize(t.FontSize - 1)
		tip.Shadow(0, 2, 8, 0, ui.RGBA(0, 0, 0, 0.2))
		tip.Children(func() { ui.Text(c, label) })
		tip.AttachTo(anchor, ui.AnchorRight, ui.AnchorLeft).Margin(0, 0, 0, 6)
	})
}

func (a *App) contextPanel(c *ui.Context) {
	target := float32(0)
	if a.panelOpen {
		target = a.panelWidth
	}
	// 打开状态下宽度目标改变（双击边缘恢复默认、拖动等）时一帧到位：
	// 滑到一半的窄面板会把头部按钮裁出可视区，正文也要跟着重排两次。
	// 仍要走 AnimateTo，动画通道得保持连续——跳过调用会让下一次开关从
	// 0 起步。切换模块本身不改宽度，因此不会走这里。
	anim := ui.Animation{Duration: 180 * time.Millisecond, Ease: ui.EaseOut}
	if !a.panelTransition && a.panelOpen && a.panelPrevTarget > 0.5 && a.panelPrevTarget != target {
		anim.Duration = 0
	}
	a.panelPrevTarget = target
	// The panel is always built so its width can slide instead of vanishing
	// from one frame to the next. The content keeps the full panel width and
	// the outer box clips it, so text does not reflow on the way; it only
	// fades as the box closes.
	panel := ui.Column(c).Shrink(0).FillHeight().Clip()
	if a.panelDirect && !a.panelTransition {
		panel.Jump("width", target)
	}
	w := panel.AnimateTo("width", target, anim)
	if a.panelTransition && panel.Settled("width") {
		a.panelTransition = false
	}
	if a.panelDirect && !a.panelDragging {
		a.panelDirect = false
	}
	a.panelW = w
	panel.Width(w).Background(sidebarColor(c.Theme()))
	if w < 0.5 {
		return
	}
	openness := w / a.panelWidth
	if openness > 1 {
		openness = 1
	}
	panel.Children(func() {
		top := float32(canvasPadding + cardBorder)
		if a.flat {
			top = 0
		}
		ui.Column(c).Width(a.panelWidth).FillHeight().Opacity(openness).
			Padding(top, 0, 0, 0).Children(func() {
			a.sidebarHeader(c)

			ui.Column(c).Grow(1).FillWidth().MinHeight(0).Children(func() {
				switch a.panel {
				case panelFiles:
					a.filesView(c)
				case panelOutline:
					a.outlineView(c)
				case panelAI:
					a.aiView(c)
				case panelSearch:
					a.searchView(c)
				}
			})
			a.sidebarFooter(c)
			// The edge goes in last, so it hits above the content.
			if !a.flat {
				a.panelResizeHandle(c)
			}
			if a.fileTreeState.TrashTargetActive() && a.panel == panelFiles && !a.treeNativeHandoff {
				a.treeTrashDropTarget(c)
			}
		})
	})
}

// panelResizeHandle stays on the editor edge, including while folded.
// Crossing the fold threshold acts immediately; reversing the held drag reopens it.
func (a *App) panelResizeHandle(c *ui.Context) {
	left, handleWidth := a.panelWidth-12, float32(12)
	if a.flat {
		// Preserve the original placement: the hit target begins at the editor
		// edge and the grip is centered 6px inside the editor.
		left = 0
	}
	h := ui.Box(c).Key("panel-resize").Absolute().Left(left).
		Top(canvasPadding + cardBorder + editorHeaderH).
		Bottom(0).Width(handleWidth).Cursor(ui.CursorResizeEW).Label(locales.SidebarResize()).
		Column().AlignItems(ui.Center)
	if dx, _, ok := h.Dragged(); ok {
		if !a.panelDragging {
			a.panelDragging = true
			a.panelDrag = 0
			if a.panelOpen {
				a.panelDrag = a.panelWidth
			}
		}
		if a.panelAwaitHandle {
			px, _, _ := h.PointerPosition()
			// Wait for the final handle position, not its animated intermediate edge.
			if h.Bounds().X+px >= a.panelWidth {
				a.panelAwaitHandle = false
				a.panelDrag = a.panelWidth
				a.panelTransition = false
				a.panelDirect = true
			}
			dx = 0
		}
		if !a.panelAwaitHandle {
			a.panelDrag += dx
			wasOpen := a.panelOpen
			if wasOpen && a.panelDrag < panelFoldOn {
				a.panelOpen = false
				a.panelDrag = 0
			} else if !wasOpen && a.panelDrag >= panelUnfoldOn {
				a.panelOpen = true
				a.panelAwaitHandle = true
				a.panelDrag = a.panelWidth
			}
			if wasOpen != a.panelOpen {
				a.panelTransition = true
				a.panelDirect = false
			} else {
				a.panelDirect = !a.panelTransition
			}
			a.panelNarrow = !a.panelOpen
			if a.panelOpen {
				a.panelWidth = min(max(a.panelDrag, panelMinWidth), panelMaxWidth)
			}
		}
		a.invalidate()
	} else if a.panelDragging {
		a.panelDragging = false
		a.panelAwaitHandle = false
		a.panelDrag = 0
		a.panelNarrow = false
	}
	if h.DoubleClicked() {
		a.panelOpen = true
		a.panelWidth = a.panelTargetWidth()
		a.invalidate()
	}
	h.Children(func() {
		// Shift the visual grip slightly above the resize region's midpoint;
		// the title/header area above the region otherwise makes it look low
		// relative to the whole window.
		ui.Box(c).Grow(0.9)
		a.panelGrip(c, h)
		ui.Box(c).Grow(1.1)
	})
}

// panelGrip is the grip on the panel's edge: a short rounded bar on the
// hairline, there while the pointer is on the edge and, brighter and longer,
// while it is dragged. It is a child of the edge box, so it is part of what a
// drag grabs and never takes the pointer away from it.
func (a *App) panelGrip(c *ui.Context, edge ui.Element) {
	t := c.Theme()
	_, _, pointerOver := edge.PointerPosition()
	hovered := edge.Hovered() || pointerOver
	held := edge.Pressed() || edge.Dragging() || a.panelDragging
	height, width, opacity := float32(8), float32(2), float32(0)
	col := t.TextMuted
	switch {
	case held && a.panelNarrow:
		height, width, opacity = 56, 4, 0.95
		col = t.Accent
	case held:
		height, width, opacity = 56, 4, 0.85
		col = t.Text
	case hovered:
		height, width, opacity = 40, 3, 0.5
	}
	grip := ui.Box(c).Key("panel-grip").Label(locales.SidebarHandle()).Radius(2).Background(col.Alpha(opacity))
	if held || a.panelGripHeld {
		// Hover shows the normal grip immediately. Holding it stretches the
		// grip with a short spring; releasing springs back to its normal or
		// hidden size without animating the hover-only appearance.
		motion := ui.Animation{Spring: ui.SpringOptions{Duration: 220 * time.Millisecond, Bounce: 0.16}}
		grip.Width(grip.AnimateTo("width", width, motion)).
			Height(grip.AnimateTo("height", height, motion))
		if !held && grip.Settled("width") && grip.Settled("height") {
			a.panelGripHeld = false
		}
	} else {
		grip.Width(grip.Jump("width", width)).
			Height(grip.Jump("height", height))
	}
	if held {
		a.panelGripHeld = true
	}
}

// panelHeader is the line over the context panel: the module names show on
// the rail, so the line holds only the way to fold the panel away, and the
// room to drag the window.
// sidebarHeader separates window controls from workspace actions.
func (a *App) sidebarHeader(c *ui.Context) {
	t := c.Theme()
	left := float32(34)
	if bar := c.TitleBar(); bar.Left+34 > left {
		left = bar.Left + 34
	}
	ui.Row(c).Height(a.headerH(c)).Shrink(0).Padding(0, 8, 0, left).DragWindow().AlignItems(ui.Center).Children(func() {
		if a.aiEnabled {
			a.smallIcon(c, componenticons.AI(), locales.AiAssistant(), func() { a.showPanel(panelAI) })
		}
	})
	ui.Row(c).Height(44).Shrink(0).Padding(0, 10).Gap(4).AlignItems(ui.Center).Children(func() {
		name := a.spaceName()
		space := ui.ButtonBase(c).Height(28).Padding(0, 4).Radius(6).Label(name).Tooltip(locales.WorkspaceLabel())
		space.Menu(func(m *ui.Menu) {
			if m.Item(locales.FilesOpenFolderMenu()).Chosen() {
				a.openFolderDialog()
			}
			if m.Item(locales.FilesOpenFileMenu()).Chosen() {
				a.openFileDialog()
			}
		})
		space.Children(func() {
			ui.Row(c).Gap(3).AlignItems(ui.Center).Children(func() {
				ui.Text(c, name).FontSize(13).FontWeight(600).SingleLine().Ellipsis("…")
				ui.Icon(c, componenticons.ChevronDown()).Size(12, 12).TextColor(t.TextMuted)
			})
		})
		ui.Spacer(c)
		a.smallIcon(c, componenticons.Search(), locales.MenuSearchWorkspace(), func() { a.openWorkspaceSearch() })
		ui.Row(c).Height(28).Radius(8).Background(t.Surface).AlignItems(ui.Center).Children(func() {
			a.smallIcon(c, componenticons.Edit(), locales.DocumentNew(), func() {
				if a.root != "" {
					a.newFileIn(a.fileTreeState.CreateTarget(a.root, a.selectedPath, a.catalogIsDirectory(a.selectedPath)))
				} else {
					a.newDoc()
				}
			})
			more := ui.ButtonBase(c).Size(20, 28).Padding(0).Label(locales.DocumentNewMenu()).Tooltip(locales.DocumentNewMenu())
			more.Menu(func(m *ui.Menu) {
				if m.Item(locales.FilesNewFolderMenu()).Chosen() {
					a.newFolderIn(a.fileTreeState.CreateTarget(a.root, a.selectedPath, a.catalogIsDirectory(a.selectedPath)))
				}
			})
			more.Children(func() { ui.Icon(c, componenticons.ChevronDown()).Size(11, 11).TextColor(t.TextMuted) })
		})
	})
	if a.panel != panelFiles {
		ui.Row(c).Height(30).Padding(0, 10).AlignItems(ui.Center).Children(func() {
			a.smallIcon(c, componenticons.ChevronLeft(), locales.FilesBackToTree(), func() { a.showPanel(panelFiles) })
		})
	}
}

// sidebarFooter holds the settings of the app itself, at the sidebar's foot.
func (a *App) sidebarFooter(c *ui.Context) {
	ui.Row(c).Shrink(0).Padding(0, 8, 10, 8).Gap(2).Label(locales.SidebarFixedFooter()).Children(func() {
		a.smallIcon(c, componenticons.Settings(), locales.SettingsTitle(), a.openSettings)
	})
}

// spaceName is the name of the place the editor is in: the workspace's folder,
// or the document's own.
func (a *App) spaceName() string {
	switch {
	case a.root != "":
		return filepath.Base(a.root)
	case a.documentPath() != "":
		return filepath.Base(filepath.Dir(a.documentPath()))
	}
	return locales.DocumentUntitled()
}

func (a *App) smallIcon(c *ui.Context, icon *ui.SVG, label string, fn func()) {
	a.smallIconTint(c, icon, label, fn, iconTint(c.Theme()))
}

// smallIconTint is smallIcon in a color of the caller's: a step that cannot be
// taken is the same icon, faint, and does nothing.
func (a *App) smallIconTint(c *ui.Context, icon *ui.SVG, label string, fn func(), tint ui.Color) ui.Element {
	b := ui.ButtonBase(c).Width(26).Height(26).Radius(6).Padding(0).
		Justify(ui.Center).AlignItems(ui.Center).Label(label).Tooltip(label)
	if b.Hovered() && fn != nil {
		b.Background(c.Theme().SurfaceHover)
	}
	if fn != nil {
		b.OnClick(fn)
	}
	b.Children(func() { ui.Icon(c, icon).Size(16, 16).TextColor(tint) })
	return b
}

func (a *App) startBusy() {
	if a.busy {
		return
	}
	a.busy = true
	if a.win != nil {
		a.win.Invalidate()
	}
}

// finish runs the state change on the UI thread and clears the busy guard.
// Every blocking desktop dialog runs in a goroutine: MyGo's dialogs wait for
// the main loop, so calling one from a click callback deadlocks the window.
func (a *App) onMain(fn func()) {
	if a.win == nil {
		// Headless: there is no event loop to post to, so the caller is a
		// background goroutine. Serialize with the frame it would race.
		a.mainMu.Lock()
		defer a.mainMu.Unlock()
		fn()
		return
	}
	mygo.RunOnMain(fn)
}

func (a *App) finish(fn func()) {
	a.onMain(func() {
		if fn != nil {
			fn()
		}
		a.busy = false
		if a.win != nil {
			a.win.Invalidate()
		}
	})
}

// pushNav records a document in the editor's own history, cutting whatever
// was ahead of it: a new document is a new branch.
func (a *App) pushNav(path string) {
	a.navigationHistory.Push(path)
}

// canNav reports whether there is a document delta steps back (-1) or forward
// (+1) from where the editor is.
func (a *App) canNav(delta int) bool {
	return a.navigationHistory.CanMove(delta)
}

// nav moves the editor through its history.
func (a *App) nav(delta int) {
	if !a.canNav(delta) {
		return
	}
	target, ok := a.navigationHistory.Plan(delta)
	if !ok {
		return
	}
	path := target.Path
	if doctypes.IsImagePath(path) {
		a.navigationHistory.Commit(target)
		a.openImageDocument(path, false)
		return
	}
	// Returning from an image to the still-open note must reveal its editor
	// state, including unsaved edits, rather than reload the on-disk version.
	if path == a.documentPath() {
		a.navigationHistory.Commit(target)
		a.closeImageDocument()
		a.noteEditor().FocusCommands().Focus()
		a.updateTitle()
		a.invalidate()
		return
	}
	if a.documentDirty() {
		if a.win == nil || a.busy {
			return
		}
		a.startBusy()
		go func() {
			if !a.confirmDiscardOffUI() {
				a.finish(nil)
				return
			}
			a.finish(func() {
				a.navigationHistory.Commit(target)
				a.openPathNow(path)
			})
		}()
		return
	}
	a.navigationHistory.Commit(target)
	a.openPathNow(path)
}

// docPath is the document's path within its workspace. The workspace folder
// itself is the vault root and is not repeated in a vault-relative path.
func (a *App) docPath() string {
	if a.imageDocumentOpen() {
		path := a.imageDocumentPath()
		if a.root != "" {
			if rel, err := filepath.Rel(a.root, path); err == nil && !strings.HasPrefix(rel, "..") {
				return filepath.ToSlash(rel)
			}
		}
		return path
	}
	if a.documentPath() == "" {
		return locales.DocumentUntitled()
	}
	if a.root == "" {
		return a.documentPath()
	}
	if rel, err := filepath.Rel(a.root, a.documentPath()); err == nil && !strings.HasPrefix(rel, "..") {
		return filepath.ToSlash(rel)
	}
	return a.documentPath()
}

// tailPath cuts a path from its front until it fits width, keeping the end:
// the file's own name is what tells one document from another.
func (a *App) tailPath(c *ui.Context, path string, size, width float32) string {
	measure := func(s string) float32 {
		w, _ := c.MeasureText(0, ui.Span{Text: s, Size: size})
		return w
	}
	if width <= 0 || measure(path) <= width {
		return path
	}
	parts := strings.Split(path, "/")
	for i := 1; i < len(parts); i++ {
		if cut := "…/" + strings.Join(parts[i:], "/"); measure(cut) <= width {
			return cut
		}
	}
	// The last segment alone is still too long: cut it, keeping its end.
	last := parts[len(parts)-1]
	r := []rune(last)
	for i := 1; i < len(r); i++ {
		if cut := "…" + string(r[i:]); measure(cut) <= width {
			return cut
		}
	}
	return "…"
}

func (a *App) displayName() string {
	if a.imageDocumentOpen() {
		return filepath.Base(a.imageDocumentPath())
	}
	if a.documentPath() == "" {
		return locales.DocumentUntitled()
	}
	return filepath.Base(a.documentPath())
}

func (a *App) location() string {
	if a.documentPath() == "" {
		return locales.DocumentNotSaved()
	}
	if a.root != "" {
		if rel, err := filepath.Rel(a.root, a.documentPath()); err == nil && !strings.HasPrefix(rel, "..") {
			dir := filepath.Dir(rel)
			if dir == "." {
				return ""
			}
			return filepath.ToSlash(dir)
		}
	}
	return filepath.Dir(a.documentPath())
}

// panelTargetWidth 是面板的默认宽度，也是双击边缘恢复到的宽度。四个模块
// 共用这一个宽度：切换模块时面板不伸缩，编辑器不会跟着重排。
func (a *App) panelTargetWidth() float32 {
	return panelDefaultWidth
}

// showPanel selects a module without ever collapsing or resizing: the rail
// is a view switch, and clicking the active icon twice used to hide the
// panel.
func (a *App) showPanel(v panelView) {
	a.panel = v
	if a.panelWidth <= 0 {
		a.panelWidth = a.panelTargetWidth()
	}
	a.panelOpen = true
	a.invalidate()
}

// collapsePanel is the panel header's dedicated close control.
func (a *App) collapsePanel() {
	a.panelOpen = false
	a.invalidate()
}

func (a *App) togglePanel(v panelView) {
	if a.panelOpen && a.panel == v {
		a.panelOpen = false
	} else {
		a.panel = v
		if a.panelWidth <= 0 {
			a.panelWidth = a.panelTargetWidth()
		}
		a.panelOpen = true
	}
	a.invalidate()
}

// canvasLeft is how far the editor card sits from the rail: 8px, plus the
// traffic lights' inset once the panel is gone (the panel normally gives
// them room). It follows the animated panel width so the card slides with
// the panel instead of jumping when panelOpen flips.
// headerH is the height of the first line of either pane. In a window whose
// title bar the view draws under the traffic lights, that line is as tall as
// they are, so its controls centre on them instead of a few pixels below.
func (a *App) headerH(c *ui.Context) float32 {
	h := float32(editorHeaderH)
	if a.flat {
		if bar := c.TitleBar(); bar.Height > 0 {
			h = max(h, bar.Height)
		}
	}
	return h
}

// topLine is where the controls of the first line centre.
func (a *App) topLine(c *ui.Context) float32 {
	if a.flat {
		return a.headerH(c) / 2
	}
	return canvasPadding + cardBorder + editorHeaderH/2
}

func (a *App) canvasLeft(c *ui.Context) float32 {
	if a.flat {
		return 0
	}
	left := canvasPadding
	if a.panelWidth <= 0 {
		return left
	}
	openness := a.panelW / a.panelWidth
	if openness > 1 {
		openness = 1
	}
	if openness < 0 {
		openness = 0
	}
	if extra := c.TitleBar().Left - railWidth; extra > 0 {
		left += extra * (1 - openness)
	}
	return left
}

func (a *App) notice(s string) {
	a.status = s
	a.statusAt = time.Now()
	if a.win != nil {
		time.AfterFunc(3*time.Second, func() {
			if time.Since(a.statusAt) >= 3*time.Second {
				a.status = ""
				a.win.Invalidate()
			}
		})
	}
}

func (a *App) updateTitle() {
	if a.win == nil {
		return
	}
	name := a.displayName()
	if a.documentDirty() {
		name = "● " + name
	}
	a.win.SetTitle(name + " — " + locales.AppTitle())
}

func (a *App) invalidate() {
	if a.win != nil {
		a.win.Invalidate()
	}
}

const (
	railWidth      float32 = 56
	railButtonSize float32 = 34
	// railInset centres the 34px buttons in the 56px rail instead of letting
	// them hug the window edge: (56-34)/2 = 11px on each side.
	railInset = (railWidth - railButtonSize) / 2

	// canvasPadding is how far the editor card sits from the window's top,
	// right and bottom, and from the context panel; editorHeaderH is the
	// height of the 44px line the panel header and the editor header share.
	canvasPadding float32 = 8
	editorHeaderH float32 = 44

	// railGapY is the vertical rhythm between the rail's buttons.
	railGapY float32 = 4
	// cardBorder is the editor card's hairline. As in CSS it takes room
	// inside the card, so what the card holds sits 1 DIP inside the padding
	// the rest of the shell measures from.
	cardBorder float32 = 1
	// topLineCenter is the height every control on the window's first line
	// centres on: the rail's module buttons, the panel header's title and
	// close, the editor header's title and the window buttons alike.
	topLineCenter = canvasPadding + cardBorder + editorHeaderH/2

	// The context panel's width, as a drag at its edge moves it: between
	// these, and folding away when a drag releases past foldWidth.
	// panelDefaultWidth is the one width of every module: switching views
	// never resizes the panel, and so never reflows the editor beside it.
	panelDefaultWidth float32 = 268
	panelMinWidth     float32 = 224
	panelMaxWidth     float32 = 440
	panelFoldOn       float32 = 200
	panelUnfoldOn     float32 = 36
)

// panelView is the page the context panel shows.
type panelView int

const (
	panelFiles panelView = iota
	panelOutline
	panelAI
	panelSearch
)

// setAppAppearance tells the system which way to draw its own surfaces —
// the acrylic material behind the rail and the panel, the title bar — so
// they follow the theme the page draws instead of the desktop's. It is a
// seam for tests: the real call waits on the app's main loop, and there is
// nothing to restyle before the app is ready.
var setAppAppearance = func(s mygo.ThemeSource) {
	if mygo.App.IsReady() {
		mygo.Theme.SetSource(s)
	}
}

// Dialog seams keep the blocking desktop APIs testable and document that
// every call runs off the UI thread.
var (
	openDialog    = mygo.Dialog.Open
	saveDialog    = mygo.Dialog.Save
	messageDialog = mygo.Dialog.Message
	errorDialog   = mygo.Dialog.Error
	shellOpenPath = mygo.Shell.OpenPath
)
