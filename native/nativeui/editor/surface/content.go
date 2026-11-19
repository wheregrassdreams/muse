// Package noteeditor composes one pane's document persistence session and
// backend-neutral editor session behind the generic window content contract.
package noteeditor

import (
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/egoist/mygo/ui"

	documentapp "muse/application/document"
	"muse/document"
	editor "github.com/wheregrassdreams/muse-editor/text/session"
	editorinput "github.com/wheregrassdreams/muse-editor/text/input"
	editorsource "github.com/wheregrassdreams/muse-editor/markdown/source"
	"muse/events"
	workspaceui "muse/nativeui/workspace"
	"muse/workspace"
)

var (
	ErrInvalidPane        = errors.New("noteeditor: pane id is required")
	ErrSurfaceUnavailable = errors.New("noteeditor: editor surface factory is not configured")
	ErrIncompleteSurface  = errors.New("noteeditor: editor surface is missing required command capabilities")
	ErrContentClosed      = errors.New("noteeditor: content is closed")
)

type ActionKind uint8

const (
	DocumentChanged ActionKind = iota + 1
	ContentClosed
)

type Snapshot struct {
	PaneID       workspace.PaneID
	Path         string
	Source       string
	DiskSource   string
	Revision     document.Revision
	CursorOffset int
	Dirty        bool
}

// MemorySnapshot is a backend-neutral estimate of resources retained by the
// active note surface. It keeps diagnostics independent of the editor widget.
type MemorySnapshot struct {
	SourceBytes          int
	Leaves               int
	VirtualLayout        bool
	ImageCandidateLeaves int
	ImageAssetViews      int
	ImageAssets          int
	LoadedImageAssets    int
	ImagePixelBytes      int64
}

type Action struct {
	Kind     ActionKind
	PaneID   workspace.PaneID
	Snapshot Snapshot
}

type Dependencies struct {
	Documents document.Repository
	Recovery  document.RecoveryRepository
	History   document.HistoryRepository
	Events    *events.Dispatcher[document.DocumentSaved]
}

// SurfaceOptions are callbacks the editor component needs from this feature
// surface. Rendering configuration stays captured by the injected factory.
type SurfaceOptions struct {
	OnChange func()
	OnClose  func()
}

type Heading struct {
	Level  int
	Text   string
	Offset int
}

type TextRange struct{ Start, End int }

// Command ports are intentionally grouped by behavior. A concrete renderer
// may implement only the capabilities it supports; document/session behavior
// remains on Content itself.
type FocusCommands interface {
	Focus()
	Focused() bool
}

type ModalCommands interface {
	Keymap() string
	SetKeymap(string)
	ModeLabel() string
	ModalAcceptsText() bool
}

type NavigationCommands interface {
	Caret() int
	SelectionRange() TextRange
	Select(start, end int)
	ScrollY() float32
	ScrollToOffset(offset int)
	NavigateToOffset(offset int)
	NavigateToRange(start, end int)
	Headings() []Heading
	WindowPoint(offset int) (x, y float32, ok bool)
}

type SearchCommands interface {
	SetHighlights(ranges []TextRange, current int)
}

type ClipboardCommands interface {
	SelectedText() string
	CopyImageObject(path string)
}

type FormattingCommands interface {
	AppendBlock(text string)
	PrependBlock(text string)
	ToggleBold()
	ToggleItalic()
	ToggleUnderline()
	ToggleStrike()
	ToggleCode()
	ToggleHighlight()
	ToggleQuote()
	ToggleList(marker string)
	SetHeading(level int)
	InsertCodeBlock()
	InsertMathBlock()
	InsertTable(rows, columns int)
	InsertRule()
	InsertLink()
}

type PresentationSettings struct {
	Header                   func(*ui.Context)
	FontSize, LineHeight     float32
	DisableSmartTypography   bool
	HeadingMarkerReveal      bool
	VimVirtualImageRows      bool
	CursorFade, SmoothCursor bool
	HideMouseCursor          bool
	SmoothPageJumps          bool
	SearchMatchPulse         bool
	VimYankFlash             bool
	TopInset, HeaderHeight   float32
	HideFrontmatter          bool
	BaseDir, AssetRoot       string
	CurrentPath, Code        string
	LinkRevision             uint64
	LinkContext              string
}

