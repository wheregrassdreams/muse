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

// Selection is a source-byte selection value that can be shared across
// backends without exposing editor implementation state.
type Selection = core.Selection

// Transaction is an atomic source edit accepted by a Session.
type Transaction = core.Transaction

// TextEdit replaces a half-open UTF-8 byte range.
type TextEdit = core.TextEdit

// TextRange is a half-open UTF-8 byte range in Markdown source.
type TextRange = core.TextRange

// InputKind is a backend-neutral editing intent. Backends translate physical
// keys and text-input callbacks into these semantic events.
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

// Frame is the immutable, backend-neutral read model for one session revision.
// Its document and render projection are shared until the source changes and
// must be treated as read-only by consumers.
type Frame struct {
	Revision  uint64
	Source    string
	Selection core.Selection
	Document  *markdown.Doc
	Rendered  *render.Document
}

type sessionHistory struct {
	undo []core.Transaction
	redo []core.Transaction
}

// Session owns mutable source, text selection, transaction history, and the
// cached Markdown presentation projection. It contains no widget or platform
// types and is safe for concurrent frame reads and serialized input dispatch.
type Session struct {
	mu        sync.RWMutex
	source    string
	selection core.Selection
	revision  uint64
	document  *markdown.Doc
	frame     Frame
	history   sessionHistory
}

func NewSession(source string) *Session {
	s := &Session{source: source}
	s.rebuildFrameLocked()
	return s
}

func (s *Session) Source() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.source
}

func (s *Session) Frame() Frame {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.frame
}

func (s *Session) SetSelection(selection Selection) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if selection.Anchor < 0 || selection.Anchor > len(s.source) || selection.Head < 0 || selection.Head > len(s.source) {
		return fmt.Errorf("selection is outside source length %d", len(s.source))
	}
	if !runeBoundary(s.source, selection.Anchor) || !runeBoundary(s.source, selection.Head) {
		return fmt.Errorf("selection splits a UTF-8 rune")
	}
	s.selection = selection
	s.frame.Selection = selection
	return nil
}

// Dispatch applies one semantic editing intent. Each source change is one
// undoable transaction, independent of the input backend that produced it.
func (s *Session) Dispatch(event InputEvent) (DispatchResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	before := s.selection
	var tx core.Transaction
	switch event.Kind {
	case InputInsertText:
		if event.Text == "" {
			return DispatchResult{Revision: s.revision}, nil
		}
		if !utf8.ValidString(event.Text) {
			return DispatchResult{}, fmt.Errorf("inserted text is not valid UTF-8")
		}
		a, z := ordered(before.Anchor, before.Head)
		tx = core.Transaction{
			Edits:  []core.TextEdit{{Range: core.TextRange{Start: a, End: z}, Text: event.Text}},
			Before: before,
			After:  core.Selection{Anchor: a + len(event.Text), Head: a + len(event.Text)},
		}
	case InputDeleteBackward, InputDeleteForward:
		a, z := ordered(before.Anchor, before.Head)
		if a == z {
			if event.Kind == InputDeleteBackward && a > 0 {
				a = previousRuneStart(s.source, a)
			} else if event.Kind == InputDeleteForward && z < len(s.source) {
				z = nextRuneEnd(s.source, z)
			}
		}
		if a == z {
			return DispatchResult{Revision: s.revision}, nil
		}
		tx = core.Transaction{Edits: []core.TextEdit{{Range: core.TextRange{Start: a, End: z}}}, Before: before, After: core.Selection{Anchor: a, Head: a}}
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
			head = previousRuneStart(s.source, head)
		} else if event.Direction > 0 && head < len(s.source) {
			head = nextRuneEnd(s.source, head)
		}
		selection := before
		selection.Head = head
		if !event.Extend {
			selection.Anchor = head
		}
		s.selection = selection
		s.frame.Selection = selection
		return DispatchResult{SelectionChanged: selection != before, Revision: s.revision}, nil
	case InputSetSelection:
		selection := core.Selection{Anchor: event.Anchor, Head: event.Head, Inclusive: event.Inclusive, Linewise: event.Linewise}
		if selection.Anchor < 0 || selection.Anchor > len(s.source) || selection.Head < 0 || selection.Head > len(s.source) {
			return DispatchResult{}, fmt.Errorf("selection is outside source length %d", len(s.source))
		}
		if !runeBoundary(s.source, selection.Anchor) || !runeBoundary(s.source, selection.Head) {
			return DispatchResult{}, fmt.Errorf("selection splits a UTF-8 rune")
		}
		s.selection = selection
		s.frame.Selection = selection
		return DispatchResult{SelectionChanged: selection != before, Revision: s.revision}, nil
	case InputUndo:
		return s.applyHistoryLocked(false)
	case InputRedo:
		return s.applyHistoryLocked(true)
	default:
		return DispatchResult{}, fmt.Errorf("unsupported input kind %d", event.Kind)
	}
	if err := tx.Validate(len(s.source)); err != nil {
		return DispatchResult{}, err
	}
	if err := s.applyLocked(tx, true); err != nil {
		return DispatchResult{}, err
	}
	return DispatchResult{Changed: true, SelectionChanged: s.selection != before, Revision: s.revision}, nil
}

