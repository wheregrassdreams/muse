package nativeui

import (
	"log"
	"os"
	"runtime"
	"strconv"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"muse/locales"
	"muse/nativeui/workbench/diagnostics"
	"muse/nativeui/workbench/features/editor/surface"
	searchui "muse/nativeui/workbench/features/search"
	settingsui "muse/nativeui/workbench/features/settings"
	"muse/nativeui/workbench/shell/windowcontrols"
)

// Run opens the native Muse window and runs the app loop. It is called by the
// native entry point; nothing in this package needs a webview or a frontend
// build.
func Run(dependencies Dependencies) {
	app := New()
	app.SetDependencies(dependencies)
	diagnostics.Start(mygo.RunOnMain, app.memoryDiagnosticsSnapshot)
	mygo.App.SetName("Muse")
	mygo.App.OnLocaleChanged(func(string) {
		app.updateTitle()
		app.installMenu()
		app.invalidate()
	})
	mygo.App.WhenReady(func() {
		app.applyNativePreferences(app.loadNativePreferences())
		app.enableDocumentServices()
		switch os.Getenv("MUSE_THEME") {
		case "light":
			app.themeMode = themeLight
		case "dark":
			app.themeMode = themeDark
		}
		// 材料在窗口创建时就按应用外观画好，先定下来再开窗。
		app.syncAppearance()
		// Explicit automation paths take precedence over the last session.
		if os.Getenv("MUSE_WORKSPACE") == "" && os.Getenv("MUSE_OPEN") == "" {
			app.restoreWorkspaceState()
		}
		if ws := os.Getenv("MUSE_WORKSPACE"); ws != "" {
			app.setWorkspace(ws)
		}
		if p := os.Getenv("MUSE_OPEN"); p != "" {
			app.openPathNow(p)
		}
		if os.Getenv("MUSE_SETTINGS") == "1" {
			app.settingsPanel.OpenAt(settingsui.AppearancePage)
		}
		// Capture hooks: MUSE_CAPTURE photographs the window, and
		// MUSE_SCROLL puts it where the shot should show.
		if off := os.Getenv("MUSE_SCROLL"); off != "" {
			if n, err := strconv.Atoi(off); err == nil {
				app.noteEditor().NavigationCommands().ScrollToOffset(n)
			}
		}
		// TEMP capture hook
		switch os.Getenv("MUSE_PANEL") {
		case "properties":
			app.propertiesPanel.Open()
		case "vault-search":
			app.openWorkspaceSearch()
			app.workspaceSearch.Apply(searchui.Action{Kind: searchui.QueryChanged, Query: os.Getenv("MUSE_SEARCH_QUERY")})
		case "outline":
			app.showPanel(panelOutline)
		case "search":
			app.showPanel(panelSearch)
		case "ai":
			if app.aiEnabled {
				app.showPanel(panelAI)
			}
		case "closed":
			app.panelOpen = false
			app.panelW = 0
		}
		app.openWindow()
	})
	mygo.Theme.OnUpdated(app.invalidate)
	mygo.App.OnActivate(func(hasVisibleWindows bool) {
		if !hasVisibleWindows {
			app.openWindow()
		}
	})
	// macOS keeps the app alive after its window closes, as every Mac app does.
	mygo.App.OnWindowAllClosed(func() {
		if runtime.GOOS != "darwin" {
			mygo.App.Quit()
		}
	})
	mygo.App.OnWillQuit(func(*mygo.QuitEvent) { app.closeLifecycle() })
	err := mygo.App.Run()
	app.closeLifecycle()
	if err != nil {
		log.Fatal(err)
	}
}

