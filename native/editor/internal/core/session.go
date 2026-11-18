package core

import (
	"fmt"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/wheregrassdreams/muse-editor/text"
	editorinput "muse/editor/input"
)

// DispatchResult describes state changes caused by one semantic input.
type DispatchResult struct {
	Handled          bool
	Changed          bool
	SelectionChanged bool
	Revision         uint64
}

// ChangeKind identifies a committed Session mutation.
type ChangeKind uint8

const (
	ChangeSelection ChangeKind = iota + 1
	ChangeEdit
	ChangeUndo
	ChangeRedo
	ChangeReplaceSource
)

// Change is an immutable source-delta notification. Ordinary edits carry
// revision-tagged deltas and do not materialize the complete source. A source
// snapshot is included only when a replacement/reset requires the parser to
// rebuild from a new baseline.
type Change struct {
	Kind           ChangeKind
	BeforeSource   string
	AfterSource    string
	BeforeRevision uint64
	Revision       uint64
	Before         Selection
	After          Selection
	Edits          []TextEdit
}

// PresentationAdapter contains host-only event routing. It cannot own the
// document buffer or source-edit history.
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

// HistorySnapshot is a detached immutable feature-state value that follows a
// source transaction through undo and redo.
type HistorySnapshot interface{ Clone() HistorySnapshot }

// HistoryParticipant contributes non-source state to the text engine's own
// history entry. The text engine remains the only owner of source, selection,
// revision and undo/redo.
type HistoryParticipant interface {
	CaptureHistory() HistorySnapshot
	RestoreHistory(HistorySnapshot)
}

type Frame struct {
	Revision  uint64
	Source    string
	Selection Selection
}

type Session struct {
	engine *text.Session

	// editMu serializes composite Muse operations around engine transactions.
	editMu sync.Mutex
	mu     sync.RWMutex

	selectionInclusive bool
	selectionLinewise  bool
	presentation       PresentationAdapter
	changeHook         func(Change)
	historyParticipant HistoryParticipant
	unbindParticipant  func()
	activeBatch        *EditBatch

	cacheValid    bool
	cacheRevision uint64
	cacheSource   string
}

// selectionMetadata is session-scoped Vim selection semantics. The byte
// anchor/head remain exclusively in text.Session.Selection.
type selectionMetadata struct{ Inclusive, Linewise bool }

type selectionParticipant struct{ session *Session }

func (p selectionParticipant) CaptureHistory() selectionMetadata {
	p.session.mu.RLock()
	defer p.session.mu.RUnlock()
	return selectionMetadata{Inclusive: p.session.selectionInclusive, Linewise: p.session.selectionLinewise}
}

func (p selectionParticipant) RestoreHistory(value selectionMetadata) {
	p.session.mu.Lock()
	p.session.selectionInclusive, p.session.selectionLinewise = value.Inclusive, value.Linewise
	p.session.mu.Unlock()
}

type externalHistoryParticipant struct{ participant HistoryParticipant }

func (p externalHistoryParticipant) CaptureHistory() HistorySnapshot {
	return cloneHistorySnapshot(p.participant.CaptureHistory())
}

func (p externalHistoryParticipant) RestoreHistory(value HistorySnapshot) {
	p.participant.RestoreHistory(cloneHistorySnapshot(value))
}

// EditBatch is a temporary transaction draft. It does not become authoritative
// until CommitTransaction applies its source delta to text.Session.
type EditBatch struct {
	session        *Session
	options        TransactionOptions
	beforeSource   string
	draft          string
	before         Selection
	selection      Selection
	beforeMetadata HistorySnapshot
	edits          []TextEdit // ranges in the batch's original coordinate space
	active         bool
}

const (
	selectionHistoryKey = "muse.selection"
	externalHistoryKey  = "muse.feature"
)

func NewSession(source string) *Session {
	engine := text.MustSession(source)
	s := &Session{engine: engine}
	_, err := text.BindHistoryParticipant(engine, selectionHistoryKey, selectionParticipant{session: s})
	if err != nil {
		panic(err)
	}
	return s
}