// ApplyTransaction applies one validated transaction against the current
// source snapshot. Callers cannot mutate the session except through this path.
func (s *Session) ApplyTransaction(tx core.Transaction) (DispatchResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if tx.Before != s.selection {
		return DispatchResult{}, fmt.Errorf("transaction selection is stale")
	}
	if err := tx.Validate(len(s.source)); err != nil {
		return DispatchResult{}, err
	}
	if len(tx.Edits) == 0 {
		return DispatchResult{Revision: s.revision}, nil
	}
	if err := s.applyLocked(tx, true); err != nil {
		return DispatchResult{}, err
	}
	return DispatchResult{Changed: true, SelectionChanged: tx.Before != tx.After, Revision: s.revision}, nil
}

func (s *Session) applyLocked(tx core.Transaction, record bool) error {
	if err := tx.Validate(len(s.source)); err != nil {
		return err
	}
	if tx.Before != s.selection {
		return fmt.Errorf("transaction selection is stale")
	}
	resultLen := len(s.source)
	for _, edit := range tx.Edits {
		if !runeBoundary(s.source, edit.Range.Start) || !runeBoundary(s.source, edit.Range.End) {
			return fmt.Errorf("transaction range splits a UTF-8 rune")
		}
		if !utf8.ValidString(edit.Text) {
			return fmt.Errorf("transaction edit contains invalid UTF-8")
		}
		resultLen += len(edit.Text) - (edit.Range.End - edit.Range.Start)
	}
	if !runeBoundaryAfterEdits(s.source, tx.Edits, resultLen, tx.After.Anchor) || !runeBoundaryAfterEdits(s.source, tx.Edits, resultLen, tx.After.Head) {
		return fmt.Errorf("transaction selection splits a UTF-8 rune")
	}
	undo := inverseTransaction(s.source, tx)
	for i := len(tx.Edits) - 1; i >= 0; i-- {
		edit := tx.Edits[i]
		s.source = s.source[:edit.Range.Start] + edit.Text + s.source[edit.Range.End:]
	}
	s.selection = tx.After
	s.revision++
	if record {
		s.history.undo = append(s.history.undo, undo)
		s.history.redo = nil
	}
	s.rebuildFrameLocked()
	return nil
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
	sourceAt := at - delta
	return runeBoundary(source, sourceAt)
}

func (s *Session) applyHistoryLocked(redo bool) (DispatchResult, error) {
	from, to := &s.history.undo, &s.history.redo
	if redo {
		from, to = &s.history.redo, &s.history.undo
	}
	if len(*from) == 0 {
		return DispatchResult{Revision: s.revision}, nil
	}
	tx := (*from)[len(*from)-1]
	if tx.Before != s.selection {
		return DispatchResult{}, fmt.Errorf("history selection is stale")
	}
	*from = (*from)[:len(*from)-1]
	toTx := inverseTransaction(s.source, tx)
	if err := s.applyLocked(tx, false); err != nil {
		*from = append(*from, tx)
		return DispatchResult{}, err
	}
	*to = append(*to, toTx)
	return DispatchResult{Changed: true, SelectionChanged: tx.Before != tx.After, Revision: s.revision}, nil
}

func inverseTransaction(source string, tx core.Transaction) core.Transaction {
	inverse := core.Transaction{Before: tx.After, After: tx.Before, Edits: make([]core.TextEdit, len(tx.Edits))}
	delta := 0
	for i, edit := range tx.Edits {
		start := edit.Range.Start + delta
		inverse.Edits[i] = core.TextEdit{
			Range: core.TextRange{Start: start, End: start + len(edit.Text)},
			Text:  source[edit.Range.Start:edit.Range.End],
		}
		delta += len(edit.Text) - (edit.Range.End - edit.Range.Start)
	}
	return inverse
}

func (s *Session) rebuildFrameLocked() {
	s.document = markdown.Parse(s.source)
	s.frame = Frame{Revision: s.revision, Source: s.source, Selection: s.selection, Document: s.document, Rendered: s.document.RenderDocument()}
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

// ReplaceSource resets history while keeping a valid selection. It is used
// for external reloads, never for ordinary edits.
func (s *Session) ReplaceSource(source string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.source = strings.Clone(source)
	s.selection = core.Selection{}
	s.history = sessionHistory{}
	s.revision++
	s.rebuildFrameLocked()
}
