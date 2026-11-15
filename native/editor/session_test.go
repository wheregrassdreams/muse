package editor

import (
	"sync"
	"testing"

	"muse/editor/internal/core"
)

func TestSessionDispatchUpdatesNeutralFrameAndHistory(t *testing.T) {
	s := NewSession("# note\n")
	if got := s.Frame(); got.Rendered == nil || len(got.Rendered.Blocks) != 1 {
		t.Fatalf("initial frame = %#v", got)
	}

	if err := s.SetSelection(Selection{Anchor: len(s.Source()), Head: len(s.Source())}); err != nil {
		t.Fatal(err)
	}
	inserted, err := s.Dispatch(InputEvent{Kind: InputInsertText, Text: "body"})
	if err != nil {
		t.Fatal(err)
	}
	if !inserted.Changed || inserted.Revision != 1 || s.Source() != "# note\nbody" {
		t.Fatalf("insert result=%+v source=%q", inserted, s.Source())
	}
	if frame := s.Frame(); frame.Source != s.Source() || frame.Revision != inserted.Revision || frame.Rendered == nil {
		t.Fatalf("updated frame does not match source: %+v", frame)
	}
	frame := s.Frame()
	frame.Document.Blocks[0].Text = "caller mutation"
	if got := s.Frame().Document.Blocks[0].Text; got == "caller mutation" {
		t.Fatal("caller mutation changed the cached session frame")
	}

	undone, err := s.Dispatch(InputEvent{Kind: InputUndo})
	if err != nil || !undone.Changed || s.Source() != "# note\n" {
		t.Fatalf("undo result=%+v source=%q err=%v", undone, s.Source(), err)
	}
	redone, err := s.Dispatch(InputEvent{Kind: InputRedo})
	if err != nil || !redone.Changed || s.Source() != "# note\nbody" {
		t.Fatalf("redo result=%+v source=%q err=%v", redone, s.Source(), err)
	}
}

func TestSessionTransactionUsesOriginalCoordinates(t *testing.T) {
	s := NewSession("abcdef")
	tx := Transaction{
		Before: Selection{},
		After:  Selection{Anchor: 3, Head: 3},
		Edits: []TextEdit{
			{Range: TextRange{Start: 1, End: 2}, Text: "XYZ"},
			{Range: TextRange{Start: 4, End: 5}, Text: "!"},
		},
	}
	if _, err := s.ApplyTransaction(tx); err != nil {
		t.Fatal(err)
	}
	if got, want := s.Source(), "aXYZcd!f"; got != want {
		t.Fatalf("source = %q, want %q", got, want)
	}
	if _, err := s.Dispatch(InputEvent{Kind: InputUndo}); err != nil {
		t.Fatal(err)
	}
	if got, want := s.Source(), "abcdef"; got != want {
		t.Fatalf("undo source = %q, want %q", got, want)
	}
}

func TestSessionRejectsUTF8InteriorSelection(t *testing.T) {
	s := NewSession("你a")
	if err := s.SetSelection(Selection{Anchor: 1, Head: 1}); err == nil {
		t.Fatal("expected selection inside a UTF-8 rune to be rejected")
	}
}

func TestSessionAcceptsEmptyAndEOFSelections(t *testing.T) {
	for _, source := range []string{"", "你"} {
		s := NewSession(source)
		atEOF := len(source)
		if err := s.SetSelection(Selection{Anchor: atEOF, Head: atEOF}); err != nil {
			t.Fatalf("SetSelection(%q, EOF): %v", source, err)
		}
		if got := s.Frame().Selection.Head; got != atEOF {
			t.Fatalf("EOF head = %d, want %d", got, atEOF)
		}
	}
}

func TestSessionRejectsInvalidUTF8Input(t *testing.T) {
	s := NewSession("")
	if _, err := s.Dispatch(InputEvent{Kind: InputInsertText, Text: string([]byte{0xff})}); err == nil {
		t.Fatal("expected invalid UTF-8 input to be rejected")
	}
	if got := s.Source(); got != "" {
		t.Fatalf("invalid input changed source to %q", got)
	}
}

func TestStandaloneSessionSerializesFramesAndInput(t *testing.T) {
	s := NewSession("start")
	var wg sync.WaitGroup
	for reader := 0; reader < 3; reader++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 40; i++ {
				_ = s.Source()
				frame := s.Frame()
				if frame.Document == nil || frame.Rendered == nil {
					t.Error("reader got an incomplete frame")
					return
				}
			}
		}()
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 40; i++ {
			if _, err := s.Dispatch(InputEvent{Kind: InputInsertText, Text: "x"}); err != nil {
				t.Error(err)
				return
			}
		}
	}()
	wg.Wait()
}

func TestSessionTransactionRejectsInvalidEditEncodingAndBoundaries(t *testing.T) {
	tests := []struct {
		name string
		src  string
		tx   Transaction
	}{
		{
			name: "range splits source rune",
			src:  "你",
			tx: Transaction{
				Before: Selection{}, After: Selection{},
				Edits: []TextEdit{{Range: TextRange{Start: 1, End: 3}, Text: "x"}},
			},
		},
		{
			name: "replacement is invalid UTF8",
			src:  "a",
			tx: Transaction{
				Before: Selection{}, After: Selection{},
				Edits: []TextEdit{{Range: TextRange{Start: 0, End: 1}, Text: string([]byte{0xff})}},
			},
		},
		{
			name: "result selection splits inserted rune",
			src:  "a",
			tx: Transaction{
				Before: Selection{}, After: Selection{Anchor: 1, Head: 1},
				Edits: []TextEdit{{Range: TextRange{Start: 0, End: 1}, Text: "你"}},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := NewSession(tt.src)
			if _, err := s.ApplyTransaction(tt.tx); err == nil {
				t.Fatal("expected transaction validation error")
			}
			if got := s.Source(); got != tt.src {
				t.Fatalf("rejected transaction changed source to %q", got)
			}
		})
	}
}

func TestRuneBoundaryAfterEditsIgnoresEditsAfterSelection(t *testing.T) {
	source := "🙂-tail-more"
	edits := []core.TextEdit{
		{Range: core.TextRange{Start: 5, End: len(source)}},
	}
	if !runeBoundaryAfterEdits(source, edits, 5, 0) {
		t.Fatal("valid boundary before a later deletion was rejected")
	}
}

func TestSessionTransactionAllowsValidMultiEditResultBoundaries(t *testing.T) {
	tests := []struct {
		name string
		src  string
		tx   Transaction
		want string
	}{
		{
			name: "adjacent multibyte replacements",
			src:  "ab",
			tx: Transaction{
				Before: Selection{}, After: Selection{Anchor: 3, Head: 3},
				Edits: []TextEdit{
					{Range: TextRange{Start: 0, End: 1}, Text: "你"},
					{Range: TextRange{Start: 1, End: 2}, Text: "好"},
				},
			},
			want: "你好",
		},
		{
			name: "deletion at EOF",
			src:  "你a",
			tx: Transaction{
				Before: Selection{}, After: Selection{Anchor: 3, Head: 3},
				Edits: []TextEdit{{Range: TextRange{Start: 3, End: 4}}},
			},
			want: "你",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := NewSession(tt.src)
			if _, err := s.ApplyTransaction(tt.tx); err != nil {
				t.Fatalf("ApplyTransaction: %v", err)
			}
			if got := s.Source(); got != tt.want {
				t.Fatalf("source = %q, want %q", got, tt.want)
			}
		})
	}
}
