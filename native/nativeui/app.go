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
	"encoding/json"
	"errors"
	"fmt"
	"muse/notestore"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/transfer"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
	"github.com/egoist/mygo/ui/shader"

	"muse/appearance"
	"muse/frontmatter"
	"muse/iconlibrary"
	"muse/locales"
	"muse/mdedit"
	"muse/mdoc"
	"muse/navicons"
	"muse/scrollui"
	"muse/services"
	"muse/vaultsearch"
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

// heading is one entry of the document outline.
type heading struct {
	level  int
	text   string
	offset int // where the heading's text starts in the source
}

// clipboardWrite is a seam for tests: the real clipboard waits on the app's
// main loop, which a headless test does not run.
var clipboardWrite = mygo.Clipboard.WriteText

// handoffTreeNativeDrag promotes the active UI drag. Keep this seam so tests
// can inspect the serialized payload without opening an OS drag session.
var handoffTreeNativeDrag = func(c *ui.Context, data transfer.Data, opts mygo.DragOptions) error {
	// Serialized metadata carries the complete selected set. A process-local
	// single-item value would make same-process native drops bypass it.
	return c.HandoffDataDrag(data, nil, opts)
}

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
	shellOpenPath = mygo.Shell.OpenPath
)

// App is the window's state: everything the view reads and the actions write.
type App struct {
	documents                   *documentRuntime
	vault                       *vaultRuntime
	pendingLinks                bool
	pendingWiki                 string
	workspaceSearch             bool
	workspaceQuery              string
	workspaceSearchSel          int
	workspaceScroll             ui.ScrollState
	workspaceSearchResults      ui.ListState
	workspaceIndicator          scrollui.Indicator
	workspaceReveal             bool
	workspaceSubmit             bool
	workspaceInputFocus         ui.Handle
	indexedPath                 string
	indexedOffset               int
	indexedRevision             string
	win                         *mygo.Window
	imagePreview                imagePreviewState
	treePreviewPath             string
	treePreviewRect             ui.Rect
	treePreviewLeave            time.Time
	treePreviewPopupHovered     bool
	linkPreview                 wikiLinkPreviewState
	wikiSuggestionRoot          string
	wikiSuggestionVault         *vaultRuntime
	wikiSuggestionRevision      uint64
	wikiSuggestionFilesRevision uint64
	wikiSuggestionCache         []string
	wikiSuggestionReady         bool
	wikiSuggestionDetailsCache  []mdedit.WikiSuggestion
	wikiSuggestionDetailsReady  bool
	navIconStore                *navicons.Store
	navIconPickerOpen           bool
	navIconPickerPath           string
	navIconPickerQuery          string
	navIconPickerLastQuery      string
	navIconPickerScroll         ui.ScrollState
	navIconPickerMu             sync.Mutex
	navIconPickerResults        []iconlibrary.Icon
	navIconPickerLoading        bool
	navIconPickerRequest        uint64
	imageDocument               imageDocumentState
	restoreEditorFocus          bool

	folderToggled map[string]time.Time

	// quick open (⌘O): fuzzy file jump inside the workspace
	quickOpen                                      bool
	quickOpenValue                                 string
	quickOpenSel                                   int
	quickOpenCatalogRoot                           string
	quickOpenIndexRevision, quickOpenFilesRevision uint64
	quickOpenCatalogReady                          bool
	quickOpenCatalogVault                          *vaultRuntime
	quickOpenRows                                  []fileRow
	quickOpenTitles                                map[string]string
	quickOpenAliases                               map[string][]string
	quickOpenCandidateQuery                        string
	quickOpenCandidateVault                        *vaultRuntime
	quickOpenCandidateRoot                         string
	quickOpenCandidateIndexRevision                uint64
	quickOpenCandidateFilesRevision                uint64
	quickOpenCandidatesReady                       bool
	quickOpenCandidateRows                         []quickOpenCandidate
	coreMigration                                  *coreMigrationPreview
	coreMigrationOpen                              bool
	coreMigrationLoading                           bool
	coreMigrationRunning                           bool

	// command palette (⌘P): searchable menu actions
	cmdPalette bool
	cmdValue   string
	cmdSel     int

	// appearance
	themeMode int    // themeLight, themeDark, or themeSystem
	accentID  string // persisted accent preset, default red
	// navHist 是打开过的文档，navAt 是当前停在的位置：编辑器的后退/前进
	// 在这里走，和浏览器一样，新的打开会截断后面的记录。
	navHist []string
	navAt   int
	// flat 是"Anytype 那种扁平"：编辑器不再是一张浮起的卡片，没有描边、
	// 没有圆角、没有投影，和窗口同色，页面直接铺满。关掉就是现在的卡片。
	flat bool
	// headerFade 让扁平风格下的头部浮在文档上：页面滚到它下面时模糊、
	// 淡出，而不是被头部的边缘切断（见 header_fade.go）。关掉就退回一
	// 块不透明的头部底色。
	headerFade bool
	// blurValues 与 veilValues 是淡出着色器的 uniforms，只建一次，每帧改值。
	blurValues *shader.Values
	veilValues *shader.Values
	dark       bool // derived from themeMode for the view
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
	// After edge-triggered opening, resize waits for the held pointer to
	// catch the final handle position rather than following its initial delta.
	panelAwaitHandle bool
	panelDragging    bool
	panelGripHeld    bool
	panelTransition  bool
	panelDirect      bool
	panelDrag        float32
	panelNarrow      bool

	// workspace
	recents []recentEntry
	// dirMu 守住 dirs/isDir 两个树缓存：children 在渲染帧里跑，而
	// 打开文件夹的 goroutine 也写它们，headless 下这两者真的并发。
	dirMu sync.Mutex
	// mainMu 在 headless 下把 onMain 和渲染帧串起来：没有窗口事件循环
	// 时 onMain 就地执行，后台 Agent 会与测试的帧同时改编辑器和 AI
	// 状态；有窗口时 onMain 本来就在帧的线程上，锁空转。
	mainMu                  sync.Mutex
	root                    string
	dirs                    map[string][]string
	isDir                   map[string]bool
	expanded                map[string]bool
	fileList                ui.ListState
	fileRowsCache           []fileRow
	fileRowsCacheRoot       string
	fileRowsDirty           bool
	fileScroll              ui.ScrollState
	fileIndicator           scrollui.Indicator
	treeDragging            bool
	treeDragItem            mdedit.VaultItemDrag
	treeDragPointer         ui.Point
	treeDragPointerValid    bool
	treePointerReleased     bool
	treeDragOperation       string
	treeDragTarget          string
	treeDropTarget          treeDropTarget
	treeDropHoverPath       string
	treeDropHoverSince      time.Time
	treeAutoExpandDropPath  string
	treeDragStarted         time.Time
	treeDragFrames          int
	treeNativeHandoff       bool
	nativeDropHandled       bool
	treeNativeDropSuggested transfer.Operation
	treeTrashTargetActive   bool
	treeSelection           map[string]bool
	treeSelectionAnchor     string
	treeDragSelection       []string
	treePressPath           string
	treePressMods           ui.Modifiers
	treePressX              float32
	treePressY              float32
	treePressApplied        bool
	outlineScroll           ui.ScrollState
	outlineIndicator        scrollui.Indicator
	searchScroll            ui.ScrollState
	searchIndicator         scrollui.Indicator
	selectedPath            string
	focusedPanel            PanelID
	treeFocusPath           string
	treeFocusHandle         ui.Handle
	treeFocusRequested      bool
	treeClipboard           []treeClipboardItem
	treeClipboardCut        bool
	treeKeySequences        *KeySequenceMatcher
	panelKeySequences       *KeySequenceMatcher
	treeSequenceKeymap      string
	treeHelpOpen            bool
	panelActions            *ActionDispatcher
	lastPanelAction         ActionID
	propertiesOpen          bool
	propertyScroll          ui.ScrollState
	propertyIndicator       scrollui.Indicator

	// document: the editor owns the Markdown being written; source mirrors
	// it for what reads the document (saving, search, the AI's tools), and
	// every change goes through the editor, as a step of its undo.
	ed       *mdedit.Editor
	path     string
	source   string
	crlf     bool
	headings []heading
	dirty    bool

	propertyEditKey      string
	propertyEditDraft    string
	propertyEditLocation string
	propertyAddKey       string
	propertyAddValue     string
	propertyAddKind      string
	propertySourceMode   bool
	propertySourceDraft  string
	propertyPrefs        documentPropertyPrefs
	propertySystem       []systemProperty
	propertyError        string
	propertyCache        propertyDocumentCache
	propertyHiddenOpen   bool
	propertyAddOpen      bool
	propertyOpenedAt     time.Time
	linkIndexDue         time.Time
	linkIndexGeneration  uint64

	// settings
	settingsOpen           bool
	settingsTab            int
	localeSetting          string
	localeConfigured       bool
	settingsDrag           ui.Point
	settingsScroll         ui.ScrollState
	settingsIndicator      scrollui.Indicator
	aiEnabled              bool
	aiIndicator            scrollui.Indicator
	fontSize               float64
	lineHeight             float64
	smartTypography        bool
	headingMarkerReveal    bool
	vimVirtualImageRows    bool
	cursorFade             bool
	smoothCursor           bool
	hideMouseCursor        bool
	mouseCursorHidden      bool
	mouseCursorHiddenUntil time.Time
	searchMatchPulse       bool
	smoothPageJumps        bool
	vimYankFlash           bool

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
		themeMode:       themeSystem,
		accentID:        "red",
		panel:           panelFiles,
		focusedPanel:    PanelIDEditor,
		panelOpen:       true,
		flat:            true,
		headerFade:      true,
		panelWidth:      panelDefaultWidth,
		panelW:          panelDefaultWidth,
		themeApplied:    -1,
		dirs:            map[string][]string{},
		isDir:           map[string]bool{},
		expanded:        map[string]bool{},
		folderToggled:   map[string]time.Time{},
		outlineRow:      -1,
		fontSize:        16,
		lineHeight:      1.7,
		smartTypography: true,
		recents:         loadRecents(),
		ai:              loadAISettings(),
		aiClient:        services.NewAI(),
		aiMessages:      loadAIChat(),
	}
	// 新窗口就是一张空白未命名文档：光标停在空段落里，直接就能写。
	s := loadEditorSettings()
	if s.SmartTypography != nil {
		a.smartTypography = *s.SmartTypography
	}
	if s.HeadingMarkerReveal != nil {
		a.headingMarkerReveal = *s.HeadingMarkerReveal
	}
	if s.VimVirtualImageRows != nil {
		a.vimVirtualImageRows = *s.VimVirtualImageRows
	}
	if s.CursorFade != nil {
		a.cursorFade = *s.CursorFade
	}
	if s.SmoothCursor != nil {
		a.smoothCursor = *s.SmoothCursor
	}
	if s.HideMouseCursor != nil {
		a.hideMouseCursor = *s.HideMouseCursor
	}
	if s.SearchMatchPulse != nil {
		a.searchMatchPulse = *s.SearchMatchPulse
	}
	if s.VimYankFlash != nil {
		a.vimYankFlash = *s.VimYankFlash
	}
	a.ed = mdedit.New("", a.editorOptions())
	a.ed.SetKeymap(s.Keymap)
	a.loadDoc("")
	return a
}

// ---------------------------------------------------------------------------
// View