type PresentationCommands interface {
	Configure(PresentationSettings)
	Settings() PresentationSettings
}

// DocumentOpenCommands clears presentation state owned by the editor widget
// before this pane's canonical session is replaced with another document.
type DocumentOpenCommands interface {
	ResetForDocumentOpen()
}

// SystemEditCommands applies application-owned document changes as a system
// patch, preserving editor history and selection semantics.
type SystemEditCommands interface {
	ReplaceSystemAll(string)
}

// SurfaceFactory adapts a backend-neutral editor session to generic window
// content. Composition injects the concrete MyGo component adapter.
type SurfaceFactory func(*editor.Session, SurfaceOptions) workspaceui.Content

type Content struct {
	paneID    workspace.PaneID
	crlf      bool
	dirty     bool
	session   *editor.Session
	document  *documentapp.Session
	surface   workspaceui.Content
	onAction  func(Action)
	closed    bool
	closeSent bool
	mutating  bool
}

// Commands is the feature-owned capability App may use while the pane handle
// is live. It deliberately omits Render and Close, which belong to workspaceui.Manager.
type Commands interface {
	FocusCommands() FocusCommands
	ModalCommands() ModalCommands
	NavigationCommands() NavigationCommands
	SearchCommands() SearchCommands
	ClipboardCommands() ClipboardCommands
	FormattingCommands() FormattingCommands
	PresentationCommands() PresentationCommands
	WantsUnmodifiedShortcuts() bool
	Configure(PresentationSettings) bool
	Source() string
	Undo() bool
	Replace(start, end int, value string) error
	Snapshot() Snapshot
	PersistenceState() documentapp.SessionState
	BeginSave(time.Time) (documentapp.SaveRequest, bool)
	StartSave(documentapp.SaveRequest)
	Results() <-chan documentapp.SaveResult
	CompleteSave(documentapp.SaveResult) bool
	InvalidatePendingSave(draftKey string) (documentapp.SaveResult, bool, error)
	MemorySnapshot() MemorySnapshot
	ListDrafts() ([]document.Draft, error)
	ReadDraft(key string) (document.Draft, error)
	ListHistory(path string) ([]document.HistoryEntry, error)
	ReadHistory(path, id string) (document.Note, error)
	DestinationRevision(path string, expected document.Revision) (document.Revision, error)
	DeleteDraft(path string) error
	ReplaceUser(source string) error
	ReplaceSystem(source string) error
	MarkDirty()
	Open(note document.Note, cursorOffset int) error
	Relocate(note document.Note) error
	SaveNow(path string, expected document.Revision) (document.Note, error)
	SaveExact(path, source string, expected document.Revision) (document.Note, error)
	PreserveDraft() (string, error)
	SetConflict(path string)
	Rebase(path string, revision document.Revision, source string) bool
	DeferExternal(note document.Note) bool
	TakeDeferredExternal() (document.Note, bool)
	ClearAutosave()
	ClearMetadataDeadline()
}

var _ Commands = (*Content)(nil)

// WithCommands resolves a live generic window handle and narrows it inside
// the noteeditor owner package, so App never stores concrete surface state.
func WithCommands(manager *workspaceui.Manager, handle workspaceui.Handle, use func(Commands) error) error {
	if use == nil {
		return workspaceui.ErrHandleUnavailable
	}
	return manager.WithContent(handle, func(content workspaceui.Content) error {
		commands, ok := content.(*Content)
		if !ok || commands == nil || commands.closed {
			return ErrContentClosed
		}
		return use(commands)
	})
}

func (c *Content) FocusCommands() FocusCommands {
	if c == nil {
		return nil
	}
	commands, _ := c.surface.(FocusCommands)
	return commands
}

func (c *Content) ModalCommands() ModalCommands {
	if c == nil {
		return nil
	}
	commands, _ := c.surface.(ModalCommands)
	return commands
}

