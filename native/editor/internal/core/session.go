package core

import (
	"fmt"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	editorinput "muse/editor/input"
)

// DispatchResult describes state changes caused by one semantic input.
type DispatchResult struct {
	Handled          bool
	Changed          bool
	SelectionChanged bool
	Revision         uint64
}

// ChangeKind identifies a committed mutation of the session-owned document.
// Presentation adapters observe changes to invalidate derived state; they do
// not write source, selection, or history back into the Session.
type ChangeKind uint8

const (
	ChangeSelection ChangeKind = iota + 1
	ChangeEdit
	ChangeUndo
	ChangeRedo
	ChangeReplaceSource
)

// Change is an immutable, detached description of one committed Session
// mutation. Go strings are immutable and can safely share the old revision's
// backing storage.
type Change struct {
	Kind         ChangeKind
	BeforeSource string
	AfterSource  string
	Before       Selection
	After        Selection
	Revision     uint64
	// Edits contains detached replacements in the source coordinate space
	// immediately before this change. Undo and redo report their actual inverse
	// and reapplied transactions respectively.
	Edits []TextEdit
}

// PresentationAdapter supplies the keymap and block-aware command effects of
// a live editor session. It never owns document data or parsed projections.
type PresentationAdapter struct {
	DispatchKey    func(editorinput.KeyEvent) bool
	ApplyOperation func(editorinput.Operation) bool
}

type HistoryGroup uint8

const (
	HistoryGroupNone HistoryGroup = iota
	HistoryGroupTyping
	HistoryGroupDelete
	HistoryGroupExplicit
)

type TransactionOptions struct {
	Group          HistoryGroup
	At             time.Time
	MergePrevious  bool
	BeforeMetadata HistorySnapshot
	AfterMetadata  HistorySnapshot
}

// HistorySnapshot is a detached, immutable value for state owned by an
// editor subsystem that must travel with source undo (for example table
// selection or typed paragraph breaks).
type HistorySnapshot interface {
	Clone() HistorySnapshot
}

// HistoryParticipant contributes feature-owned typed snapshots to the
// Session's history. Source, selection and the undo stack remain Session
// owned; participants cannot modify them.
type HistoryParticipant interface {
	CaptureHistory() HistorySnapshot
	RestoreHistory(HistorySnapshot)
}

// Frame is an immutable, backend-neutral snapshot of the source revision and
// text selection. Markdown parsing, layout, and rendering are owned by their
// respective editor and UI modules.
type Frame struct {
	Revision  uint64
	Source    string
	Selection Selection
}

// Session is the authoritative source, selection, revision and undo history
// for one document. Presentation adapters receive committed changes and keep
// only derived parser/layout/render state. A standalone Session serializes
// concurrent reads and writes; when bound to a live UI adapter, calls that
// touch widgets must stay on that adapter's owner thread.
//
// Parsed Markdown and rendered projections live outside core.Session. The
// Session frame contains only values authoritative to this module.
type Session struct {
	editMu             sync.Mutex
	mu                 sync.RWMutex
	src                string
	anchor, caret      int
	selectionInclusive bool
	selectionLinewise  bool
	rev                uint64
	undo, redo         []sessionStep
	presentation       PresentationAdapter
	changeHook         func(Change)
	historyParticipant HistoryParticipant
}

type sessionStep struct {
	edits          []historyEdit
	before         Selection
	after          Selection
	group          HistoryGroup
	at             time.Time
	closed         bool
	beforeMetadata HistorySnapshot
	afterMetadata  HistorySnapshot
	mergePrevious  bool
}

type historyEdit struct {
	at       int
	old, new string
}

// EditBatch holds the Session's source-write lock across a multi-edit command.
// Edits become visible to Session readers immediately, but enter history and
// notify presentation only when committed.
type EditBatch struct {
	session        *Session
	options        TransactionOptions
	beforeSource   string
	before         Selection
	beforeMetadata HistorySnapshot
	edits          []historyEdit
	active         bool
}

func NewSession(source string) *Session {
	return &Session{src: strings.Clone(source)}
}

