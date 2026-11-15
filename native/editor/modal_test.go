package mdedit

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestModalEditingSequence(t *testing.T) {
	e := New("alpha beta\n中文", Options{})
	e.SetKeymap("vim")
	e.Select(0, 0)
	e.modalText("d")
	e.modalText("w")
	if e.Source() != "beta\n中文" {
		t.Fatal(e.Source())
	}
	e.modalText("u")
	if e.Source() != "alpha beta\n中文" {
		t.Fatal(e.Source())
	}
	e.modalText("c")
	e.modalText("c")
	if e.modal.mode != "Insert" {
		t.Fatal(e.ModeLabel())
	}
	if e.modalText("你好") {
		t.Fatal("insert text intercepted")
	}
	e.typeText("你好")
	if e.Source() != "你好\n中文" {
		t.Fatal(e.Source())
	}
}
func TestModalRejectsCompositionPayload(t *testing.T) {
	e := New("中文", Options{})
	e.SetKeymap("helix")
	if e.ModalAcceptsText() {
		t.Fatal("normal accepts IME")
	}
	e.modalText("你好")
	if e.Source() != "中文" {
		t.Fatal(e.Source())
	}
}

func TestModalNativeSequences(t *testing.T) {
	for _, tc := range []struct{ keys, want string }{{"dw", "two\nthree\n"}, {"dd", "three\n"}, {"yyp", "one two\none two\nthree\n"}, {"2dd", ""}, {"vld", "e two\nthree\n"}} {
		e := New("one two\nthree\n", Options{})
		e.SetKeymap("vim")
		e.Select(0, 0)
		for _, r := range tc.keys {
			e.ReplaceText(nil, string(r))
		}
		if e.Source() != tc.want {
			t.Fatalf("%s: %q want %q", tc.keys, e.Source(), tc.want)
		}
	}
}

func modalKeys(e *Editor, keys string) {
	for _, r := range keys {
		e.ReplaceText(nil, string(r))
	}
}

func TestModalFindAndRepeatGraphemeAware(t *testing.T) {
	e := New("áb界ćd界\nnext界", Options{})
	e.SetKeymap("vim")
	e.Select(0, 0)
	e.modalText("f")
	e.modalText("界")
	if e.caret != len("áb") {
		t.Fatalf("f caret=%d source offset %q", e.caret, e.src[:e.caret])
	}
	e.modalText(";")
	if e.caret != len("áb界ćd") {
		t.Fatalf("; caret=%d", e.caret)
	}
	e.modalText(",")
	if e.caret != len("áb") {
		t.Fatalf(", caret=%d", e.caret)
	}
	// Counted f searches the requested grapheme occurrence and stays in this line.
	e.Select(0, 0)
	e.modalText("2")
	e.modalText("f")
	e.modalText("界")
	if e.caret != len("áb界ćd") {
		t.Fatalf("2f crossed line or missed match: %d", e.caret)
	}
}

func TestModalTillAndOperatorFind(t *testing.T) {
	e := New("abcXdefX", Options{})
	e.SetKeymap("vim")
	e.Select(0, 0)
	modalKeys(e, "tX")
	if e.caret != 2 {
		t.Fatalf("tX caret=%d", e.caret)
	}
	e.Select(0, 0)
	modalKeys(e, "dfX")
	if e.Source() != "defX" {
		t.Fatalf("dfX source=%q", e.Source())
	}
	e.Undo()
	e.Select(len("abcXdefX"), len("abcXdefX"))
	modalKeys(e, "FX")
	if e.caret != 7 {
		t.Fatalf("FX caret=%d", e.caret)
	}
}

