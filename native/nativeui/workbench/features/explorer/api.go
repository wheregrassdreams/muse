// Package explorer is the public NativeUI boundary for vault navigation.
//
// It exposes the tree's stable presentation handle and value types without
// exposing the implementation package path to workbench composition code.
// Product actions are represented as typed intents and routed by the shell;
// catalog and mutation policy remain in application/vault.
package explorer

import (
	"time"

	"github.com/egoist/mygo/transfer"
	"github.com/egoist/mygo/ui"
	"muse/locales"
	"muse/markdown/images"
	"muse/nativeui/workbench/framework/interactions/commands"
	"muse/nativeui/workbench/framework/interactions/dragdrop"
	"muse/nativeui/workbench/features/explorer/filetree"
	"muse/nativeui/workbench/framework/interactions/keybindings"
	componenticons "muse/nativeui/workbench/framework/components/icons"
)

// Controller owns Explorer's per-window transient presentation state. It is
// safe to retain for the lifetime of the native window and contains no vault
// or application object.
type Controller struct {
	state         *filetree.Handle
	treeSequences *keybindings.KeySequenceMatcher
	keymap        string
}

// Row and RecentItem are detached values used by the workbench projection.
type Row = filetree.Row
type RecentItem = filetree.RecentItem

// DragItem is the Explorer-owned semantic payload. Other features adapt it to
// their own transfer representation at the workbench boundary.
type DragItem = dragdrop.VaultItem

func NewController() *Controller { return &Controller{state: filetree.NewHandle()} }

func (controller *Controller) handle() *filetree.Handle {
	if controller == nil {
		return nil
	}
	if controller.state == nil {
		controller.state = filetree.NewHandle()
	}
	return controller.state
}

// CreateTarget resolves the directory in which a create command should run.
// The caller supplies the cached selected-entry kind so this presentation
// controller does not perform filesystem I/O.
func (controller *Controller) CreateTarget(root, selected string, selectedDirectory bool) string {
	if controller == nil {
		return root
	}
	return controller.handle().CreateTarget(root, selected, selectedDirectory)
}

// The value aliases below are the public Explorer vocabulary. Callers build a
// snapshot from application-owned data, render it, and route the typed action
// to the application/workbench owner.
type (
	Command              = filetree.Command
	CommandID            = filetree.CommandID
	CatalogEntry         = filetree.CatalogEntry
	CatalogPort          = filetree.CatalogPort
	Clipboard            = filetree.Clipboard
	ClipboardKind        = filetree.ClipboardKind
	DragOperationDisplay = filetree.DragOperationDisplay
	DragOperationKind    = filetree.DragOperationKind
	DragPreviewState     = filetree.DragPreviewState
	DropHoverState       = filetree.DropHoverState
	DropTargetRange      = filetree.DropTargetRange
	Entry                = filetree.Entry
	ImagePreview         = filetree.ImagePreview
	KeyCommand           = filetree.KeyCommand
	KeyOperation         = filetree.KeyOperation
	NativeDropPort       = filetree.NativeDropPort
	TreeBindingArgs      = filetree.TreeBindingArgs
	TreeBinding          = filetree.TreeBinding
	StandardTreeBindings = filetree.StandardTreeBindings
	VimTreeBindings      = filetree.VimTreeBindings
	PointerPressState    = filetree.PointerPressState
)

type ActionKind uint8

const (
	SelectEntry ActionKind = iota + 1
	NavigateEntry
	SubmitEntry
	ToggleEntry
	OpenEntry
	OpenRecent
	InvokeCommand
	DropEntries
	InvokeContextCommand
	HoverEntry
	LeaveEntry
	StartDrag
	BeginNativeHandoff
	EndDrag
	DropNative
	ImportFiles
	CreateFile
	CreateFolder
	TrashEntries
)

// Action is a feature-owned, typed presentation intent. The root router maps
// cross-feature intents to application/workspace APIs.
type Action struct {
	Kind      ActionKind
	ID        string
	Index     int
	CommandID CommandID
	ItemIDs   []string
	Payload   DragItem
	Modifiers ui.Modifiers
	Offer     transfer.Offer
	Drop      transfer.Drop
	Paths     []string
	Directory string
	Bounds    ui.Rect
	Recent    RecentItem
}

