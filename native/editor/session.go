package editor

import (
	"fmt"
	"strings"
	"sync"
	"unicode/utf8"

	"muse/editor/internal/core"
	"muse/markdown"
	"muse/markdown/render"
)

// Selection is a source-byte selection value shared across editor backends.
type Selection = core.Selection

// Transaction is an atomic source edit expressed against one source snapshot.
type Transaction = core.Transaction

// TextEdit replaces a half-open UTF-8 byte range.
type TextEdit = core.TextEdit

// TextRange is a half-open UTF-8 byte range in Markdown source.
type TextRange = core.TextRange

// InputKind is a backend-neutral editing intent. A platform adapter translates
// physical keys and text-input callbacks into these semantic events.
type InputKind uint8

const (
	InputInsertText InputKind = iota + 1
	InputDeleteBackward
	InputDeleteForward
	InputMoveHorizontal
	InputSetSelection
	InputUndo
	InputRedo
)

// InputEvent carries source-byte coordinates. Direction is -1 or +1 for a
// horizontal move; Anchor and Head are used by InputSetSelection.
type InputEvent struct {
	Kind      InputKind
	Text      string
	Direction int
	Extend    bool
	Anchor    int
	Head      int
	Inclusive bool
	Linewise  bool
}

// DispatchResult describes state changes caused by one semantic input.
type DispatchResult struct {
	Changed          bool
	SelectionChanged bool
	Revision         uint64
}

// Frame is an immutable backend-neutral read model. Markdown and render
// projections are built only when requested and reused until the source
// revision changes.
type Frame struct {
	Revision  uint64
	Source    string
	Selection core.Selection
	Document  *markdown.Doc
	Rendered  *render.Document
}

func (f Frame) snapshot() Frame {
	doc := markdown.Clone(f.Document)
	var rendered *render.Document
	if doc != nil {
		rendered = doc.RenderDocument()
	}
	return Frame{Revision: f.Revision, Source: f.Source, Selection: f.Selection, Document: doc, Rendered: rendered}
}

// Session is the authoritative source, selection and undo state shared by the
// editor core and its UI adapter. The embedded Editor presentation layer uses
// these fields directly so there is only one source of truth. A standalone
// Session serializes concurrent reads and writes. Once attached to Editor,
// Session methods delegate into Editor and inherit its single UI-thread
// ownership rule; callers must dispatch them on that same owner thread.
//
// `frame` is a lazy projection. It is not rebuilt by source edits; the live
// editor's parserSource keeps its block-local incremental parser. Frame returns
// detached document/render copies so consumer mutations cannot alter the cache.
type Session struct {
	mu                 sync.RWMutex
	src                string
	anchor, caret      int
	selectionInclusive bool
	selectionLinewise  bool
	rev                uint64
	hist               history
	frame              Frame
	dispatchInputFn    func(InputEvent) (DispatchResult, error)
	applyTransactionFn func(core.Transaction) (DispatchResult, error)
	setSelectionFn     func(Selection) error
	replaceSourceFn    func(string)
	sourceFn           func() string
	revisionFn         func() uint64
	selectionFn        func() Selection
	frameFn            func() Frame
}

func NewSession(source string) *Session {
	return &Session{src: strings.Clone(source)}
}

