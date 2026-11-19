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
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/i18n"
	"github.com/egoist/mygo/transfer"
	"github.com/egoist/mygo/ui"

	"github.com/wheregrassdreams/muse-editor/markdown/diagrams"
	editorsource "github.com/wheregrassdreams/muse-editor/markdown/source"
	markdownsyntax "github.com/wheregrassdreams/muse-editor/markdown/syntax"
	applicationagent "muse/application/agent"
	applicationconfig "muse/application/config"
	documentsave "muse/application/document"
	applicationvault "muse/application/vault"
	vaultfileapp "muse/application/vault/files"
	vaulticonapp "muse/application/vault/iconpreferences"
	vaultlinkapp "muse/application/vault/links"
	workspaceapp "muse/application/workspace"
	"muse/config"
	"muse/document"
	"muse/document/frontmatter"
	doctypes "muse/document/types"
	"muse/events"
	"muse/locales"
	mdoc "muse/markdown"
	"muse/markdown/images"
	agentui "muse/nativeui/workbench/features/agent"
	backlinksui "muse/nativeui/workbench/features/backlinks"
	"muse/nativeui/workbench/features/commandpalette"
	documentdialogs "muse/nativeui/workbench/features/dialogs/document"
	dialogsui "muse/nativeui/workbench/features/dialogs/export"
	filedialogs "muse/nativeui/workbench/features/dialogs/files"
	editorui "muse/nativeui/workbench/features/editor"
	inlineui "muse/nativeui/workbench/features/editor/blocks/inline"
	noteeditor "muse/nativeui/workbench/features/editor/surface"
	explorer "muse/nativeui/workbench/features/explorer"
	imagedocumentui "muse/nativeui/workbench/features/imagedocument"
	imagepreviewui "muse/nativeui/workbench/features/imagepreview"
	linkpreview "muse/nativeui/workbench/features/linkpreview"
	"muse/nativeui/workbench/features/mermaidpreview"
	naviconpicker "muse/nativeui/workbench/features/naviconpicker"
	outlineui "muse/nativeui/workbench/features/outline"
	propertypanel "muse/nativeui/workbench/features/properties"
	quickopenui "muse/nativeui/workbench/features/quickopen"
	findreplaceui "muse/nativeui/workbench/features/search/findreplace"
	searchui "muse/nativeui/workbench/features/search/workspace"
	settingsui "muse/nativeui/workbench/features/settings"
	componenticons "muse/nativeui/workbench/framework/components/icons"
	"muse/nativeui/workbench/framework/diagnostics"
	commands "muse/nativeui/workbench/framework/interactions/commands"
	"muse/nativeui/workbench/framework/interactions/dragdrop"
	keybindings "muse/nativeui/workbench/framework/interactions/keybindings"
	markdownui "muse/nativeui/workbench/framework/markdown"
	shellui "muse/nativeui/workbench/framework/shell"
	windowcontrols "muse/nativeui/workbench/framework/shell/windowcontrols"

	hostingui "muse/nativeui/workbench/framework/hosting"
	workspaceui "muse/nativeui/workbench/framework/workspace"
	vaultfiles "muse/vault/files"
	vaultlinks "muse/vault/links"
	"muse/workspace"
)

// saveDialog is the NativeUI platform seam used by export integration tests.
// Production delegates to MyGo's system save panel.
var saveDialog = mygo.Dialog.Save
var openDialog = mygo.Dialog.Open

// App is the window's state: everything the view reads and the actions write.
type App struct {
	configRepository           applicationconfig.Repository
	assistantSettingsLocation  string
	newWatcher                 func(root string) (applicationvault.WatcherPort, error)
	documentRepository         document.Repository
	documentRecovery           document.RecoveryRepository
	documentHistory            document.HistoryRepository
	documentExportWriter       documentsave.ExportWriter
	metadataReader             documentsave.MetadataReader
	documentEvents             *events.Dispatcher[document.DocumentSaved]
	documentSubscription       *events.Subscription
	agentWorkspaceMutations    *applicationagent.WorkspaceMutationService
	agentWorkspaceEvents       *events.Dispatcher[applicationagent.WorkspaceMutation]
	agentWorkspaceSubscription *events.Subscription
	vaultSearchReconciler      applicationvault.SearchReconciler
	newVaultIndex              applicationvault.IndexFactory
	newVaultCatalog            applicationvault.CatalogFactory
	fileOperations             vaultfileapp.FileOperationPort
	imageCacheStore            images.CacheStore
	imageLocalFiles            images.LocalFiles
	temporaryImageStore        documentsave.TemporaryImageStore
	openExternal               func(string) error
	workspaceEvents            *events.Dispatcher[workspace.WorkspaceRestored]
	workspaceRestorer          *workspaceapp.Restorer
	workspaceSubscription      *events.Subscription
	documents                  *documentdialogs.Handle
	migrationHost              *documentdialogs.MigrationHost
	exportMenu                 *dialogsui.ExportController
	exportHost                 *dialogsui.ExportHost
	vault                      *applicationvault.Session
	vaultSnapshot              applicationvault.Snapshot
	pendingLinks               bool
	pendingWiki                string
	workspaceSearch            *searchui.Host
	indexedPath                string
	indexedOffset              int
	indexedRevision            string
	win                        *mygo.Window
	windows                    *hostingui.Manager
	workspaceSession           *workspaceapp.Session
	workspaceHost              *workspaceui.Host
	workbench                  *shellui.WorkbenchHost
	workspaceLifecycle         *shellui.WorkspaceLifecycle
	workspaceState             workspace.State
	activePane                 workspace.PaneID
	imagePreview               imagepreviewui.Host
	imageDocument              *imagedocumentui.Controller
	documentSave               *noteeditor.SaveActions
	quickOpen                  *quickopenui.Controller
	mermaidPreviewHandle       hostingui.Handle
	backlinksHandle            hostingui.Handle
	linkPreview                linkpreview.Host
	navIconPicker              *naviconpicker.Host

	// commandpalette owns its transient query, selection, and open state.
	commandPalette *commandpalette.Host
	fileDialogs    *filedialogs.Host

	// appearance
	navigationHistory *workspaceapp.DocumentHistory
	// flat 是"Anytype 那种扁平"：编辑器不再是一张浮起的卡片，没有描边、
	// 没有圆角、没有投影，和窗口同色，页面直接铺满。关掉就是现在的卡片。
	flat bool
	// headerFade 让扁平风格下的头部浮在文档上：页面滚到它下面时模糊、
	// 淡出，而不是被头部的边缘切断（见 header_fade.go）。关掉就退回一
	// 块不透明的头部底色。
	// The component cache retains its shader resources for this window lifetime.
	headerFadeCache windowcontrols.FadeCache
	themeApplied    int

	// context panel
	panel      shellui.Surface
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
	directoryCatalog        *vaultfileapp.DirectoryCatalog
	fileTreeState           *explorer.Controller
	treeDragItem            dragdrop.VaultItem
	treeNativeHandoff       bool
	nativeDropHandled       bool
	treeNativeDropSuggested transfer.Operation
	outlinePanel            *outlineui.Handle
	findReplace             *findreplaceui.Host
	selectedPath            string
	focusedPanel            commands.PanelID
	treeFocusHandle         ui.Handle
	treeFocusRequested      bool
	panelKeySequences       *keybindings.KeySequenceMatcher
	treeHelpOpen            bool
	panelActions            *commands.ActionDispatcher
	lastPanelAction         commands.ActionID
	propertiesPanel         *propertypanel.Host

	// The active note pane is the sole owner of document identity, source,
	// line-ending policy, selection, and persistence state.
	activeLinkIndex *vaultlinkapp.ActiveLinkIndex

	// settings
	settingsPanel          *settingsui.Host
	mouseCursorHidden      bool
	mouseCursorHiddenUntil time.Time

	// AI
	assistant         *agentui.Host
	newAgentWorkspace func(string, applicationagent.CurrentDocument) (applicationagent.WorkspacePort, error)

	// outline rows

	// status line
	status   string
	statusAt time.Time

	// blocking dialog guard: one native dialog at a time, never on the UI thread
	busy bool

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
	FileOperations            vaultfileapp.FileOperationPort
	RecentItems               *workspaceapp.RecentItems
	NavIcons                  vaulticonapp.NavigationIconRepository
	ImageCacheStore           images.CacheStore
	ImageLocalFiles           images.LocalFiles
	TemporaryImages           documentsave.TemporaryImageStore
	NewWatcher                func(root string) (applicationvault.WatcherPort, error)
	NewAgentWorkspace         func(string, applicationagent.CurrentDocument) (applicationagent.WorkspacePort, error)
	OpenExternal              func(string) error
}

// New returns the app on an empty untitled document, with the panel on files.
func New() *App { return NewWithDependencies(Dependencies{}) }

