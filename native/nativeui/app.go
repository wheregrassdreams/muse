// Package nativeui is Muse's fully native shell: MyGo draws the window, the
// icon rail, the context panel and the Markdown editor, with no webview and
// no frontend build.
//
// The layout is a 56px icon rail, one shared context panel (files / outline
// / search / AI) and the editor card, which shows the document as Typora
// does: one surface the user writes on, rendered as it reads (package
// mdedit).
package nativeui

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"

	"muse/mdedit"
	"muse/mdoc"
	"muse/services"
)

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
)

// panelView is the page the context panel shows.
type panelView int

const (
	panelFiles panelView = iota
	panelOutline
	panelAI
	panelSearch
)

// heading is one entry of the document outline.
type heading struct {
	level  int
	text   string
	offset int // where the heading's text starts in the source
}

// clipboardWrite is a seam for tests: the real clipboard waits on the app's
// main loop, which a headless test does not run.
var clipboardWrite = mygo.Clipboard.WriteText

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
	trashItem     = mygo.Shell.TrashItem
)

// App is the window's state: everything the view reads and the actions write.
type App struct {
	win *mygo.Window

	folderToggled map[string]time.Time

	// quick open (⌘O): fuzzy file jump inside the workspace
	quickOpen      bool
	quickOpenValue string
	quickOpenSel   int

	// command palette (⌘P): searchable menu actions
	cmdPalette bool
	cmdValue   string
	cmdSel     int

	// appearance
	themeMode int // 0 pure white, 1 near black
	// navHist 是打开过的文档，navAt 是当前停在的位置：编辑器的后退/前进
	// 在这里走，和浏览器一样，新的打开会截断后面的记录。
	navHist []string
	navAt   int
	// flat 是"Anytype 那种扁平"：编辑器不再是一张浮起的卡片，没有描边、
	// 没有圆角、没有投影，和窗口同色，页面直接铺满。关掉就是现在的卡片。
	flat bool
	dark bool // derived from themeMode for the view
	// themeApplied 是已经交给系统的主题；系统画的材料与标题栏跟随它，
	// 与页面所画的主题保持一致。-1 表示还没交过。
	themeApplied int

	// context panel
	panel      panelView
	panelOpen  bool
	panelWidth float32
	// panelW is the animated width the last frame drew, 0 when collapsed.
	// The canvas reads it instead of panelOpen so it moves with the slide
	// instead of jumping at the ends.
	panelW float32
	// panelPrevTarget 是上一帧的宽度目标，用来识别「开着切换模块」。
	panelPrevTarget float32
	// panelDrag 累计一次边缘拖动，拖动期间面板宽度逐帧跟手；
	// panelNarrow 记录这次拖动已经越过了收起的阈值。
	panelDrag   float32
	panelNarrow bool

	// workspace
	recents []recentEntry
	// dirMu 守住 dirs/isDir 两个树缓存：children 在渲染帧里跑，而
	// 打开文件夹的 goroutine 也写它们，headless 下这两者真的并发。
	dirMu sync.Mutex
	// mainMu 在 headless 下把 onMain 和渲染帧串起来：没有窗口事件循环
	// 时 onMain 就地执行，后台 Agent 会与测试的帧同时改编辑器和 AI
	// 状态；有窗口时 onMain 本来就在帧的线程上，锁空转。
	mainMu       sync.Mutex
	root         string
	dirs         map[string][]string
	isDir        map[string]bool
	expanded     map[string]bool
	fileList     ui.ListState
	selectedPath string

	// document: the editor owns the Markdown being written; source mirrors
	// it for what reads the document (saving, search, the AI's tools), and
	// every change goes through the editor, as a step of its undo.
	ed       *mdedit.Editor
	path     string
	source   string
	crlf     bool
	headings []heading
	dirty    bool

	// settings
	settingsOpen bool
	settingsTab  int
	settingsDrag ui.Point
	fontSize     float64
	lineHeight   float64

	// AI
	ai          aiSettings
	aiClient    *services.AI
	aiMessages  []chatMessage
	aiInput     string
	aiBusy      bool
	aiPhase     string // 流式进行中的一句话状态，如「深度思考中…」
	aiRequestID string
	aiErr       string
	aiScroll    ui.ScrollState
	aiStop      atomic.Bool
	// aiToolsUnsupported 由 Agent 循环置位：端点不支持 function
	// calling 时整个会话退回纯对话，只在循环 goroutine 上读写。
	aiToolsUnsupported bool
	// aiApprovals 是等待用户确认的工具调用（主线程读写）：调用 ID →
	// 决定通道，审批卡按钮经 decideApproval 递结果。
	aiApprovals map[string]chan approvalDecision
	// aiPerm 是修改类工具的权限预设（每次询问/完全访问/只读），
	// 会话内有效；审批卡上的「总是允许」会把它切到完全访问。
	aiPerm       aiPerm
	aiModels     []services.ProviderModel
	aiModelsBusy bool
	aiModelsErr  string
	// aiMdCache keeps the text of the last few Markdown parts parsed: a
	// frame that draws a part whose text has not changed does not parse it
	// again, as scrolling or a chunk arriving elsewhere redraws it.
	aiMdCache [8]aiMdEntry
	aiMdNext  int

	// search: the matches in the document, and the row chosen
	query       string
	matches     int
	searchHits  []searchHit
	searchState ui.ListState
	searchRow   int
	// searchKey is the query and source the hits were found for.
	searchKey [2]string
	// replaceWith is what the matches are replaced with.
	replaceWith string

	// outline rows
	outlineState ui.ListState
	outlineRow   int

	// status line
	status   string
	statusAt time.Time

	// blocking dialog guard: one native dialog at a time, never on the UI thread
	busy    bool
	closing bool

	// inline rename dialog
	renameOpen  bool
	renamePath  string
	renameValue string
}

// New returns the app on an empty untitled document, with the panel on files.
func New() *App {
	a := &App{
		panel:        panelFiles,
		panelOpen:    true,
		panelWidth:   panelDefaultWidth,
		panelW:       panelDefaultWidth,
		themeApplied: -1,
		dirs:         map[string][]string{},
		isDir:        map[string]bool{},
		expanded:     map[string]bool{},
		outlineRow:   -1,
		fontSize:     16,
		lineHeight:   1.75,
		recents:      loadRecents(),
		ai:           loadAISettings(),
		aiClient:     services.NewAI(),
		aiMessages:   loadAIChat(),
	}
	// 新窗口就是一张空白未命名文档：光标停在空段落里，直接就能写。
	a.ed = mdedit.New("", a.editorOptions())
	a.loadDoc("")
	return a
}

// ---------------------------------------------------------------------------
// View

func (a *App) View(c *ui.Context) {
	a.mainMu.Lock()
	defer a.mainMu.Unlock()
	a.applyTheme(c)
	row := ui.Row(c).Fill()
	if c.Vibrancy() {
		// 亚克力在视图没画背景的地方透出来：轨道与面板后面是材料，
		// 编辑卡片仍是不透明的表面。材料只在支持它的窗口上有，其余
		// 平台画不透明的底色。
		c.Root().Background(ui.Transparent)
	} else {
		row.Background(a.windowBG())
	}
	row.Children(func() {
		a.rail(c)
		a.contextPanel(c)
		a.canvas(c)
	})

	a.windowControls(c)
	a.settingsDialog(c)
	a.renameDialog(c)
	a.quickOpenDialog(c)
	a.commandPaletteDialog(c)
}

// windowBG is a single flat color, not a wash: the light side is a neutral
// canvas with a white editor card, the dark side a near-black canvas.
func (a *App) windowBG() ui.Color {
	if a.dark {
		return ui.Hex("#1a1a1a")
	}
	return ui.Hex("#f5f5f5")
}