func (s *Session) BindPresentationAdapter(adapter PresentationAdapter) {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.presentation = adapter
	s.mu.Unlock()
}

func (s *Session) DetachPresentation() {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.presentation = PresentationAdapter{}
	s.changeHook = nil
	s.mu.Unlock()
}

func cloneHistorySnapshot(snapshot HistorySnapshot) HistorySnapshot {
	if snapshot == nil {
		return nil
	}
	return snapshot.Clone()
}

func (s *Session) BindHistoryParticipant(participant HistoryParticipant) {
	if s == nil {
		return
	}
	s.editMu.Lock()
	defer s.editMu.Unlock()
	s.mu.Lock()
	oldUnbind := s.unbindParticipant
	s.unbindParticipant = nil
	s.historyParticipant = participant
	s.mu.Unlock()
	if oldUnbind != nil {
		oldUnbind()
	}
	if participant == nil {
		return
	}
	// BindHistoryParticipant captures an initial value from the participant,
	// which can call back into the editor. Never hold Session.mu across it.
	unbind, err := text.BindHistoryParticipant(s.engine, externalHistoryKey, externalHistoryParticipant{participant})
	if err != nil {
		panic(err)
	}
	s.mu.Lock()
	s.unbindParticipant = unbind
	s.mu.Unlock()
}

func (s *Session) CaptureHistory() HistorySnapshot {
	if s == nil {
		return nil
	}
	s.mu.RLock()
	participant := s.historyParticipant
	s.mu.RUnlock()
	if participant == nil {
		return nil
	}
	return cloneHistorySnapshot(participant.CaptureHistory())
}

func (s *Session) CloseHistoryGroup() {
	if s != nil {
		s.engine.CloseHistoryGroup()
	}
}

func (s *Session) UpdateLastHistoryAfter() bool {
	if s == nil {
		return false
	}
	s.mu.RLock()
	participant := s.historyParticipant
	s.mu.RUnlock()
	if participant == nil {
		return false
	}
	if err := text.UpdateLastHistoryValue(s.engine, externalHistoryKey, cloneHistorySnapshot(participant.CaptureHistory())); err != nil {
		return false
	}
	metadata := selectionParticipant{session: s}.CaptureHistory()
	return text.UpdateLastHistoryValue(s.engine, selectionHistoryKey, metadata) == nil
}

func (s *Session) Source() string {
	if s == nil {
		return ""
	}
	s.mu.RLock()
	if batch := s.activeBatch; batch != nil {
		source := batch.draft
		s.mu.RUnlock()
		return source
	}
	s.mu.RUnlock()
	state := s.engine.Snapshot()
	s.mu.RLock()
	if s.cacheValid && s.cacheRevision == state.Revision() {
		source := s.cacheSource
		s.mu.RUnlock()
		return source
	}
	s.mu.RUnlock()
	source := state.Text()
	s.mu.Lock()
	if s.engine.Snapshot().Revision() == state.Revision() {
		s.cacheValid, s.cacheRevision, s.cacheSource = true, state.Revision(), source
	}
	s.mu.Unlock()
	return source
}

func (s *Session) Revision() uint64 {
	if s == nil {
		return 0
	}
	return s.engine.Snapshot().Revision()
}

func (s *Session) Selection() Selection {
	if s == nil {
		return Selection{}
	}
	s.mu.RLock()
	if batch := s.activeBatch; batch != nil {
		selection := batch.selection
		s.mu.RUnlock()
		return selection
	}
	inclusive, linewise := s.selectionInclusive, s.selectionLinewise
	s.mu.RUnlock()
	selection := s.engine.Snapshot().Selection()
	return Selection{Anchor: int(selection.Anchor), Head: int(selection.Head), Inclusive: inclusive, Linewise: linewise}
}

