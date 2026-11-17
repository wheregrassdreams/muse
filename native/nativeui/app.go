// Package nativeui is Muse's fully native shell: MyGo draws the window, the
// icon rail, the context panel and the Markdown editor, with no webview and
// no frontend build.
//
// The layout is a 56px icon rail, one shared context panel (files / outline
// / search / AI) and the editor card, which shows the document as Typora
// does: one surface the user writes on, rendered as it reads.
package nativeui

import (
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/transfer"
	"github.com/egoist/mygo/ui"

	applicationagent "muse/application/agent"
	applicationconfig "muse/application/config"
	documentsave "muse/application/document"
	applicationvault "muse/application/vault"
	workspaceapp "muse/application/workspace"
	"muse/config"
	"muse/document"
	"muse/document/frontmatter"
	doctypes "muse/document/types"
	editormodel "muse/editor"
	"muse/editor/findreplace"
	editorsource "muse/editor/source"
	"muse/events"
	"muse/locales"
	mdoc "muse/markdown"
	"muse/markdown/diagrams"
	"muse/markdown/images"
	markdownsyntax "muse/markdown/syntax"
	componenticons "muse/nativeui/components/icons"
	commands "muse/nativeui/workbench/commands"
	"muse/nativeui/workbench/diagnostics"
	"muse/nativeui/workbench/dragdrop"
	agentui "muse/nativeui/workbench/features/agent"
	"muse/nativeui/workbench/features/commandpalette"
	dialogsui "muse/nativeui/workbench/features/dialogs"
	documentdialogs "muse/nativeui/workbench/features/dialogs/document"
	editorui "muse/nativeui/workbench/features/editor"
	inlineui "muse/nativeui/workbench/features/editor/blocks/inline"
	noteeditor "muse/nativeui/workbench/features/editor/surface"
	filetree "muse/nativeui/workbench/features/explorer"
	imagedocumentui "muse/nativeui/workbench/features/imagedocument"
	imagepreviewui "muse/nativeui/workbench/features/imagepreview"
	linkpreview "muse/nativeui/workbench/features/linkpreview"
	markdownui "muse/nativeui/workbench/features/markdown"
	"muse/nativeui/workbench/features/mermaidpreview"
	naviconpicker "muse/nativeui/workbench/features/naviconpicker"
	outlineui "muse/nativeui/workbench/features/outline"
	propertypanel "muse/nativeui/workbench/features/properties"
	quickopenui "muse/nativeui/workbench/features/quickopen"
	searchui "muse/nativeui/workbench/features/search"
	findreplaceui "muse/nativeui/workbench/features/search/findreplace"
	settingsui "muse/nativeui/workbench/features/settings"
	vaulthome "muse/nativeui/workbench/features/workspace"
	"muse/nativeui/workbench/hosting"
	keybindings "muse/nativeui/workbench/keybindings"
	windowcontrols "muse/nativeui/workbench/shell/windowcontrols"
	"muse/nativeui/workbench/workspace"
	vaultsearch "muse/search"
	appearance "muse/theme"
	vaultfiles "muse/vault/files"
	vaultlinks "muse/vault/links"
	"muse/workspace"
)