// BindPresentationAdapter connects keymap and block-aware effects to this
// session. Rebinding replaces the whole adapter atomically.
func (s *Session) BindPresentationAdapter(adapter PresentationAdapter) {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.presentation = adapter
	s.mu.Unlock()
}

// DetachPresentation releases callbacks owned by a live editor adapter. The
// source, selection, and undo history remain valid so the Session can outlive
// a view and be attached to another one.
func (s *Session) DetachPresentation() {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.presentation = PresentationAdapter{}
	s.changeHook = nil
	s.historyParticipant = nil
	s.mu.Unlock()
}

// BindHistoryParticipant replaces the feature-owned snapshot participant for
// this session. A pane may use one aggregate participant to compose block
// subsystem snapshots without exposing those types to the editor
func (s *Session) BindHistoryParticipant(participant HistoryParticipant) {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.historyParticipant = participant
	s.mu.Unlock()
}

func cloneHistorySnapshot(snapshot HistorySnapshot) HistorySnapshot {
	if snapshot == nil {
		return nil
	}
	return snapshot.Clone()
}

func (s *Session) historyParticipantSnapshot() HistoryParticipant {
	s.mu.RLock()
	participant := s.historyParticipant
	s.mu.RUnlock()
	return participant
}

// CaptureHistory returns an immutable snapshot from the bound history
// participant. Callers capture it before changing block-specific state that
// must be restored with the next source transaction.
func (s *Session) CaptureHistory() HistorySnapshot {
	if s == nil {
		return nil
	}
	participant := s.historyParticipantSnapshot()
	if participant == nil {
		return nil
	}
	return cloneHistorySnapshot(participant.CaptureHistory())
}

// CloseHistoryGroup prevents the next transaction from joining the last
// typing or deletion step.
func (s *Session) CloseHistoryGroup() {
	if s == nil {
		return
	}
	s.editMu.Lock()
	s.mu.Lock()
	s.closeHistoryGroupLocked()
	s.mu.Unlock()
	s.editMu.Unlock()
}

func (s *Session) closeHistoryGroupLocked() {
	if n := len(s.undo); n > 0 {
		s.undo[n-1].closed = true
	}
}

// UpdateLastHistoryAfter captures the current participant snapshot as the
// after-state of the latest step. Use it when a command finalizes block-owned
// state immediately after committing its source batch.
func (s *Session) UpdateLastHistoryAfter() bool {
	if s == nil {
		return false
	}
	s.editMu.Lock()
	defer s.editMu.Unlock()
	participant := s.historyParticipantSnapshot()
	var snapshot HistorySnapshot
	if participant != nil {
		snapshot = cloneHistorySnapshot(participant.CaptureHistory())
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.undo) == 0 {
		return false
	}
	s.undo[len(s.undo)-1].afterMetadata = snapshot
	return true
}

func (s *Session) Source() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.src
}

func (s *Session) Revision() uint64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.rev
}

func (s *Session) Selection() Selection {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.selectionLocked()
}

func (s *Session) selectionLocked() Selection {
	return Selection{
		Anchor: s.anchor, Head: s.caret,
		Inclusive: s.selectionInclusive, Linewise: s.selectionLinewise,
	}
}