func (c *Content) NavigationCommands() NavigationCommands {
	if c == nil {
		return nil
	}
	commands, _ := c.surface.(NavigationCommands)
	return commands
}

func (c *Content) SearchCommands() SearchCommands {
	if c == nil {
		return nil
	}
	commands, _ := c.surface.(SearchCommands)
	return commands
}

func (c *Content) ClipboardCommands() ClipboardCommands {
	if c == nil {
		return nil
	}
	commands, _ := c.surface.(ClipboardCommands)
	return commands
}

func (c *Content) FormattingCommands() FormattingCommands {
	if c == nil {
		return nil
	}
	commands, _ := c.surface.(FormattingCommands)
	return commands
}

func (c *Content) PresentationCommands() PresentationCommands {
	if c == nil {
		return nil
	}
	commands, _ := c.surface.(PresentationCommands)
	return commands
}

// WantsUnmodifiedShortcuts reports whether the active modal keymap consumes
// plain text, leaving ordinary application shortcuts available.
func (c *Content) WantsUnmodifiedShortcuts() bool {
	commands := c.ModalCommands()
	return commands != nil && commands.Keymap() == "vim" && !commands.ModalAcceptsText()
}

func (c *Content) Configure(settings PresentationSettings) bool {
	commands := c.PresentationCommands()
	if commands == nil || c.closed {
		return false
	}
	commands.Configure(settings)
	return true
}

func (c *Content) Source() string { return c.Snapshot().Source }

func (c *Content) Undo() bool {
	if c == nil || c.closed {
		return false
	}
	commands, ok := c.surface.(interface{ Undo() bool })
	return ok && commands.Undo()
}

func (c *Content) Replace(start, end int, value string) error {
	if err := c.ensureOpen(); err != nil {
		return err
	}
	before := c.session.Source()
	selection := c.session.Selection()
	var result editor.DispatchResult
	var err error
	c.mutateSession(func() {
		result, err = c.session.ApplyTransaction(editor.Transaction{
			Before: selection,
			After:  editor.Selection{Anchor: start + len(value), Head: start + len(value)},
			Edits:  []editor.TextEdit{{Range: editor.TextRange{Start: start, End: end}, Text: value}},
		})
	})
	if err == nil && result.Changed && c.session.Source() != before {
		c.editorChanged()
	}
	return err
}

// NewContent creates the durable and editing session for one workspace pane.
// note.Source is the disk source; CRLF is normalized only inside the editor.
func NewContent(
	paneID workspace.PaneID,
	note document.Note,
	cursorOffset int,
	dependencies Dependencies,
	factory SurfaceFactory,
	onAction func(Action),
) (*Content, error) {
	if paneID == "" {
		return nil, ErrInvalidPane
	}
	if factory == nil {
		return nil, ErrSurfaceUnavailable
	}
	if note.Revision == "" {
		note = document.NewNote(note.Path, note.Source, document.RevisionOf(note.Source))
	}
	source, crlf := normalizeDiskSource(note.Source)
	doc := documentapp.NewSession(dependencies.Documents, dependencies.Recovery, dependencies.History, dependencies.Events)
	doc.Open(note.Path, note.Source)
	session := editor.NewSession(source)
	cursorOffset = clampOffset(source, cursorOffset)
	if err := session.SetSelection(editor.Selection{Anchor: cursorOffset, Head: cursorOffset}); err != nil {
		doc.Close()
		return nil, err
	}
	content := &Content{
		paneID: paneID, crlf: crlf,
		session: session, document: doc, onAction: onAction,
	}
	content.surface = factory(session, SurfaceOptions{OnChange: content.editorChanged, OnClose: content.surfaceClosed})
	if content.surface == nil {
		doc.Close()
		return nil, ErrSurfaceUnavailable
	}
	if !surfaceHasRequiredCommands(content.surface) {
		content.surface.Close()
		doc.Close()
		return nil, ErrIncompleteSurface
	}
	// The surface's presentation settings determine which source ranges are
	// visible. Apply the same source-to-display start mapping on first mount as
	// on later document opens, once those settings are available.
	cursorOffset = initialVisibleOffset(source, cursorOffset, content.surface)
	cursorOffset = clampOffset(source, cursorOffset)
	if err := session.SetSelection(editor.Selection{Anchor: cursorOffset, Head: cursorOffset}); err != nil {
		content.Close()
		return nil, err
	}
	return content, nil
}