// NewWithDependencies is the NativeUI composition constructor. Persistence,
// repository, and platform capabilities are injected before any feature host
// or workspace restoration lifecycle is created.
func NewWithDependencies(deps Dependencies) *App {
	a := &App{
		configRepository:          deps.Configuration,
		assistantSettingsLocation: deps.AssistantSettingsLocation,
		agentWorkspaceEvents:      events.New[applicationagent.WorkspaceMutation](),
		newWatcher:                deps.NewWatcher,
		documentRepository:        deps.Documents,
		documentRecovery:          deps.Recovery,
		documentHistory:           deps.History,
		documentExportWriter:      deps.DocumentExportWriter,
		metadataReader:            deps.DocumentMetadata,
		documentEvents:            deps.DocumentEvents,
		vaultSearchReconciler:     deps.VaultSearchReconciler,
		newVaultIndex:             deps.NewVaultIndex,
		newVaultCatalog:           deps.NewVaultCatalog,
		fileOperations:            deps.FileOperations,
		imageCacheStore:           deps.ImageCacheStore,
		imageLocalFiles:           deps.ImageLocalFiles,
		temporaryImageStore:       deps.TemporaryImages,
		openExternal:              deps.OpenExternal,
		workspaceEvents:           deps.WorkspaceEvents,
		workspaceRestorer:         workspaceapp.NewRestorer(deps.Workspace, deps.WorkspaceEvents),
		recentItems:               deps.RecentItems,
		newAgentWorkspace:         deps.NewAgentWorkspace,
		panel:                     shellui.SurfaceFiles,
		focusedPanel:              commands.PanelIDEditor,
		panelOpen:                 true,
		flat:                      true,
		panelWidth:                268,
		panelW:                    268,
		themeApplied:              -1,
		directoryCatalog:          vaultfileapp.NewDirectoryCatalog(),
		navigationHistory:         workspaceapp.NewDocumentHistory(),
		fileTreeState:             explorer.NewController(),
		outlinePanel:              outlineui.NewHandle(),
		workspaceSearch:           searchui.NewHost(nil),
		commandPalette:            commandpalette.NewHost(),
		activeLinkIndex:           &vaultlinkapp.ActiveLinkIndex{},
		documents:                 documentdialogs.NewHandle(),
	}
	a.findReplace = findreplaceui.NewHost(findreplaceui.Options{
		Handle: findreplaceui.NewHandle(),
		Ports: findreplaceui.Ports{
			Editor: findreplaceui.NewEditorAdapter(
				func() noteeditor.Commands { return a.noteEditor() },
				func(err error) { a.notice(err.Error()) },
			),
			Invalidate: a.invalidate,
		},
	})
	a.fileDialogs = filedialogs.NewHost(filedialogs.Ports{
		Busy: func() bool { return a.busy },
		Choose: func(_ context.Context, request filedialogs.Request) ([]string, error) {
			options := mygo.OpenDialogOptions{Parent: a.win, Title: request.Title, Directory: request.Kind == filedialogs.SelectFolder}
			if request.Kind == filedialogs.SelectFile && request.FilterName != "" {
				options.Filters = []mygo.FileFilter{{Name: request.FilterName, Extensions: append([]string(nil), request.Extensions...)}}
			}
			return openDialog(options)
		},
		ChooseSave: func(_ context.Context, request filedialogs.Request) (string, error) {
			options := mygo.SaveDialogOptions{Parent: a.win, Title: request.Title, DefaultPath: request.DefaultPath}
			if request.FilterName != "" {
				options.Filters = []mygo.FileFilter{{Name: request.FilterName, Extensions: append([]string(nil), request.Extensions...)}}
			}
			return saveDialog(options)
		},
		Start: func() { a.onMain(func() { a.busy = true; a.invalidate() }) },
		Finish: func(callback func()) {
			a.onMain(func() {
				a.busy = false
				if callback != nil {
					callback()
				}
				a.invalidate()
			})
		},
		OnFile:   func(path string) { a.requestOpen(path) },
		OnFolder: func(path string) { a.setWorkspace(path) },
		OnSave: func(path string) {
			if a.documentSave != nil {
				a.documentSave.CommitSaveAs(path)
			}
		},
	})
	a.agentWorkspaceMutations = applicationagent.NewWorkspaceMutationService(
		a.documentRepository, a.documentRecovery, a.documentEvents,
		func(root string) *vaultfileapp.FileUseCases {
			return vaultfileapp.NewFileUseCases(root, a.fileOperations, a.catalogIsDirectory)
		}, a.agentWorkspaceEvents,
	)
	a.agentWorkspaceSubscription = a.agentWorkspaceEvents.Subscribe(a.handleAgentWorkspaceMutation)
	a.assistant = newAssistantHost(a)
	a.exportHost = dialogsui.NewExportHost(dialogsui.ExportPorts{
		Busy:              func() bool { return a.busy },
		ImageDocumentOpen: a.imageDocumentOpen,
		Document: func() dialogsui.ExportDocument {
			path, source := a.documentPath(), a.documentSource()
			return dialogsui.ExportDocument{Title: frontmatter.Title(source, path), SourcePath: path, Source: source}
		},
		Spec: func(format dialogsui.ExportFormat, doc dialogsui.ExportDocument) dialogsui.ExportSpec {
			appFormat := applicationExportFormat(format)
			baseName := documentsave.ExportBaseName(doc.Title, doc.SourcePath)
			if baseName == locales.DocumentUntitled() {
				baseName = "Untitled"
			}
			label := exportFormatLabel(format)
			extension := appFormat.Extension()
			return dialogsui.ExportSpec{Label: label, Extension: extension, DefaultName: baseName + "." + extension}
		},
		ChoosePath: func(_ context.Context, spec dialogsui.ExportSpec) (string, error) {
			return saveDialog(mygo.SaveDialogOptions{
				Parent: a.win, Title: spec.Label, DefaultPath: spec.DefaultName,
				Filters: []mygo.FileFilter{{Name: spec.Label, Extensions: []string{spec.Extension}}},
			})
		},
		Prepare: func(ctx context.Context, request dialogsui.ExportRequest) ([]byte, error) {
			appFormat := applicationExportFormat(request.Format)
			return documentsave.PrepareExport(ctx, documentsave.ExportRequest{
				Format: appFormat, Title: request.Title, Source: request.Source,
				BaseURL: documentsave.FileBaseURL(request.SourcePath), Language: i18n.Locale(),
			}, dialogsui.MyGoPDFRenderer{})
		},
		Write: func(ctx context.Context, path string, data []byte) error {
			if a.documentExportWriter == nil {
				return errors.New("document export writer is unavailable")
			}
			return a.documentExportWriter.WriteExport(ctx, path, data)
		},
		StartBusy: func() {
			a.busy = true
			a.invalidate()
		},
		Finish: func(callback func()) {
			a.onMain(func() {
				a.busy = false
				if callback != nil {
					callback()
				}
				a.invalidate()
			})
		},
		Go:               func(work func()) { go work() },
		Notify:           a.notice,
		SavedMessage:     locales.DocumentExported,
		SaveErrorMessage: func(err error) string { return locales.CommonSaveError(err.Error()) },
	})
	a.exportMenu = dialogsui.NewExportController(func(action dialogsui.ExportAction) {
		a.exportHost.Select(action.Format)
	})
	a.migrationHost = documentdialogs.NewMigrationHost(a.documents, documentdialogs.MigrationPorts{
		Source: a.coreMigrationSource,
		IsCurrent: func(source documentdialogs.MigrationSource) bool {
			return a.vault != nil && coreMigrationToken(a.vault) == source.Token
		},
		Preview: func(root, openPath string, candidates []documentsave.CoreMetadataCandidate) (documentsave.CoreMetadataPlan, error) {
			migration := documentsave.NewCoreMetadataMigration(a.metadataReader, a.documentRepository, a.documentRecovery, a.documentEvents)
			return migration.Preview(root, openPath, candidates)
		},
		Apply: func(plan documentsave.CoreMetadataPlan, openPath string) (documentsave.CoreMetadataApplyResult, error) {
			migration := documentsave.NewCoreMetadataMigration(a.metadataReader, a.documentRepository, a.documentRecovery, a.documentEvents)
			return migration.Apply(plan, openPath)
		},
		Refresh: func(source documentdialogs.MigrationSource, paths []string) {
			if a.vault == nil || coreMigrationToken(a.vault) != source.Token {
				return
			}
			for _, path := range paths {
				_ = a.vault.Refresh(path)
			}
		},
		Go:         func(work func()) { go work() },
		OnMain:     a.onMain,
		Notify:     a.notice,
		Invalidate: a.invalidate,
		Applied: func() {
			if a.noteEditor() != nil {
				a.configureActiveNote(0)
			}
		},
	})
	a.windows = hostingui.NewManager(func() {
		if commands := a.noteEditor(); commands != nil {
			if focus := commands.FocusCommands(); focus != nil {
				focus.Focus()
			}
		}
	})
	a.imagePreview = imagepreviewui.NewNativeHost(imagepreviewui.NativePorts{
		Windows: a.windows, ImageCache: a.imageCacheStore, LocalFiles: a.imageLocalFiles,
		DocumentDir: a.docDir,
		CopyImage: func(source string) {
			if editor := a.noteEditor(); editor != nil {
				editor.ClipboardCommands().CopyImageObject(source)
			}
		},
		OnMain: a.onMain, Invalidate: a.invalidate,
		OnError: func(err error) { a.notice(err.Error()) },
	})
	a.imageDocument = newImageDocumentController(a)
	a.settingsPanel = settingsui.NewNativeHost(settingsui.NativePorts{
		Configuration: deps.Configuration,
		Assistant:     a.assistant,
		SettingsPath:  deps.AssistantSettingsLocation,
		Keymap: func() string {
			if commands := a.noteEditor(); commands != nil && commands.ModalCommands() != nil {
				return commands.ModalCommands().Keymap()
			}
			return ""
		},
		ApplyEditor: func(settings settingsui.Editor) {
			if commands := a.noteEditor(); commands != nil && commands.ModalCommands() != nil {
				commands.ModalCommands().SetKeymap(settings.Keymap)
			}
			a.configureActiveNote(0)
			a.invalidate()
		},
		ApplyTheme: func(settingsui.ThemeMode) {
			a.themeApplied = -1
			a.invalidate()
		},
		Notify: a.notice, Invalidate: a.invalidate,
	})
	if err := a.settingsPanel.Load(); err != nil {
		a.notice(locales.CommonSettingSaveError(err.Error()))
	}
	a.propertiesPanel = propertypanel.NewNativeHost(propertypanel.NativePorts{
		Configuration: deps.Configuration,
		CurrentPath:   a.documentPath,
		Document: func() (string, string) {
			return a.documentPath(), a.documentSource()
		},
		ImageDocument: a.imageDocumentOpen,
		Metadata:      a.metadataReader,
		Backlinks: func(string) []propertypanel.BacklinkSource {
			sources := a.currentBacklinkSources()
			result := make([]propertypanel.BacklinkSource, len(sources))
			for i, source := range sources {
				result[i] = propertypanel.BacklinkSource{Path: source.Source, Offset: source.Start, Count: source.Count}
			}
			return result
		},
		Route: a.routePropertyWorkflow, Invalidate: a.invalidate,
	})
	a.navIconPicker = naviconpicker.NewNativeHost(naviconpicker.NativePorts{
		Repository: deps.NavIcons,
		VaultRoot:  func() string { return a.root },
		RelativePath: func(path string) (string, bool) {
			return a.vaultRelativePath(path)
		},
		Invalidate: a.invalidate,
		Notify:     a.notice,
	})
	a.quickOpen = quickopenui.NewNativeHost(quickopenui.NativePorts{
		Vault: a.vault, Windows: a.windows,
		OnOpen: func(path string) {
			a.selectedPath = path
			a.requestOpen(path)
		},
		OnClose: a.invalidate,
	})
	a.linkPreview = linkpreview.NewController(linkpreview.ControllerOptions{Ports: a.linkPreviewPorts()})
	a.initializeWorkspace(deps.Workspace)
	a.documentSave = noteeditor.NewSaveActions(noteeditor.SaveActionPorts{
		Workflows:    a.documentWorkflows,
		CurrentPath:  a.documentPath,
		UntitledName: locales.DocumentUntitled,
		RequestSaveAs: func(defaultPath string) {
			if a.fileDialogs == nil {
				return
			}
			a.fileDialogs.SaveFile(filedialogs.Request{
				Title: locales.CommonSaveFile(), DefaultPath: defaultPath,
				FilterName: locales.CommonTextFiles(), Extensions: doctypes.EditableExtensions(),
			})
		},
		RecordSavedPath: func(path string) {
			if a.workspaceSession == nil {
				return
			}
			plan, err := a.workspaceSession.RecordActiveDocument(workspaceapp.ActiveDocumentSnapshot{Path: path, PaneID: a.focusedPane()})
			if err == nil {
				a.workspaceState, a.activePane = plan.State, plan.PaneID
				_ = a.workspaceRestorer.Persist(plan.State)
			}
		},
		SelectPath:  func(path string) { a.selectedPath = path },
		NotifyError: func(err error) { a.notice(locales.CommonSaveError(err.Error())) },
		NotifySaved: func() { a.notice(locales.DocumentSaved()) },
		Invalidate:  a.invalidate,
	})
	a.initializeWorkbench()
	return a
}

