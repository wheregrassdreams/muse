// Package nativeui is Muse's fully native shell: MyGo draws the window, the
// icon rail, the context panel and the Markdown editor, with no webview and
// no frontend build.
//
// The layout is a 56px icon rail, one shared context panel (files / outline
// / search / AI) and the editor card, which shows the document as Typora
// does: one surface the user writes on, rendered as it reads.
package nativeui

import (
	"context"
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
	shellOpenPath = mygo.Shell.OpenPath
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
	busy    bool
	closing bool

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

type appPropertyPreferences struct{ app *App }

func (p appPropertyPreferences) Load(path string) (map[string]propertypanel.FieldPreference, error) {
	if p.app == nil {
		return nil, errors.New("properties: application is unavailable")
	}
	stored, err := applicationconfig.NewService(p.app.configRepository).LoadDocumentPropertyPreferences(path)
	if err != nil {
		return nil, err
	}
	return fieldPreferencesFromConfig(stored), nil
}

func (p appPropertyPreferences) Save(path string, preferences map[string]propertypanel.FieldPreference) error {
	if p.app == nil {
		return errors.New("properties: application is unavailable")
	}
	return applicationconfig.NewService(p.app.configRepository).SaveDocumentPropertyPreferences(path, fieldPreferencesToConfig(preferences))
}

type appPropertyMetadata struct{ app *App }

func (m appPropertyMetadata) SetProperty(key, value string) error {
	if m.app == nil {
		return propertypanel.ErrDocumentUnavailable
	}
	return documentsave.NewMetadataEditor(m.app.noteEditor()).SetProperty(key, value)
}

func (m appPropertyMetadata) AddProperty(key, value string, kind propertypanel.FieldKind) error {
	if m.app == nil {
		return propertypanel.ErrDocumentUnavailable
	}
	return documentsave.NewMetadataEditor(m.app.noteEditor()).AddProperty(key, value, appPropertyValueKind(kind))
}

func (m appPropertyMetadata) DeleteProperty(key string) error {
	if m.app == nil {
		return propertypanel.ErrDocumentUnavailable
	}
	return documentsave.NewMetadataEditor(m.app.noteEditor()).DeleteProperty(key)
}

func (m appPropertyMetadata) ReplaceYAML(source string) error {
	if m.app == nil {
		return propertypanel.ErrDocumentUnavailable
	}
	return documentsave.NewMetadataEditor(m.app.noteEditor()).ReplaceYAML(source)
}

func appPropertyValueKind(kind propertypanel.FieldKind) documentsave.PropertyValueKind {
	switch kind {
	case propertypanel.List:
		return documentsave.PropertyList
	case propertypanel.Boolean:
		return documentsave.PropertyBoolean
	case propertypanel.Number:
		return documentsave.PropertyNumber
	case propertypanel.Date:
		return documentsave.PropertyDate
	case propertypanel.Structure:
		return documentsave.PropertyStructure
	default:
		return documentsave.PropertyText
	}
}

func fieldPreferencesFromConfig(prefs config.DocumentPropertyPreferences) map[string]propertypanel.FieldPreference {
	if len(prefs.Fields) == 0 {
		return nil
	}
	fields := make(map[string]propertypanel.FieldPreference, len(prefs.Fields))
	for key, pref := range prefs.Fields {
		fields[key] = propertypanel.FieldPreference{Pinned: pref.Pinned, Hidden: pref.Hidden, Order: pref.Order}
	}
	return fields
}

func fieldPreferencesToConfig(fields map[string]propertypanel.FieldPreference) config.DocumentPropertyPreferences {
	if len(fields) == 0 {
		return config.DocumentPropertyPreferences{}
	}
	prefs := config.DocumentPropertyPreferences{Fields: make(map[string]config.FieldDisplayPreference, len(fields))}
	for key, pref := range fields {
		prefs.Fields[key] = config.FieldDisplayPreference{Pinned: pref.Pinned, Hidden: pref.Hidden, Order: pref.Order}
	}
	return prefs
}

func (a *App) propertySystemProperties(doc propertypanel.DocumentSnapshot) []propertypanel.SystemProperty {
	body := doc.Source
	if parsed := frontmatter.Parse(doc.Source); parsed.Err == nil && parsed.End > 0 && parsed.End <= len(body) {
		body = body[parsed.End:]
	}
	snapshot := documentsave.Inspect(doc.Source, body, doc.Path, a.metadataReader)
	properties := []propertypanel.SystemProperty{
		{Key: propertypanel.SystemPropertyFileName, Value: filepath.Base(doc.Path)},
		{Key: propertypanel.SystemPropertyPath, Value: doc.Path},
		{Key: propertypanel.SystemPropertyWordCount, Value: locales.PropertyWordCount(snapshot.Text.Characters, snapshot.BodyWords)},
	}
	if snapshot.HasMetadata {
		metadata := snapshot.Metadata
		if !metadata.CreatedAt.IsZero() {
			properties = append(properties, propertypanel.SystemProperty{Key: propertypanel.SystemPropertyCreatedAt, Value: locales.PropertyDateTime(metadata.CreatedAt.Local())})
		}
		properties = append(properties,
			propertypanel.SystemProperty{Key: propertypanel.SystemPropertyFileSize, Value: formatPropertySize(metadata.Size)},
			propertypanel.SystemProperty{Key: propertypanel.SystemPropertyUpdatedAt, Value: locales.PropertyDateTime(metadata.ModifiedAt.Local())},
		)
	}
	return properties
}

func formatPropertySize(size int64) string {
	if size < 1024 {
		return fmt.Sprintf("%d B", size)
	}
	return fmt.Sprintf("%.1f KB", float64(size)/1024)
}

func (a *App) documentBacklinkSources() []vaultlinks.BacklinkSource {
	if a == nil {
		return nil
	}
	vault := a.vault
	path := a.documentPath()
	if a.imageDocumentOpen() && a.imageDocumentPath() != "" {
		path = a.imageDocumentPath()
	}
	if vault == nil || path == "" || !a.vaultSnapshot.Ready {
		return nil
	}
	return vault.BacklinkSources(propertyVaultPath(a.vaultSnapshot.Root, path))
}

func propertyVaultPath(root, path string) string {
	relative, err := filepath.Rel(root, path)
	if err != nil {
		return filepath.ToSlash(path)
	}
	return filepath.ToSlash(relative)
}

type navIconPreferences struct{ app *App }

func (p navIconPreferences) Get(itemPath string) string {
	if p.app == nil || p.app.navIconStore == nil {
		return ""
	}
	return p.app.navIconStore.Get(p.app.root, itemPath)
}

func (p navIconPreferences) Set(itemPath, iconID string) error {
	if p.app == nil || p.app.navIconStore == nil {
		return errors.New("navigation icon storage is not configured")
	}
	return p.app.navIconStore.Set(p.app.root, itemPath, iconID)
}

func (p navIconPreferences) Delete(itemPath string) error {
	if p.app == nil || p.app.navIconStore == nil {
		return errors.New("navigation icon storage is not configured")
	}
	return p.app.navIconStore.Delete(p.app.root, itemPath)
}

func (a *App) handleNavIconNotice(notice naviconpicker.Notice) {
	if a == nil {
		return
	}
	switch notice.Kind {
	case naviconpicker.PreferencesUnavailable:
		a.notice(locales.CommonOpenIconSettingsError("navigation icon storage is not configured"))
	case naviconpicker.PreferenceSaveFailed:
		if notice.Err != nil {
			a.notice(locales.CommonSaveIconError(notice.Err.Error()))
		}
	case naviconpicker.PreferenceUpdated:
		a.notice(locales.NavigationIconUpdated())
	}
}

func (a *App) vaultRelativePath(path string) (string, bool) {
	if a == nil || a.root == "" || path == "" {
		return "", false
	}
	rel, err := filepath.Rel(a.root, path)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", false
	}
	return filepath.ToSlash(rel), true
}