func (s *Session) Frame() Frame {
	if s == nil {
		return Frame{}
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return Frame{Revision: s.rev, Source: s.src, Selection: s.selectionLocked()}
}

func (s *Session) SetSelection(selection Selection) error {
	s.editMu.Lock()
	s.mu.Lock()
	if err := validateSelection(s.src, selection); err != nil {
		s.mu.Unlock()
		s.editMu.Unlock()
		return err
	}
	before := s.selectionLocked()
	s.setSelection(selection)
	if before != selection {
		s.closeHistoryGroupLocked()
	}
	change := Change{Kind: ChangeSelection, BeforeSource: s.src, AfterSource: s.src, Before: before, After: selection, Revision: s.rev}
	s.mu.Unlock()
	s.editMu.Unlock()
	if before != selection {
		s.notifyChange(change)
	}
	return nil
}

// SetChangeHook installs the presentation invalidation hook. Session calls it
// after committing a mutation and releasing its lock. The hook may inspect the
// Session but must not use this callback to mutate document state.
func (s *Session) SetChangeHook(hook func(Change)) {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.changeHook = hook
	s.mu.Unlock()
}

func (s *Session) notifyChange(change Change) {
	if s == nil {
		return
	}
	s.mu.RLock()
	hook := s.changeHook
	s.mu.RUnlock()
	if hook != nil {
		hook(change)
	}
}

func cloneEdits(edits []TextEdit) []TextEdit {
	if len(edits) == 0 {
		return nil
	}
	return append([]TextEdit(nil), edits...)
}

func (s *Session) setSelection(selection Selection) {
	s.anchor, s.caret = selection.Anchor, selection.Head
	s.selectionInclusive, s.selectionLinewise = selection.Inclusive, selection.Linewise
}

// ApplyOperation dispatches a keymap-neutral editor operation through the
// owning editor session. Operations such as Vim find/fold/put depend on the
// editor's block and modal state, so standalone source-only sessions report
// them as unhandled.
func (s *Session) ApplyOperation(operation editorinput.Operation) bool {
	if s == nil {
		return false
	}
	s.mu.RLock()
	apply := s.presentation.ApplyOperation
	s.mu.RUnlock()
	return apply != nil && apply(operation)
}

// DispatchKey forwards a physical, toolkit-neutral key to the owning
// keymap. Standalone sessions have no keymap and return false.
func (s *Session) DispatchKey(event editorinput.KeyEvent) bool {
	if s == nil {
		return false
	}
	s.mu.RLock()
	dispatch := s.presentation.DispatchKey
	s.mu.RUnlock()
	return dispatch != nil && dispatch(event)
}

// Dispatch applies one backend-neutral input intent and records a normal
// editor undo step. The production Editor's keymap adapter routes richer
// commands through the same Session-owned source/selection/history fields.
func (s *Session) Dispatch(event editorinput.Event) (result DispatchResult, err error) {
	if event.Kind == editorinput.Undo {
		return s.Undo()
	}
	if event.Kind == editorinput.Redo {
		return s.Redo()
	}
	s.editMu.Lock()
	s.mu.Lock()
	beforeSource := s.src
	before := s.selectionLocked()
	kind := ChangeEdit
	switch event.Kind {
	case editorinput.MoveHorizontal, editorinput.SetSelection:
		kind = ChangeSelection
	case editorinput.Undo:
		kind = ChangeUndo
	case editorinput.Redo:
		kind = ChangeRedo
	}
	var changedEdits []TextEdit
	defer func() {
		change := Change{Kind: kind, BeforeSource: beforeSource, AfterSource: s.src, Before: before, After: s.selectionLocked(), Revision: s.rev, Edits: changedEdits}
		changed := change.BeforeSource != change.AfterSource || change.Before != change.After
		s.mu.Unlock()
		s.editMu.Unlock()
		if changed {
			s.notifyChange(change)
		}
	}()
	var tx Transaction
	switch event.Kind {
	case editorinput.InsertText:
		if event.Text == "" {
			return DispatchResult{Revision: s.rev}, nil
		}
		if !utf8.ValidString(event.Text) {
			return DispatchResult{}, fmt.Errorf("inserted text is not valid UTF-8")
		}
		a, z := ordered(before.Anchor, before.Head)
		tx = Transaction{Before: before, After: Selection{Anchor: a + len(event.Text), Head: a + len(event.Text)}, Edits: []TextEdit{{Range: TextRange{Start: a, End: z}, Text: event.Text}}}
	case editorinput.DeleteBackward, editorinput.DeleteForward:
		a, z := ordered(before.Anchor, before.Head)
		if a == z {
			if event.Kind == editorinput.DeleteBackward && a > 0 {
				a = previousRuneStart(s.src, a)
			} else if event.Kind == editorinput.DeleteForward && z < len(s.src) {
				z = nextRuneEnd(s.src, z)
			}
		}
		if a == z {
			return DispatchResult{Handled: true, Revision: s.rev}, nil
		}
		tx = Transaction{Before: before, After: Selection{Anchor: a, Head: a}, Edits: []TextEdit{{Range: TextRange{Start: a, End: z}}}}
	case editorinput.MoveHorizontal:
		if event.Direction != -1 && event.Direction != 1 {
			return DispatchResult{}, fmt.Errorf("horizontal direction must be -1 or +1")
		}
		head := before.Head
		if !event.Extend && before.Anchor != before.Head {
			a, z := ordered(before.Anchor, before.Head)
			if event.Direction < 0 {
				head = a
			} else {
				head = z
			}
		} else if event.Direction < 0 && head > 0 {
			head = previousRuneStart(s.src, head)
		} else if event.Direction > 0 && head < len(s.src) {
			head = nextRuneEnd(s.src, head)
		}
		selection := before
		selection.Head = head
		if !event.Extend {
			selection.Anchor = head
		}
		s.setSelection(selection)
		if selection != before {
			s.closeHistoryGroupLocked()
		}
		return DispatchResult{Handled: true, SelectionChanged: selection != before, Revision: s.rev}, nil
	case editorinput.SetSelection:
		selection := Selection{Anchor: event.Anchor, Head: event.Head, Inclusive: event.Inclusive, Linewise: event.Linewise}
		if err := validateSelection(s.src, selection); err != nil {
			return DispatchResult{}, err
		}
		s.setSelection(selection)
		if selection != before {
			s.closeHistoryGroupLocked()
		}
		return DispatchResult{Handled: true, SelectionChanged: selection != before, Revision: s.rev}, nil
	default:
		return DispatchResult{}, fmt.Errorf("unsupported input kind %d", event.Kind)
	}
	result, err = s.applyTransaction(tx)
	if err == nil && result.Changed {
		changedEdits = cloneEdits(tx.Edits)
	}
	return result, err
}

// ApplyTransaction applies a validated group of edits expressed in original
// source coordinates and records it as one undo step.
func (s *Session) ApplyTransaction(tx Transaction) (DispatchResult, error) {
	return s.ApplyTransactionWithOptions(tx, TransactionOptions{})
}

// ApplyTransactionWithOptions applies one source transaction and records it in
// the Session-owned history. Typing and deletion groups can merge when their
// edits are contiguous and within the standard two-second window.
func (s *Session) ApplyTransactionWithOptions(tx Transaction, options TransactionOptions) (DispatchResult, error) {
	if s == nil {
		return DispatchResult{}, fmt.Errorf("nil editor session")
	}
	batch, err := s.BeginTransaction(options)
	if err != nil {
		return DispatchResult{}, err
	}
	if tx.Before != batch.before {
		_ = s.AbortTransaction(batch)
		return DispatchResult{}, fmt.Errorf("transaction selection is stale")
	}
	if err := validateTransactionSource(batch.beforeSource, tx); err != nil {
		_ = s.AbortTransaction(batch)
		return DispatchResult{}, err
	}
	for i := len(tx.Edits) - 1; i >= 0; i-- {
		if _, err := s.ApplyEdit(batch, tx.Edits[i]); err != nil {
			_ = s.AbortTransaction(batch)
			return DispatchResult{}, err
		}
	}
	return s.CommitTransaction(batch, tx.After)
}

// BeginTransaction starts a multi-edit source transaction. Each subsequent
// ApplyEdit immediately changes the Session source so command logic can read
// its intermediate result, but observers see one committed change.
func (s *Session) BeginTransaction(options TransactionOptions) (*EditBatch, error) {
	if s == nil {
		return nil, fmt.Errorf("nil editor session")
	}
	if options.At.IsZero() {
		options.At = time.Now()
	}
	s.editMu.Lock()
	participant := s.historyParticipantSnapshot()
	beforeMetadata := cloneHistorySnapshot(options.BeforeMetadata)
	if beforeMetadata == nil && participant != nil {
		beforeMetadata = cloneHistorySnapshot(participant.CaptureHistory())
	}
	s.mu.RLock()
	beforeSource, before := s.src, s.selectionLocked()
	s.mu.RUnlock()
	return &EditBatch{session: s, options: options, beforeSource: beforeSource, before: before, beforeMetadata: beforeMetadata, active: true}, nil
}

// ApplyEdit immediately replaces one range in the batch's current source and
// shifts the Session selection. It returns the replaced text for feature-local
// derived-state adjustment.
func (s *Session) ApplyEdit(batch *EditBatch, edit TextEdit) (string, error) {
	if batch == nil || batch.session != s || !batch.active {
		return "", fmt.Errorf("inactive or foreign edit batch")
	}
	s.mu.Lock()
	before := s.selectionLocked()
	tx := Transaction{Before: before, Edits: []TextEdit{edit}}
	newLength := len(s.src) + len(edit.Text) - (edit.Range.End - edit.Range.Start)
	tx.After = Selection{Anchor: newLength, Head: newLength}
	if err := validateTransactionSource(s.src, tx); err != nil {
		s.mu.Unlock()
		return "", err
	}
	old := s.src[edit.Range.Start:edit.Range.End]
	s.src = s.src[:edit.Range.Start] + edit.Text + s.src[edit.Range.End:]
	delta := len(edit.Text) - (edit.Range.End - edit.Range.Start)
	shift := func(offset int) int {
		switch {
		case offset >= edit.Range.End:
			return offset + delta
		case offset > edit.Range.Start:
			return edit.Range.Start + len(edit.Text)
		default:
			return offset
		}
	}
	after := before
	after.Anchor, after.Head = shift(before.Anchor), shift(before.Head)
	s.setSelection(after)
	s.rev++
	batch.edits = append(batch.edits, historyEdit{at: edit.Range.Start, old: old, new: edit.Text})
	s.mu.Unlock()
	return old, nil
}

// UpdateTransactionSelection changes the visible selection while a batch is
// active. The selection remains Session-owned, but the edit lock is already
// held for the batch, so this method only takes the state lock. The final
// selection is committed to history with CommitTransaction.
func (s *Session) UpdateTransactionSelection(batch *EditBatch, selection Selection) error {
	if batch == nil || batch.session != s || !batch.active {
		return fmt.Errorf("inactive or foreign edit batch")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := validateSelection(s.src, selection); err != nil {
		return err
	}
	s.setSelection(selection)
	return nil
}

// CommitTransaction closes a batch, stores one history step, and notifies
// presentation adapters once with the batch's sequential source edits.
func (s *Session) CommitTransaction(batch *EditBatch, after Selection) (DispatchResult, error) {
	if batch == nil || batch.session != s || !batch.active {
		return DispatchResult{}, fmt.Errorf("inactive or foreign edit batch")
	}
	participant := s.historyParticipantSnapshot()
	s.mu.Lock()
	if err := validateSelection(s.src, after); err != nil {
		s.mu.Unlock()
		_ = s.AbortTransaction(batch)
		return DispatchResult{}, err
	}
	s.setSelection(after)
	afterSource, revision := s.src, s.rev
	s.mu.Unlock()
	afterMetadata := cloneHistorySnapshot(batch.options.AfterMetadata)
	if afterMetadata == nil && participant != nil {
		afterMetadata = cloneHistorySnapshot(participant.CaptureHistory())
	}

	if len(batch.edits) > 0 {
		s.mu.Lock()
		s.pushHistoryLocked(sessionStep{
			edits: append([]historyEdit(nil), batch.edits...), before: batch.before, after: after,
			group: batch.options.Group, at: batch.options.At,
			beforeMetadata: cloneHistorySnapshot(batch.beforeMetadata), afterMetadata: afterMetadata,
			mergePrevious: batch.options.MergePrevious,
		})
		s.mu.Unlock()
	}
	batch.active = false
	s.editMu.Unlock()
	changeKind := ChangeSelection
	if len(batch.edits) > 0 {
		changeKind = ChangeEdit
	}
	change := Change{Kind: changeKind, BeforeSource: batch.beforeSource, AfterSource: afterSource, Before: batch.before, After: after, Revision: revision, Edits: historyEdits(batch.edits, false)}
	if batch.beforeSource != afterSource || batch.before != after {
		s.notifyChange(change)
	}
	return DispatchResult{Handled: true, Changed: batch.beforeSource != afterSource, SelectionChanged: batch.before != after, Revision: revision}, nil
}

// AbortTransaction restores the batch's source and selection without adding
// history, then publishes the inverse edits to derived-state observers.
func (s *Session) AbortTransaction(batch *EditBatch) error {
	if batch == nil || batch.session != s || !batch.active {
		return fmt.Errorf("inactive or foreign edit batch")
	}
	s.mu.Lock()
	for i := len(batch.edits) - 1; i >= 0; i-- {
		edit := batch.edits[i]
		s.src = s.src[:edit.at] + edit.old + s.src[edit.at+len(edit.new):]
	}
	s.setSelection(batch.before)
	s.rev++
	afterSource, revision := s.src, s.rev
	participant := s.historyParticipant
	s.mu.Unlock()
	batch.active = false
	s.editMu.Unlock()
	if participant != nil && batch.beforeMetadata != nil {
		participant.RestoreHistory(cloneHistorySnapshot(batch.beforeMetadata))
	}
	if len(batch.edits) > 0 {
		s.notifyChange(Change{Kind: ChangeEdit, BeforeSource: batch.beforeSource, AfterSource: afterSource, Before: batch.before, After: batch.before, Revision: revision, Edits: historyEdits(batch.edits, true)})
	}
	return nil
}

func historyEdits(edits []historyEdit, inverse bool) []TextEdit {
	result := make([]TextEdit, 0, len(edits))
	if inverse {
		for i := len(edits) - 1; i >= 0; i-- {
			edit := edits[i]
			result = append(result, TextEdit{Range: TextRange{Start: edit.at, End: edit.at + len(edit.new)}, Text: edit.old})
		}
		return result
	}
	for _, edit := range edits {
		result = append(result, TextEdit{Range: TextRange{Start: edit.at, End: edit.at + len(edit.old)}, Text: edit.new})
	}
	return result
}

func (s *Session) pushHistoryLocked(step sessionStep) {
	s.redo = nil
	if n := len(s.undo); n > 0 && (step.mergePrevious || canCoalesceHistory(s.undo[n-1], step)) {
		mergeHistoryStep(&s.undo[n-1], step)
		if step.mergePrevious {
			s.undo[n-1].group = HistoryGroupExplicit
			s.undo[n-1].closed = true
		}
	} else {
		s.undo = append(s.undo, step)
		if len(s.undo) > 500 {
			s.undo = s.undo[len(s.undo)-500:]
		}
	}
}

func canCoalesceHistory(previous, next sessionStep) bool {
	if previous.closed || previous.group != next.group || (next.group != HistoryGroupTyping && next.group != HistoryGroupDelete) {
		return false
	}
	if next.at.Before(previous.at) || next.at.Sub(previous.at) > 2*time.Second || len(next.edits) != 1 || len(previous.edits) == 0 {
		return false
	}
	current, prior := next.edits[0], previous.edits[len(previous.edits)-1]
	switch next.group {
	case HistoryGroupTyping:
		if current.old != "" || prior.old != "" || current.new == "" || strings.ContainsAny(current.new, "\r\n") || current.at != prior.at+len(prior.new) {
			return false
		}
		return !(strings.HasSuffix(prior.new, " ") && !strings.HasPrefix(current.new, " "))
	case HistoryGroupDelete:
		if current.new != "" || current.old == "" || prior.new != "" || prior.old == "" {
			return false
		}
		return current.at == prior.at || current.at+len(current.old) == prior.at
	default:
		return false
	}
}

func mergeHistoryStep(previous *sessionStep, next sessionStep) {
	previous.edits = append(previous.edits, next.edits...)
	previous.after = next.after
	previous.afterMetadata = cloneHistorySnapshot(next.afterMetadata)
	previous.at = next.at
}

// applyTransaction is the locked helper used by standalone Dispatch. The
// public production transaction API captures typed history participant state
// around each edit.
func (s *Session) applyTransaction(tx Transaction) (DispatchResult, error) {
	before := s.selectionLocked()
	if tx.Before != before {
		return DispatchResult{}, fmt.Errorf("transaction selection is stale")
	}
	if err := validateTransactionSource(s.src, tx); err != nil {
		return DispatchResult{}, err
	}
	if len(tx.Edits) == 0 {
		return DispatchResult{Handled: true, Revision: s.rev}, nil
	}
	changes := make([]historyEdit, 0, len(tx.Edits))
	for i := len(tx.Edits) - 1; i >= 0; i-- {
		edit := tx.Edits[i]
		old := s.src[edit.Range.Start:edit.Range.End]
		s.src = s.src[:edit.Range.Start] + edit.Text + s.src[edit.Range.End:]
		changes = append(changes, historyEdit{at: edit.Range.Start, old: old, new: edit.Text})
	}
	s.setSelection(tx.After)
	s.rev++
	s.pushHistoryLocked(sessionStep{edits: changes, before: before, after: tx.After})
	return DispatchResult{Handled: true, Changed: true, SelectionChanged: s.selectionLocked() != before, Revision: s.rev}, nil
}

// validateTransactionSource enforces the text-specific part of the shared
// transaction contract. Both standalone Session and production Editor call
// this before mutating source, so either entry point rejects malformed UTF-8
// and byte ranges that split runes in the same way.
func validateTransactionSource(source string, tx Transaction) error {
	if err := tx.Validate(len(source)); err != nil {
		return err
	}
	resultLen := len(source)
	for _, edit := range tx.Edits {
		if !runeBoundary(source, edit.Range.Start) || !runeBoundary(source, edit.Range.End) {
			return fmt.Errorf("transaction range splits a UTF-8 rune")
		}
		if !utf8.ValidString(edit.Text) {
			return fmt.Errorf("transaction edit contains invalid UTF-8")
		}
		resultLen += len(edit.Text) - (edit.Range.End - edit.Range.Start)
	}
	if !runeBoundaryAfterEdits(source, tx.Edits, resultLen, tx.After.Anchor) || !runeBoundaryAfterEdits(source, tx.Edits, resultLen, tx.After.Head) {
		return fmt.Errorf("transaction selection splits a UTF-8 rune")
	}
	return nil
}

func (s *Session) applyHistory(redo bool) (DispatchResult, []TextEdit, HistorySnapshot) {
	from, to := &s.undo, &s.redo
	if redo {
		from, to = &s.redo, &s.undo
	}
	if len(*from) == 0 {
		return DispatchResult{Handled: true, Revision: s.rev}, nil, nil
	}
	last := (*from)[len(*from)-1]
	*from = (*from)[:len(*from)-1]
	edits := make([]TextEdit, 0, len(last.edits))
	metadata := last.beforeMetadata
	if redo {
		metadata = last.afterMetadata
		last.closed = true
	} else {
		if n := len(s.undo); n > 0 {
			s.undo[n-1].closed = true
		}
	}
	if redo {
		for _, edit := range last.edits {
			edits = append(edits, TextEdit{Range: TextRange{Start: edit.at, End: edit.at + len(edit.old)}, Text: edit.new})
			s.src = s.src[:edit.at] + edit.new + s.src[edit.at+len(edit.old):]
		}
		s.setSelection(last.after)
	} else {
		for i := len(last.edits) - 1; i >= 0; i-- {
			edit := last.edits[i]
			edits = append(edits, TextEdit{Range: TextRange{Start: edit.at, End: edit.at + len(edit.new)}, Text: edit.old})
			s.src = s.src[:edit.at] + edit.old + s.src[edit.at+len(edit.new):]
		}
		s.setSelection(last.before)
	}
	*to = append(*to, last)
	s.rev++
	return DispatchResult{Handled: true, Changed: true, SelectionChanged: true, Revision: s.rev}, edits, cloneHistorySnapshot(metadata)
}

// Undo applies the latest Session-owned transaction and notifies presentation
// adapters after source, selection, and revision have advanced together.
func (s *Session) Undo() (DispatchResult, error) { return s.applyHistoryChange(false), nil }

// Redo reapplies the latest undone Session transaction.
func (s *Session) Redo() (DispatchResult, error) { return s.applyHistoryChange(true), nil }

func (s *Session) applyHistoryChange(redo bool) DispatchResult {
	if s == nil {
		return DispatchResult{}
	}
	s.editMu.Lock()
	s.mu.Lock()
	beforeSource := s.src
	before := s.selectionLocked()
	result, edits, metadata := s.applyHistory(redo)
	participant := s.historyParticipant
	kind := ChangeUndo
	if redo {
		kind = ChangeRedo
	}
	change := Change{Kind: kind, BeforeSource: beforeSource, AfterSource: s.src, Before: before, After: s.selectionLocked(), Revision: s.rev, Edits: edits}
	changed := change.BeforeSource != change.AfterSource || change.Before != change.After
	s.mu.Unlock()
	if changed {
		if participant != nil && metadata != nil {
			participant.RestoreHistory(cloneHistorySnapshot(metadata))
		}
	}
	s.editMu.Unlock()
	if changed {
		s.notifyChange(change)
	}
	return result
}

func (s *Session) CanUndo() bool {
	if s == nil {
		return false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.undo) > 0
}

func (s *Session) CanRedo() bool {
	if s == nil {
		return false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.redo) > 0
}

func validateSelection(source string, selection Selection) error {
	if selection.Anchor < 0 || selection.Anchor > len(source) || selection.Head < 0 || selection.Head > len(source) {
		return fmt.Errorf("selection is outside source length %d", len(source))
	}
	if !runeBoundary(source, selection.Anchor) || !runeBoundary(source, selection.Head) {
		return fmt.Errorf("selection splits a UTF-8 rune")
	}
	return nil
}

// ReplaceSource resets the shared document state. The Editor additionally
// clears parser, viewport and transient selection caches at this boundary.
func (s *Session) ReplaceSource(source string) {
	s.editMu.Lock()
	s.mu.Lock()
	beforeSource, before := s.src, s.selectionLocked()
	s.src = strings.Clone(source)
	s.anchor, s.caret = 0, 0
	s.selectionInclusive, s.selectionLinewise = false, false
	s.undo, s.redo = nil, nil
	s.rev++
	change := Change{Kind: ChangeReplaceSource, BeforeSource: beforeSource, AfterSource: s.src, Before: before, After: s.selectionLocked(), Revision: s.rev}
	s.mu.Unlock()
	s.editMu.Unlock()
	s.notifyChange(change)
}

func runeBoundaryAfterEdits(source string, edits []TextEdit, resultLen, at int) bool {
	if at < 0 || at > resultLen {
		return false
	}
	if at == resultLen {
		return true
	}
	delta := 0
	for _, edit := range edits {
		start := edit.Range.Start + delta
		if at < start {
			// This edit is after the requested boundary, so it cannot affect
			// the source coordinate that boundary maps back to.
			break
		}
		end := start + len(edit.Text)
		if at <= end {
			if at == start || at == end {
				return true
			}
			return runeBoundary(edit.Text, at-start)
		}
		delta += len(edit.Text) - (edit.Range.End - edit.Range.Start)
	}
	return runeBoundary(source, at-delta)
}

func ordered(a, b int) (int, int) {
	if a > b {
		return b, a
	}
	return a, b
}

func previousRuneStart(source string, at int) int {
	if at <= 0 {
		return 0
	}
	_, size := utf8.DecodeLastRuneInString(source[:at])
	return at - max(size, 1)
}

func nextRuneEnd(source string, at int) int {
	if at >= len(source) {
		return len(source)
	}
	_, size := utf8.DecodeRuneInString(source[at:])
	return at + max(size, 1)
}

func runeBoundary(source string, at int) bool {
	return at >= 0 && at <= len(source) && (at == len(source) || utf8.RuneStart(source[at]))
}