// App is the window's state: everything the view reads and the actions write.
type App struct {
	configRepository          applicationconfig.Repository
	assistantSettingsLocation string
	newWatcher                func(root string) (applicationvault.WatcherPort, error)
	documentRepository        document.Repository
	documentRecovery          document.RecoveryRepository
	documentHistory           document.HistoryRepository
	documentExportWriter      documentsave.ExportWriter
	metadataReader            documentsave.MetadataReader
	documentEvents            *events.Dispatcher[document.DocumentSaved]
	documentSubscription      *events.Subscription
	vaultSearchReconciler     applicationvault.SearchReconciler
	newVaultIndex             applicationvault.IndexFactory
	newVaultCatalog           applicationvault.CatalogFactory
	fileOperations            applicationvault.FileOperationPort
	imageCacheStore           images.CacheStore
	imageLocalFiles           images.LocalFiles
	temporaryImageStore       documentsave.TemporaryImageStore
	openExternal              func(string) error
	workspaceEvents           *events.Dispatcher[workspace.WorkspaceRestored]
	workspaceRestorer         *workspaceapp.Restorer
	workspaceSession          *workspaceapp.Session
	workspaceSubscription     *events.Subscription
	documents                 *documentdialogs.Handle
	migrationDialog           *documentdialogs.MigrationController
	exportMenu                *dialogsui.ExportController
	vault                     *applicationvault.Session
	vaultSnapshot             applicationvault.Snapshot
	vaultResultNavigation     chan vaultNavigationResult
	vaultPendingNavigation    *vaultsearch.Hit
	pendingLinks              bool
	pendingWiki               string
	workspaceSearch           *searchui.Handle
	indexedPath               string
	indexedOffset             int
	indexedRevision           string
	win                       *mygo.Window
	windows                   *window.Manager
	workspacePresenter        *workspacepresenter.Presenter
	workspacePane             window.PaneID
	editorPaneHandle          window.Handle
	workspacePaneHandles      map[window.PaneID]window.Handle
	imagePreview              *imagepreviewui.Controller
	imageDocument             *imagedocumentui.Controller
	quickOpen                 *quickopenui.Controller
	mermaidPreviewHandle      window.Handle
	backlinksHandle           window.Handle
	linkPreview               *linkpreview.Controller
	navIconStore              *applicationvault.IconPreferences
	navIconPicker             *naviconpicker.Controller

	// quick open (⌘O): fuzzy file jump inside the workspace
	coreMigration        *documentsave.CoreMetadataPlan
	coreMigrationLoading bool
	coreMigrationRunning bool

	// commandpalette owns its transient query, selection, and open state.
	commandPalette commandpalette.Handle

	// appearance
	themeMode         int    // themeLight, themeDark, or themeSystem
	accentID          string // persisted accent preset, default red
	navigationHistory *workspaceapp.DocumentHistory
	// flat 是"Anytype 那种扁平"：编辑器不再是一张浮起的卡片，没有描边、
	// 没有圆角、没有投影，和窗口同色，页面直接铺满。关掉就是现在的卡片。
	flat bool
	// headerFade 让扁平风格下的头部浮在文档上：页面滚到它下面时模糊、
	// 淡出，而不是被头部的边缘切断（见 header_fade.go）。关掉就退回一
	// 块不透明的头部底色。
	headerFade bool
	// The component cache retains its shader resources for this window lifetime.
	headerFadeCache windowcontrols.FadeCache
	dark            bool // derived from themeMode for the view
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
	recentItems *workspaceapp.RecentItems
	// mainMu 在 headless 下把 onMain 和渲染帧串起来：没有窗口事件循环
	// 时 onMain 就地执行，后台 Agent 会与测试的帧同时改编辑器和 AI
	// 状态；有窗口时 onMain 本来就在帧的线程上，锁空转。
	mainMu                  sync.Mutex
	root                    string
	directoryCatalog        *applicationvault.DirectoryCatalog
	fileTreeState           *filetree.Handle
	treeDragItem            dragpayload.VaultItem
	treeNativeHandoff       bool
	nativeDropHandled       bool
	treeNativeDropSuggested transfer.Operation
	outlinePanel            *outlineui.Handle
	findReplace             *findreplaceui.Handle
	selectedPath            string
	focusedPanel            commands.PanelID
	treeFocusHandle         ui.Handle
	treeFocusRequested      bool
	treeKeySequences        *keybindings.KeySequenceMatcher
	panelKeySequences       *keybindings.KeySequenceMatcher
	treeSequenceKeymap      string
	treeHelpOpen            bool
	panelActions            *commands.ActionDispatcher
	lastPanelAction         commands.ActionID
	propertiesPanel         *propertypanel.Controller

	// The active note pane is the sole owner of document identity, source,
	// line-ending policy, selection, and persistence state.
	activeLinkIndex *applicationvault.ActiveLinkIndex

	// settings
	settingsPanel          *settingsui.Controller
	localeSetting          string
	localeConfigured       bool
	aiEnabled              bool
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
	ai                config.AssistantSettings
	assistantProvider applicationagent.ProviderManagement
	assistantStream   applicationagent.StreamClient
	assistant         *agentui.Controller
	newAgentWorkspace func(string, applicationagent.CurrentDocument) (applicationagent.WorkspacePort, error)

	// outline rows

	// status line
	status   string
	statusAt time.Time

	// blocking dialog guard: one native dialog at a time, never on the UI thread
	busy       bool
	closeState closeConfirmationState

	// inline rename dialog
	renameOpen  bool
	renamePath  string
	renameValue string
}

// Dependencies are concrete persistence ports selected by the composition
// root. The presentation shell does not construct filesystem adapters.
type Dependencies struct {
	Configuration             applicationconfig.Repository
	AssistantSettingsLocation string
	AssistantProvider         applicationagent.ProviderManagement
	AssistantStream           applicationagent.StreamClient
	Workspace                 workspace.Repository
	WorkspaceEvents           *events.Dispatcher[workspace.WorkspaceRestored]
	Documents                 document.Repository
	DocumentExportWriter      documentsave.ExportWriter
	DocumentMetadata          documentsave.MetadataReader
	Recovery                  document.RecoveryRepository
	History                   document.HistoryRepository
	DocumentEvents            *events.Dispatcher[document.DocumentSaved]
	VaultSearchReconciler     applicationvault.SearchReconciler
	NewVaultIndex             applicationvault.IndexFactory
	NewVaultCatalog           applicationvault.CatalogFactory
	FileOperations            applicationvault.FileOperationPort
	RecentItems               *workspaceapp.RecentItems
	NavIcons                  applicationvault.NavigationIconRepository
	ImageCacheStore           images.CacheStore
	ImageLocalFiles           images.LocalFiles
	TemporaryImages           documentsave.TemporaryImageStore
	NewWatcher                func(root string) (applicationvault.WatcherPort, error)
	NewAgentWorkspace         func(string, applicationagent.CurrentDocument) (applicationagent.WorkspacePort, error)
	OpenExternal              func(string) error
}