// initializeWorkspace composes the durable workspace session with the generic
// pane host. The application service decides whether to restore documents or
// create the canonical blank tab; the feature host only opens the returned
// pane descriptors.
func (a *App) initializeWorkspace(repository workspace.Repository) {
	if a == nil {
		return
	}
	initial := workspace.State{Version: 1}
	session, err := workspaceapp.NewSession(repository, initial)
	if err != nil {
		panic(err)
	}
	plan, restoreErr := a.workspaceRestorer.Restore()
	if restoreErr != nil {
		plan, err = session.Restore(initial)
	} else {
		plan, err = session.Restore(plan.State)
	}
	if err != nil {
		panic(err)
	}
	a.workspaceSession = session
	a.workspaceState = session.Snapshot()
	if restoredRoot := plan.State.VaultPath; restoredRoot != "" {
		a.root = restoredRoot
	}
	a.workspaceHost = workspaceui.NewHost(workspaceui.Ports{
		Windows: a.windows, Host: workspaceui.MainHost, OpenPane: a.openWorkspacePane,
	})
	a.workspaceLifecycle = shellui.NewWorkspaceLifecycle(shellui.WorkspaceLifecyclePorts{
		Session: session, Restore: a.workspaceHost, CurrentDocument: a.documentPath,
		CloseVault: func() {
			if a.vault == nil {
				return
			}
			old := a.vault
			a.vault = nil
			a.vaultSnapshot = applicationvault.Snapshot{}
			a.directoryCatalog.CloseSession(old)
			go old.Close()
		},
		SetRoot: func(root string) {
			a.root, a.selectedPath = root, ""
			a.directoryCatalog.Invalidate()
			a.panel, a.panelOpen = shellui.SurfaceFiles, true
		},
		Persist: a.workspaceRestorer.Persist,
	})
	if err := a.workspaceHost.OpenRestorePlan(plan); err != nil {
		panic(err)
	}
	a.activePane = a.workspaceHost.FocusedPane(workspace.PaneID(workspaceapp.DefaultPaneID))
	if a.activePane == "" {
		a.activePane = workspace.PaneID(workspaceapp.DefaultPaneID)
	}
	a.workspaceState = session.Snapshot()
	a.configureActiveNote(0)
}

// openWorkspacePane translates one application restore descriptor into a
// feature-owned note surface and a generic window slot.
func (a *App) openWorkspacePane(request workspaceui.PaneRequest) (hostingui.Handle, error) {
	if a == nil || a.windows == nil {
		return hostingui.Handle{}, errors.New("nativeui: workspace window host is unavailable")
	}
	note := document.NewNote("", "", document.RevisionOf(""))
	if request.Path != "" {
		if a.documentRepository == nil {
			return hostingui.Handle{}, errors.New("nativeui: document repository is unavailable")
		}
		loaded, err := a.documentRepository.Read(request.Path)
		if err != nil {
			return hostingui.Handle{}, err
		}
		note = loaded
	}
	content, err := noteeditor.NewNativeContent(request.PaneID, note, request.CursorOffset,
		noteeditor.Dependencies{
			Documents: a.documentRepository, Recovery: a.documentRecovery,
			History: a.documentHistory, Events: a.documentEvents,
		}, noteeditor.NativeEditorOptions{Editor: a.editorOptions()}, func(action noteeditor.Action) {
			if action.Kind == noteeditor.DocumentChanged && a.activeLinkIndex != nil {
				a.activeLinkIndex.ScheduleDocument(action.Snapshot.Path, action.Snapshot.DiskSource, time.Now())
			}
			a.invalidate()
		})
	if err != nil {
		return hostingui.Handle{}, err
	}
	if modal := content.ModalCommands(); modal != nil && a.settingsPanel != nil {
		modal.SetKeymap(a.settingsPanel.Runtime().Editor.Keymap)
	}
	handle, err := a.windows.Open(content, hostingui.Placement{
		Host: workspaceui.MainHost, Pane: hostingui.PaneID(request.PaneID), Layer: hostingui.PaneContent,
	})
	if err != nil {
		content.Close()
		return hostingui.Handle{}, err
	}
	if request.Focused {
		a.activePane = request.PaneID
		a.configureActiveNote(0)
	}
	return handle, nil
}

func (a *App) editorOptions() editorui.Options {
	options := editorui.Options{
		MaxWidth: 800, PadX: 56,
		PreserveTypedLineBreaks: true,
		Placeholder:             locales.DocumentStartWriting(),
		Label:                   locales.DocumentDefaultLabel(),
		ImageCacheStore:         a.imageCacheStore, ImageLocalFiles: a.imageLocalFiles,
		Invalidate: a.invalidate, OnLink: a.openLink,
		ResolveImage:              a.resolveVaultImage,
		WikiLinkSuggestions:       a.wikiLinkSuggestions,
		WikiLinkSuggestionDetails: a.wikiLinkSuggestionDetails,
		OnWikiLinkHover:           a.hoverWikiLinkAt,
		ResolveWikiEmbed:          a.resolveWikiEmbed,
		ResolveWikiLink:           a.resolveWikiLinkStyle,
		OnImagePreview:            a.openImagePreview,
		OnDiagramPreview:          a.openMermaidPreview,
		SaveImage:                 a.saveEditorImage,
		OnDropFiles:               a.openDroppedDocuments,
		OnVaultItemDragOver: func() {
			if a.fileTreeState != nil {
				a.fileTreeState.SetDragOperationDisplay(explorer.DragOperationDisplay{Kind: explorer.DragOperationInsertIntoEditor})
			}
		},
	}
	if a.flat {
		options.TopInset = editorHeaderH
	}
	if a == nil || a.settingsPanel == nil {
		return options
	}
	runtime := a.settingsPanel.Runtime()
	options.FontSize = float32(runtime.Appearance.FontSize)
	options.LineHeight = float32(runtime.Appearance.LineHeight)
	options.DisableSmartTypography = !runtime.Appearance.SmartTypography
	options.SmoothPageJumps = runtime.Appearance.SmoothPageJumps
	options.HideMouseCursor = runtime.Editor.HideMouseCursor
	options.CursorFade = runtime.Editor.CursorFade
	options.SmoothCursor = runtime.Editor.SmoothCursor
	options.HeadingMarkerReveal = runtime.Editor.HeadingMarkerReveal
	options.VimVirtualImageRows = runtime.Editor.VimVirtualImageRows
	options.VimYankFlash = runtime.Editor.VimYankFlash
	options.SearchMatchPulse = runtime.Editor.SearchMatchPulse
	options.BaseDir, options.AssetRoot = a.docDir(), a.root
	if path := a.documentPath(); path != "" {
		options.CurrentPath = path
		if !doctypes.IsMarkdownPath(path) {
			options.Code = strings.TrimPrefix(filepath.Ext(path), ".")
		}
	}
	if a.propertiesPanel != nil && a.propertiesPanel.Enabled() {
		options.Header = func(c *ui.Context) { a.propertiesPanel.RenderSummary(c) }
		options.HeaderHeight = 160
		options.HideFrontmatter = !a.propertiesPanel.IsSourceMode() && a.propertiesPanel.FrontmatterValid()
	}
	return options
}

// noteEditor resolves the focused pane's feature-owned command port for use
// on the UI owner thread. The generic window manager remains the owner of the
// live handle and does not expose renderer state to App.
func (a *App) noteEditor() noteeditor.Commands {
	if a == nil || a.workspaceHost == nil || a.windows == nil {
		return nil
	}
	pane := a.workspaceHost.FocusedPane(a.activePane)
	handle := a.workspaceHost.PaneHandle(pane)
	var commands noteeditor.Commands
	_ = noteeditor.WithCommands(a.windows, handle, func(value noteeditor.Commands) error {
		commands = value
		return nil
	})
	return commands
}

func (a *App) documentSnapshot() noteeditor.Snapshot {
	if commands := a.noteEditor(); commands != nil {
		return commands.Snapshot()
	}
	return noteeditor.Snapshot{}
}

func (a *App) documentPath() string   { return a.documentSnapshot().Path }
func (a *App) documentSource() string { return a.documentSnapshot().Source }
func (a *App) diskSource() string     { return a.documentSnapshot().DiskSource }

func (a *App) docDir() string {
	if path := a.documentPath(); path != "" {
		return filepath.Dir(path)
	}
	return a.root
}

func (a *App) focusedPane() workspace.PaneID {
	if a == nil {
		return ""
	}
	if a.workspaceHost != nil {
		return a.workspaceHost.FocusedPane(a.activePane)
	}
	return a.activePane
}

func (a *App) openFileDialog() {
	if a == nil || a.fileDialogs == nil {
		return
	}
	if a.root == "" {
		a.openWorkspaceSearch()
		return
	}
	a.fileDialogs.OpenFile(filedialogs.Request{
		Title: locales.CommonDialogFileTitle(), FilterName: locales.CommonDialogOpenFileFilter(),
		Extensions: append(doctypes.EditableExtensions(), doctypes.ImageExtensions()...),
	})
}