func TestModalReplaceUsesGraphemesAndPreservesTableSeparators(t *testing.T) {
	e := New("ábc", Options{})
	e.SetKeymap("vim")
	e.Select(0, 0)
	modalKeys(e, "2r")
	e.modalText("é")
	if e.Source() != "ééc" {
		t.Fatalf("grapheme replace=%q", e.Source())
	}
	if e.caret != len("é") {
		t.Fatalf("replace caret=%d", e.caret)
	}

	table := "|a|b|\n|---|---|\n|c|d|\n"
	e = New(table, Options{})
	e.SetKeymap("vim")
	e.Select(1, 1)
	modalKeys(e, "rx")
	if e.Source() != "|x|b|\n|---|---|\n|c|d|\n" {
		t.Fatalf("table cell replace=%q", e.Source())
	}
	e.Select(2, 2) // source pipe; replacing it would corrupt the table structure
	modalKeys(e, "rz")
	if e.Source() != "|x|b|\n|---|---|\n|c|d|\n" {
		t.Fatalf("replaced table delimiter: %q", e.Source())
	}
}

func TestModalReplaceVisualSelection(t *testing.T) {
	e := New("abcdef", Options{})
	e.SetKeymap("vim")
	e.Select(1, 1)
	modalKeys(e, "vllr")
	e.modalText("x")
	if e.Source() != "axxxxf" {
		t.Fatalf("visual r source=%q", e.Source())
	}
	if e.modal.mode != "Normal" || e.selectionInclusive {
		t.Fatalf("visual r left selection active: mode=%q inclusive=%v", e.modal.mode, e.selectionInclusive)
	}
}

func TestModalFindStaysOnFoldedSourceLine(t *testing.T) {
	h := newHarness(t, foldTestSource())
	h.e.SetKeymap("vim")
	top := headingAtSource(t, h.e, "# Top")
	clickFoldChevron(h, top)
	if !h.e.isFoldedHeading(top) {
		t.Fatal("fixture section was not folded")
	}
	h.e.Select(0, 0)
	// The folded child paragraph contains C, but `fC` must remain on the
	// heading's logical source line even though its descendants are hidden.
	h.e.modalText("f")
	h.e.modalText("C")
	if h.e.caret != 0 {
		t.Fatalf("find crossed into folded source: caret=%d", h.e.caret)
	}
}

func TestModalSearchSubmitRepeatReverseAndWrap(t *testing.T) {
	e := New("one alpha two alpha", Options{})
	e.SetKeymap("vim")
	e.Select(1, 1)
	e.modalText("/")
	e.modalText("alpha")
	if got := e.ModeLabel(); got != "vim Normal" || !e.modal.searching {
		t.Fatalf("mode/searching=%q/%v", got, e.modal.searching)
	}
	if len(e.vimHighlights) != 2 || e.vimCurrent != 0 {
		t.Fatalf("vim search matches/current=%d/%d", len(e.vimHighlights), e.vimCurrent)
	}
	e.modalText(" ") // query contains spaces as ordinary text
	if e.Source() != "one alpha two alpha" {
		t.Fatalf("query inserted into source: %q", e.Source())
	}
	e.modalKey(ui.KeyBackspace, 0)
	e.modalKey(ui.KeyEnter, 0)
	if e.caret != 4 || e.modal.lastSearch != "alpha" || e.Source() != "one alpha two alpha" {
		t.Fatalf("submit caret=%d last=%q source=%q", e.caret, e.modal.lastSearch, e.Source())
	}
	e.modalText("n")
	if e.caret != 14 || e.vimCurrent != 1 {
		t.Fatalf("n caret/current=%d/%d", e.caret, e.vimCurrent)
	}
	e.modalText("N")
	if e.caret != 4 || e.vimCurrent != 0 {
		t.Fatalf("N caret/current=%d/%d", e.caret, e.vimCurrent)
	}
	e.modalText("2")
	e.modalText("n")
	if e.caret != 4 {
		t.Fatalf("counted wrapped n caret=%d", e.caret)
	}
	e.modalText("?")
	e.modalText("alpha")
	e.modalKey(ui.KeyEnter, 0)
	if e.caret != 14 {
		t.Fatalf("backward search caret=%d", e.caret)
	}
}