type DragPort struct {
	Make func(entryID string, selectedIDs []string) DragItem
	IDs  func(DragItem) []string
}

// ActionPorts are narrow application capabilities used by Explorer's action
// workflow. Root composition supplies them; Explorer owns the feature flow.
type ActionPorts struct {
	CreateNote   func(directory string) (string, error)
	CreateFolder func(directory string) error
	Open         func(string)
	OnError      func(error)
	Invalidate   func()
}

// HandleAction runs Explorer-owned file creation requests through injected
// application capabilities. It returns whether the action was consumed.
func HandleAction(action Action, ports ActionPorts) bool {
	if action.Kind != CreateFile && action.Kind != CreateFolder {
		return false
	}
	if action.Directory == "" {
		return true
	}
	var err error
	switch action.Kind {
	case CreateFile:
		if ports.CreateNote == nil {
			return true
		}
		var path string
		path, err = ports.CreateNote(action.Directory)
		if err == nil && ports.Open != nil {
			ports.Open(path)
		}
	case CreateFolder:
		if ports.CreateFolder == nil {
			return true
		}
		err = ports.CreateFolder(action.Directory)
	}
	if err != nil {
		if ports.OnError != nil {
			ports.OnError(err)
		}
	} else if ports.Invalidate != nil {
		ports.Invalidate()
	}
	return true
}