func (a *App) View(c *ui.Context) {
	logMemoryDiagnostics(a)
	a.mainMu.Lock()
	defer a.mainMu.Unlock()
	closedImagePreview := false
	if a.navIconPickerOpen && c.Shortcut(0, ui.KeyEscape) {
		a.cancelNavIconSearch()
		a.navIconPickerOpen = false
		a.invalidate()
	}
	if (a.propertyEditKey != "" || a.propertySourceMode) && c.Shortcut(0, ui.KeyEscape) {
		switch {
		case a.propertyEditKey != "":
			a.propertyEditKey, a.propertyEditLocation = "", ""
		case a.propertySourceMode:
			a.propertySourceMode = false
		}
		a.invalidate()
	}
	if a.imagePreview.open && c.Shortcut(0, ui.KeyEscape) {
		a.closeImagePreview()
		closedImagePreview = true
		a.invalidate()
	}
	if !a.anyModalOwnsKeyboard() {
		a.handlePanelShortcuts(c)
	} else {
		a.cancelPanelSequences()
		a.cancelTreeSequences()
	}
	if !a.anyModalOwnsKeyboard() && c.Shortcut(ui.Cmd, ui.KeyO) {
		a.openWorkspaceSearch()
	}
	if !a.imagePreview.open && a.restoreEditorFocus && !closedImagePreview {
		a.ed.Focus()
		a.restoreEditorFocus = false
	}
	a.vaultTick(c)
	a.clearQuickOpenCatalogIfVaultChanged()
	a.documentTick(c)
	a.linkIndexTick(c)
	a.applyTheme(c)
	row := ui.Row(c).Fill()
	rootElement := c.Root()
	rootBounds := rootElement.Bounds()
	rootElement.ObserveInput(func(ev ui.InputEvent) {
		if ev.Kind == ui.InputPointerDown {
			a.treePointerReleased = false
		}
		if ev.Kind == ui.InputPointerUp && ev.Canceled {
			a.treePointerReleased = false
			a.treeDragPointerValid = false
			a.treeAutoExpandDropPath = ""
			a.treeDropTarget = treeDropTarget{}
			return
		}
		if !a.treeDragStarted.IsZero() && (ev.Kind == ui.InputPointerMove || ev.Kind == ui.InputPointerUp) {
			a.treeDragPointer = ui.Point{X: rootBounds.X + ev.X, Y: rootBounds.Y + ev.Y}
			a.treeDragPointerValid = true
			if ev.Kind == ui.InputPointerUp {
				a.treePointerReleased = true
			}
		}
	})
	row.ObserveInput(func(ev ui.InputEvent) {
		if !a.hideMouseCursor || !a.ed.Focused() {
			return
		}
		if (a.ed.Keymap() == "vim" || a.ed.Keymap() == "helix") && ev.Kind == ui.InputKeyDown {
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
		if a.propertiesOpen {
			a.propertiesSidebar(c)
		}
	})
	if a.ed != nil && a.ed.Focused() && !a.treeFocusRequested && !a.treeFocusHandle.Focused(c) {
		a.setFocusedPanel(PanelIDEditor)
	}

	a.sidebarToggle(c)
	a.windowControls(c)
	if !a.imagePreview.open && !closedImagePreview {
		a.settingsDialog(c)
		a.renameDialog(c)
		a.documentDialogs(c)
		a.coreMetadataMigrationDialog(c)
		a.historyDialog(c)
		a.linksDialog(c)
		a.quickOpenDialog(c)
		a.commandPaletteDialog(c)
		a.workspaceSearchDialog(c)
	}
	a.imagePreviewDialog(c)
	a.wikiLinkPreviewDialog(c)
	if a.treePreviewPath != "" {
		a.treeImagePreview(c, a.treePreviewPath, a.treePreviewRect)
	}
	if a.treeAutoExpandDropPath != "" && !a.treePointerReleased {
		a.treeDragSessionProjection(c)
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
	icon := iconPanelLeftClose
	if a.panelOpen {
		tooltip = locales.AiSidebarHide()
		icon = iconPanelLeftOpen
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
		a.railButton(c, iconFiles, locales.MenuFile(), a.panelOpen && a.panel == panelFiles, func() {
			a.showPanel(panelFiles)
		})
		a.railButton(c, iconOutline, locales.MenuOutline(), a.panelOpen && a.panel == panelOutline, func() {
			a.showPanel(panelOutline)
		})
		a.railButton(c, iconSearch, locales.MenuSearchWorkspace(), false, func() {
			a.openWorkspaceSearch()
		})
		if a.aiEnabled {
			a.railButton(c, iconAI, locales.AiAssistant(), a.panelOpen && a.panel == panelAI, func() {
				a.showPanel(panelAI)
			})
		}
		ui.Spacer(c)
		a.railButton(c, iconSettings, locales.SettingsTitle(), a.settingsOpen, a.openSettings)
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
			if a.treeTrashTargetActive && a.panel == panelFiles && !a.treeNativeHandoff {
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
			a.smallIcon(c, iconAI, locales.AiAssistant(), func() { a.showPanel(panelAI) })
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
				ui.Icon(c, iconChevronDown).Size(12, 12).TextColor(t.TextMuted)
			})
		})
		ui.Spacer(c)
		a.smallIcon(c, iconSearch, locales.MenuSearchWorkspace(), func() { a.openWorkspaceSearch() })
		ui.Row(c).Height(28).Radius(8).Background(t.Surface).AlignItems(ui.Center).Children(func() {
			a.smallIcon(c, iconEdit, locales.DocumentNew(), func() {
				if a.root != "" {
					a.newFileIn(a.createTarget())
				} else {
					a.newDoc()
				}
			})
			more := ui.ButtonBase(c).Size(20, 28).Padding(0).Label(locales.DocumentNewMenu()).Tooltip(locales.DocumentNewMenu())
			more.Menu(func(m *ui.Menu) {
				if m.Item(locales.FilesNewFolderMenu()).Chosen() {
					a.newFolderIn(a.createTarget())
				}
			})
			more.Children(func() { ui.Icon(c, iconChevronDown).Size(11, 11).TextColor(t.TextMuted) })
		})
	})
	if a.panel != panelFiles {
		ui.Row(c).Height(30).Padding(0, 10).AlignItems(ui.Center).Children(func() {
			a.smallIcon(c, iconChevronLeft, locales.FilesBackToTree(), func() { a.showPanel(panelFiles) })
		})
	}
}

// sidebarFooter holds the settings of the app itself, at the sidebar's foot.
func (a *App) sidebarFooter(c *ui.Context) {
	ui.Row(c).Shrink(0).Padding(0, 8, 10, 8).Gap(2).Label(locales.SidebarFixedFooter()).Children(func() {
		a.smallIcon(c, iconSettings, locales.SettingsTitle(), a.openSettings)
	})
}