func TestModalSearchCancelRestoresSelectionAndBackspaceGrapheme(t *testing.T) {
	e := New("áx target", Options{})
	e.SetKeymap("vim")
	e.Select(0, len("á"))
	origCaret, origAnchor := e.caret, e.anchor
	origInclusive := e.selectionInclusive
	e.modalText("/")
	e.modalText("t")
	e.modalText("á")
	e.modalKey(ui.KeyBackspace, 0)
	if e.modal.searchQuery != "t" {
		t.Fatalf("backspace query=%q, want one grapheme removed", e.modal.searchQuery)
	}
	e.modalText("arget")
	e.modalKey(ui.KeyEscape, 0)
	if e.caret != origCaret || e.anchor != origAnchor || e.selectionInclusive != origInclusive || e.modal.searching {
		t.Fatalf("cancel caret/anchor/selection=%d/%d/%v searching=%v", e.caret, e.anchor, e.selectionInclusive, e.modal.searching)
	}
	if e.Source() != "áx target" {
		t.Fatalf("cancel changed source: %q", e.Source())
	}
}

func TestModalSearchUnicodeAndMarkdownSourceBoundaries(t *testing.T) {
	src := "# 标题\n\n[你好](note.md) | 世界\n"
	e := New(src, Options{})
	e.SetKeymap("vim")
	e.Select(0, 0)
	e.modalText("/")
	e.modalText("世界")
	if got := e.ModeLabel(); got != "vim Normal" || !e.modal.searching {
		t.Fatalf("mode/searching=%q/%v", got, e.modal.searching)
	}
	e.modalKey(ui.KeyEnter, 0)
	want := len("# 标题\n\n[你好](note.md) | ")
	if e.caret != want {
		t.Fatalf("unicode source offset=%d want=%d source=%q", e.caret, want, src[:e.caret])
	}
	e.modalText("n")
	if e.caret != want {
		t.Fatalf("single occurrence wrapped to itself at %d", e.caret)
	}
}

func TestModalSearchCommandLineOwnsKeyboardInput(t *testing.T) {
	h := newHarness(t, "one target two")
	h.e.SetKeymap("vim")
	h.caretAt(1)
	h.typ("/")
	h.frame()
	if !h.e.modal.searching || !h.e.modal.searchInputFocused {
		t.Fatalf("command line did not autofocus: searching=%v focused=%v", h.e.modal.searching, h.e.modal.searchInputFocused)
	}
	h.typ("target")
	h.key(0, ui.KeyBackspace)
	if h.e.modal.searchQuery != "targe" {
		t.Fatalf("input backspace query=%q", h.e.modal.searchQuery)
	}
	h.typ("t")
	if h.e.modal.searchQuery != "target" || h.e.Source() != "one target two" {
		t.Fatalf("query/source=%q/%q", h.e.modal.searchQuery, h.e.Source())
	}
	if len(h.e.vimHighlights) != 1 || h.e.vimCurrent != 0 {
		t.Fatalf("live command-line highlights/current=%d/%d", len(h.e.vimHighlights), h.e.vimCurrent)
	}
	h.shot("vim-search-commandline")
	h.key(0, ui.KeyEnter)
	h.frame()
	if h.e.modal.searching || h.e.caret != 4 || h.e.Source() != "one target two" || !h.e.Focused() {
		t.Fatalf("submit state searching=%v caret=%d source=%q editorFocused=%v", h.e.modal.searching, h.e.caret, h.e.Source(), h.e.Focused())
	}
}

func TestModalSearchCommandLineCancelRestoresEditorSelection(t *testing.T) {
	h := newHarness(t, "alpha beta")
	h.e.SetKeymap("vim")
	h.e.Select(1, 4)
	h.e.SetHighlights([]Range{{Start: 0, End: 5}}, 0)
	origCaret, origAnchor := h.e.caret, h.e.anchor
	h.typ("?")
	h.frame()
	h.typ("beta")
	if h.e.Source() != "alpha beta" {
		t.Fatalf("query changed source: %q", h.e.Source())
	}
	h.key(0, ui.KeyEscape)
	h.frame()
	if h.e.modal.searching || h.e.caret != origCaret || h.e.anchor != origAnchor || h.e.Source() != "alpha beta" || !h.e.Focused() {
		t.Fatalf("cancel searching=%v caret/anchor=%d/%d source=%q editorFocused=%v", h.e.modal.searching, h.e.caret, h.e.anchor, h.e.Source(), h.e.Focused())
	}
	if len(h.e.vimHighlights) != 0 || len(h.e.highlights) != 1 || h.e.current != 0 {
		t.Fatalf("cancel changed Cmd+F or retained Vim highlights: vim=%d cmdf=%d/%d", len(h.e.vimHighlights), len(h.e.highlights), h.e.current)
	}
}

