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
	"muse/editor"
	"muse/events"
	"muse/nativeui/window"
	"muse/workspace"
)

var (
	ErrInvalidPane        = errors.New("noteeditor: pane id is required")
	ErrSurfaceUnavailable = errors.New("noteeditor: editor surface factory is not configured")
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

// SurfaceFactory adapts a backend-neutral editor session to generic window
// content. Composition injects the concrete MyGo component adapter.
type SurfaceFactory func(*editor.Session, SurfaceOptions) window.Content

type Content struct {
	paneID    workspace.PaneID
	path      string
	crlf      bool
	dirty     bool
	session   *editor.Session
	document  *documentapp.Session
	surface   window.Content
	onAction  func(Action)
	closed    bool
	closeSent bool
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
		paneID: paneID, path: note.Path, crlf: crlf,
		session: session, document: doc, onAction: onAction,
	}
	content.surface = factory(session, SurfaceOptions{OnChange: content.editorChanged, OnClose: content.surfaceClosed})
	if content.surface == nil {
		doc.Close()
		return nil, ErrSurfaceUnavailable
	}
	return content, nil
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
		PaneID: c.paneID, Path: c.path, Source: source,
		DiskSource: c.diskSource(source), Revision: state.Revision,
		CursorOffset: selection.Head, Dirty: c.dirty,
	}
}

func (c *Content) PersistenceState() documentapp.SessionState { return c.document.Snapshot() }
func (c *Content) SessionFrame() editor.Frame                 { return c.session.Frame() }

func (c *Content) SetSelection(selection editor.Selection) error {
	if err := c.ensureOpen(); err != nil {
		return err
	}
	return c.session.SetSelection(selection)
}

func (c *Content) Dispatch(event editor.InputEvent) (editor.DispatchResult, error) {
	if err := c.ensureOpen(); err != nil {
		return editor.DispatchResult{}, err
	}
	result, err := c.session.Dispatch(event)
	if err == nil && result.Changed {
		c.editorChanged()
	}
	return result, err
}

func (c *Content) DispatchKey(event editor.KeyEvent) bool {
	if c == nil || c.closed {
		return false
	}
	beforeSource, beforeSelection := c.session.Source(), c.session.Selection()
	handled := c.session.DispatchKey(event)
	if handled && (c.session.Source() != beforeSource || c.session.Selection() != beforeSelection) {
		c.editorChanged()
	}
	return handled
}

func (c *Content) ApplyTransaction(tx editor.Transaction) (editor.DispatchResult, error) {
	if err := c.ensureOpen(); err != nil {
		return editor.DispatchResult{}, err
	}
	result, err := c.session.ApplyTransaction(tx)
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
	_, err := c.session.ApplyTransaction(editor.Transaction{
		Before: selection,
		After:  editor.Selection{Anchor: len(source), Head: len(source)},
		Edits:  []editor.TextEdit{{Range: editor.TextRange{Start: 0, End: len(before)}, Text: source}},
	})
	if err == nil {
		c.editorChanged()
	}
	return err
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
	c.document.SetApplyingMetadata(true)
	c.session.ReplaceSource(source)
	c.document.SetApplyingMetadata(false)
	c.editorChanged()
	return nil
}

func (c *Content) Open(note document.Note, cursorOffset int) error {
	if err := c.ensureOpen(); err != nil {
		return err
	}
	source, crlf := normalizeDiskSource(note.Source)
	c.session.ReplaceSource(source)
	c.path, c.crlf, c.dirty = note.Path, crlf, false
	c.document.Open(note.Path, note.Source)
	cursorOffset = clampOffset(source, cursorOffset)
	return c.session.SetSelection(editor.Selection{Anchor: cursorOffset, Head: cursorOffset})
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
	current := c.document.CompleteSave(result, snapshot.Path, snapshot.DiskSource)
	if current && result.Err == nil {
		c.dirty = snapshot.DiskSource != result.Request.Source
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
		c.path = path
		c.document.Open(path, snapshot.DiskSource)
	}
	c.dirty = false
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
	return c.document.Rebase(path, revision, source)
}

func (c *Content) ClearAutosave() {
	if c != nil && !c.closed {
		c.document.ClearAutosave()
	}
}

func (c *Content) editorChanged() {
	if c == nil || c.closed {
		return
	}
	c.dirty = true
	c.document.Changed(time.Now())
	if c.onAction != nil {
		c.onAction(Action{Kind: DocumentChanged, PaneID: c.paneID, Snapshot: c.Snapshot()})
	}
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

var _ window.Content = (*Content)(nil)