func (a *App) openFolderDialog() {
	if a == nil || a.fileDialogs == nil {
		return
	}
	a.fileDialogs.OpenFolder(filedialogs.Request{Title: locales.CommonDialogFolderTitle()})
}

func (a *App) setWorkspace(root string) {
	if a == nil || a.workspaceLifecycle == nil {
		return
	}
	if err := a.workspaceLifecycle.SwitchVault(root); err != nil {
		a.notice(err.Error())
		return
	}
	a.workspaceState = a.workspaceSession.Snapshot()
	a.activePane = a.workspaceHost.FocusedPane(a.activePane)
	a.ensureVaultSession()
	a.invalidate()
}

func (a *App) openWorkspaceSearch() {
	if a == nil || a.workspaceSearch == nil {
		return
	}
	a.closeQuickOpen()
	if a.commandPalette != nil {
		a.commandPalette.Close()
	}
	if a.root == "" && a.documentPath() != "" {
		a.root = filepath.Dir(a.documentPath())
	}
	a.workspaceSearch.Reset()
	a.workspaceSearch.Open()
	a.invalidate()
}

func (a *App) togglePanel(surface shellui.Surface) {
	if a == nil {
		return
	}
	if a.panelOpen && a.panel == surface {
		a.panelOpen = false
		a.panelW = 0
	} else {
		a.panel, a.panelOpen = surface, true
		if a.panelWidth <= 0 {
			a.panelWidth = 268
		}
		a.panelW = a.panelWidth
	}
	a.invalidate()
}

func (a *App) navigateIndexed(path string, offset int) {
	if a == nil || path == "" || a.root == "" {
		return
	}
	full := filepath.Join(a.root, filepath.FromSlash(path))
	if filepath.Clean(full) == filepath.Clean(a.documentPath()) {
		if editor := a.noteEditor(); editor != nil {
			editor.NavigationCommands().ScrollToOffset(editorsource.OffsetInNormalizedSource(a.diskSource(), offset))
		}
		return
	}
	a.indexedPath, a.indexedOffset = full, offset
	a.requestOpen(full)
	if editor := a.noteEditor(); editor != nil && filepath.Clean(editor.Snapshot().Path) == filepath.Clean(full) {
		editor.NavigationCommands().ScrollToOffset(editorsource.OffsetInNormalizedSource(editor.Snapshot().DiskSource, offset))
	}
}

func (a *App) documentWorkflows() documentsave.SessionWorkflows {
	commands := a.noteEditor()
	return documentsave.SessionWorkflows{
		Repository: a.documentRepository, Recovery: a.documentRecovery, History: a.documentHistory,
		Editing: noteeditor.WorkflowEditingPort(commands),
		Save: func(path, source string, expected document.Revision) error {
			if commands == nil {
				return documentsave.ErrWorkflowUnavailable
			}
			_, err := commands.SaveExact(path, source, expected)
			return err
		},
	}
}

func (a *App) save() {
	if a != nil && a.documentSave != nil {
		a.documentSave.Save()
	}
}

func (a *App) newDoc() {
	if a == nil || a.noteEditor() == nil {
		return
	}
	commands := a.noteEditor()
	if commands.Snapshot().Dirty {
		if _, err := commands.PreserveDraft(); err != nil {
			a.notice(err.Error())
			return
		}
	}
	if err := commands.Open(document.NewNote("", "", document.RevisionOf("")), 0); err != nil {
		a.notice(err.Error())
		return
	}
	if a.workspaceSession != nil {
		if plan, err := a.workspaceSession.RecordActiveDocument(workspaceapp.ActiveDocumentSnapshot{PaneID: a.focusedPane()}); err == nil {
			a.workspaceState, a.activePane = plan.State, plan.PaneID
			_ = a.workspaceRestorer.Persist(plan.State)
		}
	}
	a.selectedPath = ""
	a.invalidate()
}

func (a *App) catalogIsDirectory(path string) bool {
	return a != nil && a.vault != nil && path != "" &&
		a.vaultFileUseCases().IsCatalogDirectory(a.vault, path)
}

func (a *App) newFileIn(directory string) {
	if a == nil {
		return
	}
	explorer.HandleAction(explorer.Action{Kind: explorer.CreateFile, Directory: directory}, explorer.ActionPorts{
		CreateNote: func(directory string) (string, error) {
			source, err := documentsave.NewNoteSource(time.Now())
			if err != nil {
				return "", err
			}
			return a.vaultFileUseCases().CreateFile(vaultfileapp.CreateFileCommand{
				Directory: directory, Name: locales.DocumentUntitled(), Extension: ".md", Content: []byte(source),
			})
		},
		Open:       a.requestOpen,
		OnError:    func(err error) { a.notice(locales.CommonCreateFailed(err.Error())) },
		Invalidate: a.invalidate,
	})
}

func (a *App) newFolderIn(directory string) {
	if a == nil {
		return
	}
	explorer.HandleAction(explorer.Action{Kind: explorer.CreateFolder, Directory: directory}, explorer.ActionPorts{
		CreateFolder: func(directory string) error {
			_, err := a.vaultFileUseCases().CreateFolder(vaultfileapp.CreateFolderCommand{
				Directory: directory, Name: locales.FilesNewFolderDefault(),
			})
			return err
		},
		OnError:    func(err error) { a.notice(locales.CommonCreateFailed(err.Error())) },
		Invalidate: a.invalidate,
	})
}

func (a *App) vaultRelativePath(path string) (string, bool) {
	if a == nil || a.root == "" || path == "" {
		return "", false
	}
	root, err := filepath.Abs(a.root)
	if err != nil {
		return "", false
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", false
	}
	relative, err := filepath.Rel(root, absolute)
	if err != nil || relative == "." || relative == ".." || filepath.IsAbs(relative) || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", false
	}
	return filepath.ToSlash(relative), true
}

// requestOpen routes a selected text document through application/workspace
// state and asks the workspace host to replace the focused pane surface.
func (a *App) requestOpen(path string) {
	if a == nil || strings.TrimSpace(path) == "" {
		return
	}
	if doctypes.IsImagePath(path) {
		a.openImageDocument(path, true)
		return
	}
	if a.workspaceSession == nil || a.workspaceHost == nil {
		return
	}
	pane := a.focusedPane()
	if pane == "" {
		pane = workspace.PaneID(workspaceapp.DefaultPaneID)
	}
	previous := a.workspaceSession.Snapshot()
	plan, err := a.workspaceSession.RecordActiveDocument(workspaceapp.ActiveDocumentSnapshot{Path: path, PaneID: pane})
	if err != nil {
		a.notice(err.Error())
		return
	}
	request := workspaceui.PaneRequest{TabID: plan.TabID, PaneID: plan.PaneID, Path: path, Focused: true}
	if _, err := a.workspaceHost.OpenPane(request); err != nil {
		_, _ = a.workspaceSession.Restore(previous)
		a.notice(err.Error())
		return
	}
	a.workspaceState, a.activePane = plan.State, plan.PaneID
	a.selectedPath = path
	a.configureActiveNote(0)
	a.queueLinkIndexSnapshot()
	if err := a.workspaceRestorer.Persist(plan.State); err != nil && !errors.Is(err, workspaceapp.ErrUnavailable) {
		a.notice(err.Error())
	}
	a.invalidate()
}

func (a *App) configureActiveNote(_ int) {
	if a == nil || a.settingsPanel == nil {
		return
	}
	commands := a.noteEditor()
	if commands == nil {
		return
	}
	settings := a.editorOptions()
	current := commands.PresentationCommands().Settings()
	if modal := commands.ModalCommands(); modal != nil {
		modal.SetKeymap(a.settingsPanel.Runtime().Editor.Keymap)
	}
	current.FontSize, current.LineHeight = settings.FontSize, settings.LineHeight
	current.DisableSmartTypography = settings.DisableSmartTypography
	current.HeadingMarkerReveal, current.VimVirtualImageRows = settings.HeadingMarkerReveal, settings.VimVirtualImageRows
	current.CursorFade, current.SmoothCursor, current.HideMouseCursor = settings.CursorFade, settings.SmoothCursor, settings.HideMouseCursor
	current.SmoothPageJumps, current.SearchMatchPulse, current.VimYankFlash = settings.SmoothPageJumps, settings.SearchMatchPulse, settings.VimYankFlash
	current.BaseDir, current.AssetRoot, current.CurrentPath, current.Code = settings.BaseDir, settings.AssetRoot, settings.CurrentPath, settings.Code
	current.Header, current.HeaderHeight, current.HideFrontmatter = settings.Header, settings.HeaderHeight, settings.HideFrontmatter
	current.TopInset = 0
	if a.flat {
		current.TopInset = settings.TopInset
	}
	commands.PresentationCommands().Configure(current)
}

func (a *App) configureEditorTopInset(c *ui.Context, flat bool) {
	commands := a.noteEditor()
	if commands == nil || commands.PresentationCommands() == nil {
		return
	}
	settings := commands.PresentationCommands().Settings()
	inset := float32(0)
	if flat {
		inset = a.headerH(c)
	}
	if settings.TopInset == inset {
		return
	}
	settings.TopInset = inset
	commands.PresentationCommands().Configure(settings)
}

func (a *App) saveEditorImage(data []byte, ext string) (string, error) {
	if a == nil {
		return "", errors.New("editor image owner is unavailable")
	}
	if ext == "" {
		ext = ".png"
	}
	directory := a.docDir()
	if directory == "" {
		if a.temporaryImageStore == nil {
			return "", errors.New("temporary image store is unavailable")
		}
		path, err := a.temporaryImageStore.SaveTemporaryImage(data, ext)
		return filepath.ToSlash(path), err
	}
	root := a.root
	useCases := a.vaultFileUseCases()
	if _, ok := a.vaultRelativePath(directory); !ok {
		root = directory
		useCases = vaultfileapp.NewFileUseCases(root, a.fileOperations, nil)
	}
	path, err := useCases.CreateAttachment(vaultfileapp.CreateAttachmentCommand{
		Directory: filepath.Join(directory, "assets"),
		Name:      "image-" + time.Now().Format("20060102-150405.000000000"),
		Extension: ext,
		Content:   data,
	})
	if err != nil {
		return "", err
	}
	relative, err := filepath.Rel(directory, path)
	if err != nil {
		return "", err
	}
	return filepath.ToSlash(relative), nil
}