func TestModalSearchCommandLineAcceptsIMEComposition(t *testing.T) {
	h := newHarness(t, "start 世界 end")
	h.e.SetKeymap("vim")
	h.caretAt(0)
	h.typ("/")
	h.frame()
	h.tt.Compose("世", 1)
	h.frame()
	h.tt.Compose("世界", 2)
	h.frame()
	h.tt.Type("世界")
	h.frame()
	if h.e.modal.searchQuery != "世界" || h.e.Source() != "start 世界 end" {
		active, noInput := h.tt.TextInputActive()
		t.Fatalf("IME query/source=%q/%q focused=%v editor=%v inputActive=%v noInput=%v", h.e.modal.searchQuery, h.e.Source(), h.e.modal.searchInputFocused, h.e.Focused(), active, noInput)
	}
	h.key(0, ui.KeyEnter)
	if h.e.caret != len("start ") || h.e.Source() != "start 世界 end" {
		t.Fatalf("IME search caret/source=%d/%q", h.e.caret, h.e.Source())
	}
}

func TestModalSearchCancelClearsTemporaryHighlights(t *testing.T) {
	e := New("alpha alpha", Options{})
	e.SetKeymap("vim")
	e.Select(0, 0)
	e.modalText("/")
	e.modalText("alpha")
	if len(e.vimHighlights) != 2 {
		t.Fatalf("temporary matches=%d", len(e.vimHighlights))
	}
	e.modalKey(ui.KeyEscape, 0)
	if len(e.vimHighlights) != 0 || e.vimHighlightsActive {
		t.Fatalf("cancel retained Vim highlights: %d active=%v", len(e.vimHighlights), e.vimHighlightsActive)
	}
}

func TestModalSearchEscapeClearsCommittedHighlights(t *testing.T) {
	e := New("alpha alpha", Options{})
	e.SetKeymap("vim")
	e.Select(0, 0)
	e.modalText("/")
	e.modalText("alpha")
	e.modalKey(ui.KeyEnter, 0)
	if len(e.vimHighlights) != 2 || !e.vimHighlightsActive {
		t.Fatalf("submitted highlights=%d active=%v", len(e.vimHighlights), e.vimHighlightsActive)
	}
	e.modalKey(ui.KeyEscape, 0)
	if len(e.vimHighlights) != 0 || e.vimHighlightsActive {
		t.Fatalf("Escape did not clear submitted Vim highlights: %d/%v", len(e.vimHighlights), e.vimHighlightsActive)
	}
}

func TestModalSearchSmartCaseUsesGraphemeStableRanges(t *testing.T) {
	e := New("Alpha alpha Älpha älpha K k K", Options{})
	if got := len(e.modalSearchRanges("alpha")); got != 2 {
		t.Fatalf("case-insensitive ASCII hits=%d", got)
	}
	if got := len(e.modalSearchRanges("Alpha")); got != 1 {
		t.Fatalf("smartcase uppercase hits=%d", got)
	}
	if got := e.modalSearchRanges("k"); len(got) != 3 || e.src[got[0].Start:got[0].End] != "K" {
		t.Fatalf("Unicode case-fold ranges=%+v", got)
	}
	if got := e.modalSearchRanges("K"); len(got) != 1 || e.src[got[0].Start:got[0].End] != "K" {
		t.Fatalf("smartcase Unicode ranges=%+v", got)
	}
	if at, ok := e.modalSearch("Alpha", 1, 1, 0); !ok || at != 0 {
		t.Fatalf("smartcase wrapped match=%d/%v", at, ok)
	}
}