func surfaceHasRequiredCommands(surface workspaceui.Content) bool {
	if surface == nil {
		return false
	}
	_, focus := surface.(FocusCommands)
	_, modal := surface.(ModalCommands)
	_, navigation := surface.(NavigationCommands)
	_, search := surface.(SearchCommands)
	_, clipboard := surface.(ClipboardCommands)
	_, formatting := surface.(FormattingCommands)
	_, presentation := surface.(PresentationCommands)
	_, documentOpen := surface.(DocumentOpenCommands)
	_, systemEdit := surface.(SystemEditCommands)
	_, undo := surface.(interface{ Undo() bool })
	return focus && modal && navigation && search && clipboard && formatting && presentation && documentOpen && systemEdit && undo
}

func NewUntitled(
	paneID workspace.PaneID,
	source string,
	dependencies Dependencies,
	factory SurfaceFactory,
	onAction func(Action),
) (*Content, error) {
	return NewContent(paneID, document.NewNote("", source, document.RevisionOf(source)), 0, dependencies, factory, onAction)
}

func (c *Content) Render(ctx *ui.Context) {
	if c == nil || c.closed || c.surface == nil {
		return
	}
	c.surface.Render(ctx)
}

func (c *Content) Close() {
	if c == nil || c.closed {
		return
	}
	c.closed = true
	c.document.Close()
	if c.surface != nil {
		c.surface.Close()
	}
	c.emitClose()
}

func (c *Content) PaneID() workspace.PaneID { return c.paneID }

func (c *Content) Snapshot() Snapshot {
	if c == nil || c.session == nil {
		return Snapshot{}
	}
	source := c.session.Source()
	selection := c.session.Selection()
	state := c.document.Snapshot()
	return Snapshot{
		PaneID: c.paneID, Path: state.Path, Source: source,
		DiskSource: c.diskSource(source), Revision: state.Revision,
		CursorOffset: selection.Head, Dirty: c.dirty,
	}
}

func (c *Content) PersistenceState() documentapp.SessionState { return c.document.Snapshot() }
func (c *Content) SessionFrame() editor.Frame                 { return c.session.Frame() }

// MemorySnapshot forwards renderer-owned resource estimates through the
// backend-neutral editor model contract.
func (c *Content) MemorySnapshot() MemorySnapshot {
	if c == nil || c.surface == nil {
		return MemorySnapshot{}
	}
	provider, ok := c.surface.(interface{ MemorySnapshot() MemorySnapshot })
	if !ok {
		return MemorySnapshot{}
	}
	return provider.MemorySnapshot()
}

func (c *Content) ListDrafts() ([]document.Draft, error) {
	if err := c.ensureOpen(); err != nil {
		return nil, err
	}
	return c.document.ListDrafts()
}

func (c *Content) ReadDraft(key string) (document.Draft, error) {
	if err := c.ensureOpen(); err != nil {
		return document.Draft{}, err
	}
	return c.document.ReadDraft(key)
}

func (c *Content) ListHistory(path string) ([]document.HistoryEntry, error) {
	if err := c.ensureOpen(); err != nil {
		return nil, err
	}
	return c.document.ListHistory(path)
}

func (c *Content) ReadHistory(path, id string) (document.Note, error) {
	if err := c.ensureOpen(); err != nil {
		return document.Note{}, err
	}
	return c.document.ReadHistory(path, id)
}

func (c *Content) BaseSource() string { return c.PersistenceState().BaseSource }

func (c *Content) AutosaveState() documentapp.SessionState { return c.PersistenceState() }

func (c *Content) DestinationRevision(path string, expected document.Revision) (document.Revision, error) {
	if c == nil || c.document == nil {
		return "", ErrContentClosed
	}
	return c.document.DestinationRevision(path, c.document.Snapshot().Path, expected)
}

