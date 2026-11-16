package editor

import (
	"reflect"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

func paragraphEditingHarness(t *testing.T, src string) *harness {
	h := softBreakHarness(t, src)
	opts := h.e.Options()
	opts.PreserveTypedLineBreaks = true
	h.e.SetOptions(opts)
	h.frame()
	return h
}

func TestBlankLineInputKeepsSourceAndEditableLines(t *testing.T) {
	for _, tc := range []struct {
		name, src string
		at        int
		text      string
	}{
		{"after heading", "## Heading\n\nfollowing", len("## Heading\n"), "a"},
		{"between prose", "above\n\nbelow", len("above\n"), "new"},
		{"document start", "\nnext", 0, "first"},
		{"two blanks", "above\n\n\nbelow", len("above\n"), "a"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := paragraphEditingHarness(t, tc.src)
			h.caretAt(tc.at)
			if tc.name == "after heading" {
				h.shot("paragraph-input-before")
			}
			h.typ(tc.text)
			want := tc.src[:tc.at] + tc.text + tc.src[tc.at:]
			h.want(want)
			if strings.Count(h.e.Source(), "\n") != strings.Count(tc.src, "\n") {
				t.Fatal("typing manufactured source line breaks")
			}
			l := h.e.leafAt(h.e.Caret())
			if !strings.Contains(l.view.disp.text, "\n") {
				t.Fatalf("active source lines joined: %q", l.view.disp.text)
			}
			if tc.name == "after heading" {
				h.shot("paragraph-input-after")
			}
			h.e.Undo()
			h.frame()
			h.want(tc.src)
			h.e.Redo()
			h.frame()
			h.want(want)
		})
	}
}

func TestPlainCaretMovementDoesNotReflowParagraph(t *testing.T) {
	const src = "## Heading\n\nfirst\nsecond\n\nother paragraph"
	h := paragraphEditingHarness(t, src)
	h.caretAt(len(src))
	l := h.e.leaves[2]
	for _, candidate := range h.e.leaves {
		if candidate.kind == kPara && strings.Contains(candidate.view.disp.text, "first") {
			l = candidate
			break
		}
	}
	before := l.view.layout.Lines()
	for at := strings.Index(src, "first"); at <= strings.Index(src, "second")+len("second"); at++ {
		h.caretAt(at)
		if l.view.disp.text != "first second" || !reflect.DeepEqual(before, l.view.layout.Lines()) {
			t.Fatalf("plain caret at %d changed paragraph layout: %q", at, l.view.disp.text)
		}
	}
	h.caretAt(len(src))
	if !reflect.DeepEqual(before, l.view.layout.Lines()) {
		t.Fatal("leaving paragraph changed wrapping")
	}
	h.want(src)
}

func TestTypedBoundarySurvivesCaretMoveAndUndo(t *testing.T) {
	const src = "## Heading\n\nfollowing\n\nother"
	h := paragraphEditingHarness(t, src)
	at := len("## Heading\n")
	h.caretAt(at)
	h.typ("a")
	l := h.e.leafAt(h.e.Caret())
	before := l.view.layout.Lines()
	h.caretAt(len(h.e.Source()))
	if !strings.Contains(l.view.disp.text, "\n") || !reflect.DeepEqual(before, l.view.layout.Lines()) {
		t.Fatal("typed line changed on caret move")
	}
	h.e.Undo()
	h.frame()
	h.want(src)
	if h.e.typedBreaks.Len() != 0 {
		t.Fatal("undo left source break anchors")
	}
	h.e.Redo()
	h.frame()
	h.want(src[:at] + "a" + src[at:])
	if h.e.typedBreaks.Len() == 0 || !strings.Contains(h.e.leafAt(h.e.Caret()).view.disp.text, "\n") {
		t.Fatal("redo lost typed line boundary")
	}
}

func TestTypedEnterAndBackspaceControlOnlyTheirOwnBreak(t *testing.T) {
	h := paragraphEditingHarness(t, "first")
	h.caretAt(len("first"))
	h.key(0, ui.KeyEnter)
	h.typ("second")
	h.want("first\nsecond")
	if got := h.e.leafAt(h.e.Caret()).view.disp.text; got != "first\nsecond" {
		t.Fatalf("typed Enter disappeared: %q", got)
	}
	h.caretAt(len("first\n"))
	h.key(0, ui.KeyBackspace)
	h.want("firstsecond")
	if h.e.typedBreaks.Len() != 0 {
		t.Fatal("deleting newline left an anchored break")
	}
	h.e.Undo()
	h.frame()
	h.want("first\nsecond")
	if h.e.typedBreaks.Len() != 1 {
		t.Fatal("undo did not restore typed break")
	}
	h.e.SetSource("imported\nwrapped")
	h.frame()
	if h.e.typedBreaks.Len() != 0 || strings.Contains(h.e.leaves[0].view.disp.text, "\n") {
		t.Fatal("loading document retained session break anchors")
	}
}

func TestBlankLineIMEPasteAndPairsDoNotAddNewlines(t *testing.T) {
	const src = "## Heading\n\nfollowing"
	at := len("## Heading\n")
	for _, method := range []string{"ime", "range", "paste", "pair"} {
		h := paragraphEditingHarness(t, src)
		h.caretAt(at)
		text := "中文"
		switch method {
		case "ime":
			h.e.SetMarkedText(nil, "zhong", ui.TextInputRange{Start: 5, End: 5})
			h.e.ReplaceText(nil, text)
		case "range":
			h.e.ReplaceText(&ui.TextInputRange{Start: ui.UTF16Len(src[:at]), End: ui.UTF16Len(src[:at])}, text)
		case "paste":
			h.e.insertText(text)
		case "pair":
			h.typ("(")
			text = "()"
		}
		h.frame()
		h.want(src[:at] + text + src[at:])
		if !strings.Contains(h.e.leafAt(h.e.Caret()).view.disp.text, "\n") {
			t.Fatal("input joined editable lines")
		}
		h.e.Undo()
		h.frame()
		h.want(src)
	}
}