func (a *App) applyTheme(c *ui.Context) {
	a.dark = a.themeMode == 1
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
		t.Accent = ui.Hex("#6b93ff")
		t.AccentHover = ui.Hex("#7fa3ff")
		t.AccentPressed = ui.Hex("#5580f0")
		t.AccentText = ui.Hex("#101014")
		t.Selection = ui.RGBA(107, 147, 255, 0.28)
		t.Focus = ui.Hex("#6b93ff")
	} else {
		t.Dark = false
		t.Background = ui.Hex("#ffffff")
		t.Surface = ui.Hex("#f3f3f6")
		t.SurfaceHover = ui.RGBA(0, 0, 0, 0.045)
		t.SurfacePressed = ui.RGBA(0, 0, 0, 0.08)
		t.Border = ui.RGBA(0, 0, 0, 0.07)
		t.Text = ui.Hex("#1d1d1f")
		t.TextMuted = ui.Hex("#6e6e78")
		t.Accent = ui.Hex("#3b6cf6")
		t.AccentHover = ui.Hex("#4d7cf7")
		t.AccentPressed = ui.Hex("#2f5ce0")
		t.AccentText = ui.Hex("#ffffff")
		t.Selection = ui.RGBA(59, 108, 246, 0.25)
		t.Focus = ui.Hex("#3b6cf6")
	}
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
	if a.themeMode == 1 {
		setAppAppearance(mygo.ThemeDark)
		return
	}
	setAppAppearance(mygo.ThemeLight)
}