func (s *Session) Frame() Frame {
	if s == nil {
		return Frame{}
	}
	// A frame pairs source and selection from one serialized editor state.
	// Taking two independent engine snapshots can expose a source from before
	// a concurrent transaction and its selection from after it.
	s.editMu.Lock()
	defer s.editMu.Unlock()
	state := s.engine.Snapshot()
	s.mu.RLock()
	inclusive, linewise := s.selectionInclusive, s.selectionLinewise
	s.mu.RUnlock()
	engineSelection := state.Selection()
	selection := Selection{Anchor: int(engineSelection.Anchor), Head: int(engineSelection.Head), Inclusive: inclusive, Linewise: linewise}
	return Frame{Revision: state.Revision(), Source: state.Text(), Selection: selection}
}

func (s *Session) SetSelection(selection Selection) error {
	if s == nil {
		return fmt.Errorf("nil editor session")
	}
	s.editMu.Lock()
	before := s.Selection()
	if err := s.engine.SetSelection(text.NewSelection(text.Offset(selection.Anchor), text.Offset(selection.Head))); err != nil {
		s.editMu.Unlock()
		return err
	}
	s.mu.Lock()
	s.selectionInclusive, s.selectionLinewise = selection.Inclusive, selection.Linewise
	s.mu.Unlock()
	after := s.Selection()
	revision := s.Revision()
	s.editMu.Unlock()
	if before != after {
		s.notifyChange(Change{Kind: ChangeSelection, BeforeRevision: revision, Revision: revision, Before: before, After: after})
	}
	return nil
}