func (a *App) treeIconID(path string) string {
	if a == nil || a.navIconPicker == nil {
		return ""
	}
	return a.navIconPicker.IconID(path)
}

func (a *App) treeIcon(path string, directory bool) *ui.SVG {
	if a == nil || a.navIconPicker == nil {
		if directory {
			return componenticons.Folder()
		}
		return componenticons.File()
	}
	return a.navIconPicker.IconForPath(path, directory)
}

func (a *App) openNavIconPicker(path string) {
	if a == nil || a.navIconPicker == nil {
		return
	}
	if a.navIconStore == nil {
		a.handleNavIconNotice(naviconpicker.Notice{Kind: naviconpicker.PreferencesUnavailable})
		return
	}
	a.navIconPicker.Open(path)
}

func (a *App) navIconPickerPopover(c *ui.Context, anchor ui.Element) {
	if a != nil && a.navIconPicker != nil {
		a.navIconPicker.RenderPopover(c, anchor)
	}
}

type appQuickOpenRows []quickopenui.Row

func (rows appQuickOpenRows) Len() int                     { return len(rows) }
func (rows appQuickOpenRows) At(index int) quickopenui.Row { return rows[index] }

type appQuickOpenCandidates struct{ app *App }

func (source appQuickOpenCandidates) Rows(query string) quickopenui.Rows {
	a := source.app
	if a == nil || a.vault == nil || !a.vaultSnapshot.Ready {
		return appQuickOpenRows(nil)
	}
	matches := a.vault.QuickOpen(query)
	rows := make(appQuickOpenRows, 0, len(matches))
	for _, match := range matches {
		rows = append(rows, quickopenui.Row{
			Path: match.Path, Title: match.Title, PathText: match.PathText,
			TitleMatches: match.TitleMatches, PathMatches: match.PathMatches,
		})
	}
	return rows
}

func (source appQuickOpenCandidates) Revision() quickopenui.Revision {
	if source.app == nil {
		return quickopenui.Revision{}
	}
	return quickopenui.Revision{Catalog: source.app.vaultSnapshot.FilesRevision, Index: source.app.vaultSnapshot.IndexRevision}
}

func (a *App) openQuickOpen() {
	if a == nil {
		return
	}
	if a.root == "" {
		a.openFileDialog()
		return
	}
	if a.quickOpen == nil {
		a.notice(quickopenui.ErrWindowHostUnavailable.Error())
		return
	}
	if err := a.quickOpen.Open(); err != nil {
		a.notice(err.Error())
	}
}

func (a *App) closeQuickOpen() {
	if a != nil && a.quickOpen != nil {
		_ = a.quickOpen.Close()
	}
}

func (a *App) quickOpenOpen() bool {
	return a != nil && a.quickOpen != nil && a.quickOpen.IsOpen()
}

func (a *App) imagePreviewController() *imagepreviewui.Controller {
	if a == nil {
		return nil
	}
	if a.imagePreview == nil {
		a.imagePreview = imagepreviewui.NewController(imagepreviewui.ControllerOptions{
			Windows: a.windows, Placement: window.Placement{Host: "main", Layer: window.ModalOverlay},
			ImageCache: a.imageCacheStore, LocalFiles: a.imageLocalFiles,
			Label: a.imagePreviewLabel,
			CopyImage: func(source string) {
				if editor := a.noteEditor(); editor != nil {
					editor.ClipboardCommands().CopyImageObject(source)
				}
			},
			OnMain: a.onMain, Invalidate: a.invalidate,
			OnError: func(err error) { a.notice(err.Error()) },
		})
	}
	return a.imagePreview
}

func (a *App) openImagePreview(source string) {
	if controller := a.imagePreviewController(); controller != nil {
		_ = controller.Open(source)
	}
}

func (a *App) closeImagePreview() {
	if a != nil && a.imagePreview != nil {
		a.imagePreview.Close()
	}
}