func (c *Content) DeleteDraft(path string) error {
	if c == nil || c.document == nil {
		return ErrContentClosed
	}
	return c.document.DeleteDraft(path)
}

func (c *Content) SetSelection(selection editor.Selection) error {
	if err := c.ensureOpen(); err != nil {
		return err
	}
	return c.session.SetSelection(selection)
}

func (c *Content) Dispatch(event editorinput.Event) (editor.DispatchResult, error) {
	if err := c.ensureOpen(); err != nil {
		return editor.DispatchResult{}, err
	}
	var result editor.DispatchResult
	var err error
	c.mutateSession(func() { result, err = c.session.Dispatch(event) })
	if err == nil && result.Changed {
		c.editorChanged()
	}
	return result, err
}

func (c *Content) DispatchKey(event editorinput.KeyEvent) bool {
	if c == nil || c.closed {
		return false
	}
	beforeSource, beforeSelection := c.session.Source(), c.session.Selection()
	var handled bool
	c.mutateSession(func() { handled = c.session.DispatchKey(event) })
	if handled && (c.session.Source() != beforeSource || c.session.Selection() != beforeSelection) {
		c.editorChanged()
	}
	return handled
}

func (c *Content) ApplyTransaction(tx editor.Transaction) (editor.DispatchResult, error) {
	if err := c.ensureOpen(); err != nil {
		return editor.DispatchResult{}, err
	}
	var result editor.DispatchResult
	var err error
	c.mutateSession(func() { result, err = c.session.ApplyTransaction(tx) })
	if err == nil && result.Changed {
		c.editorChanged()
	}
	return result, err
}

// ReplaceUser applies a whole-source change as one undoable transaction.
func (c *Content) ReplaceUser(source string) error {
	if err := c.ensureOpen(); err != nil {
		return err
	}
	before := c.session.Source()
	if before == source {
		return nil
	}
	selection := c.session.Selection()
	var err error
	c.mutateSession(func() {
		_, err = c.session.ApplyTransaction(editor.Transaction{
			Before: selection,
			After:  editor.Selection{Anchor: len(source), Head: len(source)},
			Edits:  []editor.TextEdit{{Range: editor.TextRange{Start: 0, End: len(before)}, Text: source}},
		})
	})
	if err == nil {
		c.editorChanged()
	}
	return err
}

func (c *Content) MarkDirty() {
	if c != nil && !c.closed {
		c.editorChanged()
	}
}

// ReplaceSystem applies a system change while preserving the normal editor
// source and selection contract. The update remains part of the current draft.
func (c *Content) ReplaceSystem(source string) error {
	if err := c.ensureOpen(); err != nil {
		return err
	}
	if c.session.Source() == source {
		return nil
	}
	systemEdit, ok := c.surface.(SystemEditCommands)
	if !ok {
		return ErrIncompleteSurface
	}
	c.mutateSession(func() { systemEdit.ReplaceSystemAll(source) })
	c.metadataChanged()
	return nil
}

func (c *Content) Open(note document.Note, cursorOffset int) error {
	if err := c.ensureOpen(); err != nil {
		return err
	}
	source, crlf := normalizeDiskSource(note.Source)
	// The renderer can retain transient state from its previous document (for
	// example, the phantom first line of an empty note). Reset it before the
	// shared session is replaced and parsed again.
	if lifecycle, ok := c.surface.(DocumentOpenCommands); ok {
		lifecycle.ResetForDocumentOpen()
	}
	c.mutateSession(func() { c.session.ReplaceSource(source) })
	c.crlf, c.dirty = crlf, false
	c.document.Open(note.Path, note.Source)
	cursorOffset = initialVisibleOffset(source, cursorOffset, c.surface)
	cursorOffset = clampOffset(source, cursorOffset)
	return c.session.SetSelection(editor.Selection{Anchor: cursorOffset, Head: cursorOffset})
}