func (s *Session) Source() string {
	if s.sourceFn != nil {
		return s.sourceFn()
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.src
}

func (s *Session) Revision() uint64 {
	if s.revisionFn != nil {
		return s.revisionFn()
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.rev
}

func (s *Session) Selection() Selection {
	if s.selectionFn != nil {
		return s.selectionFn()
	}
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
	if s.frameFn != nil {
		return s.frameFn()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.frame.Document == nil || s.frame.Revision != s.rev || s.frame.Source != s.src {
		doc := markdown.Parse(s.src)
		s.frame = Frame{Revision: s.rev, Source: s.src, Document: doc, Rendered: doc.RenderDocument()}
	}
	s.frame.Selection = s.selectionLocked()
	return s.frame.snapshot()
}

func (s *Session) SetSelection(selection Selection) error {
	if s.setSelectionFn != nil {
		return s.setSelectionFn(selection)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := validateSelection(s.src, selection); err != nil {
		return err
	}
	s.setSelection(selection)
	return nil
}

func (s *Session) setSelection(selection Selection) {
	s.anchor, s.caret = selection.Anchor, selection.Head
	s.selectionInclusive, s.selectionLinewise = selection.Inclusive, selection.Linewise
	s.frame.Selection = selection
}

// Dispatch applies one backend-neutral input intent and records a normal
// editor undo step. The production Editor's keymap adapter routes richer
// commands through the same Session-owned source/selection/history fields.
func (s *Session) Dispatch(event InputEvent) (DispatchResult, error) {
	if s.dispatchInputFn != nil {
		return s.dispatchInputFn(event)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	before := s.selectionLocked()
	var tx core.Transaction
	switch event.Kind {
	case InputInsertText:
		if event.Text == "" {
			return DispatchResult{Revision: s.rev}, nil
		}
		if !utf8.ValidString(event.Text) {
			return DispatchResult{}, fmt.Errorf("inserted text is not valid UTF-8")
		}
		a, z := ordered(before.Anchor, before.Head)
		tx = core.Transaction{Before: before, After: Selection{Anchor: a + len(event.Text), Head: a + len(event.Text)}, Edits: []core.TextEdit{{Range: core.TextRange{Start: a, End: z}, Text: event.Text}}}
	case InputDeleteBackward, InputDeleteForward:
		a, z := ordered(before.Anchor, before.Head)
		if a == z {
			if event.Kind == InputDeleteBackward && a > 0 {
				a = previousRuneStart(s.src, a)
			} else if event.Kind == InputDeleteForward && z < len(s.src) {
				z = nextRuneEnd(s.src, z)
			}
		}
		if a == z {
			return DispatchResult{Revision: s.rev}, nil
		}
		tx = core.Transaction{Before: before, After: Selection{Anchor: a, Head: a}, Edits: []core.TextEdit{{Range: core.TextRange{Start: a, End: z}}}}
	case InputMoveHorizontal:
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
		return DispatchResult{SelectionChanged: selection != before, Revision: s.rev}, nil
	case InputSetSelection:
		selection := Selection{Anchor: event.Anchor, Head: event.Head, Inclusive: event.Inclusive, Linewise: event.Linewise}
		if err := validateSelection(s.src, selection); err != nil {
			return DispatchResult{}, err
		}
		s.setSelection(selection)
		return DispatchResult{SelectionChanged: selection != before, Revision: s.rev}, nil
	case InputUndo:
		return s.applyHistory(false), nil
	case InputRedo:
		return s.applyHistory(true), nil
	default:
		return DispatchResult{}, fmt.Errorf("unsupported input kind %d", event.Kind)
	}
	return s.applyTransaction(tx)
}

// ApplyTransaction applies a validated group of edits expressed in original
// source coordinates and records it as one undo step.
func (s *Session) ApplyTransaction(tx core.Transaction) (DispatchResult, error) {
	if s.applyTransactionFn != nil {
		return s.applyTransactionFn(tx)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if tx.Before != s.selectionLocked() {
		return DispatchResult{}, fmt.Errorf("transaction selection is stale")
	}
	return s.applyTransaction(tx)
}

func (s *Session) applyTransaction(tx core.Transaction) (DispatchResult, error) {
	before := s.selectionLocked()
	if tx.Before != before {
		return DispatchResult{}, fmt.Errorf("transaction selection is stale")
	}
	if err := validateTransactionSource(s.src, tx); err != nil {
		return DispatchResult{}, err
	}
	if len(tx.Edits) == 0 {
		return DispatchResult{Revision: s.rev}, nil
	}
	changeSet := step{before: [2]int{before.Anchor, before.Head}, after: [2]int{tx.After.Anchor, tx.After.Head}, kind: stepEdit}
	for i := len(tx.Edits) - 1; i >= 0; i-- {
		edit := tx.Edits[i]
		changeSet.changes = append(changeSet.changes, change{at: edit.Range.Start, old: s.src[edit.Range.Start:edit.Range.End], new: edit.Text})
		s.src = s.src[:edit.Range.Start] + edit.Text + s.src[edit.Range.End:]
	}
	s.hist.undo = append(s.hist.undo, changeSet)
	s.hist.redo = nil
	s.setSelection(tx.After)
	s.rev++
	return DispatchResult{Changed: true, SelectionChanged: s.selectionLocked() != before, Revision: s.rev}, nil
}

// validateTransactionSource enforces the text-specific part of the shared
// transaction contract. Both standalone Session and production Editor call
// this before mutating source, so either entry point rejects malformed UTF-8
// and byte ranges that split runes in the same way.
func validateTransactionSource(source string, tx core.Transaction) error {
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

func (s *Session) applyHistory(redo bool) DispatchResult {
	from, to := &s.hist.undo, &s.hist.redo
	if redo {
		from, to = &s.hist.redo, &s.hist.undo
	}
	if len(*from) == 0 {
		return DispatchResult{Revision: s.rev}
	}
	last := (*from)[len(*from)-1]
	*from = (*from)[:len(*from)-1]
	if redo {
		for _, c := range last.changes {
			s.src = s.src[:c.at] + c.new + s.src[c.at+len(c.old):]
		}
		s.setSelection(Selection{Anchor: last.after[0], Head: last.after[1]})
	} else {
		for i := len(last.changes) - 1; i >= 0; i-- {
			c := last.changes[i]
			s.src = s.src[:c.at] + c.old + s.src[c.at+len(c.new):]
		}
		s.setSelection(Selection{Anchor: last.before[0], Head: last.before[1]})
	}
	*to = append(*to, last)
	s.rev++
	return DispatchResult{Changed: true, SelectionChanged: true, Revision: s.rev}
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
	if s.replaceSourceFn != nil {
		s.replaceSourceFn(source)
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.src = strings.Clone(source)
	s.anchor, s.caret = 0, 0
	s.selectionInclusive, s.selectionLinewise = false, false
	s.hist = history{}
	s.rev++
	s.frame = Frame{}
}

func (s *Session) InvalidateFrame() {
	s.mu.Lock()
	s.frame = Frame{}
	s.mu.Unlock()
}

func runeBoundaryAfterEdits(source string, edits []core.TextEdit, resultLen, at int) bool {
	if at < 0 || at > resultLen {
		return false
	}
	if at == resultLen {
		return true
	}
	delta := 0
	for _, edit := range edits {
		start := edit.Range.Start + delta
		end := start + len(edit.Text)
		if at >= start && at <= end {
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
	return at == len(source) || utf8.RuneStart(source[at])
}