// spaceName is the name of the place the editor is in: the workspace's folder,
// or the document's own.
func (a *App) spaceName() string {
	switch {
	case a.root != "":
		return filepath.Base(a.root)
	case a.path != "":
		return filepath.Base(filepath.Dir(a.path))
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

// filesView draws the workspace tree. Directories open lazily; the list builds
// only the rows in view, so a large repository is still quick.
func (a *App) filesView(c *ui.Context) {
	t := c.Theme()
	wasDragging := a.treeDragging
	a.treeDragging = false
	treeDropCommitted := false
	a.treeTrashTargetActive = false
	a.nativeDropHandled = false
	a.treeNativeDropSuggested = transfer.None
	if !wasDragging {
		a.setTreeDragMoveOperation(treeDragMoveToFolder, filepath.Base(a.root))
	}
	if a.root == "" {
		ui.Column(c).Fill().Center().Gap(10).Padding(20).Children(func() {
			ui.Icon(c, iconFolderOpen).Size(30, 30).TextColor(iconTint(t))
			// 一个入口、一个按钮：文件与文件夹都会结束空态；下拉里还有
			// 最近打开过的记录，点一下直接回到那里。
			open := ui.Button(c, locales.CommonOpenFileOrFolder())
			open.Menu(func(m *ui.Menu) {
				if m.Item(locales.FilesOpenFileMenu()).Chosen() {
					a.openFileDialog()
				}
				if m.Item(locales.FilesOpenFolderMenu()).Chosen() {
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

	rows := a.fileRows()
	if !a.treeDragStarted.IsZero() && !a.treePointerReleased {
		root := c.Root()
		if x, y, over := root.PointerPosition(); over {
			a.treeDragPointer = ui.Point{X: x, Y: y}
			a.treeDragPointerValid = true
		}
	}
	if len(rows) == 0 {
		ui.Column(c).Fill().Center().Gap(6).Padding(20).Children(func() {
			ui.Text(c, locales.FilesNoEditableText()).FontSize(12).TextColor(t.TextMuted)
			ui.Text(c, locales.FilesEditableTypes()).FontSize(10.5).TextColor(t.TextMuted)
		})
		return
	}
	showScrollbar := a.fileIndicator.Visible(c.Now(), a.fileScroll.X, a.fileScroll.Y)
	if showScrollbar {
		c.After(a.fileIndicator.Remaining(c.Now()))
	}
	a.treePreviewPopupHovered = false
	imageRowHovered := false
	treePointerPressed := false
	treeRowDropHovered := false
	treeRootDropHovered := false
	dropRowBounds := make(map[int]ui.Rect)
	list := ui.List(c, &a.fileList, len(rows), func(i int) {
		r := rows[i]
		row := ui.Row(c).Key("file-tree-row-"+r.path).FillWidth().Height(28).Radius(6).Gap(5).
			Padding(0, 8, 0, 4+float32(r.depth*14)).Cursor(ui.CursorPointer).Label(filepath.Base(r.path))
		dragItem := mdedit.VaultItemDrag{Path: r.path, Name: filepath.Base(r.path), Directory: r.dir}
		// Drop targets live on the unchanged visible row. The transparent
		// handle below supplies the styled clone MyGo paints at the pointer.
		switch {
		case r.path == a.selectedPath || a.treeSelection[r.path]:
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
		if a.treeDropTarget.path != "" && i >= a.treeDropTarget.start && i < a.treeDropTarget.end && !a.treeSelection[r.path] {
			row.Background(t.Focus.Alpha(0.08))
		}
		var handle ui.Element
		row.Children(func() {
			if r.dir {
				if a.expanded[r.path] {
					ui.Icon(c, iconChevronDown).Size(12, 12).TextColor(iconTint(t))
				} else {
					ui.Icon(c, iconChevron).Size(12, 12).TextColor(iconTint(t))
				}
				ui.Icon(c, a.treeIcon(r.path, true)).Size(14, 14).TextColor(iconTint(t))
			} else {
				ui.Box(c).Width(12)
				icon := a.treeIcon(r.path, false)
				if isImageFile(r.path) && a.treeIconID(r.path) == "" {
					icon = iconImage
				}
				ui.Icon(c, icon).Size(14, 14).TextColor(iconTint(t))
			}
			ui.Text(c, filepath.Base(r.path)).IgnoreInput(true).FontSize(12.5).TextColor(t.Text).
				SingleLine().Ellipsis("…")
			if a.treeClipboardCut && a.treeClipboardContains(r.path) {
				ui.Text(c, locales.FilesCut()).Label(locales.FilesCutHint()).FontSize(9).TextColor(t.TextMuted.Alpha(0.58)).
					Padding(1, 4).Radius(4).Background(t.Surface)
			}
			handle = a.treeDragHandle(c, dragItem)
		})
		handle.HandleInput(func(ev ui.InputEvent) bool {
			if ev.Kind == ui.InputPointerDown {
				a.recordTreePointerDown(r.path, ev.Mods, ev.X, ev.Y)
			} else if ev.Kind == ui.InputPointerMove && ev.Button == 0 && a.treeDragStarted.IsZero() && a.treePressPath == r.path && !a.treePressApplied && treePointerMovePassedThreshold(a.treePressX, a.treePressY, ev.X, ev.Y) {
				a.prepareTreeDragPress(r.path, a.treePressMods, rows)
				a.treeDragSelection = a.selectedTreePaths(r.path)
				a.treePressApplied = true
			}
			return false
		})
		if handle.Pressed() {
			treePointerPressed = true
		}
		if handle.Hovered() {
			row.Background(t.SurfaceHover)
		}
		if r.path == a.treeFocusPath || r.path == a.treeDropTarget.path {
			row.Border(1, t.Focus.Alpha(0.48))
		}
		rowBounds := row.Bounds()
		dropRowBounds[i] = rowBounds
		dataTarget := transfer.DropOptions{Formats: []transfer.Format{transfer.Format("application/x-muse-vault-item+json")}, Operations: transfer.Copy | transfer.Move}
		_, internalOver := ui.DragOver[mdedit.VaultItemDrag](handle)
		offer, nativeOver := ui.DataDragOver(handle, dataTarget)
		fileOver := handle.FileDragOver()
		if internalOver || nativeOver || fileOver {
			treeRowDropHovered = true
			x, _, _ := handle.PointerPosition()
			if target, ok := resolveTreeDropTarget(rows, i, x, a.root); ok {
				if a.treeAutoExpandDropPath != "" && target.path != a.treeAutoExpandDropPath {
					a.treeAutoExpandDropPath = ""
				}
				a.treeDropTarget = target
				a.setTreeDragMoveOperation(treeDragMoveToFolder, filepath.Base(target.path))
				if nativeOver {
					a.treeNativeDropSuggested = offer.Suggested
				}
				row.Tooltip(a.treeDragOperationLabel())
				if r.path == target.path {
					row.Background(t.SurfaceHover)
				}
				if r.dir && target.path == r.path && !a.expanded[r.path] {
					if a.treeDropHoverPath != r.path {
						a.treeDropHoverPath, a.treeDropHoverSince = r.path, c.Now()
					}
					remaining := 600*time.Millisecond - c.Now().Sub(a.treeDropHoverSince)
					if remaining <= 0 {
						a.expanded[r.path] = true
						a.fileRowsDirty = true
						a.treeDropHoverPath = ""
						a.treeAutoExpandDropPath = r.path
						c.After(time.Millisecond)
					} else {
						c.After(remaining)
					}
				} else {
					a.treeDropHoverPath, a.treeDropHoverSince = "", time.Time{}
				}
			}
			if fileOver {
				if target, ok := defaultTreeDropTarget(rows, i, a.root); ok {
					if a.treeAutoExpandDropPath != "" && target.path != a.treeAutoExpandDropPath {
						a.treeAutoExpandDropPath = ""
					}
					a.treeDropTarget = target
					a.setTreeDragMoveOperation(treeDragMoveToFolder, filepath.Base(target.path))
				}
			}
		}
		if dropped, ok := ui.Drop[mdedit.VaultItemDrag](handle); ok {
			treeDropCommitted = true
			treeRowDropHovered = true
			if pointerWithinElement(c, handle) && treePointerWithinRect(a.treeDragPointer, a.treeDragPointerValid, dropRowBounds[i]) {
				target := a.treeDropTarget
				if target.path == "" {
					target, _ = defaultTreeDropTarget(rows, i, a.root)
				}
				if target.path != "" {
					a.moveTreeItems(a.treeDraggedPaths(dropped.Path), target.path)
				}
			}
		}
		if dropped, ok := ui.DropData(handle, dataTarget); ok {
			treeRowDropHovered = true
			a.nativeDropHandled = true
			if a.treeDropTarget.path != "" {
				a.applyTreeNativeDrop(dropped, a.treeDropTarget.path)
			}
		}
		if paths := handle.DroppedFiles(); len(paths) > 0 {
			treeRowDropHovered = true
			a.nativeDropHandled = true
			target := a.treeDropTarget
			if target.path == "" {
				target, _ = defaultTreeDropTarget(rows, i, a.root)
			}
			if target.path != "" {
				a.importDroppedInto(paths, target.path)
			}
		}
		if handle.Dragging() {
			a.treeDragging = true
			a.treeDragFrames++
			a.treeDragItem = dragItem
			if a.treeDragStarted.IsZero() {
				if a.treePressPath == r.path && !a.treePressApplied {
					a.prepareTreeDragPress(r.path, a.treePressMods, rows)
				}
				a.treeDragSelection = a.selectedTreePaths(r.path)
			}
			if a.treeDragStarted.IsZero() {
				a.treeDragStarted = c.Now()
			}
			root := c.Root()
			if x, y, over := root.PointerPosition(); over {
				a.treeDragPointer = ui.Point{X: x, Y: y}
			} else if !a.treeNativeHandoff {
				var items []mdedit.VaultItemDrag
				for _, path := range a.treeDraggedPaths(r.path) {
					info, err := os.Stat(path)
					if err == nil {
						items = append(items, mdedit.VaultItemDrag{Path: path, Name: filepath.Base(path), Directory: info.IsDir()})
					}
				}
				a.beginTreeNativeHandoffItems(c, items)
			}
		}
		openRow := func() {
			a.setFocusedPanel(PanelIDFiles)
			a.treeFocusPath = r.path
			a.treeFocusRequested = true
			mods := handle.ClickModifiers()
			if mods == 0 {
				mods = row.ClickModifiers()
			}
			a.selectTreeRow(r.path, mods, rows)
			if mods&(ui.Cmd|ui.Shift) != 0 {
				return
			}
			if r.dir {
				// 双击的第二下不算：连点不再表现为"展开又立刻折叠"。
				if last, ok := a.folderToggled[r.path]; ok && time.Since(last) < 300*time.Millisecond {
					return
				}
				a.folderToggled[r.path] = time.Now()
				a.expanded[r.path] = !a.expanded[r.path]
				a.fileRowsDirty = true
			} else if isImageFile(r.path) {
				a.requestOpen(r.path)
			} else {
				a.selectedPath = r.path
				a.requestOpen(r.path)
			}
		}
		row.OnClick(openRow)
		handle.OnClick(openRow)
		a.fileMenu(c, row, r.path)
		treeDragSuppressesPreview := a.treeDragging || wasDragging || a.treeNativeHandoff || !a.treeDragStarted.IsZero()
		if treeDragSuppressesPreview {
			a.treePreviewPath = ""
			a.treePreviewLeave = time.Time{}
		} else if !r.dir && isImageFile(r.path) && (row.Hovered() || handle.Hovered()) {
			imageRowHovered = true
			a.treePreviewPath, a.treePreviewRect = r.path, row.Bounds()
			a.treePreviewLeave = time.Time{}
		}
		if a.navIconPickerOpen && a.navIconPickerPath == r.path {
			a.navIconPickerPopover(c, row)
		}
	}).Focusable().Bind(&a.treeFocusHandle).Label(locales.FilesTreeLabel()).HandleInput(func(ev ui.InputEvent) bool {
		if ev.Kind != ui.InputKeyDown || a.anyModalOwnsKeyboard() {
			return false
		}
		return a.handleTreeKey(ev.Mods, ev.Key)
	}).Grow(1).Padding(0, 8).Gap(1).TrackScroll(&a.fileScroll).Scrollbars(showScrollbar)
	a.fileRootMenu(list)
	if a.treeFocusRequested {
		a.treeFocusHandle.Focus(c)
		if a.treeFocusHandle.Focused(c) {
			a.treeFocusRequested = false
		}
	}
	if a.treeFocusHandle.Focused(c) {
		a.setFocusedPanel(PanelIDFiles)
	}
	if _, over := ui.DragOver[mdedit.VaultItemDrag](list); over && !treeRowDropHovered {
		if target, rowIndex, ok := treeDropTargetAtPointer(c, rows, dropRowBounds, a.root); ok {
			treeRowDropHovered = true
			a.treeDropTarget = target
			a.setTreeDragMoveOperation(treeDragMoveToFolder, filepath.Base(target.path))
			_ = rowIndex
		} else {
			treeRootDropHovered = true
			a.setTreeRootDropTarget(rows)
		}
		list.Tooltip(a.treeDragOperationLabel())
	}
	if dropped, ok := ui.Drop[mdedit.VaultItemDrag](list); ok && !treeRowDropHovered {
		treeDropCommitted = true
		if pointerWithinElement(c, list) && treePointerWithinRect(a.treeDragPointer, a.treeDragPointerValid, list.Bounds()) {
			destination := a.root
			if target, _, inRow := treeDropTargetAtPointer(c, rows, dropRowBounds, a.root); inRow {
				treeRowDropHovered = true
				destination = target.path
			} else {
				treeRootDropHovered = true
			}
			a.moveTreeItems(a.treeDraggedPaths(dropped.Path), destination)
		}
	}
	rootDataTarget := transfer.DropOptions{Formats: []transfer.Format{transfer.Format("application/x-muse-vault-item+json")}, Operations: transfer.Copy | transfer.Move}
	rootFileOver := list.FileDragOver()
	if rootFileOver && !treeRowDropHovered {
		// Visible row handles consume row-level file hover. The list-level
		// native target is only the blank-area fallback, and native file drag
		// events do not provide a fresh PointerPosition to resolve here.
		treeRootDropHovered = true
		a.setTreeRootDropTarget(rows)
		list.Tooltip(a.treeDragOperationLabel())
	}
	if offer, over := ui.DataDragOver(list, rootDataTarget); over && !treeRowDropHovered {
		treeRootDropHovered = true
		a.setTreeRootDropTarget(rows)
		a.treeNativeDropSuggested = offer.Suggested
	}
	if dropped, ok := ui.DropData(list, rootDataTarget); ok && !treeRowDropHovered {
		// Per-row targets consume drops on visible rows. Reaching the list
		// handler therefore means the pointer is on its blank area; do not reuse
		// a directory left over from an earlier hover.
		destination := a.root
		treeRootDropHovered = true
		a.setTreeRootDropTarget(rows)
		a.nativeDropHandled = true
		a.applyTreeNativeDrop(dropped, destination)
	}
	if a.treePreviewPath != "" && !imageRowHovered && !a.treePreviewPopupHovered {
		if a.treePreviewLeave.IsZero() {
			a.treePreviewLeave = c.Now().Add(220 * time.Millisecond)
		}
		if remaining := a.treePreviewLeave.Sub(c.Now()); remaining <= 0 {
			a.treePreviewPath = ""
		} else {
			c.After(remaining)
		}
	}
	if !a.treeDragStarted.IsZero() && a.treeAutoExpandDropPath != "" && !a.treePointerReleased && !a.treeDragging {
		// The source row's MyGo drag state is tied to its virtual-list node.
		// Keep the app-owned session alive when expanding the tree rebuilds that
		// node, and render its projection independently of the row handle.
		a.treeDragging = true
	}
	if paths := list.DroppedFiles(); len(paths) > 0 && !a.nativeDropHandled && !treeRowDropHovered {
		// Row handles own file drops inside rows. A list-level drop is on blank
		// space, where the vault root is the stable fallback destination.
		destination := a.root
		treeRootDropHovered = true
		a.setTreeRootDropTarget(rows)
		a.nativeDropHandled = true
		a.importDroppedInto(paths, destination)
	}
	if (a.treeDragging || wasDragging) && !treeRowDropHovered && !treeRootDropHovered && a.treeAutoExpandDropPath == "" {
		a.treeDropTarget = treeDropTarget{}
	}
	// MyGo rebuilds virtual-list row nodes when a hovered folder auto-expands.
	// In that transition the release can be delivered to the original drag
	// source without a Drop value reaching the newly built target row. Preserve
	// the last resolved path and commit once on the source drag's release frame.
	releaseTarget, _, releaseOverRow := treeDropTargetAtPointer(c, rows, dropRowBounds, a.root)
	if a.treeDragPointerValid {
		releaseTarget, _, releaseOverRow = treeDropTargetAtWindowPoint(rows, dropRowBounds, a.root, a.treeDragPointer.X, a.treeDragPointer.Y)
	}
	if !a.treeDragging && !treeDropCommitted && !a.treeNativeHandoff && a.treePointerReleased && a.treeDragPointerValid &&
		a.treeAutoExpandDropPath != "" && a.treeDropTarget.path == a.treeAutoExpandDropPath &&
		a.treeDragOperation == treeDragMoveToFolder && a.treeDragTarget == filepath.Base(a.treeDropTarget.path) &&
		releaseOverRow && releaseTarget.path == a.treeAutoExpandDropPath {
		a.moveTreeItems(a.treeDraggedPaths(a.treeDragItem.Path), a.treeDropTarget.path)
		a.treeAutoExpandDropPath = ""
	}
	if a.treePointerReleased && a.treeAutoExpandDropPath != "" {
		a.treeAutoExpandDropPath = ""
		a.treeDropTarget = treeDropTarget{}
	}
	if !a.treeDragging && !a.treeNativeHandoff && (a.treePointerReleased || (!wasDragging && !treeRowDropHovered && !treeRootDropHovered && a.treeAutoExpandDropPath == "")) {
		a.treeDropTarget = treeDropTarget{}
		a.treeDropHoverPath, a.treeDropHoverSince = "", time.Time{}
		a.treeAutoExpandDropPath = ""
		if !a.treeNativeHandoff {
			a.treeDragSelection = nil
		}
		a.treeDragStarted = time.Time{}
		a.treeDragFrames = 0
		a.setTreeDragMoveOperation(treeDragMoveToFolder, filepath.Base(a.root))
		a.treeDragItem = mdedit.VaultItemDrag{}
		a.treeDragPointerValid = false
		a.treePointerReleased = false
		if !treePointerPressed {
			a.treePressPath = ""
			a.treePressMods = 0
			a.treePressApplied = false
		}
	}
	a.treeTrashTargetActive = (a.treeDragging || wasDragging) && !a.treeNativeHandoff
}

// treeTrashDropTarget covers the sidebar's bottom content while an in-window
// tree drag is active. Its inset keeps it off the panel edges and its weak
// default colors make it read as a drop affordance rather than a button.
func (a *App) treeTrashDropTarget(c *ui.Context) {
	t := c.Theme()
	// Leave the fixed footer (settings gear) outside the drop hit area. The
	// 48 DIP bottom offset is the footer's 38 DIP height plus the usual 10 DIP
	// visual gap.
	trash := ui.Box(c).Absolute().Left(10).Right(10).Bottom(48).Height(64).
		Key("tree-trash-drop-target").Label(locales.TreeTrashDropTarget()).Radius(12).
		Background(t.Background.Alpha(0.04)).Border(1, t.Border.Alpha(0.30)).BorderStyle(ui.BorderDashed).
		Padding(0, 12).AlignItems(ui.Center).Justify(ui.Center)
	_, over := ui.DragOver[mdedit.VaultItemDrag](trash)
	if over {
		a.setTreeDragMoveOperation(treeDragMoveToTrash, "")
		trash.Background(ui.RGBA(128, 66, 72, 0.10)).BorderColor(t.Border.Alpha(0.48))
	}
	if dropped, ok := ui.Drop[mdedit.VaultItemDrag](trash); ok {
		a.trashTreeItems(a.treeDraggedPaths(dropped.Path))
	}
	trash.Children(func() {
		ui.Column(c).Width(110).FillHeight().AlignItems(ui.Center).Justify(ui.Center).Gap(4).
			Label(locales.TreeTrashHintColumn()).Children(func() {
			glyph := iconTrash
			iconColor := t.TextMuted.Alpha(0.18)
			labelColor := t.TextMuted.Alpha(0.23)
			if over {
				glyph = iconTrashOpen
				iconColor = ui.Hex("#d98b92").Alpha(0.72)
				labelColor = t.Text.Alpha(0.68)
			}
			trashGlyph := ui.Icon(c, glyph).Size(18, 18).TextColor(iconColor)
			if over {
				trashGlyph.Label(locales.TreeTrashHoverIcon())
			} else {
				ui.Text(c, locales.FilesMoveToTrash()).Label(locales.TreeTrashHint()).FontSize(12).TextColor(labelColor)
			}
		})
	})
}

func (a *App) setTreeRootDropTarget(rows []fileRow) {
	a.treeDropTarget = treeDropTarget{path: a.root, start: 0, end: len(rows)}
	a.setTreeDragMoveOperation(treeDragMoveToFolder, filepath.Base(a.root))
}

func (a *App) importDroppedInto(paths []string, destination string) {
	for _, source := range paths {
		if source == "" || source == destination {
			continue
		}
		info, err := os.Lstat(source)
		if err != nil {
			continue
		}
		if info.Mode()&os.ModeSymlink != 0 {
			continue
		}
		if info.IsDir() {
			if rel, err := filepath.Rel(source, destination); err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
				continue // Never recursively copy a directory into itself.
			}
		}
		name := filepath.Base(source)
		target := filepath.Join(destination, name)
		if _, err := os.Stat(target); err == nil {
			target = uniquePath(destination, strings.TrimSuffix(name, filepath.Ext(name)), filepath.Ext(name))
		}
		if info.IsDir() {
			err = copyTree(source, target)
		} else {
			err = copyFile(source, target)
		}
		if err != nil {
			a.notice(locales.CommonImportFailed(err.Error()))
			return
		}
	}
	a.clearTreeCache()
	a.invalidate()
}

func (a *App) beginTreeNativeHandoffItems(c *ui.Context, items []mdedit.VaultItemDrag) {
	if a.treeNativeHandoff || len(items) == 0 || items[0].Path == "" {
		return
	}
	data, err := treeItemsTransferData(items)
	if err != nil {
		a.notice(locales.CommonMoveAcrossWindowsFailed(err.Error()))
		return
	}
	sourcePath := items[0].Path
	a.treeNativeHandoff = true
	opts := a.treeNativeDragOptions(sourcePath)
	if err := handoffTreeNativeDrag(c, data, opts); err != nil && a.treeNativeHandoff {
		a.treeNativeHandoff = false
		a.notice(locales.CommonMoveAcrossWindowsFailed(err.Error()))
	}
}

func (a *App) treeNativeDragOptions(sourcePath string) mygo.DragOptions {
	return mygo.DragOptions{
		Operations: transfer.Copy | transfer.Move,
		Done: func(result transfer.Result) {
			a.treeNativeHandoff = false
			a.treeDragging = false
			a.treeDragStarted = time.Time{}
			a.treeDragFrames = 0
			a.treeDragSelection = nil
			if result.Err == nil && result.Operation != transfer.None && isWithinRoot(a.root, sourcePath) {
				// The receiver owns any filesystem move. Refresh only the
				// source window's cached listings after an accepted native drop;
				// clearing after Copy is harmless and keeps explicit copies fresh.
				a.clearTreeCache()
			}
			if result.Err != nil {
				a.notice(locales.CommonWindowDragFailed(result.Err.Error()))
			}
			a.invalidate()
		},
	}
}

func treeItemTransferData(item mdedit.VaultItemDrag) (transfer.Data, error) {
	return treeItemsTransferData([]mdedit.VaultItemDrag{item})
}

func treeItemsTransferData(items []mdedit.VaultItemDrag) (transfer.Data, error) {
	if len(items) == 0 {
		return transfer.Data{}, fmt.Errorf("%s", locales.FilesNoDraggableItems())
	}
	paths := make([]string, 0, len(items))
	for _, item := range items {
		if !filepath.IsAbs(item.Path) {
			return transfer.Data{}, fmt.Errorf("%s", locales.FilesAbsolutePathRequired())
		}
		paths = append(paths, item.Path)
	}
	files, err := transfer.FileData(paths...)
	if err != nil {
		return transfer.Data{}, err
	}
	for i := range items {
		if items[i].Name == "" {
			items[i].Name = filepath.Base(items[i].Path)
		}
	}
	b, err := json.Marshal(struct {
		Path      string                 `json:"path"`
		Name      string                 `json:"name"`
		Directory bool                   `json:"directory"`
		Items     []mdedit.VaultItemDrag `json:"items,omitempty"`
	}{items[0].Path, items[0].Name, items[0].Directory, items})
	if err != nil {
		return transfer.Data{}, err
	}
	fileItems := files.Items()
	dataItems := make([]transfer.Item, 0, len(fileItems))
	for i, file := range fileItems {
		fileList, err := file.Read(transfer.FileList)
		if err != nil {
			return transfer.Data{}, err
		}
		uriList, err := file.Read(transfer.URIList)
		if err != nil {
			return transfer.Data{}, err
		}
		reps := []transfer.Representation{
			transfer.Bytes(transfer.FileList, fileList),
			transfer.Bytes(transfer.URIList, uriList),
			transfer.Bytes(transfer.Text, []byte(items[i].Path)),
		}
		if i == 0 {
			reps = append(reps, transfer.Bytes(transfer.Format("application/x-muse-vault-item+json"), b))
		}
		dataItems = append(dataItems, transfer.NewItem(reps...))
	}
	return transfer.New(dataItems...), nil
}

// applyTreeNativeDrop handles Muse's cross-window payload before the platform
// file-list fallback. The native source never removes its item in Done; the
// receiving window performs a same-vault Move exactly once here.
func (a *App) applyTreeNativeDrop(drop transfer.Drop, destination string) {
	b, err := drop.Data.Read(transfer.Format("application/x-muse-vault-item+json"))
	if err != nil {
		return
	}
	var payload struct {
		Path      string                 `json:"path"`
		Name      string                 `json:"name"`
		Directory bool                   `json:"directory"`
		Items     []mdedit.VaultItemDrag `json:"items"`
	}
	if err := json.Unmarshal(b, &payload); err != nil {
		a.notice(locales.FilesReadDropFailed())
		return
	}
	items := payload.Items
	if len(items) == 0 && payload.Path != "" {
		items = []mdedit.VaultItemDrag{{Path: payload.Path, Name: payload.Name, Directory: payload.Directory}}
	}
	if len(items) == 0 {
		a.notice(locales.FilesReadDropFailed())
		return
	}
	// Native negotiation defaults to Copy when no modifier is suggested.
	// Muse's same-vault tree semantics are Move unless the user explicitly
	// requested Copy in the native drag session.
	move := drop.Operation == transfer.Move || (drop.Operation == transfer.Copy && a.treeNativeDropSuggested != transfer.Copy)
	if move && isWithinRoot(a.root, items[0].Path) {
		paths := make([]string, 0, len(items))
		for _, item := range items {
			if item.Path != "" && isWithinRoot(a.root, item.Path) {
				paths = append(paths, item.Path)
			}
		}
		a.moveTreeItems(paths, destination)
		return
	}
	// Copy remains non-destructive. A Move from another vault/process falls
	// back to import semantics because this workspace cannot own its source.
	paths := make([]string, 0, len(items))
	for _, item := range items {
		if item.Path != "" {
			paths = append(paths, item.Path)
		}
	}
	a.importDroppedInto(paths, destination)
}

func isWithinRoot(root, path string) bool {
	if root == "" || path == "" {
		return false
	}
	rel, err := filepath.Rel(root, path)
	return err == nil && rel != ".." && !filepath.IsAbs(rel) && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func copyFile(source, destination string) error {
	b, err := os.ReadFile(source)
	if err != nil {
		return err
	}
	info, err := os.Stat(source)
	if err != nil {
		return err
	}
	return os.WriteFile(destination, b, info.Mode().Perm())
}

func copyTree(source, destination string) error {
	return filepath.WalkDir(source, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		target := filepath.Join(destination, rel)
		if entry.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return nil
		}
		return copyFile(path, target)
	})
}

// moveTreeItem moves a dragged item into a directory in the workspace.
// Existing names are preserved; collisions get a numbered suffix.
func (a *App) moveTreeItem(source, destination string) {
	if source == "" || destination == "" || source == destination || destination == filepath.Dir(source) {
		return
	}
	if rel, err := filepath.Rel(a.root, source); err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return
	}
	info, err := os.Stat(destination)
	if err != nil || !info.IsDir() || strings.HasPrefix(destination, source+string(filepath.Separator)) {
		return
	}
	name := filepath.Base(source)
	target := filepath.Join(destination, name)
	if _, err := os.Stat(target); err == nil {
		ext := filepath.Ext(name)
		base := strings.TrimSuffix(name, ext)
		target = uniquePath(destination, base, ext)
	}
	if err := os.Rename(source, target); err != nil {
		a.notice(locales.CommonMoveFailed(err.Error()))
		return
	}
	remap := func(path string) string {
		if rel, err := filepath.Rel(source, path); err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return filepath.Join(target, rel)
		}
		return path
	}
	a.path = remap(a.path)
	a.selectedPath = remap(a.selectedPath)
	a.treeFocusPath = remap(a.treeFocusPath)
	if a.treeSelection != nil {
		selection := make(map[string]bool, len(a.treeSelection))
		for path, selected := range a.treeSelection {
			if selected {
				selection[remap(path)] = true
			}
		}
		a.treeSelection = selection
	}
	a.treeSelectionAnchor = remap(a.treeSelectionAnchor)
	for i, path := range a.treeDragSelection {
		a.treeDragSelection[i] = remap(path)
	}
	for old, expanded := range a.expanded {
		if next := remap(old); next != old {
			delete(a.expanded, old)
			a.expanded[next] = expanded
		}
	}
	a.clearTreeCache()
	a.notice(locales.FilesMovedToFolder(filepath.Base(destination)))
	a.invalidate()
}

type fileRow struct {
	path  string
	depth int
	dir   bool
}

func (a *App) fileRows() []fileRow {
	if a.root == "" {
		return nil
	}
	if !a.fileRowsDirty && a.fileRowsCacheRoot == a.root {
		return a.fileRowsCache
	}
	var out []fileRow
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
	a.fileRowsCache = out
	a.fileRowsCacheRoot = a.root
	a.fileRowsDirty = false
	return a.fileRowsCache
}

func (a *App) selectTreeRow(path string, mods ui.Modifiers, rows []fileRow) {
	if a.treeSelection == nil {
		a.treeSelection = make(map[string]bool)
	}
	switch {
	case mods&ui.Shift != 0:
		anchor := a.treeSelectionAnchor
		if anchor == "" {
			anchor = path
		}
		indices := make(map[string]int, len(rows))
		for i, row := range rows {
			indices[row.path] = i
		}
		start, startOK := indices[anchor]
		end, endOK := indices[path]
		if !startOK || !endOK {
			return
		}
		clear(a.treeSelection)
		if start > end {
			start, end = end, start
		}
		for _, row := range rows[start : end+1] {
			a.treeSelection[row.path] = true
		}
	case mods&ui.Cmd != 0:
		if a.treeSelection[path] {
			delete(a.treeSelection, path)
		} else {
			a.treeSelection[path] = true
		}
		if a.treeSelectionAnchor == "" {
			a.treeSelectionAnchor = path
		}
	default:
		clear(a.treeSelection)
		a.treeSelection[path] = true
		a.treeSelectionAnchor = path
	}
}

// prepareTreeDragPress applies selection semantics at pointer-down so a drag
// that crosses the toolkit threshold (and therefore suppresses OnClick) still
// carries the intended item group. Pressing an already-selected row preserves
// the group until release; Cmd adds an unselected row, and Shift extends the
// anchored range.
func (a *App) prepareTreeDragPress(path string, mods ui.Modifiers, rows []fileRow) {
	if a.treeSelection == nil {
		a.treeSelection = make(map[string]bool)
	}
	switch {
	case mods&ui.Shift != 0:
		a.selectTreeRow(path, ui.Shift, rows)
	case mods&ui.Cmd != 0:
		if !a.treeSelection[path] {
			a.treeSelection[path] = true
		}
		if a.treeSelectionAnchor == "" {
			a.treeSelectionAnchor = path
		}
	default:
		if !a.treeSelection[path] {
			clear(a.treeSelection)
			a.treeSelection[path] = true
			a.treeSelectionAnchor = path
		}
	}
}

func (a *App) recordTreePointerDown(path string, mods ui.Modifiers, x, y float32) {
	a.treePressPath, a.treePressMods = path, mods
	a.treePressX, a.treePressY = x, y
	a.treePressApplied = false
}

func treePointerMovePassedThreshold(x0, y0, x1, y1 float32) bool {
	dx, dy := x1-x0, y1-y0
	return dx*dx+dy*dy >= 16 // MyGo's dragStart is 4 DIP.
}

func (a *App) selectedTreePaths(fallback string) []string {
	rows := a.fileRows()
	var selected []string
	for _, row := range rows {
		if a.treeSelection[row.path] {
			selected = append(selected, row.path)
		}
	}
	if len(selected) == 0 && fallback != "" {
		selected = append(selected, fallback)
	}
	return selected
}

func (a *App) treeDraggedPaths(primary string) []string {
	var paths []string
	if len(a.treeDragSelection) > 0 {
		paths = append(paths, a.treeDragSelection...)
	} else if a.treeSelection[primary] {
		paths = a.selectedTreePaths(primary)
	} else if primary != "" {
		paths = []string{primary}
	}
	// If both a directory and one of its visible descendants are selected,
	// moving/removing the directory already acts on the descendant.
	selected := make(map[string]bool, len(paths))
	for _, path := range paths {
		selected[path] = true
	}
	filtered := make([]string, 0, len(paths))
	for _, path := range paths {
		covered := false
		for parent := filepath.Dir(path); parent != path; {
			if selected[parent] {
				if info, err := os.Stat(parent); err == nil && info.IsDir() {
					covered = true
					break
				}
			}
			next := filepath.Dir(parent)
			if next == parent {
				break
			}
			parent = next
		}
		if !covered {
			filtered = append(filtered, path)
		}
	}
	return filtered
}

func (a *App) moveTreeItems(paths []string, destination string) {
	for _, path := range paths {
		a.moveTreeItem(path, destination)
	}
}

func (a *App) outlineView(c *ui.Context) {
	t := c.Theme()
	if len(a.headings) == 0 {
		ui.Column(c).Fill().Center().Padding(20).Children(func() {
			ui.Text(c, locales.DocumentNoHeading()).FontSize(12).TextColor(t.TextMuted)
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
	showScrollbar := a.outlineIndicator.Visible(c.Now(), a.outlineScroll.X, a.outlineScroll.Y)
	if showScrollbar {
		c.After(a.outlineIndicator.Remaining(c.Now()))
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
			a.ed.NavigateToOffset(h.offset, mdedit.PageJumpTopMargin)
			a.ed.Focus()
		})
	}).Grow(1).Padding(0, 8).Gap(0).TrackScroll(&a.outlineScroll).Scrollbars(showScrollbar)
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

// workspaceSearchText keeps the entire result label readable while marking
// every visible occurrence of each query term. Positions are rune-based so
// CJK text before a match does not shift the highlight.
func workspaceSearchText(c *ui.Context, t *ui.Theme, text, query string, color ui.Color, size float32) ui.Element {
	runes := []rune(text)
	lower := make([]rune, len(runes))
	for i, r := range runes {
		lower[i] = unicode.ToLower(r)
	}
	hits := make(map[int]struct{})
	for _, term := range strings.Fields(query) {
		needle := []rune(term)
		for i := range needle {
			needle[i] = unicode.ToLower(needle[i])
		}
		if len(needle) == 0 || len(needle) > len(lower) {
			continue
		}
		for start := 0; start+len(needle) <= len(lower); {
			matched := true
			for j, r := range needle {
				if lower[start+j] != r {
					matched = false
					break
				}
			}
			if !matched {
				start++
				continue
			}
			for j := range needle {
				hits[start+j] = struct{}{}
			}
			start += len(needle)
		}
	}
	if len(hits) == 0 {
		return ui.RichText(c, ui.Span{Text: text}).FontSize(size).TextColor(color).SingleLine().Ellipsis("…")
	}
	spans := make([]ui.Span, 0, len(hits)*2+1)
	for start := 0; start < len(runes); {
		_, highlighted := hits[start]
		end := start + 1
		for end < len(runes) {
			_, nextHighlighted := hits[end]
			if nextHighlighted != highlighted {
				break
			}
			end++
		}
		span := ui.Span{Text: string(runes[start:end])}
		if highlighted {
			span.Color = t.Accent
			span.Background = ui.RGBA(t.Accent.R, t.Accent.G, t.Accent.B, 0.18)
			span.Radius = 2
		}
		spans = append(spans, span)
		start = end
	}
	return ui.RichText(c, spans...).FontSize(size).TextColor(color).SingleLine().Ellipsis("…")
}

func (a *App) searchView(c *ui.Context) {
	a.documentSearchView(c)
}

func (a *App) openWorkspaceSearch() {
	a.quickOpen = false
	a.clearQuickOpenCatalog()
	a.cmdPalette = false
	if a.root == "" && a.path != "" {
		a.root = filepath.Dir(a.path)
	}
	a.enableDocumentServices()
	a.workspaceSearch = true
	a.workspaceQuery = ""
	a.workspaceSearchSel = 0
	a.workspaceReveal = true
	a.workspaceSubmit = false
	a.invalidate()
}

func (a *App) closeWorkspaceSearch() {
	a.workspaceSearch = false
	a.workspaceSubmit = false
	if a.vault != nil {
		a.vault.queryToken++
		if a.vault.queryCancel != nil {
			a.vault.queryCancel()
			a.vault.queryCancel = nil
		}
	}
	a.invalidate()
}

func (a *App) workspaceRecentNotes() []recentEntry {
	var out []recentEntry
	root := a.root
	for _, entry := range a.recents {
		if entry.IsDir || filepath.Ext(entry.Path) != ".md" {
			continue
		}
		if root != "" {
			rel, err := filepath.Rel(root, entry.Path)
			if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
				continue
			}
		}
		if _, err := os.Stat(entry.Path); err == nil {
			out = append(out, entry)
			if len(out) == recentShow {
				break
			}
		}
	}
	return out
}

func (a *App) openWorkspaceRecent(entry recentEntry) {
	a.closeWorkspaceSearch()
	a.requestOpen(entry.Path)
}

func (a *App) openSelectedWorkspaceHit() {
	if a.vault == nil || a.workspaceSearchSel < 0 || a.workspaceSearchSel >= len(a.vault.hits) {
		return
	}
	hit := a.vault.hits[a.workspaceSearchSel]
	a.closeWorkspaceSearch()
	a.navigateSearchHit(hit)
}

func (a *App) openSelectedWorkspaceItem() {
	if strings.TrimSpace(a.workspaceQuery) != "" {
		a.openSelectedWorkspaceHit()
		return
	}
	recent := a.workspaceRecentNotes()
	if a.workspaceSearchSel >= 0 && a.workspaceSearchSel < len(recent) {
		a.openWorkspaceRecent(recent[a.workspaceSearchSel])
	}
}

func (a *App) submitWorkspaceSearch() {
	if !a.workspaceSearch || a.workspaceSubmit {
		return
	}
	a.workspaceSubmit = true
	a.openSelectedWorkspaceItem()
}

func (a *App) workspaceSearchDialog(c *ui.Context) {
	if !a.workspaceSearch {
		return
	}
	if c.Shortcut(0, ui.KeyEscape) {
		a.closeWorkspaceSearch()
		return
	}
	if c.Shortcut(0, ui.KeyEnter) {
		a.submitWorkspaceSearch()
		return
	}
	a.syncWorkspaceSearchQuery()
	v := a.vault
	querying := strings.TrimSpace(a.workspaceQuery) != ""
	count := 0
	if querying && v != nil {
		count = len(v.hits)
	} else if !querying {
		count = len(a.workspaceRecentNotes())
	}
	if c.Shortcut(0, ui.KeyUp) && count > 0 {
		a.workspaceSearchSel = (a.workspaceSearchSel - 1 + count) % count
		a.workspaceReveal = true
	}
	if c.Shortcut(0, ui.KeyDown) && count > 0 {
		a.workspaceSearchSel = (a.workspaceSearchSel + 1) % count
		a.workspaceReveal = true
	}
	if a.workspaceSearchSel >= count {
		a.workspaceSearchSel = max(0, count-1)
	}
	t := c.Theme()
	ui.DialogBase(c, &a.workspaceSearch, func(backdrop, panel ui.Element) {
		backdrop.Background(ui.RGBA(0, 0, 0, 0.30))
		panel.Width(760).Height(620).MaxWidthPercent(94).MaxHeightPercent(86).Padding(0).Gap(0).Radius(14).
			Background(t.Background).Border(1, t.Border).
			Shadow(0, 18, 44, -8, ui.RGBA(0, 0, 0, 0.32)).Clip().Children(func() {
			ui.Row(c).Height(58).Padding(0, 18).Gap(12).AlignItems(ui.Center).Children(func() {
				ui.Icon(c, iconSearch).Size(18, 18).TextColor(t.TextMuted)
				ui.TextInputBase(c, &a.workspaceQuery).Ref(&a.workspaceInputFocus).Grow(1).MinWidth(0).
					Placeholder(locales.WorkspaceSearchPlaceholder()).Label(locales.MenuSearchWorkspace()).AutoFocus().
					OnSubmit(func() { a.submitWorkspaceSearch() }).
					OnTextKey(func(k ui.TextKey) bool {
						if k.Mods != ui.Ctrl || (k.Key != ui.KeyN && k.Key != ui.KeyP) || count == 0 {
							return false
						}
						step := 1
						if k.Key == ui.KeyP {
							step = -1
						}
						a.workspaceSearchSel = (a.workspaceSearchSel + step + count) % count
						a.workspaceReveal = true
						return true
					})
				fullLabel, literalLabel := locales.WorkspaceSearchFullText(), locales.WorkspaceSearchLiteral()
				if v == nil || !v.searchLiteral {
					fullLabel = locales.WorkspaceSearchFullTextSelected()
				} else {
					literalLabel = locales.WorkspaceSearchLiteralSelected()
				}
				if ui.Button(c, fullLabel).Clicked() && v != nil && v.searchLiteral {
					v.searchLiteral = false
					v.queryToken++
					v.hits = nil
					a.workspaceReveal = true
					v.due = time.Now()
					a.workspaceInputFocus.Focus(c)
				}
				if ui.Button(c, literalLabel).Clicked() && v != nil && !v.searchLiteral {
					v.searchLiteral = true
					v.queryToken++
					v.hits = nil
					a.workspaceReveal = true
					v.due = time.Now()
					a.workspaceInputFocus.Focus(c)
				}
			})
			ui.Box(c).Height(1).FillWidth().Background(t.Border)
			if !querying {
				showSearchScroll := a.workspaceIndicator.Visible(c.Now(), a.workspaceScroll.X, a.workspaceScroll.Y)
				if showSearchScroll {
					c.After(a.workspaceIndicator.Remaining(c.Now()))
				}
				quietScroll(c, &a.workspaceScroll, &a.workspaceIndicator).FillWidth().Grow(1).MinHeight(100).Padding(10, 12).Scrollbars(showSearchScroll).Children(func() {
					recent := a.workspaceRecentNotes()
					if len(recent) == 0 {
						ui.Text(c, locales.WorkspaceRecentEmpty()).FontSize(12).TextColor(t.TextMuted)
					}
					for i, entry := range recent {
						e := entry
						a.workspaceSearchRow(c, t, i == a.workspaceSearchSel, filepath.Base(e.Path), a.workspacePathLabel(e.Path), "", func() { a.openWorkspaceRecent(e) })
					}
				})
			} else if a.root == "" {
				ui.Text(c, locales.WorkspaceSearchHint()).FontSize(12).TextColor(t.TextMuted)
			} else {
				a.workspaceSearchView(c)
			}
			ui.Box(c).Height(1).FillWidth().Background(t.Border)
			ui.Row(c).Height(34).Padding(0, 16).AlignItems(ui.Center).Children(func() {
				if querying && v != nil && v.search != nil {
					if v.searchLiteral {
						ui.Text(c, locales.SearchMatchCount(len(v.hits))).FontSize(10.5).TextColor(t.TextMuted)
					} else {
						ui.Text(c, locales.WorkspaceNoteCount(len(v.hits))).FontSize(10.5).TextColor(t.TextMuted)
					}
				} else if !querying {
					ui.Text(c, locales.WorkspaceRecentNotes()).FontSize(10.5).TextColor(t.TextMuted)
				}
				ui.Spacer(c)
				ui.Text(c, locales.WorkspaceQuickKeys()).FontSize(10.5).TextColor(t.TextMuted)
			})
		})
	})
}

func (a *App) workspaceSearchRow(c *ui.Context, t *ui.Theme, selected bool, title, path, snippet string, open func()) {
	row := ui.Row(c).FillWidth().Height(64).Shrink(0).Padding(9, 11).Gap(10).Radius(8).AlignItems(ui.Center).Cursor(ui.CursorPointer).Label(locales.WorkspaceOpenNote(title))
	if selected {
		row.Background(t.SurfaceHover)
	} else if row.Hovered() {
		row.Background(t.SurfaceHover)
	}
	row.Children(func() {
		ui.Icon(c, iconFile).Size(16, 16).TextColor(iconTint(t))
		ui.Column(c).FillWidth().AlignItems(ui.Start).Gap(3).Children(func() {
			ui.Text(c, title).FontSize(13).FontWeight(620).TextColor(t.Text).SingleLine().Ellipsis("…")
			ui.Text(c, path).FontSize(10.5).TextColor(t.TextMuted).SingleLine().Ellipsis("…")
			if snippet != "" {
				ui.Text(c, strings.Join(strings.Fields(snippet), " ")).FontSize(11.5).TextColor(t.TextMuted).SingleLine().Ellipsis("…")
			}
		})
	})
	if selected && a.workspaceReveal {
		row.ScrollIntoView()
		a.workspaceReveal = false
	}
	row.OnClick(open)
}
func (a *App) documentSearchView(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).Fill().Padding(10).Gap(8).Children(func() {
		ui.TextInput(c, &a.query).Placeholder(locales.DocumentFindPlaceholder()).Label(locales.DocumentFind())
		a.refreshSearch()
		if a.query == "" {
			ui.Text(c, locales.DocumentFindHint()).FontSize(11.5).TextColor(t.TextMuted).Wrap()
			return
		}
		if a.matches == 0 {
			ui.Text(c, locales.SearchNoMatches()).FontSize(11.5).TextColor(t.TextMuted)
			return
		}
		ui.Row(c).FillWidth().Gap(6).AlignItems(ui.Center).Children(func() {
			ui.TextInput(c, &a.replaceWith).Placeholder(locales.DocumentReplace()).Label(locales.DocumentReplace()).Grow(1).MinWidth(0)
			if ui.Button(c, locales.DocumentReplaceOne()).Clicked() {
				a.replaceHit()
			}
			if ui.Button(c, locales.CommonAll()).Tooltip(locales.DocumentReplaceAll()).Clicked() {
				a.replaceAllHits()
			}
		})
		ui.Text(c, locales.SearchMatchCount(a.matches)).FontSize(11).TextColor(t.TextMuted)
		showScrollbar := a.searchIndicator.Visible(c.Now(), a.searchScroll.X, a.searchScroll.Y)
		if showScrollbar {
			c.After(a.searchIndicator.Remaining(c.Now()))
		}
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
		}).Grow(1).Gap(1).TrackScroll(&a.searchScroll).Scrollbars(showScrollbar)
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
	a.ed.NavigateToRange(h.start, h.end)
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
	a.notice(locales.CommonReplaceSuccess(n))
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
	if a.imageDocument.open {
		a.imageDocumentView(c)
		return
	}
	if a.root != "" && a.path == "" && a.source == "" {
		a.vaultHome(c)
		return
	}
	ui.Column(c).Fill().Children(func() {
		opts := a.editorOptions()
		if !a.flat {
			a.editorHeader(c)
			a.ed.SetOptions(opts)
			ed := a.ed.View(c).Grow(1)
			if a.mouseCursorHidden {
				ed.Cursor(ui.CursorNone)
			}
			return
		}
		opts.TopInset = a.headerH(c)
		a.ed.SetOptions(opts)
		ed := a.ed.View(c).Grow(1)
		if a.mouseCursorHidden {
			ed.Cursor(ui.CursorNone)
		}
		a.headerVeil(c)
		a.editorHeader(c)
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
			a.smallIcon(c, iconChevronLeft, locales.ToolbarBack(), func() { a.nav(-1) })
		} else {
			a.smallIconTint(c, iconChevronLeft, locales.ToolbarBack(), nil, tint.Alpha(0.35))
		}
		if a.canNav(1) {
			a.smallIcon(c, iconChevron, locales.ToolbarForward(), func() { a.nav(1) })
		} else {
			a.smallIconTint(c, iconChevron, locales.ToolbarForward(), nil, tint.Alpha(0.35))
		}
		// The path sits in the middle of what is left, its end kept: the file
		// name is the part that says which document this is.
		ui.Row(c).Grow(1).MinWidth(0).Justify(ui.Center).Padding(8, 0).Children(func() {
			vw, _ := c.Size()
			avail := vw - railWidth - a.panelW - 220
			ui.Text(c, a.tailPath(c, a.docPath(), 12.5, avail)).
				FontSize(12.5).TextColor(t.TextMuted).SingleLine()
		})
		if a.imageDocument.open {
			a.smallIcon(c, iconImage, locales.ToolbarGlobalPreview(), func() { a.openImagePreview(a.imageDocument.path) })
		}
		if label := a.ed.ModeLabel(); label != "" {
			ui.Text(c, label).FontSize(11).TextColor(t.TextMuted).Label(locales.DocumentMode())
		}
		if a.status != "" {
			ui.Text(c, a.status).FontSize(11).TextColor(t.TextMuted).SingleLine().Ellipsis("…")
		}
		if a.dirty && !a.imageDocument.open {
			a.smallIcon(c, iconSave, locales.MenuSave(), func() { a.save() })
		}
		if a.propertiesEnabled() {
			a.smallIconTint(c, iconInfo, locales.DocumentInfo(), func() { a.openProperties() }, tint)
		} else {
			info := a.smallIconTint(c, iconInfo, locales.DocumentInfo(), nil, tint)
			info.Menu(func(m *ui.Menu) {
				chars, _, lines := countText(a.source)
				m.Item(locales.DocumentPathValue(a.docPath()))
				m.Item(locales.DocumentCharacterLineCount(chars, lines))
				if st, err := os.Stat(a.path); err == nil {
					m.Item(locales.DocumentModifiedAt(st.ModTime()))
				}
				m.Separator()
				if m.Item(locales.FilesInFinder()).Chosen() && a.path != "" {
					mygo.Shell.ShowItemInFolder(a.path)
				}
			})
		}
		more := a.smallIconTint(c, iconMore, locales.DocumentMore(), nil, tint)
		more.Menu(func(m *ui.Menu) {
			if !a.imageDocument.open && codeLanguage(a.path) == "" {
				m.Submenu(locales.DocumentExport(), func(export *ui.Menu) {
					if export.Item(locales.DocumentExportMarkdown()).Chosen() {
						a.exportDocument(noteExportMarkdown)
					}
					if export.Item(locales.DocumentExportHTML()).Chosen() {
						a.exportDocument(noteExportHTML)
					}
					if export.Item(locales.DocumentExportPDF()).Chosen() {
						a.exportDocument(noteExportPDF)
					}
				})
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
			if m.Item(locales.DocumentCopyPathMenu()).Chosen() && a.path != "" {
				clipboardWrite(a.path)
				a.notice(locales.DocumentCopiedPath())
			}
			if m.Item(locales.FilesSetWorkspace()).Chosen() && a.path != "" {
				a.setWorkspace(filepath.Dir(a.path))
			}
		})
	})
}

// ---------------------------------------------------------------------------
// Files

func (a *App) setWorkspace(root string) {
	// A workspace is the start of a new editing context. Do not leave the
	// previous vault's document visible after switching folders.
	if filepath.Clean(root) != filepath.Clean(a.root) {
		a.clearQuickOpenCatalog()
		a.resetDoc()
	}
	a.root = root
	a.fileRowsDirty = true
	a.dirMu.Lock()
	a.dirs = map[string][]string{}
	a.isDir = map[string]bool{}
	a.dirMu.Unlock()
	a.expanded = map[string]bool{}
	a.folderToggled = map[string]time.Time{}
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
		if isVisibleWorkspaceFile(path) {
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

func (a *App) snapshotDoc() (dirty bool, name, path, source, revision string) {
	a.onMain(func() {
		dirty, name, path, source = a.dirty, a.displayName(), a.path, a.diskSource()
		if a.documents != nil {
			revision = a.documents.revision
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
	a.startBusy()
	go func() {
		paths, err := openDialog(mygo.OpenDialogOptions{
			Parent: a.win,
			Title:  locales.CommonDialogFileTitle(),
			Filters: []mygo.FileFilter{
				{Name: locales.CommonDialogOpenFileFilter(), Extensions: append(textFileFilter(), imageFileExtensions()...)},
			},
		})
		if err != nil || len(paths) == 0 {
			a.finish(nil)
			return
		}
		if isImageFile(paths[0]) {
			a.finish(func() { a.openImageDocument(paths[0], true) })
			return
		}
		if !a.confirmDiscardOffUI() {
			a.finish(nil)
			return
		}
		data, err := os.ReadFile(paths[0])
		if err != nil {
			errorDialog(locales.DialogOpenFailed(), err.Error())
			a.finish(nil)
			return
		}
		if !isTextData(data) {
			errorDialog(locales.DialogCannotOpen(), locales.DialogNotTextFile())
			a.finish(nil)
			return
		}
		a.finish(func() { a.applyOpen(paths[0], string(data)) })
	}()
}

func (a *App) openPathNow(path string) {
	if isImageFile(path) {
		a.openImageDocument(path, true)
		return
	}
	if !isEditableFile(path) {
		a.notice(locales.FilesUnsupportedType(filepath.Base(path)))
		a.invalidate()
		return
	}
	data, err := os.ReadFile(path)
	if err != nil {
		a.notice(locales.CommonOpenFileFailed(err.Error()))
		a.invalidate()
		return
	}
	if !isTextData(data) {
		a.notice(locales.DocumentNotTextWithName(filepath.Base(path)))
		a.invalidate()
		return
	}
	a.applyOpen(path, string(data))
	a.invalidate()
}

// requestOpen opens a path from the UI. A clean document opens at once; an
// edited one asks first, with the dialog off the UI thread.
func (a *App) requestOpen(path string) {
	if isImageFile(path) {
		a.openImageDocument(path, true)
		return
	}
	if !isEditableFile(path) {
		a.notice(locales.FilesUnsupportedType(filepath.Base(path)))
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
			a.finish(func() { a.indexedPath = "" })
			return
		}
		data, err := os.ReadFile(path)
		if err != nil {
			errorDialog(locales.DialogOpenFailed(), err.Error())
			a.finish(nil)
			return
		}
		a.finish(func() { a.applyOpen(path, string(data)) })
	}()
}

func (a *App) applyOpen(path, data string) {
	if a.indexedPath == path && a.indexedRevision != "" && notestore.Revision(data) != a.indexedRevision && a.vault != nil {
		rel, _ := filepath.Rel(a.vault.root, path)
		hit := vaultsearch.Hit{Path: filepath.ToSlash(rel), Revision: a.indexedRevision}
		a.indexedPath, a.indexedRevision = "", ""
		a.notice(locales.WorkspaceSearchMoved())
		a.navigateSearchHit(hit)
		return
	}
	a.pushNav(path)
	a.imageDocument = imageDocumentState{}
	a.path = path
	a.selectedPath = path
	if a.vault != nil && a.vault.watch != nil {
		_ = a.vault.watch.WatchFile(path)
	}
	a.loadDoc(data)
	a.dirty = false
	a.updateTitle()
	a.addRecent(path, false)
	a.ed.Focus()
	if a.indexedPath == path {
		a.ed.ScrollToOffset(editorOffset(data, a.indexedOffset))
		a.indexedPath = ""
		a.indexedRevision = ""
	}
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
	a.imageDocument = imageDocumentState{}
	a.path = ""
	a.selectedPath = ""
	a.loadDoc("")
	a.dirty = false
	a.updateTitle()
	a.ed.Focus()
}

func (a *App) save() {
	a.flushManagedTimestamp()
	if a.path == "" {
		a.saveAs()
		return
	}
	if !isEditableFile(a.path) {
		a.notice(locales.FilesUnsupportedType(filepath.Base(a.path)))
		a.invalidate()
		return
	}
	revision := ""
	if a.documents != nil {
		revision = a.documents.revision
	}
	if err := a.writeDocument(a.path, a.diskSource(), revision); err != nil {
		if a.documents != nil && errors.Is(err, notestore.ErrConflict) {
			a.documents.conflict = a.path
			a.documents.conflictOpen = true
		}
		a.notice(locales.CommonSaveError(err.Error()))
	} else {
		a.dirty = false
		if a.documents != nil {
			a.documents.revision = notestore.Revision(a.diskSource())
			a.documents.baseSource = a.diskSource()
			a.documents.due = time.Time{}
			a.documents.updatedAtDue = time.Time{}
			_ = a.documents.store.DeleteDraft(a.path)
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
	dirty, name, path, source, revision := a.snapshotDoc()
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
		if defaultName == "" || defaultName == locales.DocumentUntitled() {
			defaultName = "Untitled.md"
		} else if !strings.HasSuffix(strings.ToLower(defaultName), ".md") {
			defaultName += ".md"
		}
		savePath, err = saveDialog(mygo.SaveDialogOptions{
			Parent:      a.win,
			Title:       locales.CommonSaveFile(),
			DefaultPath: defaultName,
			Filters:     []mygo.FileFilter{{Name: locales.CommonTextFiles(), Extensions: textFileFilter()}},
		})
		if err != nil || savePath == "" {
			return false
		}
	}
	if !isEditableFile(savePath) {
		errorDialog(locales.DialogCannotSave(), locales.DialogChooseTextExtension())
		return false
	}
	expected, err := a.destinationRevision(savePath, path, revision)
	if err == nil {
		err = a.writeDocument(savePath, source, expected)
	}
	if err != nil {
		errorDialog(locales.DialogSaveFailed(), err.Error())
		return false
	}
	a.onMain(func() {
		a.path = savePath
		a.selectedPath = savePath
		if a.documents != nil {
			a.documents.revision = notestore.Revision(source)
			a.documents.baseSource = source
		}
		a.dirty = a.diskSource() != source
		a.updateTitle()
	})
	return true
}

func (a *App) saveAs() {
	if a.busy {
		return
	}
	a.flushManagedTimestamp()
	var name, source, originalPath, revision string
	a.onMain(func() {
		name = locales.DocumentUntitled() + ".md"
		if a.path != "" {
			name = filepath.Base(a.path)
		}
		source = a.diskSource()
		originalPath = a.path
		if a.documents != nil {
			revision = a.documents.revision
		}
	})
	a.startBusy()
	go func() {
		path, err := saveDialog(mygo.SaveDialogOptions{
			Parent:      a.win,
			Title:       locales.CommonSaveFile(),
			DefaultPath: name,
			Filters:     []mygo.FileFilter{{Name: locales.CommonTextFiles(), Extensions: textFileFilter()}},
		})
		if err != nil || path == "" {
			a.finish(nil)
			return
		}
		if !isEditableFile(path) {
			errorDialog(locales.DialogCannotSave(), locales.DialogChooseTextExtension())
			a.finish(nil)
			return
		}
		createdAt := ""
		if originalPath == "" && codeLanguage(path) == "" {
			source, createdAt, err = ensureCreatedAt(source, time.Now())
			if err != nil {
				errorDialog(locales.DialogSaveFailed(), err.Error())
				a.finish(nil)
				return
			}
		}
		expected, err := a.destinationRevision(path, originalPath, revision)
		if err == nil {
			err = a.writeDocument(path, source, expected)
		}
		if err != nil {
			errorDialog(locales.DialogSaveFailed(), err.Error())
			a.finish(nil)
			return
		}
		a.finish(func() {
			a.path = path
			a.selectedPath = path
			if a.documents != nil {
				a.documents.revision = notestore.Revision(source)
				a.documents.baseSource = source
				a.documents.due = time.Time{}
				a.documents.updatedAtDue = time.Time{}
			}
			if createdAt != "" && codeLanguage(path) == "" {
				createdTime, _ := time.Parse(time.RFC3339Nano, createdAt)
				current, _, metadataErr := ensureCreatedAt(a.ed.Source(), createdTime)
				if metadataErr != nil {
					a.notice(locales.CommonExternalUpdateTimestampFailed(metadataErr.Error()))
				} else {
					if a.documents != nil {
						a.documents.applyingMetadata = true
					}
					a.ed.ReplaceSystemAll(current)
					if a.documents != nil {
						a.documents.applyingMetadata = false
					}
				}
			}
			a.dirty = a.diskSource() != source
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
	a.fileRowsDirty = true
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
	path := uniquePath(dir, locales.DocumentUntitled(), ".md")
	source, err := frontmatter.EnsureCore("")
	if err == nil {
		source, err = frontmatter.SetTyped(source, "created_at", time.Now().UTC().Format(time.RFC3339Nano), "text")
	}
	if err != nil {
		a.notice(locales.CommonCreateFailed(err.Error()))
		a.invalidate()
		return
	}
	if err := os.WriteFile(path, []byte(source), 0o644); err != nil {
		a.notice(locales.CommonCreateFailed(err.Error()))
		a.invalidate()
		return
	}
	a.clearTreeCache()
	a.requestOpen(path)
}

func (a *App) newFolderIn(dir string) {
	path := uniquePath(dir, locales.FilesNewFolderDefault(), "")
	if err := os.Mkdir(path, 0o755); err != nil {
		a.notice(locales.CommonCreateFailed(err.Error()))
	} else {
		a.clearTreeCache()
	}
	a.invalidate()
}

func (a *App) clearTreeCache() {
	a.fileRowsCache = nil
	a.fileRowsCacheRoot = ""
	a.fileRowsDirty = true
	a.dirMu.Lock()
	a.dirs = map[string][]string{}
	a.isDir = map[string]bool{}
	a.dirMu.Unlock()
}

// fileMenu is the file tree's context menu: directory actions, path copying
// (relative or absolute), rename, trash and reveal.
func (a *App) fileMenu(c *ui.Context, row ui.Element, path string) {
	row.ContextMenu(func(m *ui.Menu) {
		if a.treeSelection == nil {
			a.treeSelection = make(map[string]bool)
		}
		if !a.treeSelection[path] {
			clear(a.treeSelection)
			a.treeSelection[path] = true
			a.treeSelectionAnchor = path
		}
		if m.Item(locales.FilesCollapseMenu()).Chosen() {
			a.collapseAll()
		}
		m.Separator()
		a.dirMu.Lock()
		isDir := a.isDir[path]
		a.dirMu.Unlock()
		if isDir {
			if m.Item(locales.FilesNewFile()).Chosen() {
				a.dispatchTreeAction(path, ActionCreateFile, nil)
			}
			if m.Item(locales.FilesNewFolderMenu()).Chosen() {
				a.dispatchTreeAction(path, ActionCreateFolder, nil)
			}
			m.Separator()
		}
		if m.Item(locales.FilesCopyItem()).Chosen() {
			a.dispatchTreeAction(path, ActionCopy, nil)
		}
		if m.Item(locales.FilesCutItem()).Chosen() {
			a.dispatchTreeAction(path, ActionCut, nil)
		}
		if len(a.treeClipboard) > 0 {
			if m.Item(locales.FilesPasteItem()).Chosen() {
				a.dispatchTreeAction(path, ActionPaste, nil)
			}
		}
		m.Separator()
		if m.Item(locales.FilesChangeIcon()).Chosen() {
			a.openNavIconPicker(path)
		}
		if m.Item(locales.FilesCopyRelativePath()).Chosen() {
			a.dispatchTreeAction(path, ActionCopyPath, map[string]any{"relative": true})
		}
		if m.Item(locales.FilesCopyAbsolutePath()).Chosen() {
			a.dispatchTreeAction(path, ActionCopyPath, map[string]any{"relative": false})
		}
		m.Separator()
		if m.Item(locales.FilesRenameEllipsis()).Chosen() {
			a.dispatchTreeAction(path, ActionRename, nil)
		}
		if m.Item(locales.FilesMoveToTrash()).Chosen() {
			a.dispatchTreeAction(path, ActionTrash, nil)
		}
		m.Separator()
		if m.Item(locales.FilesRevealInFinder()).Chosen() {
			a.dispatchTreeAction(path, ActionRevealInFinder, nil)
		}
	})
}

// fileRootMenu serves context clicks on the tree panel's empty area. It only
// exposes actions whose destination is the vault root; destructive item
// actions remain available only from a concrete row menu.
func (a *App) fileRootMenu(list ui.Element) {
	list.ContextMenu(func(m *ui.Menu) {
		if m.Item(locales.FilesNewFile()).Chosen() {
			a.dispatchTreeAction(a.root, ActionCreateFile, nil)
		}
		if m.Item(locales.FilesNewFolderMenu()).Chosen() {
			a.dispatchTreeAction(a.root, ActionCreateFolder, nil)
		}
		if len(a.treeClipboard) > 0 && m.Item(locales.FilesPasteItem()).Chosen() {
			a.dispatchTreeAction(a.root, ActionPaste, nil)
		}
		m.Separator()
		if m.Item(locales.FilesCollapseMenu()).Chosen() {
			a.collapseAll()
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
		a.notice(locales.DocumentCopiedRelativePath())
	} else {
		a.notice(locales.DocumentCopiedAbsolutePath())
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
			Message:       locales.CommonTrashConfirm(name),
			Detail:        locales.CommonTrashRecoverable(),
			Buttons:       []string{locales.CommonTrashButton(), locales.CommonCancel()},
			DefaultButton: 0,
			CancelButton:  1,
		})
		if err != nil || res.Button != 0 {
			a.finish(nil)
			return
		}
		if err := trashItem(path); err != nil {
			errorDialog(locales.DialogDeleteFailed(), err.Error())
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
			a.notice(locales.FilesMovedToTrash())
		})
	}()
}

func (a *App) trashTreeItems(paths []string) {
	if len(paths) == 0 {
		return
	}
	if len(paths) == 1 {
		a.trash(paths[0])
		return
	}
	if a.busy {
		return
	}
	a.startBusy()
	go func() {
		res, err := messageDialog(mygo.MessageOptions{
			Parent:        a.win,
			Type:          mygo.MessageWarning,
			Message:       locales.CommonTrashManyConfirm(len(paths)),
			Detail:        locales.CommonTrashRecoverable(),
			Buttons:       []string{locales.CommonTrashButton(), locales.CommonCancel()},
			DefaultButton: 0,
			CancelButton:  1,
		})
		if err != nil || res.Button != 0 {
			a.finish(nil)
			return
		}
		for _, path := range paths {
			if err := trashItem(path); err != nil {
				errorDialog(locales.DialogDeleteFailed(), err.Error())
				a.finish(nil)
				return
			}
		}
		a.finish(func() {
			for _, path := range paths {
				delete(a.treeSelection, path)
				if a.path == path || strings.HasPrefix(a.path, path+string(filepath.Separator)) {
					a.path = ""
					a.selectedPath = ""
					a.loadDoc("")
					a.dirty = false
					a.updateTitle()
				}
			}
			a.clearTreeCache()
			a.notice(locales.FilesMovedToTrash())
		})
	}()
}

func (a *App) renameDialog(c *ui.Context) {
	t := c.Theme()
	ui.Modal(c, &a.renameOpen, func() {
		ui.Column(c).Width(420).Gap(12).Padding(18).Radius(12).
			Background(t.Background).Border(1, t.Border).
			Shadow(0, 18, 50, 0, ui.RGBA(0, 0, 0, 0.22)).Children(func() {
			ui.Text(c, locales.FilesRenameTitle()).FontSize(14).FontWeight(600)
			input := ui.TextInput(c, &a.renameValue).AutoFocus().Label(locales.FilesNewName())
			ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
				if ui.Button(c, locales.CommonCancel()).Clicked() {
					a.renameOpen = false
				}
				if ui.PrimaryButton(c, locales.CommonRenameButton()).Clicked() || input.Submitted() {
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
		a.notice(locales.CommonRenameFailed(err.Error()))
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
	a.notice(locales.FilesRenamed())
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
	showScrollbar := a.settingsIndicator.Visible(c.Now(), a.settingsScroll.X, a.settingsScroll.Y)
	if showScrollbar {
		c.After(a.settingsIndicator.Remaining(c.Now()))
	}
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
					ui.Text(c, locales.SettingsTitle()).FontSize(14).FontWeight(600)
					ui.Text(c, locales.SettingsImmediate()).FontSize(11.5).TextColor(t.TextMuted)
					ui.Spacer(c)
					a.smallIcon(c, iconClose, locales.SettingsClose(), func() { a.settingsOpen = false })
				})
				ui.Row(c).Grow(1).MinHeight(0).AlignItems(ui.Stretch).Children(func() {
					// Left navigation, 184px.
					ui.Column(c).Width(184).Shrink(0).FillHeight().Padding(0, 8, 8, 4).Gap(2).Children(func() {
						a.settingsNavItem(c, locales.SettingsAppearance(), 0)
						a.settingsNavItem(c, locales.SettingsEditor(), 1)
						a.settingsNavItem(c, locales.SettingsAI(), 2)
					})
					// Right content card, with its own 44px page header.
					ui.Column(c).Grow(1).MinWidth(0).FillHeight().Padding(0, 8, 8, 8).Children(func() {
						card := ui.Column(c).FillWidth().FillHeight().Radius(14).Clip().
							Background(t.Background).Border(1, t.Border)
						card.Children(func() {
							title := locales.SettingsAppearance()
							switch a.settingsTab {
							case 1:
								title = locales.SettingsEditor()
							case 2:
								title = locales.SettingsAI()
							}
							ui.Row(c).Height(44).Shrink(0).Padding(0, 18).AlignItems(ui.Center).
								BorderWidth(0, 0, 1, 0).BorderColor(t.Border).Children(func() {
								ui.Text(c, title).FontSize(15).FontWeight(650)
							})
							ui.Scroll(c).Fill().TrackScroll(&a.settingsScroll).Scrollbars(showScrollbar).Children(func() {
								ui.Column(c).FillWidth().Padding(18).Gap(18).Children(func() {
									switch a.settingsTab {
									case 0:
										a.appearanceSettings(c)
									case 1:
										a.editorSettings(c)
									default:
										a.aiSettings(c)
									}
								})
							})
						})
					})
				})
				footer := locales.SettingsFooter()
				if a.settingsTab == 2 {
					footer = locales.SettingsAIFooter()
				} else {
					footer = locales.SettingsFooter()
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

// settingsRowToggle is settingsRow for a switch: its label turns it too, so
// the left of the row is as much the switch as the switch is.
func settingsRowToggle(c *ui.Context, label, detail string, on *bool) {
	settingsRowToggleAction(c, label, detail, on, func() { *on = !*on })
}

func settingsRowToggleAction(c *ui.Context, label, detail string, on *bool, toggle func()) {
	t := c.Theme()
	ui.Row(c).FillWidth().Padding(11, 0).Gap(16).AlignItems(ui.Center).Children(func() {
		ui.Column(c).Grow(1).MinWidth(0).Gap(3).OnClick(toggle).Children(func() {
			ui.Text(c, label).FontSize(13.5).TextColor(t.Text)
			if detail != "" {
				ui.Text(c, detail).FontSize(12).TextColor(t.TextMuted).Wrap()
			}
		})
		checked := *on
		control := ui.ButtonBase(c).Width(38).Height(22).Radius(11).Padding(2).
			Label(locales.SettingsToggleStateLabel(label)).Tooltip(label).AlignItems(ui.Center)
		if checked {
			control.Background(t.Accent)
		} else {
			control.Background(t.Border)
		}
		control.OnClick(toggle).Children(func() {
			if checked {
				ui.Row(c).Fill().Justify(ui.End).Children(func() {
					ui.Box(c).Size(18, 18).Radius(9).Background(t.Background)
				})
			} else {
				ui.Row(c).Fill().Justify(ui.Start).Children(func() { ui.Box(c).Size(18, 18).Radius(9).Background(t.Background) })
			}
		})
	})
}

func (a *App) appearanceSettings(c *ui.Context) {
	t := c.Theme()
	settingsGroup(c, locales.Language(), func() {
		settingsRow(c, locales.SettingsLanguage(), locales.SettingsLanguageDetail(), func() {
			choices := []struct{ tag, label string }{
				{"", locales.LanguageSystem()},
				{"zh-Hans", locales.LanguageChinese()},
				{"en", locales.LanguageEnglish()},
			}
			for _, choice := range choices {
				choice := choice
				b := ui.ButtonBase(c).Height(28).Padding(0, 9).Radius(6).Label(choice.label)
				selected := (choice.tag == "" && (!a.localeConfigured || a.localeSetting == "")) ||
					(choice.tag != "" && a.localeConfigured && a.localeSetting == choice.tag)
				if selected {
					b.Background(c.Theme().Background).Border(1, c.Theme().Border)
				} else if b.Hovered() {
					b.Background(c.Theme().SurfaceHover)
				}
				b.OnClick(func() { a.setLocale(choice.tag) }).Children(func() { ui.Text(c, choice.label) })
			}
		})
	})
	settingsGroup(c, locales.ThemeTitle(), func() {
		a.settingsDivider(c, false)
		settingsRow(c, locales.ThemeTitle(), locales.ThemeDescription(), func() {
			for mode, label := range []string{locales.ThemeLight(), locales.ThemeDark(), locales.ThemeSystem()} {
				mode, label := mode, label
				b := ui.ButtonBase(c).Height(28).Padding(0, 9).Radius(6).Label(locales.ThemeControl(label))
				if a.themeMode == mode {
					b.Background(c.Theme().Background).Border(1, c.Theme().Border)
				} else if b.Hovered() {
					b.Background(c.Theme().SurfaceHover)
				}
				b.OnClick(func() { a.setThemeMode(mode) }).Children(func() { ui.Text(c, label) })
			}
		})
		a.settingsDivider(c, true)
		a.accentSettings(c)
		a.settingsDivider(c, true)
		settingsRowToggle(c, locales.MenuHeaderFade(), locales.SettingsHeaderFadeDetail(), &a.headerFade)
	})
	settingsGroup(c, locales.SettingsNavigation(), func() {
		settingsRowToggleAction(c, locales.SettingsSmoothPageJumps(), locales.SettingsSmoothPageJumpsDetail(), &a.smoothPageJumps, func() {
			a.setSmoothPageJumps(!a.smoothPageJumps)
		})
	})
	a.typographySettings(c)
	ui.Text(c, locales.NativeRenderingNote()).
		FontSize(11.5).TextColor(t.TextMuted).Padding(0, 4)
}

func (a *App) typographySettings(c *ui.Context) {
	settingsGroup(c, locales.SettingsTypography(), func() {
		a.settingsDivider(c, false)
		settingsRowToggleAction(c, locales.SettingsSmartTypography(), locales.SettingsSmartTypographyDetail(), &a.smartTypography, func() { a.setSmartTypography(!a.smartTypography) })
		a.settingsDivider(c, true)
		settingsRow(c, locales.SettingsFontSize(), locales.SettingsFontSizeDetail(), func() {
			ui.Textf(c, "%.0fpx", a.fontSize).FontSize(12).TextColor(c.Theme().TextMuted).Width(38)
			ui.Slider(c, &a.fontSize, 12, 22).Width(190).Label(locales.SettingsFontSize())
		})
		a.settingsDivider(c, true)
		settingsRow(c, locales.SettingsLineHeight(), locales.SettingsLineHeightDetail(), func() {
			ui.Textf(c, "%.1f", a.lineHeight).FontSize(12).TextColor(c.Theme().TextMuted).Width(38)
			ui.Slider(c, &a.lineHeight, 1.4, 2.2).Width(190).Label(locales.SettingsLineHeight())
		})
	})
}

func (a *App) editorSettings(c *ui.Context) {
	settingsGroup(c, locales.SettingsKeymap(), func() { a.keymapSettings(c) })
	settingsGroup(c, locales.SettingsCursor(), func() {
		settingsRowToggleAction(c, locales.SettingsHideMouseCursor(), locales.SettingsHideMouseCursorDetail(), &a.hideMouseCursor, func() {
			a.setHideMouseCursor(!a.hideMouseCursor)
		})
		a.settingsDivider(c, true)
		settingsRowToggleAction(c, locales.SettingsCursorFade(), locales.SettingsCursorFadeDetail(), &a.cursorFade, func() { a.setCursorFade(!a.cursorFade) })
		if a.ed.Keymap() == "vim" || a.ed.Keymap() == "helix" {
			a.settingsDivider(c, true)
			settingsRowToggleAction(c, locales.SettingsSmoothCursor(), locales.SettingsSmoothCursorDetail(), &a.smoothCursor, func() { a.setSmoothCursor(!a.smoothCursor) })
		}
	})
	settingsGroup(c, locales.SettingsEditorGroup(), func() {
		settingsRowToggleAction(c, locales.SettingsHeadingMarkerReveal(), locales.SettingsHeadingMarkerRevealDetail(), &a.headingMarkerReveal, func() {
			a.setHeadingMarkerReveal(!a.headingMarkerReveal)
		})
	})
	if a.ed.Keymap() == "vim" {
		settingsGroup(c, "Vim", func() {
			settingsRowToggleAction(c, locales.SettingsVimImageRows(), locales.SettingsVimImageRowsDetail(), &a.vimVirtualImageRows, func() {
				a.setVimVirtualImageRows(!a.vimVirtualImageRows)
			})
			a.settingsDivider(c, true)
			settingsRowToggleAction(c, locales.SettingsVimYankFlash(), locales.SettingsVimYankFlashDetail(), &a.vimYankFlash, func() {
				a.setVimYankFlash(!a.vimYankFlash)
			})
		})
	}
	settingsGroup(c, locales.SettingsSearch(), func() {
		settingsRowToggleAction(c, locales.SettingsSearchMatchPulse(), locales.SettingsSearchMatchPulseDetail(), &a.searchMatchPulse, func() {
			a.setSearchMatchPulse(!a.searchMatchPulse)
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
		Header:                  a.propertyHeader(),
		HeaderHeight:            a.propertyHeaderHeight(),
		HideFrontmatter:         a.propertiesEnabled() && !a.propertySourceMode && a.propertyDocument().Err == nil,
		FontSize:                float32(a.fontSize),
		LineHeight:              float32(a.lineHeight),
		MaxWidth:                800,
		PadX:                    56,
		BaseDir:                 a.docDir(),
		Placeholder:             locales.DocumentStartWriting(),
		Label:                   locales.DocumentDefaultLabel(),
		Code:                    codeLanguage(a.path),
		Invalidate:              a.invalidate,
		OnChange:                a.docChanged,
		OnLink:                  a.openLink,
		OnWikiLinkHover: func(target string, x, y float32, bounds ui.Rect) {
			a.hoverWikiLinkAt(target, x, y, bounds)
		},
		WikiLinkSuggestions:       a.wikiLinkSuggestions,
		WikiLinkSuggestionDetails: a.wikiLinkSuggestionDetails,
		ResolveWikiEmbed:          a.resolveWikiEmbed,
		CurrentPath:               a.path,
		ResolveWikiLink:           a.resolveWikiLinkStyle,
		LinkRevision:              a.editorLinkRevision(),
		LinkContext:               a.editorLinkContext(),
		AssetRoot:                 a.root,
		ResolveImage:              a.resolveVaultImage,
		OnImagePreview:            a.openImagePreview,
		SaveImage:                 a.saveImage,
		OnDropFiles:               a.openDropped,
		OnVaultItemDragOver:       func() { a.setTreeDragMoveOperation(treeDragInsertIntoEditor, "") },
	}
}

func (a *App) editorLinkRevision() uint64 {
	if a.vault == nil || a.vault.index == nil {
		return 0
	}
	return a.vault.index.Revision()
}

func (a *App) editorLinkContext() string {
	root := ""
	if a.vault != nil {
		root = a.vault.root
	}
	return filepath.Clean(root) + "\x00" + filepath.Clean(a.path)
}

func (a *App) resolveWikiLinkStyle(currentPath, target string) mdedit.LinkTargetState {
	if a.vault == nil || a.vault.index == nil {
		return mdedit.LinkTargetUnknown
	}
	current := ""
	if currentPath == "" {
		currentPath = a.path
	}
	if currentPath != "" {
		if rel, err := filepath.Rel(a.vault.root, currentPath); err == nil {
			current = filepath.ToSlash(rel)
		}
	}
	if _, ok := a.vault.index.Resolve(current, target); ok {
		return mdedit.LinkTargetValid
	}
	if a.resolveWikiImageTargetFrom(currentPath, target) != "" {
		return mdedit.LinkTargetValid
	}
	return mdedit.LinkTargetInvalid
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
	a.linkPreview = wikiLinkPreviewState{}
	a.propertyOpenedAt = time.Now()
	a.propertyEditKey, a.propertyEditLocation = "", ""
	a.documentLoaded(src)
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
	a.documentChanged()
	a.queueLinkIndexSnapshot()
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
				a.ed.NavigateToOffset(h.offset, mdedit.PageJumpTopMargin)
				return
			}
		}
		return
	case strings.Contains(href, "://") || strings.HasPrefix(href, "mailto:"):
		go openExternal(href)
		return
	}
	if a.indexedLink(href) {
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
	a.linkPreview = wikiLinkPreviewState{}
	if imagePath := a.resolveWikiImageTarget(target); imagePath != "" {
		a.pendingWiki = ""
		a.openImagePreview(imagePath)
		return
	}
	// The vault index is the single authority for note path and anchor
	// resolution. In particular, do not fall back to a path-only resolver
	// after an anchor failed to resolve: that would open the note while
	// silently ignoring the requested heading or block.
	if a.vault == nil || a.vault.index == nil {
		a.pendingWiki = target
		return
	}
	a.pendingWiki = ""
	a.indexedLink(target)
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
	target := a.navAt + delta
	path := a.navHist[target]
	if isImageFile(path) {
		a.navAt = target
		a.openImageDocument(path, false)
		return
	}
	// Returning from an image to the still-open note must reveal its editor
	// state, including unsaved edits, rather than reload the on-disk version.
	if path == a.path {
		a.navAt = target
		a.imageDocument = imageDocumentState{}
		a.ed.Focus()
		a.updateTitle()
		a.invalidate()
		return
	}
	if a.dirty {
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
				a.navAt = target
				a.openPathNow(path)
			})
		}()
		return
	}
	a.navAt = target
	a.openPathNow(path)
}

// docPath is the document's path within its workspace. The workspace folder
// itself is the vault root and is not repeated in a vault-relative path.
func (a *App) docPath() string {
	if a.imageDocument.open {
		path := a.imageDocument.path
		if a.root != "" {
			if rel, err := filepath.Rel(a.root, path); err == nil && !strings.HasPrefix(rel, "..") {
				return filepath.ToSlash(rel)
			}
		}
		return path
	}
	if a.path == "" {
		return locales.DocumentUntitled()
	}
	if a.root == "" {
		return a.path
	}
	if rel, err := filepath.Rel(a.root, a.path); err == nil && !strings.HasPrefix(rel, "..") {
		return filepath.ToSlash(rel)
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
	if a.imageDocument.open {
		return filepath.Base(a.imageDocument.path)
	}
	if a.path == "" {
		return locales.DocumentUntitled()
	}
	return filepath.Base(a.path)
}

func (a *App) location() string {
	if a.path == "" {
		return locales.DocumentNotSaved()
	}
	if a.root != "" {
		if rel, err := filepath.Rel(a.root, a.path); err == nil && !strings.HasPrefix(rel, "..") {
			dir := filepath.Dir(rel)
			if dir == "." {
				return ""
			}
			return filepath.ToSlash(dir)
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
	if a.dirty {
		name = "● " + name
	}
	a.win.SetTitle(name + " — " + locales.AppTitle())
}

func (a *App) invalidate() {
	if a.win != nil {
		a.win.Invalidate()
	}
}