func initialVisibleOffset(source string, requested int, surface workspaceui.Content) int {
	if requested != 0 {
		return requested
	}
	presentation, ok := surface.(PresentationCommands)
	if !ok || !presentation.Settings().HideFrontmatter {
		return requested
	}
	frontmatterEnd := editorsource.FrontmatterEnd(source)
	if frontmatterEnd == 0 {
		return requested
	}
	for offset := frontmatterEnd; offset < len(source); {
		for offset < len(source) && (source[offset] == '\r' || source[offset] == '\n') {
			offset++
		}
		if offset >= len(source) {
			return len(source)
		}
		end := strings.IndexByte(source[offset:], '\n')
		if end < 0 {
			end = len(source)
		} else {
			end += offset
		}
		if strings.TrimSpace(source[offset:end]) != "" {
			return offset
		}
		offset = end + 1
	}
	return len(source)
}

// Relocate changes the document's durable identity after a vault move while
// keeping its active buffer and selection. If the destination differs from
// the saved source, the current buffer remains a dirty draft against the new
// destination baseline.
func (c *Content) Relocate(note document.Note) error {
	if err := c.ensureOpen(); err != nil {
		return err
	}
	current := c.session.Source()
	_, crlf := normalizeDiskSource(note.Source)
	c.crlf = crlf
	c.document.Open(note.Path, note.Source)
	c.dirty = c.diskSource(current) != note.Source
	if c.dirty {
		c.document.Changed(time.Now())
	}
	return nil
}

func (c *Content) Load(path string, cursorOffset int) error {
	if err := c.ensureOpen(); err != nil {
		return err
	}
	note, err := c.document.Load(path)
	if err != nil {
		return err
	}
	return c.Open(note, cursorOffset)
}

func (c *Content) BeginSave(now time.Time) (documentapp.SaveRequest, bool) {
	if c == nil || c.closed {
		return documentapp.SaveRequest{}, false
	}
	snapshot := c.Snapshot()
	return c.document.BeginSave(snapshot.Path, snapshot.DiskSource, snapshot.Dirty, now)
}

func (c *Content) StartSave(request documentapp.SaveRequest) {
	if c != nil && !c.closed {
		c.document.StartSave(request)
	}
}

func (c *Content) Results() <-chan documentapp.SaveResult {
	if c == nil || c.document == nil {
		return nil
	}
	return c.document.Results()
}

func (c *Content) CompleteSave(result documentapp.SaveResult) bool {
	if c == nil || c.closed {
		return false
	}
	snapshot := c.Snapshot()
	current := c.document.CompleteSave(result, snapshot.Path)
	if current && result.Err == nil {
		// A conflict save writes a recovery draft instead of changing the
		// durable note. Keep the live document dirty so the conflict remains
		// visible and an explicit resolution is still required.
		c.dirty = result.Request.Conflict || snapshot.DiskSource != result.Request.Source
	}
	return current
}

func (c *Content) SaveNow(path string, expected document.Revision) (document.Note, error) {
	if err := c.ensureOpen(); err != nil {
		return document.Note{}, err
	}
	snapshot := c.Snapshot()
	if path == "" {
		path = snapshot.Path
	}
	key := path
	if key == "" {
		key = c.document.Snapshot().DraftKey
	}
	if path != snapshot.Path {
		var err error
		expected, err = c.document.DestinationRevision(path, snapshot.Path, expected)
		if err != nil {
			return document.Note{}, err
		}
	}
	note, err := c.document.SaveNow(key, path, snapshot.DiskSource, expected)
	if err != nil {
		return document.Note{}, err
	}
	if path != snapshot.Path {
		c.document.Open(path, snapshot.DiskSource)
	}
	c.dirty = false
	return note, nil
}

// SaveExact persists an application-captured source through the session save
// gate without changing the active document identity. Save As writes its
// prepared snapshot through this method, then relocates the editor after.
func (c *Content) SaveExact(path, source string, expected document.Revision) (document.Note, error) {
	if c == nil || c.document == nil {
		return document.Note{}, ErrContentClosed
	}
	state := c.document.Snapshot()
	if path == "" {
		path = state.Path
	}
	key := path
	if key == "" {
		key = state.DraftKey
	}
	note, err := c.document.SaveNow(key, path, source, expected)
	if err != nil {
		return document.Note{}, err
	}
	return note, nil
}