// canvas is the floating editor card: 8px away from the window's top, right
// and bottom, and 8px from the context panel. The rail and the panel paint
// nothing, so the window color — or the acrylic material — shows behind them.
func (a *App) canvas(c *ui.Context) {
	t := c.Theme()
	ui.Row(c).Grow(1).FillHeight().MinWidth(0).Padding(canvasPadding, canvasPadding, canvasPadding, a.canvasLeft(c)).Children(func() {
		if a.flat {
			// Flat: the page is the window, one sheet, no card around it.
			card := ui.Column(c).Fill().Clip().Background(a.windowBG())
			card.Children(func() {
				a.editorContent(c)
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

// rail is the 56px icon-only column. Its background is the window color; the active
// view is a raised card, as the demo's chosen sidebar row is.
func (a *App) rail(c *ui.Context) {
	bar := c.TitleBar()
	ui.Column(c).Width(railWidth).FillHeight().Padding(0, railInset).GapY(railGapY).Children(func() {
		// The room above the first button: macOS's traffic lights when the
		// system reports them, else just enough to centre the first button
		// on the window's one top line.
		ui.Box(c).Height(max(bar.Height, topLineCenter-railButtonSize/2-railGapY)).DragWindow()
		// 最左侧只放模块切换；新建/打开都是文件模块的内容（面板顶部与菜单）。
		a.railButton(c, iconFiles, "文件", a.panelOpen && a.panel == panelFiles, func() {
			a.showPanel(panelFiles)
		})
		a.railButton(c, iconOutline, "大纲", a.panelOpen && a.panel == panelOutline, func() {
			a.showPanel(panelOutline)
		})
		a.railButton(c, iconSearch, "搜索", a.panelOpen && a.panel == panelSearch, func() {
			a.showPanel(panelSearch)
		})
		a.railButton(c, iconAI, "AI 助手", a.panelOpen && a.panel == panelAI, func() {
			a.showPanel(panelAI)
		})
		ui.Spacer(c)
		themeIcon := iconMoon
		themeLabel := "切换到深色"
		if a.dark {
			themeIcon = iconSun
			themeLabel = "切换到浅色"
		}
		a.railButton(c, themeIcon, themeLabel, false, func() { a.themeMode = 1 - a.themeMode })
		a.railButton(c, iconSettings, "设置", a.settingsOpen, a.openSettings)
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

// ---------------------------------------------------------------------------
// Context panel

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
	if a.panelOpen && a.panelPrevTarget > 0.5 && a.panelPrevTarget != target {
		anim.Duration = time.Millisecond
	}
	a.panelPrevTarget = target
	// The panel is always built so its width can slide instead of vanishing
	// from one frame to the next. The content keeps the full panel width and
	// the outer box clips it, so text does not reflow on the way; it only
	// fades as the box closes.
	panel := ui.Column(c).Shrink(0).FillHeight().Clip()
	w := panel.AnimateTo("width", target, anim)
	a.panelW = w
	panel.Width(w)
	if w < 0.5 {
		return
	}
	openness := w / a.panelWidth
	if openness > 1 {
		openness = 1
	}
	panel.Children(func() {
		ui.Column(c).Width(a.panelWidth).FillHeight().Opacity(openness).
			Padding(canvasPadding+cardBorder, 0, 0, 0).Children(func() {
			a.panelHeader(c)
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
			// The edge goes in last, so it hits above the content.
			if a.panelW > a.panelWidth-0.5 {
				a.panelResizeHandle(c)
			}
		})
	})
}

// panelResizeHandle is the panel's right edge, which a drag moves, a double
// click puts back at the module's width, and a drag released past the fold
// collapses.
func (a *App) panelResizeHandle(c *ui.Context) {
	h := ui.Box(c).Key("panel-resize").Absolute().Left(a.panelWidth - 12).
		Top(canvasPadding + cardBorder + editorHeaderH).
		Bottom(0).Width(12).Cursor(ui.CursorResizeEW).Label("调整面板宽度").
		Column().AlignItems(ui.Center)
	if dx, _, ok := h.Dragged(); ok {
		if a.panelDrag == 0 {
			a.panelDrag = a.panelWidth
		}
		a.panelDrag += dx
		a.panelNarrow = a.panelDrag < panelFoldOn
		a.panelWidth = min(max(a.panelDrag, panelMinWidth), panelMaxWidth)
	} else if a.panelDrag != 0 {
		a.panelDrag = 0
		if a.panelNarrow {
			a.panelNarrow = false
			a.collapsePanel()
			return
		}
	}
	if h.DoubleClicked() {
		a.panelWidth = a.panelTargetWidth()
		a.invalidate()
	}
	h.Children(func() {
		ui.Box(c).Grow(1)
		a.panelGrip(c, h)
		ui.Box(c).Grow(1)
	})
}

// panelGrip is the grip on the panel's edge: a short rounded bar on the
// hairline, there while the pointer is on the edge and, brighter and longer,
// while it is dragged. It is a child of the edge box, so it is part of what a
// drag grabs and never takes the pointer away from it.
func (a *App) panelGrip(c *ui.Context, edge ui.Element) {
	if !edge.Hovered() && a.panelDrag == 0 {
		return
	}
	t := c.Theme()
	drag := a.panelDrag != 0
	height := float32(40)
	if drag {
		height = 56
	}
	col, op := t.TextMuted, float32(0.5)
	switch {
	case drag && a.panelNarrow:
		col, op = t.Accent, 0.95
	case drag:
		col, op = t.Text, 0.85
	}
	ui.Box(c).Width(3).Height(height).Radius(1.5).Background(col.Alpha(op))
}

// panelHeader is the line over the context panel: the module names show on
// the rail, so the line holds only the way to fold the panel away, and the
// room to drag the window.
func (a *App) panelHeader(c *ui.Context) {
	bar := c.TitleBar()
	left := float32(12)
	if extra := bar.Left - railWidth + 8; extra > left {
		left = extra
	}
	ui.Row(c).Height(editorHeaderH).Shrink(0).Padding(0, 12, 0, left).Gap(4).DragWindow().
		AlignItems(ui.Center).Children(func() {
		ui.Spacer(c)
		a.smallIcon(c, iconPanelLeftClose, "收起侧栏", a.collapsePanel)
	})
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

// filesView draws the workspace tree. Directories open lazily; the list builds
// only the rows in view, so a large repository is still quick.
func (a *App) filesView(c *ui.Context) {
	t := c.Theme()
	if a.root == "" {
		ui.Column(c).Fill().Center().Gap(10).Padding(20).Children(func() {
			ui.Icon(c, iconFolderOpen).Size(30, 30).TextColor(iconTint(t))
			// 一个入口、一个按钮：文件与文件夹都会结束空态；下拉里还有
			// 最近打开过的记录，点一下直接回到那里。
			open := ui.Button(c, "打开文件或文件夹")
			open.Menu(func(m *ui.Menu) {
				if m.Item("打开文件…").Chosen() {
					a.openFileDialog()
				}
				if m.Item("打开文件夹…").Chosen() {
					a.openFolderDialog()
				}
				live, labels := a.liveRecents()
				if len(live) > 0 {
					m.Separator()
					for i, r := range live {
						entry := r
						if m.Item(labels[i]).Chosen() {
							a.openRecent(entry)
						}
					}
				}
			})
		})
		return
	}

	// 新建文件/新建文件夹在列表内容顶部成排展示（Obsidian 的资源管理器
	// 头部方式），落在选中的文件夹里；未选中任何目录时落在根目录。
	ui.Row(c).Shrink(0).Padding(6, 10, 4, 6).Gap(2).AlignItems(ui.Center).Children(func() {
		a.smallIcon(c, iconFilePlus, "新建文件", func() { a.newFileIn(a.createTarget()) })
		a.smallIcon(c, iconFolderPlus, "新建文件夹", func() { a.newFolderIn(a.createTarget()) })
		ui.Spacer(c)
		a.smallIcon(c, iconChevronDown, "全部折叠", a.collapseAll)
	})

	rows := a.fileRows()
	if len(rows) == 0 {
		ui.Column(c).Fill().Center().Gap(6).Padding(20).Children(func() {
			ui.Text(c, "没有可编辑的文本文件").FontSize(12).TextColor(t.TextMuted)
			ui.Text(c, "支持 md / txt / json 等文本类型").FontSize(10.5).TextColor(t.TextMuted)
		})
		return
	}
	ui.List(c, &a.fileList, len(rows), func(i int) {
		r := rows[i]
		row := ui.Row(c).Height(28).Radius(6).Gap(5).
			Padding(0, 8, 0, 4+float32(r.depth*14)).Cursor(ui.CursorPointer)
		switch {
		case r.path == a.selectedPath:
			if t.Dark {
				row.Background(ui.RGBA(255, 255, 255, 0.08))
			} else {
				row.Background(t.Background)
				row.Shadow(0, 0.5, 2, 0, ui.RGBA(0, 0, 0, 0.06))
			}
		case !r.dir && r.path == a.path:
			// 当前打开的文件在树里保持高亮（Obsidian 式）。
			row.Background(t.SurfaceHover)
		case row.Hovered():
			row.Background(t.SurfaceHover)
		}
		row.Children(func() {
			if r.dir {
				if a.expanded[r.path] {
					ui.Icon(c, iconChevronDown).Size(12, 12).TextColor(iconTint(t))
				} else {
					ui.Icon(c, iconChevron).Size(12, 12).TextColor(iconTint(t))
				}
				ui.Icon(c, iconFolder).Size(14, 14).TextColor(iconTint(t))
			} else {
				ui.Box(c).Width(12)
				ui.Icon(c, iconFile).Size(14, 14).TextColor(iconTint(t))
			}
			ui.Text(c, filepath.Base(r.path)).FontSize(12.5).TextColor(t.Text).
				SingleLine().Ellipsis("…")
		})
		row.OnClick(func() {
			if r.dir {
				// 双击的第二下不算：连点不再表现为"展开又立刻折叠"。
				if last, ok := a.folderToggled[r.path]; ok && time.Since(last) < 300*time.Millisecond {
					return
				}
				a.folderToggled[r.path] = time.Now()
				a.expanded[r.path] = !a.expanded[r.path]
			} else {
				a.selectedPath = r.path
				a.requestOpen(r.path)
			}
		})
		a.fileMenu(c, row, r.path)
	}).Grow(1).Padding(0, 8).Gap(1)
}

type fileRow struct {
	path  string
	depth int
	dir   bool
}

func (a *App) fileRows() []fileRow {
	var out []fileRow
	if a.root == "" {
		return nil
	}
	var walk func(dir string, depth int)
	walk = func(dir string, depth int) {
		for _, p := range a.children(dir) {
			a.dirMu.Lock()
			dir := a.isDir[p]
			a.dirMu.Unlock()
			out = append(out, fileRow{path: p, depth: depth, dir: dir})
			if dir && a.expanded[p] {
				walk(p, depth+1)
			}
		}
	}
	walk(a.root, 0)
	return out
}

func (a *App) outlineView(c *ui.Context) {
	t := c.Theme()
	if len(a.headings) == 0 {
		ui.Column(c).Fill().Center().Padding(20).Children(func() {
			ui.Text(c, "这篇文档还没有标题").FontSize(12).TextColor(t.TextMuted)
		})
		return
	}
	current := -1
	for i, h := range a.headings {
		if h.offset <= a.ed.Caret() {
			current = i
		}
	}
	if current >= 0 {
		a.outlineRow = current
	}
	ui.List(c, &a.outlineState, len(a.headings), func(i int) {
		h := a.headings[i]
		row := ui.Row(c).Height(26).Radius(6).Padding(0, 8, 0, 6+float32(h.level-1)*14).
			Gap(7).AlignItems(ui.Center).Cursor(ui.CursorPointer)
		switch {
		case i == a.outlineRow:
			row.Background(t.SurfaceHover)
		case row.Hovered():
			row.Background(t.SurfaceHover)
		}
		row.Children(func() {
			// 层级条：H1 实心强调点，深层弱化为细条，替代整行大色块
			if i == a.outlineRow {
				ui.Box(c).Width(2.5).Height(14).Radius(1.5).Background(t.Accent)
			} else {
				ui.Box(c).Width(2.5).Height(14).Radius(1.5).Opacity(0)
			}
			label := ui.Text(c, h.text).SingleLine().Ellipsis("…")
			switch {
			case h.level == 1:
				label.FontSize(12.5).FontWeight(600).TextColor(t.Text)
			case h.level == 2:
				label.FontSize(12).FontWeight(500).TextColor(t.Text)
			default:
				label.FontSize(11.5).TextColor(t.TextMuted)
			}
		})
		row.OnClick(func() {
			a.outlineRow = i
			a.ed.ScrollToOffset(h.offset)
			a.ed.Focus()
		})
	}).Grow(1).Padding(0, 8).Gap(0)
}

// snippetParts splits a search snippet at the first case-insensitive
// occurrence of the query: before, match, after. match is "" when the
// snippet no longer contains the query.
func snippetParts(snippet, query string) (string, string, string) {
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return snippet, "", ""
	}
	lower := strings.ToLower(snippet)
	idx := strings.Index(lower, q)
	if idx < 0 {
		return snippet, "", ""
	}
	return snippet[:idx], snippet[idx : idx+len(q)], snippet[idx+len(q):]
}

func (a *App) searchView(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).Fill().Padding(10).Gap(8).Children(func() {
		ui.TextInput(c, &a.query).Placeholder("在当前文档中查找").Label("查找")
		a.refreshSearch()
		if a.query == "" {
			ui.Text(c, "输入关键词，点击结果跳到对应位置").FontSize(11.5).TextColor(t.TextMuted).Wrap()
			return
		}
		if a.matches == 0 {
			ui.Text(c, "没有匹配").FontSize(11.5).TextColor(t.TextMuted)
			return
		}
		ui.Row(c).FillWidth().Gap(6).AlignItems(ui.Center).Children(func() {
			ui.TextInput(c, &a.replaceWith).Placeholder("替换为").Label("替换为").Grow(1).MinWidth(0)
			if ui.Button(c, "替换").Clicked() {
				a.replaceHit()
			}
			if ui.Button(c, "全部").Tooltip("全部替换").Clicked() {
				a.replaceAllHits()
			}
		})
		ui.Textf(c, "%d 处匹配", a.matches).FontSize(11).TextColor(t.TextMuted)
		ui.List(c, &a.searchState, len(a.searchHits), func(i int) {
			row := ui.Row(c).Height(28).Radius(6).Padding(0, 8).Cursor(ui.CursorPointer)
			switch {
			case i == a.searchRow:
				if t.Dark {
					row.Background(ui.RGBA(255, 255, 255, 0.08))
				} else {
					row.Background(t.Background)
					row.Shadow(0, 0.5, 2, 0, ui.RGBA(0, 0, 0, 0.06))
				}
			case row.Hovered():
				row.Background(t.SurfaceHover)
			}
			row.Children(func() {
				// 行内关键词强调色高亮（首个命中，大小写不敏感）。
				before, hit, after := snippetParts(a.searchHits[i].snippet, a.query)
				ui.Text(c, before).FontSize(11.5).TextColor(t.Text)
				if hit != "" {
					ui.Text(c, hit).FontSize(11.5).FontWeight(620).TextColor(t.Accent)
				}
				ui.Text(c, after).FontSize(11.5).TextColor(t.Text)
			})
			row.OnClick(func() { a.goToHit(i) })
		}).Grow(1).Gap(1)
	})
}

// searchHit is one match of the search in the document.
type searchHit struct {
	start, end int
	snippet    string
}

// refreshSearch finds the query in the document again when either changed,
// and shows the matches in the editor.
func (a *App) refreshSearch() {
	key := [2]string{a.query, a.source}
	if key == a.searchKey {
		return
	}
	a.searchKey = key
	a.searchHits = findHits(a.source, a.query)
	a.matches = len(a.searchHits)
	if a.searchRow >= a.matches {
		a.searchRow = -1
	}
	ranges := make([]mdedit.Range, len(a.searchHits))
	for i, h := range a.searchHits {
		ranges[i] = mdedit.Range{Start: h.start, End: h.end}
	}
	a.ed.SetHighlights(ranges, a.searchRow)
}

// goToHit selects match i in the editor.
func (a *App) goToHit(i int) {
	if i < 0 || i >= len(a.searchHits) {
		return
	}
	a.searchRow = i
	h := a.searchHits[i]
	ranges := make([]mdedit.Range, len(a.searchHits))
	for k, x := range a.searchHits {
		ranges[k] = mdedit.Range{Start: x.start, End: x.end}
	}
	a.ed.SetHighlights(ranges, i)
	a.ed.Select(h.start, h.end)
	a.ed.Focus()
}

// replaceHit replaces the chosen match, or the first, and goes to the next.
func (a *App) replaceHit() {
	i := max(a.searchRow, 0)
	if i >= len(a.searchHits) {
		return
	}
	h := a.searchHits[i]
	a.ed.Replace(h.start, h.end, a.replaceWith)
	a.docChanged()
	a.refreshSearch()
	if a.matches > 0 {
		a.goToHit(min(i, a.matches-1))
	}
}

// replaceAllHits replaces every match, as one step of undo.
func (a *App) replaceAllHits() {
	if len(a.searchHits) == 0 {
		return
	}
	var b strings.Builder
	at := 0
	for _, h := range a.searchHits {
		b.WriteString(a.source[at:h.start])
		b.WriteString(a.replaceWith)
		at = h.end
	}
	b.WriteString(a.source[at:])
	n := len(a.searchHits)
	a.editDoc(b.String())
	a.refreshSearch()
	a.notice(fmt.Sprintf("已替换 %d 处", n))
}

// findHits returns the matches of query in src, ignoring case, at most 500,
// each with the line it is on, cut for a row.
func findHits(src, query string) []searchHit {
	q := strings.TrimSpace(query)
	if q == "" {
		return nil
	}
	hay, needle := src, q
	if lower := strings.ToLower(src); len(lower) == len(src) {
		hay, needle = lower, strings.ToLower(q)
	}
	var out []searchHit
	for from := 0; len(out) < 500; {
		i := strings.Index(hay[from:], needle)
		if i < 0 {
			break
		}
		s := from + i
		e := s + len(needle)
		out = append(out, searchHit{start: s, end: e, snippet: hitSnippet(src, s, e)})
		from = e
	}
	return out
}

// hitSnippet is the line around a match, trimmed and cut to about 60
// characters around it: enough to recognise the passage.
func hitSnippet(src string, s, e int) string {
	ls := strings.LastIndexByte(src[:s], '\n') + 1
	le := len(src)
	if k := strings.IndexByte(src[e:], '\n'); k >= 0 {
		le = e + k
	}
	line := []rune(strings.TrimSpace(src[ls:le]))
	if len(line) <= 60 {
		return string(line)
	}
	at := len([]rune(strings.TrimLeft(src[ls:s], " \t")))
	from := max(0, min(at-20, len(line)-60))
	out := string(line[from:min(from+60, len(line))])
	if from > 0 {
		out = "…" + out
	}
	if from+60 < len(line) {
		out += "…"
	}
	return out
}

// ---------------------------------------------------------------------------
// Editor

func (a *App) editorContent(c *ui.Context) {
	ui.Column(c).Fill().Children(func() {
		a.editorHeader(c)
		a.ed.SetOptions(a.editorOptions())
		a.ed.View(c).Grow(1)
	})
}

func (a *App) editorHeader(c *ui.Context) {
	t := c.Theme()
	// Windows and Linux put the view's own window buttons at the top right
	// (frame.go); the header keeps them clear. macOS keeps the system's
	// traffic lights at the left, and c.TitleBar says how much they take.
	bar := c.TitleBar()
	padRight := 18 + bar.Right
	if controlsAtRight {
		padRight += windowButtonsRoom
	}
	ui.Row(c).Height(editorHeaderH).Shrink(0).Padding(0, padRight, 0, 6).Gap(2).
		DragWindow().AlignItems(ui.Center).Children(func() {
		// The panel's reopen slot: with the sidebar folded away this is what
		// is left at the window's top left, the way the reference keeps its
		// toggle there.
		if !a.panelOpen {
			a.smallIcon(c, iconPanelLeftOpen, "打开侧栏", func() { a.showPanel(a.panel) })
		} else {
			ui.Box(c).Width(2).Shrink(0)
		}
		tint := iconTint(t)
		if a.canNav(-1) {
			a.smallIcon(c, iconChevronLeft, "后退", func() { a.nav(-1) })
		} else {
			a.smallIconTint(c, iconChevronLeft, "后退", nil, tint.Alpha(0.35))
		}
		if a.canNav(1) {
			a.smallIcon(c, iconChevron, "前进", func() { a.nav(1) })
		} else {
			a.smallIconTint(c, iconChevron, "前进", nil, tint.Alpha(0.35))
		}
		// The path sits in the middle of what is left, its end kept: the file
		// name is the part that says which document this is.
		ui.Row(c).Grow(1).MinWidth(0).Justify(ui.Center).Padding(8, 0).Children(func() {
			vw, _ := c.Size()
			avail := vw - railWidth - a.panelW - 220
			ui.Text(c, a.tailPath(c, a.docPath(), 12.5, avail)).
				FontSize(12.5).TextColor(t.TextMuted).SingleLine()
		})
		if a.status != "" {
			ui.Text(c, a.status).FontSize(11).TextColor(t.TextMuted).SingleLine().Ellipsis("…")
		}
		if a.dirty {
			a.smallIcon(c, iconSave, "保存", func() { a.save() })
		}
		info := a.smallIconTint(c, iconInfo, "信息", nil, tint)
		info.Menu(func(m *ui.Menu) {
			chars, _, lines := countText(a.source)
			m.Item("路径：" + a.docPath())
			m.Item(fmt.Sprintf("%d 字 · %d 行", chars, lines))
			if st, err := os.Stat(a.path); err == nil {
				m.Item("修改于 " + st.ModTime().Format("2006-01-02 15:04"))
			}
			m.Separator()
			if m.Item("在访达中显示").Chosen() && a.path != "" {
				mygo.Shell.ShowItemInFolder(a.path)
			}
		})
		more := a.smallIconTint(c, iconMore, "更多", nil, tint)
		more.Menu(func(m *ui.Menu) {
			if m.Item("保存").Chosen() {
				a.save()
			}
			if m.Item("复制路径").Chosen() && a.path != "" {
				clipboardWrite(a.path)
				a.notice("已复制路径")
			}
			if m.Item("设为工作区").Chosen() && a.path != "" {
				a.setWorkspace(filepath.Dir(a.path))
			}
		})
	})
}

// ---------------------------------------------------------------------------
// Files

func (a *App) setWorkspace(root string) {
	a.root = root
	a.dirMu.Lock()
	a.dirs = map[string][]string{}
	a.isDir = map[string]bool{}
	a.dirMu.Unlock()
	a.expanded = map[string]bool{}
	a.fileList = ui.ListState{}
	a.selectedPath = ""
	a.children(root)
	a.addRecent(root, true)
	a.panel = panelFiles
	a.panelOpen = true
}

func (a *App) children(dir string) []string {
	a.dirMu.Lock()
	defer a.dirMu.Unlock()
	if entries, ok := a.dirs[dir]; ok {
		return entries
	}
	des, err := os.ReadDir(dir)
	if err != nil {
		a.dirs[dir] = nil
		return nil
	}
	names := make([]string, 0, len(des))
	for _, de := range des {
		name := de.Name()
		if strings.HasPrefix(name, ".") || name == "node_modules" {
			continue
		}
		path := filepath.Join(dir, name)
		if de.IsDir() {
			a.isDir[path] = true
			names = append(names, path)
			continue
		}
		if isEditableFile(path) {
			names = append(names, path)
		}
	}
	sort.SliceStable(names, func(i, j int) bool {
		di, dj := a.isDir[names[i]], a.isDir[names[j]]
		if di != dj {
			return di
		}
		return strings.ToLower(filepath.Base(names[i])) < strings.ToLower(filepath.Base(names[j]))
	})
	a.dirs[dir] = names
	return names
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

func (a *App) snapshotDoc() (dirty bool, name, path, source string) {
	a.onMain(func() {
		dirty, name, path, source = a.dirty, a.displayName(), a.path, a.diskSource()
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
			Title:     "打开文件夹",
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
	a.startBusy()
	go func() {
		paths, err := openDialog(mygo.OpenDialogOptions{
			Parent: a.win,
			Title:  "打开文本文件",
			Filters: []mygo.FileFilter{
				{Name: "文本文件", Extensions: textFileFilter()},
			},
		})
		if err != nil || len(paths) == 0 {
			a.finish(nil)
			return
		}
		if !a.confirmDiscardOffUI() {
			a.finish(nil)
			return
		}
		data, err := os.ReadFile(paths[0])
		if err != nil {
			errorDialog("打开失败", err.Error())
			a.finish(nil)
			return
		}
		if !isTextData(data) {
			errorDialog("无法打开", "这个文件不是文本文件。")
			a.finish(nil)
			return
		}
		a.finish(func() { a.applyOpen(paths[0], string(data)) })
	}()
}

func (a *App) openPathNow(path string) {
	if !isEditableFile(path) {
		a.notice("不支持的文件类型：" + filepath.Base(path))
		a.invalidate()
		return
	}
	data, err := os.ReadFile(path)
	if err != nil {
		a.notice(fmt.Sprintf("打开失败：%v", err))
		a.invalidate()
		return
	}
	if !isTextData(data) {
		a.notice("这个文件不是文本文件：" + filepath.Base(path))
		a.invalidate()
		return
	}
	a.applyOpen(path, string(data))
	a.invalidate()
}

// requestOpen opens a path from the UI. A clean document opens at once; an
// edited one asks first, with the dialog off the UI thread.
func (a *App) requestOpen(path string) {
	if !isEditableFile(path) {
		a.notice("不支持的文件类型：" + filepath.Base(path))
		a.invalidate()
		return
	}
	if !a.dirty {
		a.openPathNow(path)
		return
	}
	if a.win == nil || a.busy {
		return
	}
	a.startBusy()
	go func() {
		if !a.confirmDiscardOffUI() {
			a.finish(nil)
			return
		}
		data, err := os.ReadFile(path)
		if err != nil {
			errorDialog("打开失败", err.Error())
			a.finish(nil)
			return
		}
		a.finish(func() { a.applyOpen(path, string(data)) })
	}()
}

func (a *App) applyOpen(path, data string) {
	a.pushNav(path)
	a.path = path
	a.selectedPath = path
	a.loadDoc(data)
	a.dirty = false
	a.updateTitle()
	a.addRecent(path, false)
	a.ed.Focus()
}

func (a *App) newDoc() {
	if !a.dirty {
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
	a.path = ""
	a.selectedPath = ""
	a.loadDoc("")
	a.dirty = false
	a.updateTitle()
	a.ed.Focus()
}

func (a *App) save() {
	if a.path == "" {
		a.saveAs()
		return
	}
	if !isEditableFile(a.path) {
		a.notice("不支持的文件类型：" + filepath.Base(a.path))
		a.invalidate()
		return
	}
	if err := writeFileAtomic(a.path, []byte(a.diskSource())); err != nil {
		a.notice(fmt.Sprintf("保存失败：%v", err))
	} else {
		a.dirty = false
		a.notice("已保存")
	}
	a.updateTitle()
	a.invalidate()
}

// confirmDiscardOffUI asks about an edited document without blocking the UI
// thread. It runs in a goroutine; every state change hops back to the main
// thread. It returns true when it is safe to replace the document.
func (a *App) confirmDiscardOffUI() bool {
	dirty, name, path, source := a.snapshotDoc()
	if !dirty {
		return true
	}
	res, err := messageDialog(mygo.MessageOptions{
		Parent:        a.win,
		Type:          mygo.MessageWarning,
		Message:       "要保存对「" + name + "」的更改吗？",
		Detail:        "如果不保存，更改会丢失。",
		Buttons:       []string{"保存", "不保存", "取消"},
		DefaultButton: 0,
		CancelButton:  2,
	})
	if err != nil || res.Button == 2 {
		return false
	}
	if res.Button == 1 {
		a.onMain(func() { a.dirty = false })
		return true
	}
	savePath := path
	if savePath == "" {
		defaultName := name
		if defaultName == "" || defaultName == "未命名" {
			defaultName = "Untitled.md"
		} else if !strings.HasSuffix(strings.ToLower(defaultName), ".md") {
			defaultName += ".md"
		}
		savePath, err = saveDialog(mygo.SaveDialogOptions{
			Parent:      a.win,
			Title:       "保存文件",
			DefaultPath: defaultName,
			Filters:     []mygo.FileFilter{{Name: "文本文件", Extensions: textFileFilter()}},
		})
		if err != nil || savePath == "" {
			return false
		}
	}
	if !isEditableFile(savePath) {
		errorDialog("无法保存", "请选择文本文件扩展名。")
		return false
	}
	if err := writeFileAtomic(savePath, []byte(source)); err != nil {
		errorDialog("保存失败", err.Error())
		return false
	}
	a.onMain(func() {
		a.path = savePath
		a.selectedPath = savePath
		a.dirty = false
		a.updateTitle()
	})
	return true
}

func (a *App) saveAs() {
	if a.busy {
		return
	}
	var name, source string
	a.onMain(func() {
		name = "Untitled.md"
		if a.path != "" {
			name = filepath.Base(a.path)
		}
		source = a.diskSource()
	})
	a.startBusy()
	go func() {
		path, err := saveDialog(mygo.SaveDialogOptions{
			Parent:      a.win,
			Title:       "保存文件",
			DefaultPath: name,
			Filters:     []mygo.FileFilter{{Name: "文本文件", Extensions: textFileFilter()}},
		})
		if err != nil || path == "" {
			a.finish(nil)
			return
		}
		if !isEditableFile(path) {
			errorDialog("无法保存", "请选择文本文件扩展名。")
			a.finish(nil)
			return
		}
		if err := writeFileAtomic(path, []byte(source)); err != nil {
			errorDialog("保存失败", err.Error())
			a.finish(nil)
			return
		}
		a.finish(func() {
			a.path = path
			a.selectedPath = path
			a.dirty = false
			a.updateTitle()
			a.clearTreeCache()
		})
	}()
}

// collapseAll folds every expanded directory in the tree.
func (a *App) collapseAll() {
	for p := range a.expanded {
		a.expanded[p] = false
	}
	a.folderToggled = map[string]time.Time{}
	a.invalidate()
}

// createTarget is where panel "new" actions land: the selected folder when
// one is selected, otherwise the workspace root.
func (a *App) createTarget() string {
	a.dirMu.Lock()
	isDir := a.isDir[a.selectedPath]
	a.dirMu.Unlock()
	if a.selectedPath != "" && isDir {
		return a.selectedPath
	}
	return a.root
}

func (a *App) newFileIn(dir string) {
	path := uniquePath(dir, "未命名", ".md")
	if err := os.WriteFile(path, []byte(""), 0o644); err != nil {
		a.notice(fmt.Sprintf("新建失败：%v", err))
		a.invalidate()
		return
	}
	a.clearTreeCache()
	a.requestOpen(path)
}

func (a *App) newFolderIn(dir string) {
	path := uniquePath(dir, "新建文件夹", "")
	if err := os.Mkdir(path, 0o755); err != nil {
		a.notice(fmt.Sprintf("新建失败：%v", err))
	} else {
		a.clearTreeCache()
	}
	a.invalidate()
}

func (a *App) clearTreeCache() {
	a.dirMu.Lock()
	a.dirs = map[string][]string{}
	a.isDir = map[string]bool{}
	a.dirMu.Unlock()
}

// fileMenu is the file tree's context menu: directory actions, path copying
// (relative or absolute), rename, trash and reveal.
func (a *App) fileMenu(c *ui.Context, row ui.Element, path string) {
	row.ContextMenu(func(m *ui.Menu) {
		a.dirMu.Lock()
		isDir := a.isDir[path]
		a.dirMu.Unlock()
		if isDir {
			if m.Item("新建文件").Chosen() {
				a.newFileIn(path)
			}
			if m.Item("新建文件夹").Chosen() {
				a.newFolderIn(path)
			}
			m.Separator()
		}
		if m.Item("复制相对路径").Chosen() {
			a.copyPath(path, true)
		}
		if m.Item("复制绝对路径").Chosen() {
			a.copyPath(path, false)
		}
		m.Separator()
		if m.Item("重命名…").Chosen() {
			a.renamePath = path
			a.renameValue = filepath.Base(path)
			a.renameOpen = true
		}
		if m.Item("移到废纸篓").Chosen() {
			a.trash(path)
		}
		m.Separator()
		if m.Item("在 Finder 中显示").Chosen() {
			mygo.Shell.ShowItemInFolder(path)
		}
	})
}

func (a *App) copyPath(path string, relative bool) {
	text := path
	if relative {
		if rel, err := filepath.Rel(a.root, path); err == nil && !strings.HasPrefix(rel, "..") {
			text = rel
		}
	}
	clipboardWrite(text)
	if relative {
		a.notice("已复制相对路径")
	} else {
		a.notice("已复制绝对路径")
	}
	a.invalidate()
}

func (a *App) trash(path string) {
	if a.busy {
		return
	}
	a.startBusy()
	go func() {
		name := filepath.Base(path)
		res, err := messageDialog(mygo.MessageOptions{
			Parent:        a.win,
			Type:          mygo.MessageWarning,
			Message:       "确定把「" + name + "」移到废纸篓？",
			Detail:        "可以从废纸篓恢复。",
			Buttons:       []string{"移到废纸篓", "取消"},
			DefaultButton: 0,
			CancelButton:  1,
		})
		if err != nil || res.Button != 0 {
			a.finish(nil)
			return
		}
		if err := trashItem(path); err != nil {
			errorDialog("删除失败", err.Error())
			a.finish(nil)
			return
		}
		a.finish(func() {
			if a.path == path {
				a.path = ""
				a.selectedPath = ""
				a.loadDoc("")
				a.dirty = false
				a.updateTitle()
			}
			a.clearTreeCache()
			a.notice("已移到废纸篓")
		})
	}()
}

func (a *App) renameDialog(c *ui.Context) {
	t := c.Theme()
	ui.Modal(c, &a.renameOpen, func() {
		ui.Column(c).Width(420).Gap(12).Padding(18).Radius(12).
			Background(t.Background).Border(1, t.Border).
			Shadow(0, 18, 50, 0, ui.RGBA(0, 0, 0, 0.22)).Children(func() {
			ui.Text(c, "重命名").FontSize(14).FontWeight(600)
			input := ui.TextInput(c, &a.renameValue).AutoFocus().Label("新名称")
			ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
				if ui.Button(c, "取消").Clicked() {
					a.renameOpen = false
				}
				if ui.PrimaryButton(c, "重命名").Clicked() || input.Submitted() {
					a.applyRename()
				}
			})
		})
	})
}

func (a *App) applyRename() {
	name := strings.TrimSpace(a.renameValue)
	if name == "" || a.renamePath == "" {
		a.renameOpen = false
		return
	}
	old := a.renamePath
	newPath := filepath.Join(filepath.Dir(old), name)
	if err := os.Rename(old, newPath); err != nil {
		a.notice(fmt.Sprintf("重命名失败：%v", err))
		a.renameOpen = false
		a.invalidate()
		return
	}
	if a.path == old {
		a.path = newPath
	}
	a.renameOpen = false
	a.renamePath = ""
	a.clearTreeCache()
	a.notice("已重命名")
	a.updateTitle()
	a.invalidate()
}

func uniquePath(dir, base, ext string) string {
	path := filepath.Join(dir, base+ext)
	for i := 2; ; i++ {
		if _, err := os.Stat(path); os.IsNotExist(err) {
			return path
		}
		path = filepath.Join(dir, fmt.Sprintf("%s %d%s", base, i, ext))
	}
}

// ---------------------------------------------------------------------------
// Settings

func (a *App) settingsDialog(c *ui.Context) {
	t := c.Theme()
	w, h := c.Size()
	panelW := min(820, w-64)
	panelH := min(620, h-64)
	maxX := (w-panelW)/2 - 12
	maxY := (h-panelH)/2 - 12
	bounds := ui.Rect{X: -maxX, Y: -maxY, W: 2 * maxX, H: 2 * maxY}
	ui.DialogBase(c, &a.settingsOpen, func(backdrop, panel ui.Element) {
		// term-fleet's settings is a floating window, not a sheet: the page
		// behind stays visible while it is open.
		backdrop.Background(ui.Transparent)
		panel.Width(panelW).Height(panelH).Radius(14).Clip().
			Background(t.Surface).Border(1, t.Border).
			Shadow(0, 18, 50, 0, ui.RGBA(0, 0, 0, 0.28)).
			Draggable(&a.settingsDrag, ui.DraggableOptions{Bounds: &bounds, Rigid: true}).
			Children(func() {
				ui.Row(c).Height(44).Shrink(0).Padding(0, 10, 0, 16).Gap(10).Children(func() {
					ui.Text(c, "设置").FontSize(14).FontWeight(600)
					ui.Text(c, "更改会立即生效").FontSize(11.5).TextColor(t.TextMuted)
					ui.Spacer(c)
					a.smallIcon(c, iconClose, "关闭设置", func() { a.settingsOpen = false })
				})
				ui.Row(c).Grow(1).MinHeight(0).AlignItems(ui.Stretch).Children(func() {
					// Left navigation, 184px.
					ui.Column(c).Width(184).Shrink(0).FillHeight().Padding(0, 8, 8, 4).Gap(2).Children(func() {
						ui.Text(c, "外观").FontSize(11).FontWeight(600).TextColor(t.TextMuted).Padding(10, 10, 4, 10)
						a.settingsNavItem(c, "主题", 0)
						a.settingsNavItem(c, "排版", 1)
						a.settingsNavItem(c, "模型", 2)
					})
					// Right content card, with its own 44px page header.
					ui.Column(c).Grow(1).MinWidth(0).FillHeight().Padding(0, 8, 8, 8).Children(func() {
						card := ui.Column(c).FillWidth().FillHeight().Radius(14).Clip().
							Background(t.Background).Border(1, t.Border)
						card.Children(func() {
							title := "主题"
							switch a.settingsTab {
							case 1:
								title = "排版"
							case 2:
								title = "模型"
							}
							ui.Row(c).Height(44).Shrink(0).Padding(0, 18).AlignItems(ui.Center).
								BorderWidth(0, 0, 1, 0).BorderColor(t.Border).Children(func() {
								ui.Text(c, title).FontSize(15).FontWeight(650)
							})
							ui.Scroll(c).Fill().Children(func() {
								ui.Column(c).FillWidth().Padding(18).Gap(18).Children(func() {
									switch a.settingsTab {
									case 0:
										a.appearanceSettings(c)
									case 1:
										a.typographySettings(c)
									default:
										a.modelSettings(c)
									}
								})
							})
						})
					})
				})
				footer := "设置会立即生效"
				if a.settingsTab == 2 {
					footer = "模型设置保存后生效"
				}
				ui.Row(c).Height(34).Shrink(0).Padding(0, 16).Gap(7).AlignItems(ui.Center).
					BorderWidth(1, 0, 0, 0).BorderColor(t.Border).Children(func() {
					ui.Box(c).Size(6, 6).Radius(3).Background(t.TextMuted)
					ui.Text(c, footer).FontSize(11.5).TextColor(t.TextMuted)
				})
			})
	})
}

func (a *App) settingsNavItem(c *ui.Context, label string, tab int) {
	t := c.Theme()
	b := ui.ButtonBase(c).Width(168).Height(32).Padding(0, 10).Radius(7).Label(label)
	switch {
	case a.settingsTab == tab:
		if t.Dark {
			b.Background(ui.RGBA(255, 255, 255, 0.08))
		} else {
			b.Background(t.Background).Border(1, t.Border)
		}
	case b.Hovered():
		b.Background(t.SurfaceHover)
	}
	b.OnClick(func() { a.settingsTab = tab })
	b.Children(func() {
		ui.Text(c, label).FontSize(13).TextColor(t.Text)
	})
}

// settingsGroup is the inset panel the settings pages group their rows in.
func settingsGroup(c *ui.Context, title string, fn func()) {
	t := c.Theme()
	ui.Column(c).FillWidth().Gap(8).Children(func() {
		if title != "" {
			ui.Text(c, title).FontSize(12).FontWeight(600).TextColor(t.TextMuted).Padding(0, 4)
		}
		ui.Column(c).FillWidth().Radius(12).Background(t.Surface).Padding(4, 16).Children(fn)
	})
}

func settingsRow(c *ui.Context, label, detail string, control func()) {
	t := c.Theme()
	ui.Row(c).FillWidth().Padding(11, 0).Gap(16).AlignItems(ui.Center).Children(func() {
		ui.Column(c).Grow(1).MinWidth(0).Gap(3).Children(func() {
			ui.Text(c, label).FontSize(13.5).TextColor(t.Text)
			if detail != "" {
				ui.Text(c, detail).FontSize(12).TextColor(t.TextMuted).Wrap()
			}
		})
		ui.Row(c).Shrink(0).Gap(8).AlignItems(ui.Center).Children(control)
	})
}

func (a *App) appearanceSettings(c *ui.Context) {
	t := c.Theme()
	settingsGroup(c, "", func() {
		a.settingsDivider(c, false)
		settingsRow(c, "主题", "Rex 色洗的浅色或深色；编辑器、上下文栏与弹层同步切换。", func() {
			ui.Segmented(c, &a.themeMode, "浅色", "深色").Label("主题")
		})
	})
	ui.Text(c, "native 渲染与编辑器共用同一套颜色；切换后立即生效。").
		FontSize(11.5).TextColor(t.TextMuted).Padding(0, 4)
}

func (a *App) typographySettings(c *ui.Context) {
	settingsGroup(c, "", func() {
		a.settingsDivider(c, false)
		settingsRow(c, "字号", "编辑器正文的基准字号。", func() {
			ui.Textf(c, "%.0fpx", a.fontSize).FontSize(12).TextColor(c.Theme().TextMuted).Width(38)
			ui.Slider(c, &a.fontSize, 12, 22).Width(190).Label("字号")
		})
		a.settingsDivider(c, true)
		settingsRow(c, "行高", "正文的行距倍数。", func() {
			ui.Textf(c, "%.1f", a.lineHeight).FontSize(12).TextColor(c.Theme().TextMuted).Width(38)
			ui.Slider(c, &a.lineHeight, 1.4, 2.2).Width(190).Label("行高")
		})
	})
}

// settingsDivider draws the hairline between rows, never above the first or
// below the last, as the platform's settings windows do.
func (a *App) settingsDivider(c *ui.Context, between bool) {
	if between {
		ui.Box(c).FillWidth().Height(1).Background(c.Theme().Border)
	}
}

// ---------------------------------------------------------------------------
// The document

// docDir is the folder images and links of the document resolve in.
func (a *App) docDir() string {
	if a.path != "" {
		return filepath.Dir(a.path)
	}
	return a.root
}

// editorOptions are the editor's options, from the settings and the
// document being edited.
func (a *App) editorOptions() mdedit.Options {
	return mdedit.Options{
		FontSize:    float32(a.fontSize),
		LineHeight:  float32(a.lineHeight),
		BaseDir:     a.docDir(),
		Placeholder: "开始写作…",
		Label:       "文档",
		Code:        codeLanguage(a.path),
		Invalidate:  a.invalidate,
		OnChange:    a.docChanged,
		OnLink:      a.openLink,
		SaveImage:   a.saveImage,
		OnDropFiles: a.openDropped,
	}
}

// openDropped opens the first document among files dropped on the editor.
func (a *App) openDropped(paths []string) {
	for _, p := range paths {
		if st, err := os.Stat(p); err == nil && st.IsDir() {
			a.setWorkspace(p)
			a.invalidate()
			return
		}
		if isEditableFile(p) {
			a.requestOpen(p)
			return
		}
	}
}

// codeLanguage returns the language a file is code in, for the editor to
// show it as code, or "" for Markdown: .md files, and README-like files
// without an extension.
func codeLanguage(path string) string {
	if path == "" {
		return ""
	}
	base := filepath.Base(path)
	ext := strings.ToLower(filepath.Ext(base))
	switch ext {
	case ".md", ".markdown", ".mdx":
		return ""
	}
	if lang := editor.LanguageFor(base); lang != nil {
		if strings.EqualFold(lang.Name, "markdown") {
			return ""
		}
		return lang.Name
	}
	if ext == "" {
		return ""
	}
	return "text"
}

// loadDoc makes src the document, as opening a file does: the history of
// undo starts over. Windows line endings are kept as \n while editing and
// written back as they were.
func (a *App) loadDoc(src string) {
	a.crlf = strings.Contains(src, "\r\n")
	if a.crlf {
		src = strings.ReplaceAll(src, "\r\n", "\n")
	}
	a.ed.SetOptions(a.editorOptions())
	a.ed.SetSource(src)
	a.source = src
	a.searchKey = [2]string{}
	a.refreshHeadings()
}

// docChanged takes a change of the document from the editor: the user's
// typing, or an edit the app made through it.
func (a *App) docChanged() {
	src := a.ed.Source()
	if src == a.source {
		return
	}
	a.source = src
	a.dirty = true
	a.refreshHeadings()
	a.updateTitle()
}

// refreshHeadings reads the outline from the editor's parse.
func (a *App) refreshHeadings() {
	a.headings = a.headings[:0]
	for _, h := range a.ed.Headings() {
		a.headings = append(a.headings, heading{level: h.Level, text: h.Text, offset: h.Offset})
	}
	if a.outlineRow >= len(a.headings) {
		a.outlineRow = -1
	}
}

// editDoc replaces the document with src as an edit the user can undo, as
// the AI's tools change it.
func (a *App) editDoc(src string) {
	a.ed.ReplaceAll(src)
	a.docChanged()
	a.invalidate()
}

// diskSource is the document as it is written to disk.
func (a *App) diskSource() string {
	if a.crlf {
		return strings.ReplaceAll(a.source, "\n", "\r\n")
	}
	return a.source
}

// openLink opens a link Cmd-clicked in the document: a heading of the
// document by its anchor, a document of the workspace by its path, or a
// page in the browser.
func (a *App) openLink(href string) {
	href = strings.TrimSpace(href)
	switch {
	case href == "":
		return
	case strings.HasPrefix(href, "wiki:"):
		a.openWiki(strings.TrimPrefix(href, "wiki:"))
		return
	case strings.HasPrefix(href, "#"):
		slug := strings.TrimPrefix(href, "#")
		for _, h := range a.headings {
			if headingSlug(h.text) == strings.ToLower(slug) {
				a.ed.ScrollToOffset(h.offset)
				return
			}
		}
		return
	case strings.Contains(href, "://") || strings.HasPrefix(href, "mailto:"):
		go openExternal(href)
		return
	}
	p := href
	if i := strings.IndexAny(p, "#?"); i >= 0 {
		p = p[:i]
	}
	if !filepath.IsAbs(p) {
		p = filepath.Join(a.docDir(), filepath.FromSlash(p))
	}
	if st, err := os.Stat(p); err == nil && !st.IsDir() && isEditableFile(p) {
		a.requestOpen(p)
		return
	}
	go openExternal(p)
}

// openWiki opens a `[[wiki link]]` by its target (see resolveWiki).
func (a *App) openWiki(target string) {
	if p := resolveWiki(a.root, a.docDir(), a.fileRows(), target); p != "" {
		a.requestOpen(p)
	}
}

// resolveWiki returns the file a `[[wiki link]]` opens, or "" when none
// does: a path under the workspace root, else a path beside the open
// document, else a note of the workspace by its name — the shortest
// matching path, the way Obsidian resolves a bare title. A `#heading` or
// `^block` anchor is not resolved yet.
func resolveWiki(root, docDir string, files []fileRow, target string) string {
	target = strings.TrimSpace(target)
	if i := strings.IndexAny(target, "#^"); i >= 0 {
		target = strings.TrimSpace(target[:i])
	}
	if target == "" {
		return ""
	}
	if !strings.HasSuffix(strings.ToLower(target), ".md") {
		target += ".md"
	}
	rel := filepath.FromSlash(target)
	for _, base := range []string{root, docDir} {
		if base == "" {
			continue
		}
		p := filepath.Join(base, rel)
		if st, err := os.Stat(p); err == nil && !st.IsDir() && isEditableFile(p) {
			return p
		}
	}
	want := strings.ToLower(filepath.Base(rel))
	best := ""
	for _, r := range files {
		if r.dir || strings.ToLower(filepath.Base(r.path)) != want {
			continue
		}
		if best == "" || len(r.path) < len(best) {
			best = r.path
		}
	}
	return best
}

// headingSlug is the anchor of a heading, as GitHub makes it.
func headingSlug(text string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(strings.TrimSpace(text)) {
		switch {
		case r == ' ' || r == '-':
			b.WriteRune('-')
		case r == '_' || r >= '0' && r <= '9' || r >= 'a' && r <= 'z' || r > 127:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// saveImage stores an image pasted into the document in an assets folder
// next to it, and returns the path the document links it by. An unsaved
// document keeps it in the temporary folder, by its absolute path.
func (a *App) saveImage(data []byte, ext string) (string, error) {
	if ext == "" {
		ext = ".png"
	}
	dir := a.docDir()
	rel := dir != ""
	if !rel {
		dir = filepath.Join(os.TempDir(), "muse-images")
	} else {
		dir = filepath.Join(dir, "assets")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	p := uniquePath(dir, "image-"+time.Now().Format("20060102-150405"), ext)
	if err := os.WriteFile(p, data, 0o644); err != nil {
		return "", err
	}
	if rel {
		return "assets/" + filepath.Base(p), nil
	}
	return filepath.ToSlash(p), nil
}

func plainSpans(spans []mdoc.Span) string {
	var b strings.Builder
	for _, s := range spans {
		b.WriteString(s.Text)
	}
	return b.String()
}

func countText(s string) (chars, words, lines int) {
	lines = strings.Count(s, "\n") + 1
	for _, r := range s {
		if r == '\n' || r == '\r' || r == '\t' || r == ' ' {
			continue
		}
		chars++
	}
	words = len(strings.Fields(s))
	return
}

// pushNav records a document in the editor's own history, cutting whatever
// was ahead of it: a new document is a new branch.
func (a *App) pushNav(path string) {
	if a.navAt >= 0 && a.navAt < len(a.navHist) && a.navHist[a.navAt] == path {
		return
	}
	a.navHist = append(a.navHist[:min(a.navAt+1, len(a.navHist))], path)
	a.navAt = len(a.navHist) - 1
}

// canNav reports whether there is a document delta steps back (-1) or forward
// (+1) from where the editor is.
func (a *App) canNav(delta int) bool {
	i := a.navAt + delta
	return i >= 0 && i < len(a.navHist)
}

// nav moves the editor through its history.
func (a *App) nav(delta int) {
	if !a.canNav(delta) {
		return
	}
	a.navAt += delta
	a.openPathNow(a.navHist[a.navAt])
}

// docPath is the document's path as the header reads it: from the workspace's
// own name down, so that the line names where in the vault it lives.
func (a *App) docPath() string {
	if a.path == "" {
		return "未命名"
	}
	if a.root == "" {
		return a.path
	}
	if rel, err := filepath.Rel(a.root, a.path); err == nil && !strings.HasPrefix(rel, "..") {
		return filepath.Join(filepath.Base(a.root), rel)
	}
	return a.path
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
	if a.path == "" {
		return "未命名"
	}
	return filepath.Base(a.path)
}

func (a *App) location() string {
	if a.path == "" {
		return "未保存"
	}
	rootName := func() string {
		if a.root == "" {
			return ""
		}
		return filepath.Base(a.root)
	}()
	if a.root != "" {
		if rel, err := filepath.Rel(a.root, a.path); err == nil && !strings.HasPrefix(rel, "..") {
			dir := filepath.Dir(rel)
			if dir == "." {
				return rootName
			}
			if rootName == "" {
				return dir
			}
			return filepath.Join(rootName, dir)
		}
	}
	return filepath.Dir(a.path)
}

func (a *App) openSettings() {
	a.settingsDrag = ui.Point{}
	a.settingsOpen = true
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
func (a *App) canvasLeft(c *ui.Context) float32 {
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
	if a.dirty {
		name = "● " + name
	}
	a.win.SetTitle(name + " — Muse")
}

func (a *App) invalidate() {
	if a.win != nil {
		a.win.Invalidate()
	}
}