func (a *App) openDroppedDocuments(paths []string) {
	if a == nil {
		return
	}
	for _, path := range paths {
		if info, err := os.Stat(path); err == nil && info.IsDir() {
			a.setWorkspace(path)
			return
		}
		if doctypes.IsEditablePath(path) {
			a.requestOpen(path)
			return
		}
	}
}

// SetDependencies exists for in-process NativeUI fixtures that need to replace
// a single injected capability after constructing an otherwise headless App.
// Production startup uses NewWithDependencies so construction observes one
// complete dependency set.
func (a *App) SetDependencies(deps Dependencies) {
	if a == nil {
		return
	}
	if deps.Configuration != nil {
		a.configRepository = deps.Configuration
	}
	if deps.AssistantSettingsLocation != "" {
		a.assistantSettingsLocation = deps.AssistantSettingsLocation
	}
	if deps.NewWatcher != nil {
		a.newWatcher = deps.NewWatcher
	}
	if deps.Documents != nil {
		a.documentRepository = deps.Documents
	}
	if deps.DocumentExportWriter != nil {
		a.documentExportWriter = deps.DocumentExportWriter
	}
	if deps.DocumentMetadata != nil {
		a.metadataReader = deps.DocumentMetadata
	}
	if deps.Recovery != nil {
		a.documentRecovery = deps.Recovery
	}
	if deps.History != nil {
		a.documentHistory = deps.History
	}
	if deps.DocumentEvents != nil {
		a.documentEvents = deps.DocumentEvents
	}
	if deps.VaultSearchReconciler != nil {
		a.vaultSearchReconciler = deps.VaultSearchReconciler
	}
	if deps.NewVaultIndex != nil {
		a.newVaultIndex = deps.NewVaultIndex
	}
	if deps.NewVaultCatalog != nil {
		a.newVaultCatalog = deps.NewVaultCatalog
	}
	if deps.FileOperations != nil {
		a.fileOperations = deps.FileOperations
	}
	if deps.RecentItems != nil {
		a.recentItems = deps.RecentItems
	}
	if deps.NavIcons != nil {
		a.navIconPicker = naviconpicker.NewNativeHost(naviconpicker.NativePorts{
			Repository: deps.NavIcons, VaultRoot: func() string { return a.root },
			RelativePath: func(path string) (string, bool) { return a.vaultRelativePath(path) },
			Invalidate:   a.invalidate, Notify: a.notice,
		})
	}
	if deps.ImageCacheStore != nil {
		a.imageCacheStore = deps.ImageCacheStore
	}
	if deps.ImageLocalFiles != nil {
		a.imageLocalFiles = deps.ImageLocalFiles
	}
	if deps.TemporaryImages != nil {
		a.temporaryImageStore = deps.TemporaryImages
	}
	if deps.NewAgentWorkspace != nil {
		a.newAgentWorkspace = deps.NewAgentWorkspace
	}
	if deps.OpenExternal != nil {
		a.openExternal = deps.OpenExternal
	}
	if deps.Workspace != nil {
		a.workspaceRestorer = workspaceapp.NewRestorer(deps.Workspace, deps.WorkspaceEvents)
	}
	if deps.WorkspaceEvents != nil {
		a.workspaceEvents = deps.WorkspaceEvents
	}
	a.invalidate()
}

// invalidate schedules a redraw when the root window is live. Before the
// shell creates a window there is no presentation tree to invalidate.
func (a *App) invalidate() {
	if a != nil && a.win != nil && !a.win.IsDestroyed() {
		a.win.Invalidate()
	}
}

// onMain executes root-owned UI state changes on MyGo's owner thread. Headless
// tests have no native event loop, so they serialize callbacks with mainMu.
func (a *App) onMain(fn func()) {
	if a == nil || fn == nil {
		return
	}
	if a.win != nil {
		mygo.RunOnMain(fn)
		return
	}
	a.mainMu.Lock()
	defer a.mainMu.Unlock()
	fn()
}

// notice records short-lived shell feedback. The shell overlay reads this
// value from the root's frame snapshot.
func (a *App) notice(message string) {
	if a == nil || message == "" {
		return
	}
	a.status = message
	a.statusAt = time.Now()
	a.invalidate()
}

func (a *App) openSettings() {
	if a != nil && a.settingsPanel != nil {
		a.settingsPanel.OpenAt(settingsui.AppearancePage)
		a.invalidate()
	}
}

func (a *App) settingsDialog(c *ui.Context) {
	if a != nil && a.settingsPanel != nil {
		a.settingsPanel.RenderDialog(c)
	}
}

func (a *App) routePropertyWorkflow(request propertypanel.WorkflowRequest) error {
	if a == nil {
		return propertypanel.ErrUnavailable
	}
	switch request.Kind {
	case propertypanel.SetValue, propertypanel.AddValue, propertypanel.DeleteValue, propertypanel.ReplaceSource:
		if a.noteEditor() == nil {
			return propertypanel.ErrUnavailable
		}
		metadata := documentsave.NewMetadataEditor(a.noteEditor())
		switch request.Kind {
		case propertypanel.SetValue:
			return metadata.SetProperty(request.FieldKey, request.Value)
		case propertypanel.AddValue:
			return metadata.AddProperty(request.Add.Key, request.Add.Value, appPropertyValueKind(request.Add.Kind))
		case propertypanel.DeleteValue:
			return metadata.DeleteProperty(request.FieldKey)
		default:
			return metadata.ReplaceYAML(request.Value)
		}
	case propertypanel.NavigateBacklink:
		a.navigateIndexed(request.Backlink.Path, request.Backlink.Offset)
	case propertypanel.NavigateBacklinks:
		a.openLinks()
	case propertypanel.OpenInspector:
		a.propertiesPanel.Open()
	case propertypanel.OpenMigration:
		if a.migrationHost != nil {
			a.migrationHost.OpenPreview()
		}
	}
	return nil
}

func (a *App) currentBacklinkSources() []vaultlinks.BacklinkSource {
	if a == nil || a.vault == nil || !a.vaultSnapshot.Ready {
		return nil
	}
	path := a.documentPath()
	if a.imageDocumentOpen() && a.imageDocumentPath() != "" {
		path = a.imageDocumentPath()
	}
	if path == "" {
		return nil
	}
	return a.vault.BacklinkSourcesForDocument(path)
}

func (a *App) openCommandPalette() {
	if a == nil || a.commandPalette == nil {
		return
	}
	a.commandPalette.Open(commandpalette.Catalog(a.settingsPanel != nil && a.settingsPanel.Runtime().AIEnabled))
	a.invalidate()
}

func (a *App) closeCommandPalette() {
	if a != nil && a.commandPalette != nil {
		a.commandPalette.Close()
		a.invalidate()
	}
}

func (a *App) dispatchCommandPaletteAction(action commandpalette.Action) {
	if a == nil || a.commandPalette == nil {
		return
	}
	routed := a.commandPalette.Apply(action)
	switch routed.Kind {
	case commandpalette.Execute:
		a.runPaletteCommand(routed.ID)
	case commandpalette.Dismiss:
		a.invalidate()
	case 0:
		if action.Kind == commandpalette.QueryChanged || action.Kind == commandpalette.MoveSelection {
			a.invalidate()
		}
	}
}

func (a *App) runPaletteCommand(id commandpalette.CommandID) {
	switch id {
	case commandpalette.DocumentNew:
		a.newDoc()
	case commandpalette.DocumentSave:
		a.save()
	case commandpalette.DocumentSaveAs:
		if a.documentSave != nil {
			a.documentSave.RequestSaveAs()
		}
	case commandpalette.WorkspaceSearch, commandpalette.WorkspaceSearchAll:
		a.openWorkspaceSearch()
	case commandpalette.WorkspaceQuickOpen:
		a.openQuickOpen()
	case commandpalette.DocumentOpenFile:
		a.openFileDialog()
	case commandpalette.WorkspaceOpenFolder:
		a.openFolderDialog()
	case commandpalette.WorkspaceNewFile:
		a.newFileIn(a.fileTreeState.CreateTarget(a.root, a.selectedPath, a.catalogIsDirectory(a.selectedPath)))
	case commandpalette.WorkspaceNewFolder:
		a.newFolderIn(a.fileTreeState.CreateTarget(a.root, a.selectedPath, a.catalogIsDirectory(a.selectedPath)))
	case commandpalette.PanelFiles:
		a.togglePanel(shellui.SurfaceFiles)
	case commandpalette.PanelOutline:
		a.togglePanel(shellui.SurfaceOutline)
	case commandpalette.AppSettings:
		a.openSettings()
	case commandpalette.PanelAssistant:
		a.togglePanel(shellui.SurfaceAgent)
	default:
		// The host emits Execute only for an ID in its visible candidate list.
		a.invalidate()
	}
}