func (c *Content) PreserveDraft() (string, error) {
	if err := c.ensureOpen(); err != nil {
		return "", err
	}
	snapshot := c.Snapshot()
	return c.document.PreserveDraft(snapshot.Path, snapshot.DiskSource)
}

func (c *Content) SetConflict(path string) {
	if c != nil && !c.closed {
		c.document.SetConflict(path)
	}
}

func (c *Content) Rebase(path string, revision document.Revision, source string) bool {
	if c == nil || c.closed {
		return false
	}
	if !c.document.Rebase(path, revision, source) {
		return false
	}
	c.dirty = c.diskSource(c.session.Source()) != source
	if c.dirty {
		c.document.Changed(time.Now())
	}
	return true
}

func (c *Content) DeferExternal(note document.Note) bool {
	return c != nil && !c.closed && c.document.DeferExternal(note)
}

func (c *Content) TakeDeferredExternal() (document.Note, bool) {
	if c == nil || c.closed {
		return document.Note{}, false
	}
	return c.document.TakeDeferredExternal()
}

func (c *Content) ClearAutosave() {
	if c != nil && !c.closed {
		c.document.ClearAutosave()
	}
}

func (c *Content) InvalidatePendingSave(draftKey string) (documentapp.SaveResult, bool, error) {
	if c == nil || c.closed || c.document == nil {
		return documentapp.SaveResult{}, false, documentapp.ErrWorkflowUnavailable
	}
	return c.document.InvalidatePendingSave(draftKey)
}

func (c *Content) ClearMetadataDeadline() {
	if c != nil && !c.closed {
		c.document.ClearMetadataDeadline()
	}
}

func (c *Content) editorChanged() {
	c.sourceChanged(false)
}

func (c *Content) metadataChanged() {
	c.sourceChanged(true)
}

func (c *Content) sourceChanged(metadata bool) {
	if c == nil || c.closed {
		return
	}
	if c.mutating {
		return
	}
	c.dirty = true
	if metadata {
		c.document.MetadataChanged(time.Now())
	} else {
		c.document.Changed(time.Now())
	}
	if c.onAction != nil {
		c.onAction(Action{Kind: DocumentChanged, PaneID: c.paneID, Snapshot: c.Snapshot()})
	}
}

// mutateSession suppresses the renderer's OnChange callback while Content is
// applying a semantic operation. The operation reports one change after it
// returns, so persistence and App subscribers receive exactly one notification.
func (c *Content) mutateSession(apply func()) {
	if c == nil || apply == nil {
		return
	}
	wasMutating := c.mutating
	c.mutating = true
	defer func() { c.mutating = wasMutating }()
	apply()
}

func (c *Content) surfaceClosed() {
	if c == nil || c.closed {
		return
	}
	c.closed = true
	c.document.Close()
	c.emitClose()
}

func (c *Content) emitClose() {
	if c.closeSent {
		return
	}
	c.closeSent = true
	if c.onAction != nil {
		c.onAction(Action{Kind: ContentClosed, PaneID: c.paneID})
	}
}

func (c *Content) ensureOpen() error {
	if c == nil || c.closed {
		return ErrContentClosed
	}
	return nil
}

func (c *Content) diskSource(source string) string {
	if c.crlf {
		return strings.ReplaceAll(source, "\n", "\r\n")
	}
	return source
}

func normalizeDiskSource(source string) (string, bool) {
	crlf := strings.Contains(source, "\r\n")
	if crlf {
		source = strings.ReplaceAll(source, "\r\n", "\n")
	}
	return source, crlf
}

func clampOffset(source string, offset int) int {
	offset = max(0, min(offset, len(source)))
	for offset > 0 && offset < len(source) && !utf8.RuneStart(source[offset]) {
		offset--
	}
	return offset
}

var _ workspaceui.Content = (*Content)(nil)