func (a *App) imagePreviewOpen() bool {
	return a != nil && a.imagePreview != nil && a.imagePreview.IsOpen()
}

func (a *App) imagePreviewLabel(source string) string {
	if strings.HasPrefix(source, "http://") || strings.HasPrefix(source, "https://") || strings.HasPrefix(source, "data:") {
		return source
	}
	base := a.docDir()
	if base == "" {
		return filepath.ToSlash(source)
	}
	if absolute, err := filepath.Abs(source); err == nil {
		source = absolute
	}
	if relative, err := filepath.Rel(base, source); err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return filepath.ToSlash(relative)
	}
	return filepath.ToSlash(source)
}

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

const (
	commandNewDocument     commandpalette.CommandID = "document.new"
	commandSave            commandpalette.CommandID = "document.save"
	commandSaveAs          commandpalette.CommandID = "document.save-as"
	commandWorkspaceSearch commandpalette.CommandID = "workspace.search"
	commandQuickOpen       commandpalette.CommandID = "workspace.quick-open"
	commandOpenFile        commandpalette.CommandID = "document.open-file"
	commandOpenFolder      commandpalette.CommandID = "workspace.open-folder"
	commandNewFile         commandpalette.CommandID = "workspace.new-file"
	commandNewFolder       commandpalette.CommandID = "workspace.new-folder"
	commandFilePanel       commandpalette.CommandID = "panel.files"
	commandOutlinePanel    commandpalette.CommandID = "panel.outline"
	commandSearchWorkspace commandpalette.CommandID = "workspace.search-all"
	commandSettings        commandpalette.CommandID = "app.settings"
	commandAIAssistant     commandpalette.CommandID = "panel.ai-assistant"
)

type paletteCommand struct {
	id   commandpalette.CommandID
	name string
	hint string
	run  func()
}

// paletteCommands projects app actions into the feature-owned command palette.
func (a *App) paletteCommands() []paletteCommand {
	toggle := func(p panelView) func() {
		return func() { a.togglePanel(p) }
	}
	cmds := []paletteCommand{
		{id: commandNewDocument, name: locales.DocumentNew(), hint: "⌘N", run: a.newDoc},
		{id: commandSave, name: locales.MenuSave(), hint: "⌘S", run: a.save},
		{id: commandSaveAs, name: locales.CommonSaveAs(), hint: "⇧⌘S", run: a.saveAs},
		{id: commandWorkspaceSearch, name: locales.MenuWorkspaceSearch(), hint: "⌘O", run: a.openWorkspaceSearch},
		{id: commandQuickOpen, name: locales.WorkspaceQuickOpenPath(), run: a.openQuickOpen},
		{id: commandOpenFile, name: locales.CommonOpenFile(), run: a.openFileDialog},
		{id: commandOpenFolder, name: locales.CommonOpenFolder(), hint: "⇧⌘O", run: a.openFolderDialog},
		{id: commandNewFile, name: locales.PaletteNewFile(), run: func() {
			a.newFileIn(a.fileTreeState.CreateTarget(a.root, a.selectedPath, a.catalogIsDirectory(a.selectedPath)))
		}},
		{id: commandNewFolder, name: locales.PaletteNewFolder(), run: func() {
			a.newFolderIn(a.fileTreeState.CreateTarget(a.root, a.selectedPath, a.catalogIsDirectory(a.selectedPath)))
		}},
		{id: commandFilePanel, name: locales.WorkspaceFilePanel(), run: toggle(panelFiles)},
		{id: commandOutlinePanel, name: locales.WorkspaceOutlinePanel(), run: toggle(panelOutline)},
		{id: commandSearchWorkspace, name: locales.MenuSearchWorkspace(), hint: "⇧⌘F", run: a.openWorkspaceSearch},
		{id: commandSettings, name: locales.MenuSettings(), run: a.openSettings},
	}
	if a.aiEnabled {
		cmds = append(cmds, paletteCommand{id: commandAIAssistant, name: locales.AiAssistantPanel(), run: toggle(panelAI)})
	}
	return cmds
}

func (a *App) paletteProjection() (commandpalette.ViewModel, map[commandpalette.CommandID]paletteCommand) {
	cmds := a.paletteCommands()
	items := make([]commandpalette.Command, len(cmds))
	byID := make(map[commandpalette.CommandID]paletteCommand, len(cmds))
	for i, cmd := range cmds {
		items[i] = commandpalette.Command{ID: cmd.id, Name: cmd.name, Hint: cmd.hint}
		byID[cmd.id] = cmd
	}
	return a.commandPalette.ViewModel(items), byID
}

func (a *App) paletteCandidates() []paletteCommand {
	model, byID := a.paletteProjection()
	out := make([]paletteCommand, 0, len(model.Commands))
	for _, match := range model.Commands {
		if cmd, ok := byID[match.ID]; ok {
			out = append(out, cmd)
		}
	}
	return out
}

func (a *App) openCommandPalette() {
	a.commandPalette.Open()
	a.invalidate()
}

func (a *App) closeCommandPalette() {
	a.commandPalette.Close()
	a.invalidate()
}

func (a *App) applyCommandPalette() {
	model, byID := a.paletteProjection()
	if len(model.Commands) == 0 {
		a.closeCommandPalette()
		return
	}
	selected := model.Commands[model.Selected]
	cmd, ok := byID[selected.ID]
	if !ok {
		a.closeCommandPalette()
		return
	}
	a.runPaletteCommand(cmd.id)
}

func (a *App) runPaletteCommand(id commandpalette.CommandID) {
	for _, cmd := range a.paletteCandidates() {
		if cmd.id == id {
			a.commandPalette.Close()
			a.invalidate()
			cmd.run()
			return
		}
	}
	a.closeCommandPalette()
}