func (a *App) openWindow() {
	if a.win != nil && !a.win.IsDestroyed() {
		a.win.Show()
		a.win.Focus()
		return
	}
	a.win = mygo.NewWindow(mygo.WindowOptions{
		Title:           locales.AppTitle(),
		Width:           1200,
		Height:          800,
		MinWidth:        720,
		MinHeight:       480,
		BackgroundColor: "light-dark(#f5f5f5, #1a1a1a)",
		StateKey:        "muse-native",
		Content:         ui.View(a.View),
		// 亚克力：窗口背景透明，桌面的模糊由系统的材料给出；视图只在
		// 编辑卡片上画不透明的表面，轨道与面板后面透出材料来。不支持
		// 材料的平台上视图自己画不透明的底色。
		Transparent: true,
		Vibrancy:    mygo.VibrancyAcrylic,
		// The rail's first 44px hold the traffic lights; the rest of the
		// view starts beside them, as the web app did. On Windows and
		// Linux the system's buttons go away and the view draws its own.
		TitleBarStyle:        mygo.TitleBarHidden,
		TitleBarHeight:       44,
		TrafficLightPosition: &mygo.Point{X: 12, Y: 15},
		HideWindowButtons:    windowcontrols.Visible(),
	})
	a.installMenu()
	a.updateTitle()
	// The window buttons' drawing changes with the maximize state, and the
	// state can change from outside the view (a double click on the header,
	// the system's snap). Redraw when it does.
	a.win.OnMaximize(a.invalidate)
	a.win.OnClose(func(e *mygo.CloseEvent) {
		if !a.documentDirty() {
			a.persistWorkspaceState()
			a.closeVaultSessionNow()
			return
		}
		e.PreventDefault()
		if a.closing || a.busy {
			return
		}
		a.closing = true
		a.startBusy()
		// Message dialogs block on the main loop; ask from a goroutine and
		// hop back only to close the window.
		go func() {
			ok := a.confirmDiscardOffUI()
			mygo.RunOnMain(func() {
				a.closing = false
				a.busy = false
				if ok {
					a.persistWorkspaceState()
					a.win.Close()
					return
				}
				a.win.Invalidate()
			})
		}()
	})

	// MUSE_CAPTURE=/path.png renders the native window once and exits: the
	// same tool mdshot gave the webview, now without a webview.
	a.win.Show()
	a.win.Focus()
	mygo.App.Focus()

	if path := os.Getenv("MUSE_CAPTURE"); path != "" {
		go func() {
			delay := 1200 * time.Millisecond
			if ms := os.Getenv("MUSE_CAPTURE_DELAY_MS"); ms != "" {
				if n, err := strconv.Atoi(ms); err == nil {
					delay = time.Duration(n) * time.Millisecond
				}
			}
			time.Sleep(delay)
			if png, err := a.win.CapturePage(); err == nil {
				_ = os.WriteFile(path, png, 0o644)
			}
			mygo.App.Exit(0)
		}()
	}
}

func (a *App) installMenu() {
	if a.win == nil {
		return
	}
	w := a.win
	click := func(fn func()) func(*mygo.MenuItem, *mygo.Window) {
		return func(*mygo.MenuItem, *mygo.Window) {
			fn()
			w.Invalidate()
		}
	}
	mygo.App.SetMenu(mygo.NewMenu([]*mygo.MenuItem{
		{Role: mygo.RoleAppMenu},
		{
			Label: locales.MenuFile(),
			Submenu: []*mygo.MenuItem{
				{Label: locales.MenuNew(), Accelerator: "CmdOrCtrl+N", Click: click(a.newDoc)},
				{Label: locales.MenuCommandPalette(), Accelerator: "CmdOrCtrl+P", Click: click(a.openCommandPalette)},
				{Label: locales.MenuWorkspaceSearch(), Accelerator: "CmdOrCtrl+O", Click: click(a.openWorkspaceSearch)},
				{Label: locales.MenuOpen(), Click: click(a.openFileDialog)},
				{Label: locales.MenuOpenFolder(), Accelerator: "Shift+CmdOrCtrl+O", Click: click(a.openFolderDialog)},
				mygo.Separator(),
				{Label: locales.MenuSave(), Accelerator: "CmdOrCtrl+S", Click: click(a.save)},
				{Label: locales.MenuSaveAs(), Accelerator: "Shift+CmdOrCtrl+S", Click: click(a.saveAs)},
				mygo.Separator(),
				{Role: mygo.RoleClose},
			},
		},
		{Role: mygo.RoleEditMenu},
		// 段落与格式：快捷键由编辑器自己处理（见右键菜单），这里不再挂
		// Accelerator，免得同一按键在菜单和编辑器里各触发一次。
		{Label: locales.MenuParagraph(), Submenu: a.paragraphMenu(click)},
		{Label: locales.MenuFormat(), Submenu: a.formatMenu(click)},
		{
			Label: locales.MenuView(),
			Submenu: []*mygo.MenuItem{
				{Label: locales.MenuFiles(), Accelerator: "Shift+CmdOrCtrl+L", Click: click(func() { a.togglePanel(panelFiles) })},
				{Label: locales.MenuOutline(), Click: click(func() { a.togglePanel(panelOutline) })},
				{Label: locales.MenuFindDocument(), Accelerator: "CmdOrCtrl+F", Click: click(func() { a.closeWorkspaceSearch(); a.showPanel(panelSearch) })},
				{Label: locales.MenuSearchWorkspace(), Accelerator: "CmdOrCtrl+Shift+F", Click: click(a.openWorkspaceSearch)},
				{Label: locales.MenuFlatStyle(), Click: click(func() { a.flat = !a.flat; a.invalidate() })},
				{Label: locales.MenuHeaderFade(), Click: click(func() { a.headerFade = !a.headerFade; a.invalidate() })},
				mygo.Separator(),
				{Label: locales.MenuSettings(), Accelerator: "CmdOrCtrl+,", Click: click(a.openSettings)},
			},
		},
		{Role: mygo.RoleWindowMenu},
	}))
}