func (s *Session) SetChangeHook(hook func(Change)) {
	if s != nil {
		s.mu.Lock()
		s.changeHook = hook
		s.mu.Unlock()
	}
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

func (s *Session) ApplyOperation(operation editorinput.Operation) bool {
	if s == nil {
		return false
	}
	s.mu.RLock()
	apply := s.presentation.ApplyOperation
	s.mu.RUnlock()
	return apply != nil && apply(operation)
}

func (s *Session) DispatchKey(event editorinput.KeyEvent) bool {
	if s == nil {
		return false
	}
	s.mu.RLock()
	dispatch := s.presentation.DispatchKey
	s.mu.RUnlock()
	return dispatch != nil && dispatch(event)
}

func (s *Session) Dispatch(event editorinput.Event) (DispatchResult, error) {
	if s == nil {
		return DispatchResult{}, fmt.Errorf("nil editor session")
	}
	switch event.Kind {
	case editorinput.Undo:
		return s.Undo()
	case editorinput.Redo:
		return s.Redo()
	case editorinput.SetSelection:
		before := s.Selection()
		after := Selection{Anchor: event.Anchor, Head: event.Head, Inclusive: event.Inclusive, Linewise: event.Linewise}
		if err := s.SetSelection(after); err != nil {
			return DispatchResult{}, err
		}
		return DispatchResult{Handled: true, SelectionChanged: before != after, Revision: s.Revision()}, nil
	case editorinput.MoveHorizontal:
		if event.Direction != -1 && event.Direction != 1 {
			return DispatchResult{}, fmt.Errorf("horizontal direction must be -1 or +1")
		}
		selection := s.Selection()
		state := s.engine.Snapshot()
		view := state.View()
		head := text.Offset(selection.Head)
		if !event.Extend && selection.Anchor != selection.Head {
			from, to := ordered(selection.Anchor, selection.Head)
			if event.Direction < 0 {
				head = text.Offset(from)
			} else {
				head = text.Offset(to)
			}
		} else if event.Direction < 0 {
			head = view.PreviousGrapheme(head)
		} else {
			head = view.NextGrapheme(head)
		}
		after := selection
		after.Head = int(head)
		if !event.Extend {
			after.Anchor = after.Head
			after.Inclusive = false
			after.Linewise = false
		}
		if err := s.SetSelection(after); err != nil {
			return DispatchResult{}, err
		}
		return DispatchResult{Handled: true, SelectionChanged: selection != after, Revision: s.Revision()}, nil
	case editorinput.InsertText, editorinput.DeleteBackward, editorinput.DeleteForward:
		state := s.engine.Snapshot()
		before := s.Selection()
		from, to := ordered(before.Anchor, before.Head)
		insert := ""
		switch event.Kind {
		case editorinput.InsertText:
			if !utf8.ValidString(event.Text) {
				return DispatchResult{}, fmt.Errorf("inserted text is not valid UTF-8")
			}
			insert = event.Text
		case editorinput.DeleteBackward:
			if from == to && from > 0 {
				from = int(state.View().PreviousGrapheme(text.Offset(from)))
			}
		case editorinput.DeleteForward:
			if from == to && to < state.Len() {
				to = int(state.View().NextGrapheme(text.Offset(to)))
			}
		}
		if from == to && insert == "" {
			return DispatchResult{Handled: true, Revision: state.Revision()}, nil
		}
		afterAt := from + len(insert)
		return s.ApplyTransactionWithOptions(Transaction{Before: before, After: Selection{Anchor: afterAt, Head: afterAt}, Edits: []TextEdit{{Range: TextRange{Start: from, End: to}, Text: insert}}}, TransactionOptions{Group: map[bool]HistoryGroup{true: HistoryGroupTyping, false: HistoryGroupDelete}[event.Kind == editorinput.InsertText], At: time.Now()})
	default:
		return DispatchResult{}, fmt.Errorf("unsupported input kind %d", event.Kind)
	}
}

func (s *Session) ApplyTransaction(tx Transaction) (DispatchResult, error) {
	return s.ApplyTransactionWithOptions(tx, TransactionOptions{})
}

func historyGroupName(group HistoryGroup) string {
	switch group {
	case HistoryGroupTyping:
		return "typing"
	case HistoryGroupDelete:
		return "delete"
	case HistoryGroupExplicit:
		return "explicit"
	default:
		return ""
	}
}

func (s *Session) historyMetadata(snapshot HistorySnapshot) text.HistoryMetadata {
	var metadata text.HistoryMetadata
	if snapshot != nil {
		_ = text.SetHistoryValue(&metadata, externalHistoryKey, cloneHistorySnapshot(snapshot))
	}
	return metadata
}

func historyMetadataWithSelection(metadata text.HistoryMetadata, selection Selection) text.HistoryMetadata {
	_ = text.SetHistoryValue(&metadata, selectionHistoryKey, selectionMetadata{Inclusive: selection.Inclusive, Linewise: selection.Linewise})
	return metadata
}

func (s *Session) ApplyTransactionWithOptions(tx Transaction, options TransactionOptions) (DispatchResult, error) {
	if s == nil {
		return DispatchResult{}, fmt.Errorf("nil editor session")
	}
	s.editMu.Lock()
	state := s.engine.Snapshot()
	before := s.Selection()
	if tx.Before != before {
		s.editMu.Unlock()
		return DispatchResult{}, fmt.Errorf("transaction selection is stale")
	}
	engineTx := state.Transaction()
	for _, edit := range tx.Edits {
		engineTx.Replace(text.Range{From: text.Offset(edit.Range.Start), To: text.Offset(edit.Range.End)}, edit.Text)
	}
	postSelection := text.NewSelection(text.Offset(tx.After.Anchor), text.Offset(tx.After.Head))
	engineTx.SetSelection(postSelection)
	engineTx.Group, engineTx.Origin, engineTx.Timestamp = historyGroupName(options.Group), "muse.user", options.At
	engineTx.MergePrevious = options.MergePrevious
	engineTx.HistoryBefore = historyMetadataWithSelection(s.historyMetadata(options.BeforeMetadata), before)
	engineTx.HistoryAfter = historyMetadataWithSelection(s.historyMetadata(options.AfterMetadata), tx.After)
	result, err := s.engine.Apply(engineTx)
	if err != nil {
		s.editMu.Unlock()
		return DispatchResult{}, err
	}
	s.mu.Lock()
	s.selectionInclusive, s.selectionLinewise = tx.After.Inclusive, tx.After.Linewise
	s.cacheValid = false
	s.mu.Unlock()
	after := s.Selection()
	beforeRevision, revision := state.Revision(), result.State.Revision()
	s.editMu.Unlock()
	change := Change{Kind: ChangeEdit, BeforeRevision: beforeRevision, Revision: revision, Before: before, After: after, Edits: cloneEdits(tx.Edits)}
	if len(tx.Edits) > 0 || before != after {
		s.notifyChange(change)
	}
	return DispatchResult{Handled: true, Changed: len(tx.Edits) > 0, SelectionChanged: before != after, Revision: revision}, nil
}

func (s *Session) BeginTransaction(options TransactionOptions) (*EditBatch, error) {
	if s == nil {
		return nil, fmt.Errorf("nil editor session")
	}
	if options.At.IsZero() {
		options.At = time.Now()
	}
	s.editMu.Lock()
	state := s.engine.Snapshot()
	before := s.Selection()
	snapshot := options.BeforeMetadata
	if snapshot == nil {
		snapshot = s.CaptureHistory()
	}
	batch := &EditBatch{session: s, options: options, beforeSource: state.Text(), draft: state.Text(), before: before, selection: before, beforeMetadata: cloneHistorySnapshot(snapshot), active: true}
	s.mu.Lock()
	s.activeBatch = batch
	s.mu.Unlock()
	return batch, nil
}

func (s *Session) ApplyEdit(batch *EditBatch, edit TextEdit) (string, error) {
	if batch == nil || batch.session != s || !batch.active {
		return "", fmt.Errorf("inactive or foreign edit batch")
	}
	if err := validateTransactionSource(batch.draft, Transaction{Before: batch.selection, After: batch.selection, Edits: []TextEdit{edit}}); err != nil {
		return "", err
	}
	s.mu.Lock()
	old := batch.draft[edit.Range.Start:edit.Range.End]
	originalRange, canTrack := batch.toOriginalRange(edit.Range)
	batch.draft = batch.draft[:edit.Range.Start] + edit.Text + batch.draft[edit.Range.End:]
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
	batch.selection.Anchor, batch.selection.Head = shift(batch.selection.Anchor), shift(batch.selection.Head)
	if canTrack && !batch.overlaps(originalRange) {
		batch.edits = append(batch.edits, TextEdit{Range: originalRange, Text: edit.Text})
	} else {
		batch.edits = nil
	}
	s.cacheValid = false
	s.mu.Unlock()
	return old, nil
}

func (b *EditBatch) toOriginalRange(r TextRange) (TextRange, bool) {
	mapOffset := func(pos int, after bool) (int, bool) {
		delta := 0
		for _, edit := range b.edits {
			start := edit.Range.Start + delta
			end := start + len(edit.Text)
			if pos < start {
				break
			}
			if pos <= end {
				if pos == start {
					return edit.Range.Start, true
				}
				if pos == end {
					return edit.Range.End, true
				}
				if after {
					return edit.Range.End, false
				}
				return edit.Range.Start, false
			}
			delta += len(edit.Text) - (edit.Range.End - edit.Range.Start)
		}
		return pos - delta, true
	}
	from, a := mapOffset(r.Start, false)
	to, z := mapOffset(r.End, true)
	return TextRange{Start: from, End: to}, a && z
}

func (b *EditBatch) overlaps(r TextRange) bool {
	for _, edit := range b.edits {
		if r.Start < edit.Range.End && edit.Range.Start < r.End || r.Start == edit.Range.Start {
			return true
		}
	}
	return false
}

func (s *Session) UpdateTransactionSelection(batch *EditBatch, selection Selection) error {
	if batch == nil || batch.session != s || !batch.active {
		return fmt.Errorf("inactive or foreign edit batch")
	}
	if err := validateSelection(batch.draft, selection); err != nil {
		return err
	}
	s.mu.Lock()
	batch.selection = selection
	s.mu.Unlock()
	return nil
}

func (s *Session) CommitTransaction(batch *EditBatch, after Selection) (DispatchResult, error) {
	if batch == nil || batch.session != s || !batch.active {
		return DispatchResult{}, fmt.Errorf("inactive or foreign edit batch")
	}
	view, err := text.NewBuffer(batch.draft)
	if err != nil {
		_ = s.AbortTransaction(batch)
		return DispatchResult{}, err
	}
	selection := view.View().NormalizeSelection(text.NewSelection(text.Offset(after.Anchor), text.Offset(after.Head)))
	after.Anchor, after.Head = int(selection.Anchor), int(selection.Head)
	if err := validateSelection(batch.draft, after); err != nil {
		_ = s.AbortTransaction(batch)
		return DispatchResult{}, err
	}
	state := s.engine.Snapshot()
	engineTx := state.Transaction()
	edits := batch.edits
	if edits == nil {
		if diff, ok := sourceDiff(batch.beforeSource, batch.draft); ok {
			edits = []TextEdit{diff}
		}
	}
	for _, edit := range edits {
		engineTx.Replace(text.Range{From: text.Offset(edit.Range.Start), To: text.Offset(edit.Range.End)}, edit.Text)
	}
	engineTx.SetSelection(text.NewSelection(text.Offset(after.Anchor), text.Offset(after.Head)))
	engineTx.Group, engineTx.Origin, engineTx.Timestamp = historyGroupName(batch.options.Group), "muse.user", batch.options.At
	engineTx.MergePrevious = batch.options.MergePrevious
	engineTx.HistoryBefore = historyMetadataWithSelection(s.historyMetadata(batch.beforeMetadata), batch.before)
	engineTx.HistoryAfter = historyMetadataWithSelection(s.historyMetadata(batch.options.AfterMetadata), after)
	result, err := s.engine.Apply(engineTx)
	if err != nil {
		_ = s.AbortTransaction(batch)
		return DispatchResult{}, err
	}
	s.mu.Lock()
	s.activeBatch = nil
	s.selectionInclusive, s.selectionLinewise = after.Inclusive, after.Linewise
	s.cacheValid = false
	s.mu.Unlock()
	batch.active = false
	beforeRevision, revision := state.Revision(), result.State.Revision()
	s.editMu.Unlock()
	changeKind := ChangeSelection
	if len(edits) > 0 {
		changeKind = ChangeEdit
	}
	change := Change{Kind: changeKind, BeforeRevision: beforeRevision, Revision: revision, Before: batch.before, After: after, Edits: cloneEdits(edits)}
	if changeKind == ChangeEdit {
		s.notifyChange(change)
	} else if batch.before != after {
		s.notifyChange(change)
	}
	return DispatchResult{Handled: true, Changed: len(edits) > 0, SelectionChanged: batch.before != after, Revision: revision}, nil
}

func (s *Session) AbortTransaction(batch *EditBatch) error {
	if batch == nil || batch.session != s || !batch.active {
		return fmt.Errorf("inactive or foreign edit batch")
	}
	s.mu.Lock()
	s.activeBatch = nil
	s.cacheValid = false
	participant := s.historyParticipant
	s.mu.Unlock()
	if participant != nil && batch.beforeMetadata != nil {
		participant.RestoreHistory(cloneHistorySnapshot(batch.beforeMetadata))
	}
	batch.active = false
	s.editMu.Unlock()
	return nil
}

func sourceDiff(before, after string) (TextEdit, bool) {
	if before == after {
		return TextEdit{}, false
	}
	start := 0
	for start < len(before) && start < len(after) && before[start] == after[start] {
		start++
	}
	for start > 0 && (!runeBoundary(before, start) || !runeBoundary(after, start)) {
		start--
	}
	oldEnd, newEnd := len(before), len(after)
	for oldEnd > start && newEnd > start && before[oldEnd-1] == after[newEnd-1] {
		oldEnd--
		newEnd--
	}
	for oldEnd < len(before) && !runeBoundary(before, oldEnd) {
		oldEnd++
	}
	for newEnd < len(after) && !runeBoundary(after, newEnd) {
		newEnd++
	}
	return TextEdit{Range: TextRange{Start: start, End: oldEnd}, Text: after[start:newEnd]}, true
}

func (s *Session) Undo() (DispatchResult, error) {
	if s == nil {
		return DispatchResult{}, nil
	}
	s.editMu.Lock()
	beforeState := s.engine.Snapshot()
	before := s.Selection()
	afterState, ok := s.engine.Undo()
	if !ok {
		s.editMu.Unlock()
		return DispatchResult{Handled: true, Revision: beforeState.Revision()}, nil
	}
	s.mu.Lock()
	s.cacheValid = false
	s.mu.Unlock()
	after := s.Selection()
	var edits []TextEdit
	if edit, changed := sourceDiff(beforeState.Text(), afterState.Text()); changed {
		edits = []TextEdit{edit}
	}
	s.editMu.Unlock()
	s.notifyChange(Change{Kind: ChangeUndo, BeforeRevision: beforeState.Revision(), Revision: afterState.Revision(), Before: before, After: after, Edits: edits})
	return DispatchResult{Handled: true, Changed: true, SelectionChanged: before != after, Revision: afterState.Revision()}, nil
}

func (s *Session) Redo() (DispatchResult, error) {
	if s == nil {
		return DispatchResult{}, nil
	}
	s.editMu.Lock()
	beforeState := s.engine.Snapshot()
	before := s.Selection()
	afterState, ok := s.engine.Redo()
	if !ok {
		s.editMu.Unlock()
		return DispatchResult{Handled: true, Revision: beforeState.Revision()}, nil
	}
	s.mu.Lock()
	s.cacheValid = false
	s.mu.Unlock()
	after := s.Selection()
	var edits []TextEdit
	if edit, changed := sourceDiff(beforeState.Text(), afterState.Text()); changed {
		edits = []TextEdit{edit}
	}
	s.editMu.Unlock()
	s.notifyChange(Change{Kind: ChangeRedo, BeforeRevision: beforeState.Revision(), Revision: afterState.Revision(), Before: before, After: after, Edits: edits})
	return DispatchResult{Handled: true, Changed: true, SelectionChanged: before != after, Revision: afterState.Revision()}, nil
}

func (s *Session) CanUndo() bool { return s != nil && s.engine.CanUndo() }
func (s *Session) CanRedo() bool { return s != nil && s.engine.CanRedo() }

// ReplaceSource installs a fresh Markdown source baseline in the independent
// text engine. Parser and rendering state observe the typed reset delta.
func (s *Session) ReplaceSource(source string) {
	if s == nil {
		return
	}
	s.editMu.Lock()
	beforeState := s.engine.Snapshot()
	before := s.Selection()
	state, err := s.engine.ResetSource(source)
	if err != nil {
		s.editMu.Unlock()
		return
	}
	s.mu.Lock()
	s.selectionInclusive, s.selectionLinewise = false, false
	s.cacheValid = false
	s.mu.Unlock()
	after := s.Selection()
	s.editMu.Unlock()
	s.notifyChange(Change{Kind: ChangeReplaceSource, AfterSource: source, BeforeRevision: beforeState.Revision(), Revision: state.Revision(), Before: before, After: after})
}

func validateSelection(source string, selection Selection) error {
	view, err := text.NewBuffer(source)
	if err != nil {
		return err
	}
	requested := text.NewSelection(text.Offset(selection.Anchor), text.Offset(selection.Head))
	if view.View().NormalizeSelection(requested) != requested {
		return fmt.Errorf("selection is not on extended grapheme boundaries")
	}
	return nil
}

func validateTransactionSource(source string, tx Transaction) error {
	if err := tx.Validate(len(source)); err != nil {
		return err
	}
	for _, edit := range tx.Edits {
		if !runeBoundary(source, edit.Range.Start) || !runeBoundary(source, edit.Range.End) {
			return fmt.Errorf("transaction range splits a UTF-8 rune")
		}
		if !utf8.ValidString(edit.Text) {
			return fmt.Errorf("transaction edit contains invalid UTF-8")
		}
	}
	return nil
}

func runeBoundary(source string, at int) bool {
	if at < 0 || at > len(source) {
		return false
	}
	return at == 0 || at == len(source) || utf8.RuneStart(source[at])
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