// RenderTrashDropTarget renders Explorer's recoverable-trash affordance and
// returns a typed trash intent. Confirmation and mutation remain application
// responsibilities supplied by the host router.
func RenderTrashDropTarget(c *ui.Context, controller *Controller, onAction func(Action)) {
	if controller == nil {
		return
	}
	t := c.Theme()
	trash := ui.Box(c).Absolute().Left(10).Right(10).Bottom(48).Height(64).
		Key("tree-trash-drop-target").Label(locales.TreeTrashDropTarget()).Radius(12).
		Background(t.Background.Alpha(0.04)).Border(1, t.Border.Alpha(0.30)).BorderStyle(ui.BorderDashed).
		Padding(0, 12).AlignItems(ui.Center).Justify(ui.Center)
	_, over := ui.DragOver[DragItem](trash)
	if over {
		controller.SetDragOperationDisplay(DragOperationDisplay{Kind: DragOperationMoveToTrash})
		trash.Background(ui.RGBA(128, 66, 72, 0.10)).BorderColor(t.Border.Alpha(0.48))
	}
	if dropped, ok := ui.Drop[DragItem](trash); ok {
		action := controller.TrashAction(dropped)
		if onAction != nil && len(action.ItemIDs) > 0 {
			onAction(action)
		}
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

// TrashAction resolves a dropped row to the selected group when the row is
// already selected; otherwise only that row is included.
func (c *Controller) TrashAction(item DragItem) Action {
	paths := []string{item.Path}
	if item.Path != "" && c.IsSelected(item.Path) {
		paths = c.SelectedIDs()
	}
	if item.Path == "" {
		paths = nil
	}
	return Action{Kind: TrashEntries, ItemIDs: paths, Payload: item}
}

type PanelResult struct {
	NativeOffer    *transfer.Offer
	PopoverAnchor  ui.Element
	Focused        bool
	FocusRequested bool
}

const (
	CommandCreateFile   = filetree.CommandCreateFile
	CommandCreateFolder = filetree.CommandCreateFolder
	CommandPaste        = filetree.CommandPaste
	CommandCollapse     = filetree.CommandCollapse
	CommandCopy         = filetree.CommandCopy
	CommandCut          = filetree.CommandCut
	CommandChangeIcon   = filetree.CommandChangeIcon
	CommandCopyRelative = filetree.CommandCopyRelative
	CommandCopyAbsolute = filetree.CommandCopyAbsolute
	CommandRename       = filetree.CommandRename
	CommandTrash        = filetree.CommandTrash
	CommandReveal       = filetree.CommandReveal
	CommandOpenFile     = filetree.CommandOpenFile
	CommandOpenFolder   = filetree.CommandOpenFolder

	KeyEnsureFocus                = filetree.KeyEnsureFocus
	KeyFocus                      = filetree.KeyFocus
	KeyToggleExpanded             = filetree.KeyToggleExpanded
	KeyNavigate                   = filetree.KeyNavigate
	KeySelectRange                = filetree.KeySelectRange
	KeyCloseNode                  = filetree.KeyCloseNode
	KeyCollapseAll                = filetree.KeyCollapseAll
	KeySelectAll                  = filetree.KeySelectAll
	KeyToggleSelection            = filetree.KeyToggleSelection
	KeyInvertSelection            = filetree.KeyInvertSelection
	ClipboardCopy                 = filetree.ClipboardCopy
	ClipboardCut                  = filetree.ClipboardCut
	DragOperationNone             = filetree.DragOperationNone
	DragOperationMoveToFolder     = filetree.DragOperationMoveToFolder
	DragOperationMoveToTrash      = filetree.DragOperationMoveToTrash
	DragOperationInsertIntoEditor = filetree.DragOperationInsertIntoEditor

	CatalogFile      = filetree.CatalogFile
	CatalogDirectory = filetree.CatalogDirectory
)

// PanelCapabilities are the small UI services Explorer needs from its host.
// They describe rendering abilities; they do not expose an application object.
type PanelSnapshot struct {
	Entries              []EntrySnapshot
	PopoverAnchorID      string
	EmptyMessage         string
	EmptyActionLabel     string
	EmptyActions         []Command
	Recent               []RecentItem
	RootLabel            string
	RootCanDrop          bool
	NativeDrop           NativeDropPort
	Drag                 *DragSnapshot
	FocusRequested       bool
	NativeHandoffPending bool
}

// EntrySnapshot contains detached catalog and visual facts for one tree row.
type EntrySnapshot struct {
	Row         Row
	Icon        *ui.SVG
	IconTint    ui.Color
	Previewable bool
}

// DragSnapshot contains detached display facts for the currently dragged row.
type DragSnapshot struct {
	ItemName  string
	DisplayID string
	Directory bool
	ItemIcon  *ui.SVG
	FileIcon  *ui.SVG
	IconTint  ui.Color
	ImagePath string
}

type PanelCapabilities struct {
	Focus           *ui.Handle
	KeyInput        func(ui.InputEvent) bool
	Drag            DragPort
	OnAction        func(Action)
	ImageCacheStore images.CacheStore
	LocalFiles      images.LocalFiles
	OnImageLoad     func()
	IsDirectory     func(string) bool
}

// RenderPanel renders a snapshot of the vault tree and reports typed user
// actions through the host capability. Catalog access and policy stay outside
// this package.
func (controller *Controller) RenderPanel(c *ui.Context, snapshot PanelSnapshot, caps PanelCapabilities) PanelResult {
	state := controller.handle()
	if state == nil {
		return PanelResult{}
	}
	entries := make([]filetree.ModelEntryInput, len(snapshot.Entries))
	for i, entry := range snapshot.Entries {
		entries[i] = filetree.ModelEntryInput{Row: entry.Row, Icon: entry.Icon, IconTint: entry.IconTint, Previewable: entry.Previewable}
	}
	model := filetree.ModelInput{
		Entries: entries, PopoverAnchorID: snapshot.PopoverAnchorID,
		EmptyMessage: snapshot.EmptyMessage, EmptyActionLabel: snapshot.EmptyActionLabel,
		EmptyActions: append([]Command(nil), snapshot.EmptyActions...),
		Recent:       append([]RecentItem(nil), snapshot.Recent...), RootLabel: snapshot.RootLabel,
		RootCanDrop: snapshot.RootCanDrop, NativeDrop: snapshot.NativeDrop,
	}
	rows := make([]Row, len(snapshot.Entries))
	for i, entry := range snapshot.Entries {
		rows[i] = entry.Row
	}
	projection := filetree.DragProjection{}
	if drag := snapshot.Drag; drag != nil {
		projection = filetree.BuildDragProjection(state, rows, filetree.DragProjectionInput{
			ItemName: drag.ItemName, DisplayID: drag.DisplayID, ItemDirectory: drag.Directory,
			ItemIcon: drag.ItemIcon, FileIcon: drag.FileIcon, IconTint: drag.IconTint,
			ImagePath: drag.ImagePath, ImageCacheStore: caps.ImageCacheStore,
			LocalFiles: caps.LocalFiles, OnImageLoad: caps.OnImageLoad, IsDirectory: caps.IsDirectory,
		})
	}
	result := filetree.RenderPanel(c, state, filetree.PanelInput[DragItem]{
		Model: model, Drag: filetree.DragPort[DragItem]{Make: caps.Drag.Make, IDs: caps.Drag.IDs}, DragProjection: projection,
		Focus: caps.Focus, FocusRequested: snapshot.FocusRequested,
		NativeHandoffPending: snapshot.NativeHandoffPending, KeyInput: caps.KeyInput,
		Dispatch: func(action filetree.Action[DragItem]) {
			if caps.OnAction != nil {
				translated := Action{
					Kind: ActionKind(action.Kind), ID: action.ID, Index: action.Index,
					CommandID: CommandID(action.CommandID), ItemIDs: action.ItemIDs,
					Payload: action.Payload, Modifiers: action.Modifiers, Offer: action.Offer,
					Drop: action.Drop, Paths: action.Paths, Bounds: action.Bounds,
					Recent: action.Recent,
				}
				if action.Kind == filetree.InvokeCommand || action.Kind == filetree.InvokeContextCommand {
					switch CommandID(action.CommandID) {
					case CommandCreateFile:
						translated.Kind = CreateFile
						translated.Directory = state.CreateTarget(snapshot.RootLabel, action.ID, caps.IsDirectory != nil && caps.IsDirectory(action.ID))
					case CommandCreateFolder:
						translated.Kind = CreateFolder
						translated.Directory = state.CreateTarget(snapshot.RootLabel, action.ID, caps.IsDirectory != nil && caps.IsDirectory(action.ID))
					}
				}
				caps.OnAction(translated)
			}
		},
	})
	return PanelResult{NativeOffer: result.NativeOffer, PopoverAnchor: result.PopoverAnchor, Focused: result.Focused, FocusRequested: result.FocusRequested}
}

// HandleKey maps a panel keystroke to a typed Explorer command. The caller
// supplies the active keymap and routes any returned workbench action.
func (controller *Controller) HandleKey(mods ui.Modifiers, key ui.Key, keymap string) (TreeBinding, bool) {
	if controller == nil {
		return TreeBinding{}, false
	}
	if controller.keymap != "" && controller.keymap != keymap {
		controller.CancelKeySequence()
	}
	controller.keymap = keymap
	if keymap == "vim" {
		if name := keyStrokeName(key); name != "" {
			if binding, handled := controller.feedTreeSequence(keybindings.KeyStroke{Key: name, Mods: keybindingModifiers(mods)}); handled {
				return binding, true
			}
		}
	} else {
		controller.CancelKeySequence()
	}
	binding := StandardTreeBindings{}.Resolve(mods, key)
	if !binding.Handled && keymap == "vim" {
		binding = VimTreeBindings{}.Resolve(mods, key)
	}
	return binding, binding.Handled
}

func (controller *Controller) CancelKeySequence() {
	if controller != nil && controller.treeSequences != nil {
		controller.treeSequences.Cancel()
	}
}

func (controller *Controller) SetKeymap(keymap string) bool {
	if controller == nil {
		return false
	}
	changed := controller.keymap != "" && controller.keymap != keymap
	if changed {
		controller.CancelKeySequence()
	}
	controller.keymap = keymap
	return changed
}

func (controller *Controller) TreeSequencePending() bool {
	return controller != nil && controller.treeSequences != nil && controller.treeSequences.Pending()
}

func (controller *Controller) feedTreeSequence(stroke keybindings.KeyStroke) (TreeBinding, bool) {
	if controller.treeSequences == nil {
		controller.treeSequences, _ = keybindings.NewKeySequenceMatcher(800*time.Millisecond, nil, VimTreeBindings{}.Sequences()...)
	}
	for attempt := 0; attempt < 2; attempt++ {
		result := controller.treeSequences.Match(stroke)
		if result.ActionID != "" {
			args := TreeBindingArgs{}
			args.Delta, _ = result.Values["delta"].(int)
			args.Direction, _ = result.Values["direction"].(int)
			args.Index, args.HasIndex = result.Values["index"].(int)
			args.TargetID, _ = result.Values["path"].(string)
			return TreeBinding{Action: result.ActionID, Args: args, Handled: true}, true
		}
		if !result.Reprocess {
			return TreeBinding{}, result.Handled
		}
	}
	return TreeBinding{}, true
}

func keyStrokeName(key ui.Key) string {
	if key >= ui.KeyA && key <= ui.KeyZ {
		return string(rune('A' + key - ui.KeyA))
	}
	if key >= ui.Key0 && key <= ui.Key9 {
		return string(rune('0' + key - ui.Key0))
	}
	switch key {
	case ui.KeyEscape:
		return "Escape"
	case ui.KeyLeft:
		return "Left"
	case ui.KeyRight:
		return "Right"
	case ui.KeyUp:
		return "Up"
	case ui.KeyDown:
		return "Down"
	}
	return ""
}

func keybindingModifiers(mods ui.Modifiers) uint32 {
	var value uint32
	if mods&ui.Shift != 0 {
		value |= keybindings.ModShift
	}
	if mods&ui.Alt != 0 {
		value |= keybindings.ModAlt
	}
	if mods&ui.Cmd != 0 {
		value |= keybindings.ModPrimary
	}
	if ui.Cmd != ui.Ctrl && mods&ui.Ctrl != 0 {
		value |= keybindings.ModCtrl
	}
	if ui.Cmd != ui.Super && mods&ui.Super != 0 {
		value |= keybindings.ModSuper
	}
	return value
}

func BuildRows(controller *Controller, root string, catalog CatalogPort) []Row {
	return filetree.BuildRows(controller.handle(), root, catalog)
}

func ProjectChildren(root string, children []CatalogEntry, visible func(string) bool) []string {
	return filetree.ProjectChildren(root, children, visible)
}

func RenderImagePreview(c *ui.Context, preview ImagePreview) bool {
	return filetree.RenderImagePreview(c, preview)
}

func KeyCommandForAction(id commands.ActionID, args TreeBindingArgs, paths []string) (KeyCommand, bool) {
	return filetree.KeyCommandForAction(id, args, paths)
}

// Rows projects the current catalog snapshot through Explorer's expansion
// state and visible-row policy.
func (c *Controller) Rows(root string, catalog CatalogPort) []Row {
	return filetree.BuildRows(c.handle(), root, catalog)
}

// ApplyKeyAction translates a shared workbench action into a tree-owned state
// transition. It returns false when the action is not owned by Explorer.
func (c *Controller) ApplyKeyAction(id commands.ActionID, args TreeBindingArgs, preferredIDs []string, rows []Row) bool {
	command, ok := filetree.KeyCommandForAction(id, args, preferredIDs)
	if !ok {
		return false
	}
	c.handle().ApplyKey(command, rows)
	return true
}

// ActionForKeyBinding translates a resolved tree binding into a typed feature
// intent. Navigation and selection state transitions are applied separately
// through ApplyKeyAction so the host can keep editor routing explicit.
func (c *Controller) ActionForKeyBinding(binding TreeBinding, root, selected string, isDirectory func(string) bool) Action {
	switch binding.Action {
	case commands.ActionOpen:
		return Action{Kind: OpenEntry, ID: c.FocusID()}
	case commands.ActionCreateFile:
		return c.CreateAction(CreateFile, root, selected, isDirectory)
	case commands.ActionCreateFolder:
		return c.CreateAction(CreateFolder, root, selected, isDirectory)
	default:
		return Action{Kind: InvokeCommand, ID: string(binding.Action)}
	}
}

// ActionPaths returns selected paths in visible row order, with the focused
// row as fallback. The caller supplies the primary row for drag semantics.
func (c *Controller) ActionPaths(rows []Row, primary string) []string {
	return c.handle().SelectedPaths(rows, primary)
}

// DraggedPaths returns the selected set for a tree drag and drops descendants
// when their selected ancestor directory already covers them.
func (c *Controller) DraggedPaths(rows []Row, primary string, normalize func([]string) []string) []string {
	paths := []string{primary}
	if c.handle().IsSelected(primary) {
		paths = c.handle().SelectedPaths(rows, primary)
	}
	if normalize != nil {
		return normalize(paths)
	}
	return paths
}

func (c *Controller) ApplyKey(command KeyCommand, rows []Row) bool {
	return c.handle().ApplyKey(command, rows)
}
func (c *Controller) CaptureClipboard(paths []string, cut bool) Clipboard {
	return c.handle().CaptureClipboard(paths, cut)
}
func (c *Controller) ClearDropHover()       { c.handle().ClearDropHover() }
func (c *Controller) ClearDropTargetRange() { c.handle().ClearDropTargetRange() }
func (c *Controller) ClearPointerPress()    { c.handle().ClearPointerPress() }
func (c *Controller) Clipboard() Clipboard  { return c.handle().Clipboard() }
func (c *Controller) CollapseAll()          { c.handle().CollapseAll() }
func (c *Controller) CreateAction(kind ActionKind, root, selected string, isDirectory func(string) bool) Action {
	if kind != CreateFile && kind != CreateFolder {
		return Action{}
	}
	selectedDirectory := selected != "" && isDirectory != nil && isDirectory(selected)
	return Action{Kind: kind, Directory: c.handle().CreateTarget(root, selected, selectedDirectory)}
}
func (c *Controller) DragOperationDisplay() DragOperationDisplay {
	return c.handle().DragOperationDisplay()
}
func (c *Controller) DragPreview() DragPreviewState       { return c.handle().DragPreview() }
func (c *Controller) DropHover() DropHoverState           { return c.handle().DropHover() }
func (c *Controller) DropTargetRange() DropTargetRange    { return c.handle().DropTargetRange() }
func (c *Controller) ExpandedIDs() []string               { return c.handle().ExpandedIDs() }
func (c *Controller) FocusID() string                     { return c.handle().FocusID() }
func (c *Controller) InvalidateRows()                     { c.handle().InvalidateRows() }
func (c *Controller) IsExpanded(id string) bool           { return c.handle().IsExpanded(id) }
func (c *Controller) IsSelected(id string) bool           { return c.handle().IsSelected(id) }
func (c *Controller) PreviewBounds() ui.Rect              { return c.handle().PreviewBounds() }
func (c *Controller) PreviewID() string                   { return c.handle().PreviewID() }
func (c *Controller) RemapIDs(mapper func(string) string) { c.handle().RemapIDs(mapper) }
func (c *Controller) RemoveSelected(id string)            { c.handle().RemoveSelected(id) }
func (c *Controller) RetainCutPaths(moved []string)       { c.handle().RetainCutPaths(moved) }
func (c *Controller) SelectID(id string, mods ui.Modifiers, entries []Entry) {
	c.handle().SelectID(id, mods, entries)
}
func (c *Controller) SelectedIDs() []string { return c.handle().SelectedIDs() }
func (c *Controller) SelectedPaths(rows []Row, fallback string) []string {
	return c.handle().SelectedPaths(rows, fallback)
}
func (c *Controller) SetDragOperationDisplay(display DragOperationDisplay) {
	c.handle().SetDragOperationDisplay(display)
}
func (c *Controller) SetDragPreview(preview DragPreviewState) { c.handle().SetDragPreview(preview) }
func (c *Controller) SetDropTargetRange(target string, start, end int) {
	c.handle().SetDropTargetRange(target, start, end)
}
func (c *Controller) SetExpanded(id string, expanded bool) { c.handle().SetExpanded(id, expanded) }
func (c *Controller) SetFocusID(id string)                 { c.handle().SetFocusID(id) }
func (c *Controller) SetPointerReleased(released bool)     { c.handle().SetPointerReleased(released) }
func (c *Controller) SetPreviewPopupHovered(hovered bool)  { c.handle().SetPreviewPopupHovered(hovered) }
func (c *Controller) SetScrollOffset(offset ui.Point)      { c.handle().SetScrollOffset(offset) }
func (c *Controller) SetSelection(ids []string)            { c.handle().SetSelection(ids) }
func (c *Controller) TrashTargetActive() bool              { return c.handle().TrashTargetActive() }