func (a *App) commandPaletteDialog(c *ui.Context) {
	if a != nil && a.commandPalette != nil {
		a.commandPalette.Render(c, a.dispatchCommandPaletteAction)
	}
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

func applicationExportFormat(format dialogsui.ExportFormat) documentsave.ExportFormat {
	switch format {
	case dialogsui.ExportHTML:
		return documentsave.ExportHTML
	case dialogsui.ExportPDF:
		return documentsave.ExportPDF
	default:
		return documentsave.ExportMarkdown
	}
}

func exportFormatLabel(format dialogsui.ExportFormat) string {
	switch format {
	case dialogsui.ExportHTML:
		return locales.DocumentExportHTML()
	case dialogsui.ExportPDF:
		return locales.DocumentExportPDF()
	default:
		return locales.DocumentExportMarkdown()
	}
}

func (a *App) coreMigrationSource() documentdialogs.MigrationSource {
	if a == nil || a.vault == nil {
		return documentdialogs.MigrationSource{}
	}
	session := a.vault
	root := a.vaultSnapshot.Root
	if root == "" {
		root = session.Snapshot().Root
	}
	candidates := documentsave.CollectCoreMetadataCandidates(appCoreMetadataSource{session: session, root: root})
	return documentdialogs.MigrationSource{
		Ready: a.vaultSnapshot.Ready, Token: coreMigrationToken(session),
		Root: root, OpenPath: a.documentPath(), Candidates: candidates,
	}
}

type appCoreMetadataSource struct {
	session *applicationvault.Session
	root    string
}

func (s appCoreMetadataSource) RootPath() string { return s.root }

func (s appCoreMetadataSource) Paths() []string {
	if s.session == nil {
		return nil
	}
	return s.session.Paths()
}

func (s appCoreMetadataSource) ReadDocument(relativePath string) (documentsave.CoreMetadataDocument, bool) {
	if s.session == nil {
		return documentsave.CoreMetadataDocument{}, false
	}
	doc, ok := s.session.Document(relativePath)
	if !ok {
		return documentsave.CoreMetadataDocument{}, false
	}
	return documentsave.CoreMetadataDocument{Source: doc.Source, Revision: doc.Revision}, true
}

func coreMigrationToken(session *applicationvault.Session) string {
	if session == nil {
		return ""
	}
	return fmt.Sprintf("%p", session)
}

// renderCoreMetadataMigration is called by the shell's overlay composition.
func (a *App) renderCoreMetadataMigration(c *ui.Context) {
	if a != nil && a.migrationHost != nil {
		a.migrationHost.Render(c)
	}
}

// newAssistantHost is the NativeUI composition root for the agent feature.
// The Host receives feature ports; all App and workspace adaptation remains
// here at the root boundary.
func newAssistantHost(app *App) *agentui.Host {
	if app == nil {
		return nil
	}
	settings := config.DefaultAssistantSettings()
	session := applicationagent.NewSession(settings, nil, applicationagent.Capabilities{
		Tools: applicationagent.Executor{Operations: applicationagent.DynamicWorkspaceOperations{
			Provider: assistantWorkspaceAdapter(app),
		}},
		Policy: applicationagent.DefaultToolPolicy{},
	})
	return agentui.NewHost(agentui.HostOptions{
		Session: session, Settings: settings,
		Ports: agentui.Ports{
			Icon: func() *ui.SVG { return componenticons.AI() },
			RenderMarkdown: func(c *ui.Context, source string, width float32) {
				doc := mdoc.Parse(source)
				markdownui.BlocksView(c, doc.Blocks, assistantMarkdownOptions(app, width))
			},
			OpenSettings:      func() { app.settingsPanel.OpenAt(settingsui.AssistantPage) },
			OpenImageDocument: func(path string) { app.openImageDocument(path, true) },
			OpenDocument:      app.requestOpen,
			InsertAtStart:     app.insertAIContentAtStart,
			InsertAtEnd:       app.insertAIContent,
			SaveSettings: func(settings config.AssistantSettings) error {
				if app.configRepository == nil {
					return applicationagent.ErrUnavailable
				}
				return applicationconfig.NewService(app.configRepository).SaveAssistantSettings(settings)
			},
			RunOnMain: mygo.RunOnMain,
			SetDefaultModel: func(model string) {
				app.onMain(func() {
					settings := app.assistant.Settings()
					if strings.TrimSpace(settings.Model) == "" {
						settings.Model = model
						app.assistant.SetSettings(settings)
						app.invalidate()
					}
				})
			},
			Notify: app.notice, Invalidate: app.invalidate,
		},
	})
}

func (a *App) saveAssistantSettings() error {
	if a == nil || a.assistant == nil {
		return applicationagent.ErrUnavailable
	}
	return a.assistant.SaveSettings()
}

func (a *App) saveAITools() {
	if err := a.saveAssistantSettings(); err != nil {
		a.notice(locales.CommonSaveFailed(err.Error()))
	}
	a.invalidate()
}

func (a *App) setAIService(magpie bool) {
	if a == nil || a.assistant == nil {
		return
	}
	current := a.assistant.Settings()
	updated := a.assistant.SelectProvider(magpie)
	if updated == current {
		return
	}
	a.invalidate()
}

func (a *App) insertAIContent(text string) {
	if a == nil || a.noteEditor() == nil {
		return
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return
	}
	a.noteEditor().FormattingCommands().AppendBlock(text)
	a.notice(locales.DocumentInserted())
	a.invalidate()
}

func (a *App) insertAIContentAtStart(text string) {
	if a == nil || a.noteEditor() == nil {
		return
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return
	}
	a.noteEditor().FormattingCommands().PrependBlock(text)
	a.notice(locales.DocumentInserted())
	a.invalidate()
}

func (a *App) fetchAIModels() {
	if a != nil && a.assistant != nil {
		a.assistant.FetchModels()
	}
}

func assistantMarkdownOptions(app *App, width float32) markdownui.Options {
	if app == nil {
		return markdownui.Options{}
	}
	return markdownui.Options{
		BodyFontSize: 12.5, SoftBreaksAsSpaces: true, ContentWidth: width,
		BaseDir: app.docDir(), AssetRoot: app.root, LocalFiles: app.imageLocalFiles,
		ImageCacheStore: app.imageCacheStore, OnImagePreview: app.openImagePreview,
		Padding: 0, IndentStep: 14,
	}
}

type appAssistantModelProvider struct {
	provider applicationagent.ProviderManagement
}

func (p appAssistantModelProvider) ListModels(ctx context.Context, credentials applicationagent.ProviderCredentials) (applicationagent.ProviderModelList, error) {
	if p.provider == nil {
		return applicationagent.ProviderModelList{}, applicationagent.ErrUnavailable
	}
	return p.provider.ListModels(ctx, credentials)
}

// assistantWorkspaceAdapter binds the application-owned mutation workflow to
// the feature-owned typed workspace capability. UI work is limited to editor
// snapshot/edit access and the marshalled mutation event consumer.
func assistantWorkspaceAdapter(a *App) *agentui.WorkspaceAdapter {
	return agentui.NewWorkspaceAdapter(agentui.WorkspaceAdapterPorts{
		Current: func() (root, path, source string) {
			if a != nil {
				a.onMain(func() { root, path, source = a.root, a.documentPath(), a.documentSource() })
			}
			return
		},
		NewWorkspace: func(root string, current applicationagent.CurrentDocument) (applicationagent.WorkspacePort, error) {
			if a == nil || a.newAgentWorkspace == nil {
				return nil, applicationagent.ErrUnavailable
			}
			return a.newAgentWorkspace(root, current)
		},
		CurrentDocumentDirty: func() bool {
			dirty := false
			if a != nil {
				a.onMain(func() {
					if editor := a.noteEditor(); editor != nil {
						dirty = editor.Snapshot().Dirty
					}
				})
			}
			return dirty
		},
		DestinationRevision: a.agentWorkspaceMutations.DestinationRevision,
		SaveDocument:        a.agentWorkspaceMutations.SaveDocument,
		MoveDocument:        a.agentWorkspaceMutations.MoveDocument,
		TrashDocuments:      a.agentWorkspaceMutations.TrashDocuments,
		CreateFolder: func(root, path string) error {
			return a.agentWorkspaceMutations.CreateFolder(root, path)
		},
		ApplyCurrentDocument: func(source string) error {
			if a == nil {
				return applicationagent.ErrUnavailable
			}
			var err error
			a.onMain(func() {
				if editor := a.noteEditor(); editor == nil || editor.Snapshot().Path == "" && editor.Snapshot().Source == "" {
					err = errors.New("current document is empty")
					return
				} else {
					err = editor.ReplaceUser(source)
				}
			})
			return err
		},
	})
}

func (a *App) handleAgentWorkspaceMutation(mutation applicationagent.WorkspaceMutation) {
	if a == nil {
		return
	}
	a.onMain(func() {
		if a.directoryCatalog != nil {
			a.directoryCatalog.Invalidate()
		}
		if a.fileTreeState != nil {
			a.fileTreeState.InvalidateRows()
		}
		switch mutation.Kind {
		case applicationagent.WorkspaceDocumentMoved:
			for _, move := range mutation.Moves {
				if a.fileTreeState != nil {
					a.fileTreeState.RemapIDs(func(path string) string {
						if path == move.Source || strings.HasPrefix(path, move.Source+string(filepath.Separator)) {
							return move.Target + strings.TrimPrefix(path, move.Source)
						}
						return path
					})
				}
				if a.recentItems != nil {
					_ = a.recentItems.Add(move.Target, false)
				}
				if filepath.Clean(a.documentPath()) == filepath.Clean(move.Source) && a.documentRepository != nil {
					if note, err := a.documentRepository.Read(move.Target); err == nil {
						if editor := a.noteEditor(); editor != nil {
							_ = editor.Relocate(note)
						}
						if a.workspaceSession != nil {
							if plan, err := a.workspaceSession.RecordActiveDocument(workspaceapp.ActiveDocumentSnapshot{Path: move.Target, PaneID: a.focusedPane()}); err == nil {
								a.workspaceState = plan.State
								_ = a.workspaceRestorer.Persist(plan.State)
							}
						}
					}
				}
				if a.vault != nil {
					_ = a.vault.Refresh(move.Source, move.Target)
				}
			}
		case applicationagent.WorkspaceDocumentsTrashed:
			for _, path := range mutation.Trashed {
				if filepath.Clean(path) == filepath.Clean(a.documentPath()) {
					if editor := a.noteEditor(); editor != nil {
						_ = editor.Open(document.NewNote("", "", document.RevisionOf("")), 0)
					}
				}
			}
			if a.vault != nil {
				_ = a.vault.Refresh(mutation.Trashed...)
			}
		case applicationagent.WorkspaceFolderCreated:
			if a.vault != nil {
				_ = a.vault.Refresh(mutation.Path)
			}
		}
		a.invalidate()
	})
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

func newImageDocumentController(a *App) *imagedocumentui.Controller {
	return imagedocumentui.NewController(imagedocumentui.Ports{
		Host: a.windows,
		Placement: func() hostingui.Placement {
			return hostingui.Placement{Host: "main", Pane: hostingui.PaneID(a.focusedPane()), Layer: hostingui.PaneContent}
		},
		Cache: func() images.CacheStore { return a.imageCacheStore },
		Files: func() images.LocalFiles { return a.imageLocalFiles },
		Surface: imagedocumentui.SurfaceOptions{
			Header:        func(c *ui.Context) { a.renderEditorHeader(c, a.flat) },
			HeaderHeight:  func(c *ui.Context) float32 { return a.headerH(c) },
			BacklinkCount: func() int { return len(a.currentBacklinkSources()) },
			OnBacklinks:   a.openLinks,
			OnFullscreen:  a.openImagePreview,
			OnCopy: func(path string) {
				if a.noteEditor() != nil {
					a.noteEditor().ClipboardCommands().CopyImageObject(path)
				}
			},
		},
		OnSelectedPath: func(path string) {
			if a.propertiesPanel != nil {
				a.propertiesPanel.Close()
				a.propertiesPanel.CancelEdit()
			}
			a.selectedPath = path
		},
		OnRecordHistory: a.navigationHistory.Push,
		OnNotice: func(err error) {
			if err != nil {
				a.notice(err.Error())
			}
		},
		OnDecodeError: func(path string) { a.notice(locales.ImageDecodeError(filepath.Base(path))) },
		OnChanged: func() {
			a.invalidate()
		},
	})
}

func (a *App) openImageDocument(path string, record bool) {
	if path == "" || a.imageDocument == nil {
		return
	}
	_ = a.imageDocument.Open(path, record)
}

func (a *App) closeImageDocument() {
	if a.imageDocument != nil {
		a.imageDocument.Close()
	}
}

func (a *App) imageDocumentOpen() bool {
	return a.imageDocument != nil && a.imageDocument.IsOpen()
}

func (a *App) imageDocumentPath() string {
	if !a.imageDocumentOpen() {
		return ""
	}
	if path, ok := a.navigationHistory.Current(); ok && doctypes.IsImagePath(path) {
		return path
	}
	if path := a.imageDocument.Path(); path != "" {
		return path
	}
	return a.selectedPath
}

// routeExplorerAction is deliberately small: the feature owns presentation
// state and typed intent construction; application/vault owns path safety and
// file mutations.
func (a *App) routeExplorerAction(action explorer.Action) {
	if a == nil {
		return
	}
	switch action.Kind {
	case explorer.OpenEntry:
		a.selectedPath = action.ID
		a.requestOpen(action.ID)
		return
	case explorer.OpenRecent:
		if action.Recent.Path == "" {
			return
		}
		if action.Recent.IsDir {
			a.setWorkspace(action.Recent.Path)
		} else {
			a.selectedPath = action.Recent.Path
			a.requestOpen(action.Recent.Path)
		}
		return
	case explorer.StartDrag:
		a.treeDragItem = action.Payload
		return
	case explorer.EndDrag:
		a.treeDragItem = dragdrop.VaultItem{}
		return
	case explorer.DropEntries:
		paths := action.ItemIDs
		if len(paths) == 0 && action.Payload.Path != "" {
			paths = []string{action.Payload.Path}
		}
		a.moveTreeEntries(paths, a.vaultFileUseCases().DestinationForPath(action.ID))
		return
	case explorer.DropNative:
		a.dropExplorerTransfer(action.Drop, a.vaultFileUseCases().DestinationForPath(action.ID))
		return
	case explorer.ImportFiles:
		a.importExplorerFiles(action.Paths, a.vaultFileUseCases().DestinationForPath(action.ID))
		return
	case explorer.InvokeCommand, explorer.InvokeContextCommand:
		if action.CommandID == explorer.CommandOpenFile {
			a.openFileDialog()
			return
		}
		if action.CommandID == explorer.CommandOpenFolder {
			a.openFolderDialog()
			return
		}
	}
	explorer.HandleAction(action, explorer.ActionPorts{
		CreateNote: a.vaultFileUseCases().CreateUntitledNote,
		CreateFolder: func(directory string) error {
			_, err := a.vaultFileUseCases().CreateDefaultFolder(directory)
			return err
		},
		Open: a.requestOpen,
		OnError: func(err error) {
			a.notice(locales.CommonCreateFailed(err.Error()))
			a.invalidate()
		},
		Invalidate: func() {
			if a.directoryCatalog != nil {
				a.directoryCatalog.Invalidate()
			}
			if a.fileTreeState != nil {
				a.fileTreeState.InvalidateRows()
			}
			a.invalidate()
		},
	})
}

func (a *App) moveTreeEntries(paths []string, destination string) {
	if a == nil || destination == "" || len(paths) == 0 {
		return
	}
	mutation, err := a.vaultFileUseCases().MoveFiles(paths, destination)
	if err != nil {
		a.notice(locales.CommonMoveFailed(err.Error()))
		return
	}
	a.applyTreeMoves(mutation.Moves)
	a.invalidate()
}

func (a *App) importExplorerFiles(paths []string, destination string) {
	if a == nil || destination == "" || len(paths) == 0 {
		return
	}
	if _, err := a.vaultFileUseCases().ImportFiles(paths, destination); err != nil {
		a.notice(locales.CommonImportFailed(err.Error()))
		return
	}
	if a.directoryCatalog != nil {
		a.directoryCatalog.Invalidate()
	}
	if a.fileTreeState != nil {
		a.fileTreeState.InvalidateRows()
	}
	a.invalidate()
}

func (a *App) dropExplorerTransfer(drop transfer.Drop, destination string) {
	b, err := drop.Data.Read(transfer.Format("application/x-muse-vault-item+json"))
	if err != nil {
		return
	}
	var payload struct {
		Path  string `json:"path"`
		Items []struct {
			Path string `json:"path"`
		} `json:"items"`
	}
	if err := json.Unmarshal(b, &payload); err != nil {
		a.notice(locales.FilesReadDropFailed())
		return
	}
	paths := make([]string, 0, len(payload.Items)+1)
	for _, item := range payload.Items {
		if item.Path != "" {
			paths = append(paths, item.Path)
		}
	}
	if len(paths) == 0 && payload.Path != "" {
		paths = append(paths, payload.Path)
	}
	if len(paths) == 0 {
		a.notice(locales.FilesReadDropFailed())
		return
	}
	if drop.Operation == transfer.Move {
		a.moveTreeEntries(paths, destination)
		return
	}
	a.importExplorerFiles(paths, destination)
}

func (a *App) applyTreeMoves(moves []vaultfileapp.FileMove) {
	if a == nil {
		return
	}
	for _, move := range moves {
		remap := func(path string) string { return remapMovedPath(path, move) }
		if a.fileTreeState != nil {
			a.fileTreeState.RemapIDs(remap)
		}
		a.selectedPath = remap(a.selectedPath)
		if a.recentItems != nil {
			_ = a.recentItems.Add(move.Target, false)
		}
		if filepath.Clean(a.documentPath()) == filepath.Clean(move.Source) && a.documentRepository != nil {
			if note, err := a.documentRepository.Read(move.Target); err == nil {
				if editor := a.noteEditor(); editor != nil {
					_ = editor.Relocate(note)
				}
				if a.workspaceSession != nil {
					if plan, err := a.workspaceSession.RecordActiveDocument(workspaceapp.ActiveDocumentSnapshot{Path: move.Target, PaneID: a.focusedPane()}); err == nil {
						a.workspaceState = plan.State
						if a.workspaceRestorer != nil {
							_ = a.workspaceRestorer.Persist(plan.State)
						}
					}
				}
			}
		}
		if a.vault != nil {
			_ = a.vault.Refresh(move.Source, move.Target)
		}
	}
	if a.directoryCatalog != nil {
		a.directoryCatalog.Invalidate()
	}
	if a.fileTreeState != nil {
		a.fileTreeState.InvalidateRows()
	}
}

func remapMovedPath(path string, move vaultfileapp.FileMove) string {
	rel, err := filepath.Rel(move.Source, path)
	if err != nil || rel == ".." || filepath.IsAbs(rel) || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return path
	}
	return filepath.Join(move.Target, rel)
}

func (a *App) queueLinkIndexSnapshot() {
	if a == nil || a.activeLinkIndex == nil {
		return
	}
	if a.vault == nil {
		a.activeLinkIndex.Clear()
		return
	}
	a.activeLinkIndex.ScheduleDocument(a.documentPath(), a.diskSource(), time.Now())
}

func (a *App) linkIndexTick(c *ui.Context) {
	if a == nil || a.activeLinkIndex == nil || a.vault == nil {
		return
	}
	ticket, remaining, ready := a.activeLinkIndex.Take(time.Now())
	if remaining > 0 {
		if c != nil {
			c.After(remaining)
		}
		return
	}
	if !ready {
		return
	}
	session := a.vault
	go func() {
		result := a.activeLinkIndex.Apply(ticket, session)
		a.onMain(func() {
			if a.vault != session {
				return
			}
			snapshot, accepted := a.activeLinkIndex.Accept(result, a.documentPath(), a.diskSource())
			if !accepted {
				return
			}
			a.vaultSnapshot = snapshot
			if a.noteEditor() != nil {
				a.configureActiveNote(0)
			}
			a.invalidate()
		})
	}()
}

func (a *App) indexedLink(target string) bool {
	if a == nil || a.vault == nil {
		return false
	}
	root := a.vaultSnapshot.Root
	current, err := filepath.Rel(root, a.documentPath())
	if err != nil {
		return false
	}
	if location, ok := a.vault.ResolveLink(filepath.ToSlash(current), target); ok {
		a.navigateIndexed(location.Path, location.Offset)
		return true
	}
	return false
}

func (a *App) openLinks() {
	if a == nil {
		return
	}
	session := a.vault
	if session == nil || !a.vaultSnapshot.Ready {
		a.pendingLinks = true
		return
	}
	if a.windows == nil {
		return
	}
	if a.windows.Contains(a.backlinksHandle) {
		_ = a.windows.Focus(a.backlinksHandle)
		return
	}
	a.backlinksHandle = hostingui.Handle{}
	path := a.documentPath()
	if a.imageDocumentOpen() && a.imageDocumentPath() != "" {
		path = a.imageDocumentPath()
	}
	rel, err := filepath.Rel(a.vaultSnapshot.Root, path)
	if err != nil {
		return
	}
	if err := session.OpenLinks(filepath.ToSlash(rel)); err != nil {
		a.notice(locales.VaultIndexBuildingShort())
		return
	}
	rel = filepath.ToSlash(rel)
	content := backlinksui.NewHost(backlinksui.Ports{
		Data: func() backlinksui.Model {
			if a.vault != session {
				return backlinksui.Model{}
			}
			snapshot := session.Snapshot()
			return backlinksui.Model{
				Ready: snapshot.Ready, Busy: snapshot.LinksBusy,
				Incoming: session.BacklinkSources(rel), Outgoing: snapshot.Outgoing,
			}
		},
		OpenSource: func(path string, offset int) {
			if a.vault == session {
				a.navigateIndexed(path, offset)
			}
		},
		OpenTarget: func(target string) {
			if a.vault == session {
				a.indexedLink(target)
			}
		},
		OnClose: func() {
			_ = session.CloseLinks()
			if a.vault == session {
				a.vaultSnapshot = session.Snapshot()
			}
			a.backlinksHandle = hostingui.Handle{}
			a.invalidate()
		},
	})
	handle, err := a.windows.Open(content, hostingui.Placement{Host: "main", Layer: hostingui.ModalOverlay})
	if err != nil {
		_ = session.CloseLinks()
		return
	}
	a.vaultSnapshot = session.Snapshot()
	a.backlinksHandle = handle
	a.invalidate()
}

func (a *App) closeLinks() {
	if a == nil {
		return
	}
	if a.windows != nil && a.windows.Contains(a.backlinksHandle) {
		_ = a.windows.Close(a.backlinksHandle)
		return
	}
	if a.vault != nil {
		_ = a.vault.CloseLinks()
		a.vaultSnapshot = a.vault.Snapshot()
	}
}

func (a *App) vaultFileUseCases() *vaultfileapp.FileUseCases {
	if a == nil {
		return vaultfileapp.NewFileUseCases("", nil, nil)
	}
	return vaultfileapp.NewFileUseCases(a.root, a.fileOperations, func(path string) bool {
		if a.fileOperations == nil {
			return false
		}
		kind, err := a.fileOperations.Kind(path)
		return err == nil && kind == vaultfiles.EntryDirectory
	})
}

// outlineView adapts the editor's heading projection to the Outline feature
// and routes its typed selection action back through editor navigation.
func (a *App) outlineView(c *ui.Context) {
	if a == nil || a.noteEditor() == nil {
		return
	}
	navigation := a.noteEditor().NavigationCommands()
	parsed := navigation.Headings()
	headings := make([]outlineui.Heading, 0, len(parsed))
	for _, heading := range parsed {
		headings = append(headings, outlineui.Heading{Level: heading.Level, Text: heading.Text, Offset: heading.Offset})
	}
	a.outlinePanel.RenderSnapshot(c, outlineui.Snapshot{Headings: headings, Caret: navigation.Caret()}, func(action outlineui.Action) {
		if action.Kind != outlineui.SelectHeading {
			return
		}
		navigation.NavigateToOffset(action.Offset)
		a.noteEditor().FocusCommands().Focus()
	})
}

// memoryDiagnosticsSnapshot composes measurements from the active editor and
// shared Markdown renderer for the opt-in diagnostics service. It is part of
// application assembly because only App knows which hosted feature is active.
func (a *App) memoryDiagnosticsSnapshot() diagnostics.Snapshot {
	if a == nil {
		return diagnostics.Snapshot{}
	}
	a.mainMu.Lock()
	var editor diagnostics.EditorSnapshot
	if note := a.noteEditor(); note != nil {
		state := note.MemorySnapshot()
		editor = diagnostics.EditorSnapshot{
			SourceBytes: state.SourceBytes, Leaves: state.Leaves, VirtualLayout: state.VirtualLayout,
			ImageCandidateLeaves: state.ImageCandidateLeaves, ImageAssetViews: state.ImageAssetViews,
			ImageAssets: state.ImageAssets, LoadedImageAssets: state.LoadedImageAssets,
			ImagePixelBytes: state.ImagePixelBytes,
		}
	}
	vaultReady := a.vault != nil && a.vaultSnapshot.Ready
	a.mainMu.Unlock()
	pending, active := markdownui.ImageDecodeQueueStats()
	cacheEntries, cacheBytes := markdownui.ImageAssetCacheStats()
	return diagnostics.Snapshot{
		VaultReady: vaultReady, Editor: editor,
		ImageCacheEntries: cacheEntries, ImageCacheEstimatedBytes: cacheBytes,
		ImageDecodePending: pending, ImageDecodeActive: active,
	}
}

// linkNavigation composes the application use case with the active vault's
// file-kind capability. It owns target policy; App only enacts its result.
func (a *App) linkNavigation() *vaultlinkapp.LinkNavigation {
	return vaultlinkapp.NewLinkNavigation(a.fileOperations, a.imageLocalFiles)
}

func (a *App) linkPreviewPorts() linkpreview.Ports {
	navigation := a.linkNavigation()
	return linkpreview.Ports{
		Suppressed: func() bool { return a.imagePreviewOpen() || a.workspaceSearch.IsOpen() },
		ResolveImage: func(target string) (string, bool) {
			return navigation.ResolveWikiImage(a.vault, a.documentPath(), target)
		},
		ResolveNote: func(target string) (linkpreview.NoteTarget, bool) {
			note, ok := navigation.ResolveWikiPreview(a.vault, a.documentPath(), target)
			if !ok {
				return linkpreview.NoteTarget{}, false
			}
			return linkpreview.NoteTarget{Title: note.Title, Path: note.Path, Source: note.Source, Offset: note.Offset, Root: note.Root}, true
		},
		LoadImage: func(path string) *markdownui.ImageAsset {
			return markdownui.LoadImageAssetWithLocalFiles(path, "", a.imageLocalFiles, a.imageCacheStore, nil, a.invalidate)
		},
		Anchor: func(c *ui.Context, state linkpreview.Snapshot) ui.Rect { return a.wikiPreviewAnchor(c, state) },
		MarkdownOptions: func(note linkpreview.NoteTarget) markdownui.Options {
			return markdownui.Options{
				BodyFontSize: 12, Padding: 0, ContentWidth: 346,
				ImageCacheStore: a.imageCacheStore, LocalFiles: a.imageLocalFiles,
				ResolveImage: func(src, base, root string) string {
					return navigation.ResolveImageSource(a.vault, src, base, root, a.imageLocalFiles)
				},
				OnAsync: a.invalidate,
				OnImagePreview: func(src string) {
					base := filepath.Join(note.Root, filepath.Dir(note.Path))
					resolved := navigation.ResolveImageSource(a.vault, src, base, note.Root, a.imageLocalFiles)
					if resolved != "" {
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

func (a *App) wikiLinkSuggestions(_ string) []string {
	suggestions := a.wikiLinkSuggestionDetails("")
	items := make([]string, len(suggestions))
	for i, suggestion := range suggestions {
		items[i] = suggestion.Target
	}
	return items
}

func (a *App) wikiLinkSuggestionDetails(_ string) []vaultlinks.Suggestion {
	if a == nil || a.vault == nil || !a.vaultSnapshot.Ready {
		return nil
	}
	return a.vault.WikiSuggestions()
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
	path, _ := a.linkNavigation().ResolveWikiImage(a.vault, currentPath, target)
	return path
}

func (a *App) resolveVaultImage(src, base, root string) string {
	return a.linkNavigation().ResolveImageSource(a.vault, src, base, root, a.imageLocalFiles)
}

func (a *App) resolveWikiEmbed(currentPath, target string) (path, source string, ok bool) {
	embed, ok := a.linkNavigation().ResolveWikiEmbed(a.vault, currentPath, target)
	return embed.Path, embed.Source, ok
}

func (a *App) resolveWikiPreviewNote(target string) (linkpreview.NoteTarget, bool) {
	note, ok := a.linkNavigation().ResolveWikiPreview(a.vault, a.documentPath(), target)
	if !ok {
		return linkpreview.NoteTarget{}, false
	}
	return linkpreview.NoteTarget{Title: note.Title, Path: note.Path, Source: note.Source, Offset: note.Offset, Root: note.Root}, true
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
	if currentPath == "" {
		currentPath = a.documentPath()
	}
	switch a.linkNavigation().WikiLinkValidity(a.vault, currentPath, target) {
	case vaultlinkapp.LinkValid:
		return inlineui.LinkTargetValid
	case vaultlinkapp.LinkInvalid:
		return inlineui.LinkTargetInvalid
	default:
		return inlineui.LinkTargetUnknown
	}
}

// openLink opens a link Cmd-clicked in the document: a heading of the
// document by its anchor, a document of the workspace by its path, or a
// page in the browser.
func (a *App) openLink(href string) {
	if a == nil {
		return
	}
	a.routeLinkDestination(a.linkNavigation().ResolveClick(a.vault, a.documentPath(), href))
}

func (a *App) routeLinkDestination(destination vaultlinkapp.LinkDestination) {
	if destination.Kind != vaultlinkapp.LinkDestinationPending {
		a.pendingWiki = ""
	}
	switch destination.Kind {
	case vaultlinkapp.LinkDestinationHeading:
		if a.noteEditor() == nil {
			return
		}
		slug := destination.Value
		for _, h := range a.noteEditor().NavigationCommands().Headings() {
			if markdownsyntax.HeadingSlug(h.Text) == strings.ToLower(slug) {
				a.noteEditor().NavigationCommands().NavigateToOffset(h.Offset)
				return
			}
		}
	case vaultlinkapp.LinkDestinationPending:
		a.pendingWiki = destination.Value
	case vaultlinkapp.LinkDestinationNote:
		a.navigateIndexed(destination.Path, destination.Offset)
	case vaultlinkapp.LinkDestinationFile:
		a.requestOpen(destination.Path)
	case vaultlinkapp.LinkDestinationImage:
		a.pendingWiki = ""
		a.openImagePreview(destination.Path)
	case vaultlinkapp.LinkDestinationExternal:
		a.launchExternal(destination.Value)
	}
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
	a.routeLinkDestination(a.linkNavigation().ResolveClick(a.vault, a.documentPath(), "wiki:"+target))
}

func (a *App) openImagePreview(source string) {
	if a != nil && a.imagePreview != nil {
		_ = a.imagePreview.Open(source)
	}
}

func (a *App) openMermaidPreview(source string, result diagrams.Result) {
	if a == nil || a.windows == nil {
		return
	}
	if !a.mermaidPreviewHandle.IsZero() {
		_ = a.windows.Close(a.mermaidPreviewHandle)
		a.mermaidPreviewHandle = hostingui.Handle{}
	}
	content := mermaidpreview.NewContent(source, result, func() {
		a.mermaidPreviewHandle = hostingui.Handle{}
		a.invalidate()
	})
	handle, err := a.windows.Open(content, hostingui.Placement{Host: workspaceui.MainHost, Layer: hostingui.Overlay})
	if err != nil {
		content.Close()
		a.notice(err.Error())
		return
	}
	a.mermaidPreviewHandle = handle
	a.invalidate()
}

func (a *App) closeImagePreview() {
	if a != nil && a.imagePreview != nil {
		a.imagePreview.Close()
	}
}

func (a *App) imagePreviewOpen() bool {
	return a != nil && a.imagePreview != nil && a.imagePreview.IsOpen()
}
