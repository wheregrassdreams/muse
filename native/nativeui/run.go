package nativeui

import (
	"log"
	"os"
	"runtime"
	"strconv"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"muse/mdedit"
)

// Run opens the native Muse window and runs the app loop. It is called by the
// native entry point; nothing in this package needs a webview or a frontend
// build.
func Run() {
	app := New()
	mygo.App.SetName("Muse")
	mygo.App.WhenReady(func() {
		switch os.Getenv("MUSE_THEME") {
		case "light":
			app.themeMode = 0
		case "dark":
			app.themeMode = 1
		default:
			if mygo.Theme.IsDark() {
				app.themeMode = 1
			}
		}
		// 材料在窗口创建时就按应用外观画好，先定下来再开窗。
		app.syncAppearance()
		if ws := os.Getenv("MUSE_WORKSPACE"); ws != "" {
			app.setWorkspace(ws)
		}
		if p := os.Getenv("MUSE_OPEN"); p != "" {
			app.openPathNow(p)
		}
		if os.Getenv("MUSE_SETTINGS") == "1" {
			app.settingsOpen = true
		}
		// TEMP capture hook
		switch os.Getenv("MUSE_PANEL") {
		case "outline":
			app.showPanel(panelOutline)
		case "search":
			app.showPanel(panelSearch)
		case "ai":
			app.showPanel(panelAI)
		case "closed":
			app.panelOpen = false
			app.panelW = 0
		}
		app.openWindow()
	})
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
	if err := mygo.App.Run(); err != nil {
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
		Title:           "Muse",
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
		HideWindowButtons:    controlsAtRight,
	})
	a.installMenu()
	a.updateTitle()
	// The window buttons' drawing changes with the maximize state, and the
	// state can change from outside the view (a double click on the header,
	// the system's snap). Redraw when it does.
	a.win.OnMaximize(a.invalidate)
	a.win.OnClose(func(e *mygo.CloseEvent) {
		if !a.dirty {
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
			Label: "文件",
			Submenu: []*mygo.MenuItem{
				{Label: "新建", Accelerator: "CmdOrCtrl+N", Click: click(a.newDoc)},
				{Label: "命令面板…", Accelerator: "CmdOrCtrl+P", Click: click(a.openCommandPalette)},
				{Label: "跳转到文件…", Accelerator: "CmdOrCtrl+O", Click: click(a.openQuickOpen)},
				{Label: "打开…", Click: click(a.openFileDialog)},
				{Label: "打开文件夹…", Accelerator: "Shift+CmdOrCtrl+O", Click: click(a.openFolderDialog)},
				mygo.Separator(),
				{Label: "保存", Accelerator: "CmdOrCtrl+S", Click: click(a.save)},
				{Label: "另存为…", Accelerator: "Shift+CmdOrCtrl+S", Click: click(a.saveAs)},
				mygo.Separator(),
				{Role: mygo.RoleClose},
			},
		},
		{Role: mygo.RoleEditMenu},
		// 段落与格式：快捷键由编辑器自己处理（见右键菜单），这里不再挂
		// Accelerator，免得同一按键在菜单和编辑器里各触发一次。
		{Label: "段落", Submenu: a.paragraphMenu(click)},
		{Label: "格式", Submenu: a.formatMenu(click)},
		{
			Label: "视图",
			Submenu: []*mygo.MenuItem{
				{Label: "文件栏", Accelerator: "Shift+CmdOrCtrl+L", Click: click(func() { a.togglePanel(panelFiles) })},
				{Label: "大纲", Click: click(func() { a.togglePanel(panelOutline) })},
				{Label: "搜索", Accelerator: "CmdOrCtrl+F", Click: click(func() { a.togglePanel(panelSearch) })},
				{Label: "AI 助手", Click: click(func() { a.togglePanel(panelAI) })},
				mygo.Separator(),
				{Label: "切换主题", Accelerator: "CmdOrCtrl+Shift+T", Click: click(func() { a.themeMode = 1 - a.themeMode })},
				{Label: "扁平风格", Accelerator: "CmdOrCtrl+Shift+F", Click: click(func() { a.flat = !a.flat; a.invalidate() })},
				{Label: "顶部模糊", Click: click(func() { a.headerFade = !a.headerFade; a.invalidate() })},
				mygo.Separator(),
				{Label: "设置…", Accelerator: "CmdOrCtrl+,", Click: click(a.openSettings)},
			},
		},
		{Role: mygo.RoleWindowMenu},
	}))
}

// editorAction runs fn on the editor and gives it the keyboard back, as a
// menu item takes it.
func (a *App) editorAction(fn func(e *mdedit.Editor)) func() {
	return func() {
		fn(a.ed)
		a.ed.Focus()
	}
}

// paragraphMenu is the menu bar's 段落: headings, quotations, lists and
// the blocks of code, math and tables.
func (a *App) paragraphMenu(click func(func()) func(*mygo.MenuItem, *mygo.Window)) []*mygo.MenuItem {
	var items []*mygo.MenuItem
	for i, name := range []string{"一级标题", "二级标题", "三级标题", "四级标题", "五级标题", "六级标题"} {
		level := i + 1
		items = append(items, &mygo.MenuItem{Label: name, Click: click(a.editorAction(func(e *mdedit.Editor) { e.SetHeading(level) }))})
	}
	return append(items,
		&mygo.MenuItem{Label: "正文", Click: click(a.editorAction(func(e *mdedit.Editor) { e.SetHeading(0) }))},
		mygo.Separator(),
		&mygo.MenuItem{Label: "引用", Click: click(a.editorAction((*mdedit.Editor).ToggleQuote))},
		&mygo.MenuItem{Label: "有序列表", Click: click(a.editorAction(func(e *mdedit.Editor) { e.ToggleList("1.") }))},
		&mygo.MenuItem{Label: "无序列表", Click: click(a.editorAction(func(e *mdedit.Editor) { e.ToggleList("-") }))},
		&mygo.MenuItem{Label: "任务列表", Click: click(a.editorAction(func(e *mdedit.Editor) { e.ToggleList("- [ ]") }))},
		mygo.Separator(),
		&mygo.MenuItem{Label: "代码块", Click: click(a.editorAction((*mdedit.Editor).InsertCodeBlock))},
		&mygo.MenuItem{Label: "公式块", Click: click(a.editorAction((*mdedit.Editor).InsertMathBlock))},
		&mygo.MenuItem{Label: "表格", Click: click(a.editorAction(func(e *mdedit.Editor) { e.InsertTable(3, 3) }))},
		&mygo.MenuItem{Label: "分隔线", Click: click(a.editorAction((*mdedit.Editor).InsertRule))},
	)
}

// formatMenu is the menu bar's 格式: the formats of text.
func (a *App) formatMenu(click func(func()) func(*mygo.MenuItem, *mygo.Window)) []*mygo.MenuItem {
	return []*mygo.MenuItem{
		{Label: "加粗", Click: click(a.editorAction((*mdedit.Editor).ToggleBold))},
		{Label: "斜体", Click: click(a.editorAction((*mdedit.Editor).ToggleItalic))},
		{Label: "下划线", Click: click(a.editorAction((*mdedit.Editor).ToggleUnderline))},
		{Label: "删除线", Click: click(a.editorAction((*mdedit.Editor).ToggleStrike))},
		{Label: "行内代码", Click: click(a.editorAction((*mdedit.Editor).ToggleCode))},
		{Label: "高亮", Click: click(a.editorAction((*mdedit.Editor).ToggleHighlight))},
		mygo.Separator(),
		{Label: "链接", Click: click(a.editorAction((*mdedit.Editor).InsertLink))},
	}
}