// editorAction runs fn on the editor and gives it the keyboard back, as a
// menu item takes it.
func (a *App) editorAction(fn func(e noteeditor.FormattingCommands)) func() {
	return func() {
		if a.noteEditor() == nil || a.noteEditor().FormattingCommands() == nil {
			return
		}
		fn(a.noteEditor().FormattingCommands())
		a.noteEditor().FocusCommands().Focus()
	}
}

// paragraphMenu is the menu bar's 段落: headings, quotations, lists and
// the blocks of code, math and tables.
func (a *App) paragraphMenu(click func(func()) func(*mygo.MenuItem, *mygo.Window)) []*mygo.MenuItem {
	var items []*mygo.MenuItem
	for i, name := range []string{locales.ParagraphHeading1(), locales.ParagraphHeading2(), locales.ParagraphHeading3(), locales.ParagraphHeading4(), locales.ParagraphHeading5(), locales.ParagraphHeading6()} {
		level := i + 1
		items = append(items, &mygo.MenuItem{Label: name, Click: click(a.editorAction(func(e noteeditor.FormattingCommands) { e.SetHeading(level) }))})
	}
	return append(items,
		&mygo.MenuItem{Label: locales.ParagraphBody(), Click: click(a.editorAction(func(e noteeditor.FormattingCommands) { e.SetHeading(0) }))},
		mygo.Separator(),
		&mygo.MenuItem{Label: locales.ParagraphQuote(), Click: click(a.editorAction(noteeditor.FormattingCommands.ToggleQuote))},
		&mygo.MenuItem{Label: locales.ParagraphOrderedList(), Click: click(a.editorAction(func(e noteeditor.FormattingCommands) { e.ToggleList("1.") }))},
		&mygo.MenuItem{Label: locales.ParagraphUnorderedList(), Click: click(a.editorAction(func(e noteeditor.FormattingCommands) { e.ToggleList("-") }))},
		&mygo.MenuItem{Label: locales.ParagraphTaskList(), Click: click(a.editorAction(func(e noteeditor.FormattingCommands) { e.ToggleList("- [ ]") }))},
		mygo.Separator(),
		&mygo.MenuItem{Label: locales.ParagraphCodeBlock(), Click: click(a.editorAction(noteeditor.FormattingCommands.InsertCodeBlock))},
		&mygo.MenuItem{Label: locales.ParagraphMathBlock(), Click: click(a.editorAction(noteeditor.FormattingCommands.InsertMathBlock))},
		&mygo.MenuItem{Label: locales.ParagraphTable(), Click: click(a.editorAction(func(e noteeditor.FormattingCommands) { e.InsertTable(3, 3) }))},
		&mygo.MenuItem{Label: locales.ParagraphRule(), Click: click(a.editorAction(noteeditor.FormattingCommands.InsertRule))},
	)
}

// formatMenu is the menu bar's 格式: the formats of text.
func (a *App) formatMenu(click func(func()) func(*mygo.MenuItem, *mygo.Window)) []*mygo.MenuItem {
	return []*mygo.MenuItem{
		{Label: locales.FormatBold(), Click: click(a.editorAction(noteeditor.FormattingCommands.ToggleBold))},
		{Label: locales.FormatItalic(), Click: click(a.editorAction(noteeditor.FormattingCommands.ToggleItalic))},
		{Label: locales.FormatUnderline(), Click: click(a.editorAction(noteeditor.FormattingCommands.ToggleUnderline))},
		{Label: locales.FormatStrike(), Click: click(a.editorAction(noteeditor.FormattingCommands.ToggleStrike))},
		{Label: locales.FormatInlineCode(), Click: click(a.editorAction(noteeditor.FormattingCommands.ToggleCode))},
		{Label: locales.FormatHighlight(), Click: click(a.editorAction(noteeditor.FormattingCommands.ToggleHighlight))},
		mygo.Separator(),
		{Label: locales.FormatLink(), Click: click(a.editorAction(noteeditor.FormattingCommands.InsertLink))},
	}
}