func (a *App) commandPaletteDialog(c *ui.Context) {
	if !a.commandPalette.IsOpen() {
		return
	}
	model, _ := a.paletteProjection()
	commandpalette.Render(c, model, func(action commandpalette.Action) {
		a.commandPalette.Apply(action)
		switch action.Kind {
		case commandpalette.QueryChanged, commandpalette.MoveSelection:
			a.invalidate()
		case commandpalette.Execute:
			a.runPaletteCommand(action.ID)
		case commandpalette.Dismiss:
			a.closeCommandPalette()
		}
	})
}

// ---------------------------------------------------------------------------
// View

func (a *App) View(c *ui.Context) {
	logMemoryDiagnostics(a)
	a.mainMu.Lock()
	defer a.mainMu.Unlock()
	closedManagedSurface := false
	if c.Shortcut(0, ui.KeyEscape) && a.windows != nil && a.windows.HandleEscape() {
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

// filesView draws the workspace tree. Directories open lazily; the list builds
// only the rows in view, so a large repository is still quick.
func (a *App) filesView(c *ui.Context) {
	a.fileTreePanelView(c)
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
	_, over := ui.DragOver[dragpayload.VaultItem](trash)
	if over {
		a.setTreeDragOperationDisplay(filetree.DragOperationMoveToTrash, "")
		trash.Background(ui.RGBA(128, 66, 72, 0.10)).BorderColor(t.Border.Alpha(0.48))
	}
	if dropped, ok := ui.Drop[dragpayload.VaultItem](trash); ok {
		a.trashTreeItems(a.treeDraggedPaths(dropped.Path))
	}
	trash.Children(func() {
		ui.Column(c).Width(110).FillHeight().AlignItems(ui.Center).Justify(ui.Center).Gap(4).
			Label(locales.TreeTrashHintColumn()).Children(func() {
			glyph := componenticons.Trash()
			iconColor := t.TextMuted.Alpha(0.18)
			labelColor := t.TextMuted.Alpha(0.23)
			if over {
				glyph = componenticons.TrashOpen()
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

func (a *App) setTreeRootDropTarget(rows []filetree.Row) {
	a.fileTreeState.SetDropTargetRange(a.root, 0, len(rows))
	a.setTreeDragOperationDisplay(filetree.DragOperationMoveToFolder, filepath.Base(a.root))
}

func (a *App) importDroppedInto(paths []string, destination string) {
	_, err := a.vaultFileUseCases().ImportFiles(paths, destination)
	if err != nil {
		a.notice(locales.CommonImportFailed(err.Error()))
	}
	if len(paths) > 0 {
		a.clearTreeCache()
		a.invalidate()
	}
}

func (a *App) beginTreeNativeHandoffItems(c *ui.Context, items []dragpayload.VaultItem) {
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
			a.fileTreeState.SetDragPreview(filetree.DragPreviewState{})
			if result.Err == nil && result.Operation != transfer.None && a.vaultFileUseCases().OwnsPath(sourcePath) {
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

func treeItemTransferData(item dragpayload.VaultItem) (transfer.Data, error) {
	return treeItemsTransferData([]dragpayload.VaultItem{item})
}

func treeItemsTransferData(items []dragpayload.VaultItem) (transfer.Data, error) {
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
		Path      string                  `json:"path"`
		Name      string                  `json:"name"`
		Directory bool                    `json:"directory"`
		Items     []dragpayload.VaultItem `json:"items,omitempty"`
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
		Path      string                  `json:"path"`
		Name      string                  `json:"name"`
		Directory bool                    `json:"directory"`
		Items     []dragpayload.VaultItem `json:"items"`
	}
	if err := json.Unmarshal(b, &payload); err != nil {
		a.notice(locales.FilesReadDropFailed())
		return
	}
	items := payload.Items
	if len(items) == 0 && payload.Path != "" {
		items = []dragpayload.VaultItem{{Path: payload.Path, Name: payload.Name, Directory: payload.Directory}}
	}
	if len(items) == 0 {
		a.notice(locales.FilesReadDropFailed())
		return
	}
	// Native negotiation defaults to Copy when no modifier is suggested.
	// Muse's same-vault tree semantics are Move unless the user explicitly
	// requested Copy in the native drag session.
	move := drop.Operation == transfer.Move || (drop.Operation == transfer.Copy && a.treeNativeDropSuggested != transfer.Copy)
	paths := make([]string, 0, len(items))
	for _, item := range items {
		if item.Path != "" {
			paths = append(paths, item.Path)
		}
	}
	intent := applicationvault.DropCopy
	if move {
		intent = applicationvault.DropMove
	}
	a.applyVaultDrop(paths, destination, intent)
}

// moveTreeItem moves a dragged item into a directory in the workspace.
// Existing names are preserved; collisions get a numbered suffix.
func (a *App) moveTreeItem(source, destination string) {
	a.moveTreeItems([]string{source}, destination)
}

func (a *App) moveTreeItems(paths []string, destination string) {
	mutation, err := a.vaultFileUseCases().MoveFiles(paths, destination)
	moves := mutation.Moves
	if len(moves) > 0 {
		a.remapTreePaths(moves)
		a.clearTreeCache()
		a.notice(locales.FilesMovedToFolder(filepath.Base(destination)))
		a.invalidate()
	}
	if err != nil {
		a.notice(locales.CommonMoveFailed(err.Error()))
	}
}

func (a *App) remapTreePaths(moves []applicationvault.FileMove) {
	remap := func(path string) string {
		for _, move := range moves {
			if rel, err := filepath.Rel(move.Source, path); err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
				path = filepath.Join(move.Target, rel)
			}
		}
		return path
	}
	if next := remap(a.documentPath()); next != a.documentPath() {
		if note, err := a.readDocument(next); err == nil && a.noteEditor() != nil {
			_ = a.noteEditor().Relocate(note)
		}
	}
	a.selectedPath = remap(a.selectedPath)
	a.fileTreeState.RemapIDs(remap)
	if a.treeDragItem.Path != "" {
		a.treeDragItem.Path = remap(a.treeDragItem.Path)
	}
}

func (a *App) fileRows() []filetree.Row {
	if a == nil || a.fileTreeState == nil {
		return nil
	}
	return filetree.BuildRows(a.fileTreeState, a.root, filetree.CatalogPort{
		Children:    a.treeCatalogChildren,
		VisibleFile: doctypes.IsVisibleWorkspacePath,
	})
}

func (a *App) selectTreeRow(path string, mods ui.Modifiers, rows []filetree.Row) {
	entries := make([]filetree.Entry, len(rows))
	for i, row := range rows {
		entries[i].ID = row.Path
	}
	a.fileTreeState.SelectID(path, mods, entries)
}

func (a *App) treeDraggedPaths(primary string) []string {
	var paths []string
	if a.fileTreeState.IsSelected(primary) {
		paths = a.fileTreeState.SelectedPaths(a.fileRows(), primary)
	} else if primary != "" {
		paths = []string{primary}
	}
	return a.vaultFileUseCases().NormalizeTargets(paths)
}

func (a *App) outlineView(c *ui.Context) {
	if a.noteEditor() == nil {
		return
	}
	parsedHeadings := a.noteEditor().NavigationCommands().Headings()
	headings := make([]outlineui.Heading, 0, len(parsedHeadings))
	for _, heading := range parsedHeadings {
		headings = append(headings, outlineui.Heading{Level: heading.Level, Text: heading.Text, Offset: heading.Offset})
	}
	a.outlinePanel.SyncActiveHeading(headings, a.noteEditor().NavigationCommands().Caret())
	a.outlinePanel.Render(c, outlineui.ViewModel{Headings: headings}, func(action outlineui.Action) {
		if action.Kind != outlineui.SelectHeading {
			return
		}
		a.noteEditor().NavigationCommands().NavigateToOffset(action.Offset)
		a.noteEditor().FocusCommands().Focus()
	})
}

func (a *App) searchView(c *ui.Context) {
	a.documentSearchView(c)
}

func (a *App) openWorkspaceSearch() {
	a.closeQuickOpen()
	a.commandPalette.Close()
	if a.root == "" && a.documentPath() != "" {
		a.root = filepath.Dir(a.documentPath())
	}
	a.enableDocumentServices()
	a.workspaceSearch.Open()
	a.invalidate()
}

func (a *App) closeWorkspaceSearch() {
	a.workspaceSearch.Close()
	if a.vault != nil {
		literal := a.workspaceSearch.Mode() == searchui.Literal
		a.vault.ResetSearchSchedule()
		_, _ = a.vault.Query("", literal, a.documentPath(), a.diskSource())
		a.vaultSnapshot = a.vault.Snapshot()
	}
	a.invalidate()
}

func (a *App) workspaceRecentNotes() []recentEntry {
	if a.recentItems == nil {
		return nil
	}
	return a.recentItems.LiveNotes(a.root, workspaceapp.MaxVisibleRecent)
}

func (a *App) openWorkspaceRecent(entry recentEntry) {
	a.closeWorkspaceSearch()
	a.requestOpen(entry.Path)
}

func (a *App) workspaceSearchDialog(c *ui.Context) {
	if !a.workspaceSearch.IsOpen() {
		return
	}
	if delay := a.syncWorkspaceSearchQuery(); delay > 0 {
		c.After(delay)
	}

	model := searchui.View{
		SearchIcon: componenticons.Search(),
		FileIcon:   componenticons.File(), IconTint: iconTint(c.Theme()),
	}
	if a.root == "" {
		model.Availability = searchui.VaultRequired
	} else if a.vault == nil || !a.vaultSnapshot.SearchAvailable {
		model.Availability = searchui.IndexBuilding
		model.Error = a.vaultSnapshot.Search.Error
		if model.Error == "" {
			model.Error = a.vaultSnapshot.Error
		}
	} else {
		model.Availability = searchui.Ready
		model.Busy = a.vaultSnapshot.Search.Busy
		model.Error = a.vaultSnapshot.Search.Error
	}
	model.CanChangeMode = a.vault != nil
	for _, entry := range a.workspaceRecentNotes() {
		model.Recent = append(model.Recent, searchui.RecentItem{
			Path: entry.Path, Title: filepath.Base(entry.Path), PathLabel: a.workspacePathLabel(entry.Path),
		})
	}
	if a.vault != nil {
		hits := a.vaultSnapshot.Search.Hits
		for _, hit := range hits {
			title := filepath.Base(hit.Path)
			if noteTitle, _, ok := a.vault.NoteContent(hit.Path); ok && strings.TrimSpace(noteTitle) != "" {
				title = noteTitle
			}
			model.Results = append(model.Results, searchui.Result{
				Path: hit.Path, Revision: hit.Revision, Offset: hit.Start,
				Title: title, PathLabel: a.workspacePathLabel(hit.Path), Snippet: hit.Snippet,
			})
		}
	}
	a.workspaceSearch.Render(c, model, func(action searchui.Action) {
		switch action.Kind {
		case searchui.QueryChanged:
		case searchui.ModeChanged:
			if a.vault != nil {
				a.vault.ForceSearch(time.Now(), 0)
			}
			a.vaultSnapshot.Search.Hits = nil
			a.workspaceSearch.FocusInput(c)
		case searchui.MoveSelection:
		case searchui.OpenRecent:
			a.openWorkspaceRecent(recentEntry{Path: action.Recent.Path})
		case searchui.OpenResult:
			a.closeWorkspaceSearch()
			a.navigateSearchHit(vaultsearch.Hit{
				Path: action.Result.Path, Revision: action.Result.Revision, Start: action.Result.Offset,
			})
		case searchui.Dismiss:
			a.closeWorkspaceSearch()
		}
	})
}

func (a *App) documentSearchView(c *ui.Context) {
	a.refreshSearch()
	findreplaceui.Render(c, a.findReplace, a.dispatchFindReplaceAction)
}

func (a *App) dispatchFindReplaceAction(action findreplaceui.Action) {
	switch action.Kind {
	case findreplaceui.QueryChanged:
		a.refreshSearch()
		a.invalidate()
	case findreplaceui.ReplacementChanged:
		// The panel owns this text state; no document edit occurs yet.
	case findreplaceui.OpenMatch:
		a.findReplace.Apply(action)
		a.goToHit(action.Index)
	case findreplaceui.ReplaceOne:
		a.replaceHit()
	case findreplaceui.ReplaceAll:
		a.replaceAllHits()
	}
}

// refreshSearch finds the query in the document again when either changed,
// and shows the matches in the editor.
func (a *App) refreshSearch() {
	if a.noteEditor() == nil || !a.findReplace.Sync(a.documentSource()) {
		return
	}
	a.syncFindReplaceHighlights()
}

func (a *App) syncFindReplaceHighlights() {
	if a.noteEditor() == nil {
		return
	}
	snapshot := a.findReplace.Snapshot()
	matches := snapshot.Matches
	ranges := make([]noteeditor.TextRange, len(matches))
	for i, h := range matches {
		ranges[i] = noteeditor.TextRange{Start: h.Start, End: h.End}
	}
	a.noteEditor().SearchCommands().SetHighlights(ranges, snapshot.Selected)
}

// goToHit selects match i in the editor.
func (a *App) goToHit(i int) {
	snapshot := a.findReplace.Snapshot()
	matches := snapshot.Matches
	if i < 0 || i >= len(matches) {
		return
	}
	a.findReplace.Apply(findreplaceui.Action{Kind: findreplaceui.OpenMatch, Index: i})
	h := matches[i]
	ranges := make([]noteeditor.TextRange, len(matches))
	for k, x := range matches {
		ranges[k] = noteeditor.TextRange{Start: x.Start, End: x.End}
	}
	a.noteEditor().SearchCommands().SetHighlights(ranges, i)
	a.noteEditor().NavigationCommands().NavigateToRange(h.Start, h.End)
	a.noteEditor().FocusCommands().Focus()
}

// replaceHit replaces the chosen match, or the first, and goes to the next.
func (a *App) replaceHit() {
	snapshot := a.findReplace.Snapshot()
	matches := snapshot.Matches
	i := max(snapshot.Selected, 0)
	if i >= len(matches) {
		return
	}
	h := matches[i]
	_ = a.noteEditor().Replace(h.Start, h.End, snapshot.Replacement)
	a.refreshSearch()
	if count := len(a.findReplace.Snapshot().Matches); count > 0 {
		a.goToHit(min(i, count-1))
	}
}

// replaceAllHits replaces every match, as one step of undo.
func (a *App) replaceAllHits() {
	snapshot := a.findReplace.Snapshot()
	matches := snapshot.Matches
	if len(matches) == 0 {
		return
	}
	source := a.documentSource()
	replaced, n := findreplace.ReplaceAll(source, matches, snapshot.Replacement)
	if n == 0 {
		return
	}
	a.editDoc(replaced)
	a.refreshSearch()
	a.notice(locales.CommonReplaceSuccess(n))
}

// ---------------------------------------------------------------------------
// Editor

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

// focusedPane returns the window host's active pane, falling back to the
// initial editor pane before any pane content has been opened.
func (a *App) focusedPane() window.PaneID {
	if a.windows != nil {
		if pane, ok := a.windows.FocusedPane("main"); ok {
			if handle := a.workspacePaneHandles[pane]; !handle.IsZero() {
				a.editorPaneHandle = handle
			}
			return pane
		}
	}
	if a.workspacePane != "" {
		return a.workspacePane
	}
	return "editor"
}

func (a *App) renderFocusedPane(c *ui.Context) {
	if a.windows == nil {
		return
	}
	a.windows.RenderPane(c, "main", a.focusedPane())
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

// ---------------------------------------------------------------------------
// Files

func (a *App) setWorkspace(root string) {
	// A workspace is the start of a new editing context. Do not leave the
	// previous vault's document or pane descriptors visible after switching folders.
	if filepath.Clean(root) != filepath.Clean(a.root) {
		a.activeLinkIndex.Clear()
		a.clearWikiSuggestionCache()
		a.closeWorkspacePaneSurfaces()
		a.workspacePane = "editor"
		if a.workspaceSession != nil {
			if err := a.workspaceSession.Restore(workspace.State{Version: 1, VaultPath: root}); err != nil {
				a.notice(err.Error())
			}
		}
		if err := a.createActiveNote(document.NewNote("", "", document.RevisionOf("")), 0); err != nil {
			a.notice(err.Error())
		}
		a.resetDoc()
		if a.vault != nil {
			old := a.vault
			a.vault = nil
			a.vaultSnapshot = applicationvault.Snapshot{}
			a.directoryCatalog.CloseSession(old)
			go old.Close()
		}
	}
	a.root = root
	a.clearTreeCache()
	a.fileTreeState.CollapseAll()
	a.fileTreeState.SetSelection(nil)
	a.fileTreeState.SetFocusID("")
	a.fileTreeState.SetScrollOffset(ui.Point{})
	a.selectedPath = ""
	a.children(root)
	a.addRecent(root, true)
	a.panel = panelFiles
	a.panelOpen = true
}

func (a *App) children(dir string) []string {
	if a == nil || a.root == "" {
		return nil
	}
	return filetree.ProjectChildren(a.root, a.treeCatalogChildren(dir), doctypes.IsVisibleWorkspacePath)
}

// treeCatalogChildren adapts the active vault catalog to the explorer's
// narrow, backend-neutral catalog facts. Filtering, ordering, and flattening
// remain owned by the explorer feature.
func (a *App) treeCatalogChildren(dir string) []filetree.CatalogEntry {
	if a == nil || a.root == "" || a.vault == nil || a.directoryCatalog == nil || filepath.Clean(a.vaultSnapshot.Root) != filepath.Clean(a.root) {
		return nil
	}
	rel, err := filepath.Rel(a.root, dir)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return nil
	}
	entries := a.directoryCatalog.Children(a.vault, filepath.ToSlash(rel))
	children := make([]filetree.CatalogEntry, 0, len(entries))
	for _, entry := range entries {
		switch entry.Kind {
		case vaultfiles.EntryDirectory:
			children = append(children, filetree.CatalogEntry{Path: entry.Path, Kind: filetree.CatalogDirectory})
		case vaultfiles.EntryRegular:
			children = append(children, filetree.CatalogEntry{Path: entry.Path, Kind: filetree.CatalogFile})
		}
	}
	return children
}

func (a *App) catalogEntry(path string) (vaultfiles.Entry, bool) {
	if a == nil || a.vault == nil || a.root == "" || filepath.Clean(a.vaultSnapshot.Root) != filepath.Clean(a.root) {
		return vaultfiles.Entry{}, false
	}
	rel, err := filepath.Rel(a.root, path)
	if err != nil || rel == "." || rel == ".." || filepath.IsAbs(rel) || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return vaultfiles.Entry{}, false
	}
	return a.vault.Entry(filepath.ToSlash(rel))
}

func (a *App) catalogIsDirectory(path string) bool {
	entry, ok := a.catalogEntry(path)
	return ok && entry.Kind == vaultfiles.EntryDirectory
}

func (a *App) pathKind(path string) (vaultfiles.EntryKind, error) {
	if entry, ok := a.catalogEntry(path); ok {
		return entry.Kind, nil
	}
	if a == nil || a.fileOperations == nil {
		return vaultfiles.EntryOther, errors.New("vault file metadata is not configured")
	}
	return a.fileOperations.Kind(path)
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

// collapseAll folds every expanded directory in the tree.
func (a *App) collapseAll() {
	a.fileTreeState.CollapseAll()
	a.invalidate()
}

func (a *App) clearTreeCache() {
	a.fileTreeState.InvalidateRows()
	a.directoryCatalog.Invalidate()
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
		if _, err := a.vaultFileUseCases().TrashFiles([]string{path}); err != nil {
			errorDialog(locales.DialogDeleteFailed(), err.Error())
			a.finish(nil)
			return
		}
		a.finish(func() {
			if docPath := a.documentPath(); docPath == path || strings.HasPrefix(docPath, path+string(filepath.Separator)) {
				a.selectedPath = ""
				a.loadDocumentAt("", "")
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
		mutation, trashErr := a.vaultFileUseCases().TrashFiles(paths)
		trashed := mutation.Trashed
		a.finish(func() {
			for _, path := range trashed {
				a.fileTreeState.RemoveSelected(path)
				if docPath := a.documentPath(); docPath == path || strings.HasPrefix(docPath, path+string(filepath.Separator)) {
					a.selectedPath = ""
					a.loadDocumentAt("", "")
					a.updateTitle()
				}
			}
			a.clearTreeCache()
			if len(trashed) > 0 {
				a.notice(locales.FilesMovedToTrash())
			}
			if trashErr != nil {
				errorDialog(locales.DialogDeleteFailed(), trashErr.Error())
			}
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
	mutation, err := a.vaultFileUseCases().RenameFile(old, name)
	if err != nil {
		a.notice(locales.CommonRenameFailed(err.Error()))
		a.renameOpen = false
		a.invalidate()
		return
	}
	if len(mutation.Moves) == 0 {
		a.renameOpen = false
		return
	}
	a.remapTreePaths(mutation.Moves)
	a.renameOpen = false
	a.renamePath = ""
	a.clearTreeCache()
	a.notice(locales.FilesRenamed())
	a.updateTitle()
	a.invalidate()
}

// ---------------------------------------------------------------------------
// Settings

func (a *App) settingsDialog(c *ui.Context) {
	a.settingsPanel.RenderDialog(c, a.settingsViewModel(), a.dispatchSettingsAction)
}

func (a *App) settingsViewModel() settingsui.ViewModel {
	mode := settingsui.ThemeSystem
	switch a.themeMode {
	case themeLight:
		mode = settingsui.ThemeLight
	case themeDark:
		mode = settingsui.ThemeDark
	}
	accents := make([]settingsui.Accent, 0, len(appearance.Presets))
	for _, preset := range appearance.Presets {
		accents = append(accents, settingsui.Accent{ID: preset.ID, Label: settingsui.LocalizedAccentName(preset.ID), Color: preset.Color})
	}
	modelCatalog := a.assistant.ModelCatalogSnapshot()
	assistantModels := make([]string, 0, len(modelCatalog.Models))
	for _, model := range modelCatalog.Models {
		assistantModels = append(assistantModels, model.ID)
	}
	return settingsui.ViewModel{
		Appearance: settingsui.Appearance{
			Theme: mode, Accents: accents, AccentID: a.accentID,
			Locale: a.localeSetting, LocaleConfigured: a.localeConfigured,
			HeaderFade: a.headerFade, SmoothPageJumps: a.smoothPageJumps,
		},
		Editor: settingsui.Editor{
			Keymap: a.noteEditor().ModalCommands().Keymap(), SmartTypography: a.smartTypography,
			FontSize: a.fontSize, LineHeight: a.lineHeight, HideMouseCursor: a.hideMouseCursor,
			CursorFade: a.cursorFade, SmoothCursor: a.smoothCursor,
			HeadingMarkerReveal: a.headingMarkerReveal, VimVirtualImageRows: a.vimVirtualImageRows,
			VimYankFlash: a.vimYankFlash, SearchMatchPulse: a.searchMatchPulse,
		},
		Assistant: settingsui.Assistant{
			Enabled: a.aiEnabled, Provider: a.ai.Provider, BaseURL: a.ai.BaseURL, APIKey: a.ai.APIKey,
			Model: a.ai.Model, SystemPrompt: a.ai.System, AvailableModels: assistantModels,
			ModelsLoading: modelCatalog.Loading, ModelsError: modelCatalog.Error,
			Capabilities: settingsui.Capabilities{Browse: a.ai.Tools.Browse, Edit: a.ai.Tools.Edit, Manage: a.ai.Tools.Manage},
			SettingsPath: a.assistantSettingsLocation, CapabilitiesDetail: locales.AiCapabilitiesDetail(),
		},
	}
}

func (a *App) dispatchSettingsAction(action settingsui.Action) {
	a.settingsPanel.Apply(action)
	switch action.Kind {
	case settingsui.SelectPage:
	case settingsui.ThemeChanged:
		mode := themeSystem
		switch action.Theme {
		case settingsui.ThemeLight:
			mode = themeLight
		case settingsui.ThemeDark:
			mode = themeDark
		}
		a.setThemeMode(mode)
	case settingsui.AccentChanged:
		a.setAccent(action.AccentID)
	case settingsui.LocaleChanged:
		a.setLocale(action.Locale)
	case settingsui.HeaderFadeChanged:
		a.headerFade = action.Enabled
		if err := a.saveNativePreferences(); err != nil {
			a.notice(locales.CommonSettingSaveError(err.Error()))
		}
		a.invalidate()
	case settingsui.SmoothPageJumpsChanged:
		a.setSmoothPageJumps(action.Enabled)
	case settingsui.KeymapChanged:
		a.setEditorKeymap(action.Text)
	case settingsui.SmartTypographyChanged:
		a.setSmartTypography(action.Enabled)
	case settingsui.FontSizeChanged:
		a.fontSize = action.Number
		a.configureActiveNote(0)
		a.invalidate()
	case settingsui.LineHeightChanged:
		a.lineHeight = action.Number
		a.configureActiveNote(0)
		a.invalidate()
	case settingsui.HideMouseCursorChanged:
		a.setHideMouseCursor(action.Enabled)
	case settingsui.CursorFadeChanged:
		a.setCursorFade(action.Enabled)
	case settingsui.SmoothCursorChanged:
		a.setSmoothCursor(action.Enabled)
	case settingsui.HeadingMarkerRevealChanged:
		a.setHeadingMarkerReveal(action.Enabled)
	case settingsui.VimVirtualImageRowsChanged:
		a.setVimVirtualImageRows(action.Enabled)
	case settingsui.VimYankFlashChanged:
		a.setVimYankFlash(action.Enabled)
	case settingsui.SearchMatchPulseChanged:
		a.setSearchMatchPulse(action.Enabled)
	case settingsui.AssistantEnabledChanged:
		a.setAIEnabled(action.Enabled)
	case settingsui.ProviderChanged:
		a.setAIService(action.Text == config.AssistantProviderMagpie)
	case settingsui.BaseURLChanged:
		a.ai.BaseURL = action.Text
	case settingsui.APIKeyChanged:
		a.ai.APIKey = action.Text
	case settingsui.ModelChanged:
		a.ai.Model = action.Text
	case settingsui.SystemPromptChanged:
		a.ai.System = action.Text
	case settingsui.CapabilityChanged:
		switch action.Capability {
		case settingsui.BrowseCapability:
			a.ai.Tools.Browse = action.Enabled
		case settingsui.EditCapability:
			a.ai.Tools.Edit = action.Enabled
		case settingsui.ManageCapability:
			a.ai.Tools.Manage = action.Enabled
		}
		a.saveAITools()
	case settingsui.FetchModels:
		a.fetchAIModels()
	case settingsui.SaveAssistant:
		if err := a.saveAssistantSettings(); err != nil {
			a.notice(locales.CommonSaveFailed(err.Error()))
		} else {
			a.notice(locales.AiModelSettingsSaved())
		}
	case settingsui.Close:
	}
}

// ---------------------------------------------------------------------------
// The document

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

// openDropped opens the first document among files dropped on the editor.
func (a *App) openDropped(paths []string) {
	for _, p := range paths {
		kind, err := a.pathKind(p)
		if err == nil && kind == vaultfiles.EntryDirectory {
			a.setWorkspace(p)
			a.invalidate()
			return
		}
		if doctypes.IsEditablePath(p) {
			a.requestOpen(p)
			return
		}
	}
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

// saveImage stores an image pasted into the document in an assets folder
// next to it, and returns the path the document links it by. An unsaved
// document keeps it in the temporary folder, by its absolute path.
func (a *App) saveImage(data []byte, ext string) (string, error) {
	if ext == "" {
		ext = ".png"
	}
	dir := a.docDir()
	rel := dir != ""
	base := "image-" + time.Now().Format("20060102-150405")
	if rel {
		dir = filepath.Join(dir, "assets")
		root := a.root
		if root == "" {
			root = a.docDir()
		}
		files := applicationvault.NewFileUseCases(root, a.fileOperations, a.isVaultDirectory)
		path, err := files.CreateAttachment(applicationvault.CreateAttachmentCommand{
			Directory: dir, Name: base, Extension: ext, Content: data,
		})
		if err != nil {
			return "", err
		}
		return "assets/" + filepath.Base(path), nil
	}
	if a.temporaryImageStore == nil {
		return "", errors.New("temporary image store is not configured")
	}
	path, err := a.temporaryImageStore.SaveTemporaryImage(data, ext)
	if err != nil {
		return "", err
	}
	return filepath.ToSlash(path), nil
}

func plainSpans(spans []mdoc.Span) string {
	var b strings.Builder
	for _, s := range spans {
		b.WriteString(s.Text)
	}
	return b.String()
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

func (a *App) openSettings() {
	a.settingsPanel.OpenAt(settingsui.AppearancePage)
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