// New returns the app on an empty untitled document, with the panel on files.
func New() *App {
	a := &App{
		themeMode:            themeSystem,
		accentID:             "red",
		panel:                panelFiles,
		focusedPanel:         commands.PanelIDEditor,
		panelOpen:            true,
		flat:                 true,
		headerFade:           true,
		panelWidth:           panelDefaultWidth,
		panelW:               panelDefaultWidth,
		themeApplied:         -1,
		directoryCatalog:     applicationvault.NewDirectoryCatalog(),
		navigationHistory:    workspaceapp.NewDocumentHistory(),
		fileTreeState:        filetree.NewHandle(),
		settingsPanel:        settingsui.NewController(),
		findReplace:          findreplaceui.NewHandle(),
		outlinePanel:         outlineui.NewHandle(),
		workspaceSearch:      searchui.NewHandle(),
		commandPalette:       commandpalette.NewHandle(),
		fontSize:             16,
		lineHeight:           1.7,
		smartTypography:      true,
		ai:                   config.DefaultAssistantSettings(),
		activeLinkIndex:      &applicationvault.ActiveLinkIndex{},
		documents:            documentdialogs.NewHandle(),
		workspacePresenter:   workspacepresenter.New(),
		workspaceSession:     newWorkspaceSession(nil),
		workspacePaneHandles: make(map[window.PaneID]window.Handle),
	}
	a.assistant = newAssistantController(a)
	a.exportMenu = dialogsui.NewExportController(func(action dialogsui.ExportAction) {
		a.exportDocument(action.Format)
	})
	a.migrationDialog = documentdialogs.NewMigrationController(a.documents, documentdialogs.MigrationActions{
		Apply: a.applyCoreMetadataMigration,
		Close: a.invalidate,
	})
	// 新窗口就是一张空白未命名文档：光标停在空段落里，直接就能写。
	s := a.loadEditorSettings()
	a.applyEditorSettings(s)
	a.windows = window.NewManager(func() {
		if commands := a.noteEditor(); commands != nil {
			if focus := commands.FocusCommands(); focus != nil {
				focus.Focus()
			}
		}
	})
	a.imageDocument = newImageDocumentController(a)
	a.propertiesPanel = propertypanel.NewController(propertypanel.ControllerPorts{
		Document: func() propertypanel.DocumentSnapshot {
			path := a.documentPath()
			return propertypanel.DocumentSnapshot{
				Path: path, Source: a.documentSource(),
				Markdown: !a.imageDocumentOpen() && doctypes.IsMarkdownPath(path),
			}
		},
		Preferences: appPropertyPreferences{app: a},
		Metadata:    appPropertyMetadata{app: a},
		SystemProperties: func(doc propertypanel.DocumentSnapshot) []propertypanel.SystemProperty {
			return a.propertySystemProperties(doc)
		},
		Backlinks: func() []propertypanel.BacklinkSource {
			sources := a.documentBacklinkSources()
			result := make([]propertypanel.BacklinkSource, 0, len(sources))
			for _, source := range sources {
				result = append(result, propertypanel.BacklinkSource{Path: source.Source, Offset: source.Start, Count: source.Count})
			}
			return result
		},
		OpenBacklink:  func(path string, offset int) { a.navigateIndexed(path, offset) },
		OpenBacklinks: a.openLinks,
		OpenMigration: a.openCoreMigrationPreview,
		Invalidate:    a.invalidate,
	})
	a.navIconPicker = naviconpicker.NewController(naviconpicker.Ports{
		Preferences: navIconPreferences{app: a},
		RelativePath: func(path string) (string, bool) {
			return a.vaultRelativePath(path)
		},
		Invalidate: a.invalidate,
		Notify:     a.handleNavIconNotice,
	})
	a.quickOpen = quickopenui.NewController(quickopenui.ControllerOptions{
		Candidates: appQuickOpenCandidates{app: a},
		FileIcon:   componenticons.File(),
		IconTint:   func(theme *ui.Theme) ui.Color { return iconTint(theme) },
		Windows:    a.windows,
		Placement:  window.Placement{Host: "main", Layer: window.ModalOverlay},
		OnOpen: func(path string) {
			a.selectedPath = path
			a.requestOpen(path)
		},
		OnClose: a.invalidate,
	})
	a.linkPreview = linkpreview.NewController(linkpreview.ControllerOptions{Ports: a.linkPreviewPorts()})
	if err := a.createActiveNote(document.NewNote("", "", document.RevisionOf("")), 0); err != nil {
		panic(err)
	}
	if commands := a.noteEditor(); commands != nil {
		commands.ModalCommands().SetKeymap(s.Keymap)
	}
	a.loadDoc("")
	return a
}
