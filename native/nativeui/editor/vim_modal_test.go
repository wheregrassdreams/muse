package editor

import (
	"testing"

	"github.com/egoist/mygo/ui"
	inputadapter "muse/nativeui/editor/inputadapter"
)

// modalKey preserves the older UI-oriented test helper while production modal
// behavior consumes backend-neutral key events.
func (e *Editor) modalKey(key ui.Key, modifiers ui.Modifiers) bool {
	event, ok := inputadapter.KeyEvent(ui.InputEvent{Kind: ui.InputKeyDown, Key: key, Mods: modifiers})
	return ok && e.modalKeyEvent(event)
}

func TestProductionViewRoutesVimInputThroughSession(t *testing.T) {
	e := New("abc", Options{})
	e.SetKeymap("vim")
	tester := ui.NewTester(func(c *ui.Context) { e.View(c).Fill() }, 640, 480)
	tester.Frame()
	tester.Frame()
	e.Focus()
	tester.Frame()
	tester.Frame()
	tester.Type("l")
	tester.Type("i")
	tester.Type("X")
	tester.KeyDown(ui.KeyEscape)
	tester.Frame()
	if got, want := e.Session.Source(), "aXbc"; got != want {
		t.Fatalf("Session source = %q, want %q", got, want)
	}
	if got := e.Session.Selection().Head; got != 1 {
		t.Fatalf("Session selection head = %d, want Vim's inserted-character offset 1", got)
	}
}

func TestVimAppendEscapeLeavesCursorOnInsertedCharacter(t *testing.T) {
	e := New("abc", Options{})
	e.SetKeymap("vim")
	e.Select(1, 1)
	e.modalText("a")
	e.typeText("X")
	e.modalKey(ui.KeyEscape, 0)
	if got, want := e.Source(), "abXc"; got != want {
		t.Fatalf("source = %q, want %q", got, want)
	}
	if got := e.Session.Selection().Head; got != 2 {
		t.Fatalf("normal-mode cursor = %d, want inserted character at offset 2", got)
	}
}

func TestVimLeadingSpaceEscapeKeepsFocusAndCaretVisible(t *testing.T) {
	e := New("alpha\n\nbeta", Options{})
	e.SetKeymap("vim")
	tester := ui.NewTester(func(c *ui.Context) { e.View(c).Fill() }, 640, 480)
	tester.Frame()
	tester.Frame()
	e.Focus()
	tester.Frame()
	e.Select(len("alpha\n\n"), len("alpha\n\n"))
	tester.Type("i")
	tester.Type(" ")
	tester.KeyDown(ui.KeyEscape)
	tester.Frame()

	if got, want := e.Source(), "alpha\n\n beta"; got != want {
		t.Fatalf("source after Vim insert and Escape = %q, want %q", got, want)
	}
	if !e.Focused() {
		t.Fatal("Vim Escape after leading space dropped editor focus")
	}
	if _, ok := e.caretRect(e.Caret()); !ok {
		t.Fatalf("Vim caret after Escape has no visible geometry: offset=%d", e.Caret())
	}
}

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
	if e.vimController().Mode() != "Insert" {
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
			e.imeClient.ReplaceText(nil, string(r))
		}
		if e.Source() != tc.want {
			t.Fatalf("%s: %q want %q", tc.keys, e.Source(), tc.want)
		}
	}
}

func modalKeys(e *Editor, keys string) {
	for _, r := range keys {
		e.imeClient.ReplaceText(nil, string(r))
	}
}

func TestModalFindAndRepeatGraphemeAware(t *testing.T) {
	e := New("áb界ćd界\nnext界", Options{})
	e.SetKeymap("vim")
	e.Select(0, 0)
	e.modalText("f")
	e.modalText("界")
	if e.Caret() != len("áb") {
		t.Fatalf("f caret=%d source offset %q", e.Caret(), e.Source()[:e.Caret()])
	}
	e.modalText(";")
	if e.Caret() != len("áb界ćd") {
		t.Fatalf("; caret=%d", e.Caret())
	}
	e.modalText(",")
	if e.Caret() != len("áb") {
		t.Fatalf(", caret=%d", e.Caret())
	}
	// Counted f searches the requested grapheme occurrence and stays in this line.
	e.Select(0, 0)
	e.modalText("2")
	e.modalText("f")
	e.modalText("界")
	if e.Caret() != len("áb界ćd") {
		t.Fatalf("2f crossed line or missed match: %d", e.Caret())
	}
}

func TestModalTillAndOperatorFind(t *testing.T) {
	e := New("abcXdefX", Options{})
	e.SetKeymap("vim")
	e.Select(0, 0)
	modalKeys(e, "tX")
	if e.Caret() != 2 {
		t.Fatalf("tX caret=%d", e.Caret())
	}
	e.Select(0, 0)
	modalKeys(e, "dfX")
	if e.Source() != "defX" {
		t.Fatalf("dfX source=%q", e.Source())
	}
	e.Undo()
	e.Select(len("abcXdefX"), len("abcXdefX"))
	modalKeys(e, "FX")
	if e.Caret() != 7 {
		t.Fatalf("FX caret=%d", e.Caret())
	}

	e = New("abcXdefX", Options{})
	e.SetKeymap("vim")
	e.Select(0, 0)
	modalKeys(e, "yfX")
	if e.Source() != "abcXdefX" || e.vimController().View().Register != "abcX" || e.vimController().Mode() != "Normal" {
		t.Fatalf("yfX source/register/mode=%q/%q/%v", e.Source(), e.vimController().View().Register, e.vimController().Mode())
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
	if e.Caret() != len("é") {
		t.Fatalf("replace caret=%d", e.Caret())
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
	if e.vimController().Mode() != "Normal" || e.Session.Selection().Inclusive {
		t.Fatalf("visual r left selection active: mode=%q inclusive=%v", e.vimController().Mode(), e.Session.Selection().Inclusive)
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
	if h.e.Caret() != 0 {
		t.Fatalf("find crossed into folded source: caret=%d", h.e.Caret())
	}
}

func TestModalSearchSubmitRepeatReverseAndWrap(t *testing.T) {
	e := New("one alpha two alpha", Options{})
	e.SetKeymap("vim")
	e.Select(1, 1)
	e.modalText("/")
	e.modalText("alpha")
	if got := e.ModeLabel(); got != "vim Normal" || !e.vimController().View().Searching {
		t.Fatalf("mode/searching=%q/%v", got, e.vimController().View().Searching)
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
	if e.Caret() != 4 || e.vimController().View().LastSearch != "alpha" || e.Source() != "one alpha two alpha" {
		t.Fatalf("submit caret=%d last=%q source=%q", e.Caret(), e.vimController().View().LastSearch, e.Source())
	}
	e.modalText("n")
	if e.Caret() != 14 || e.vimCurrent != 1 {
		t.Fatalf("n caret/current=%d/%d", e.Caret(), e.vimCurrent)
	}
	e.modalText("N")
	if e.Caret() != 4 || e.vimCurrent != 0 {
		t.Fatalf("N caret/current=%d/%d", e.Caret(), e.vimCurrent)
	}
	e.modalText("2")
	e.modalText("n")
	if e.Caret() != 4 {
		t.Fatalf("counted wrapped n caret=%d", e.Caret())
	}
	e.modalText("?")
	e.modalText("alpha")
	e.modalKey(ui.KeyEnter, 0)
	if e.Caret() != 14 {
		t.Fatalf("backward search caret=%d", e.Caret())
	}
}

func TestModalSearchCancelRestoresSelectionAndBackspaceGrapheme(t *testing.T) {
	e := New("áx target", Options{})
	e.SetKeymap("vim")
	e.Select(0, len("á"))
	origCaret, origAnchor := e.Caret(), e.Session.Selection().Anchor
	origInclusive := e.Session.Selection().Inclusive
	e.modalText("/")
	e.modalText("t")
	e.modalText("á")
	e.modalKey(ui.KeyBackspace, 0)
	if e.vimController().View().SearchQuery != "t" {
		t.Fatalf("backspace query=%q, want one grapheme removed", e.vimController().View().SearchQuery)
	}
	e.modalText("arget")
	e.modalKey(ui.KeyEscape, 0)
	if e.Caret() != origCaret || e.Session.Selection().Anchor != origAnchor || e.Session.Selection().Inclusive != origInclusive || e.vimController().View().Searching {
		t.Fatalf("cancel caret/anchor/selection=%d/%d/%v searching=%v", e.Caret(), e.Session.Selection().Anchor, e.Session.Selection().Inclusive, e.vimController().View().Searching)
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
	if got := e.ModeLabel(); got != "vim Normal" || !e.vimController().View().Searching {
		t.Fatalf("mode/searching=%q/%v", got, e.vimController().View().Searching)
	}
	e.modalKey(ui.KeyEnter, 0)
	want := len("# 标题\n\n[你好](note.md) | ")
	if e.Caret() != want {
		t.Fatalf("unicode source offset=%d want=%d source=%q", e.Caret(), want, src[:e.Caret()])
	}
	e.modalText("n")
	if e.Caret() != want {
		t.Fatalf("single occurrence wrapped to itself at %d", e.Caret())
	}
}

func TestModalSearchCommandLineOwnsKeyboardInput(t *testing.T) {
	h := newHarness(t, "one target two")
	h.e.SetKeymap("vim")
	h.caretAt(1)
	h.typ("/")
	h.frame()
	if !h.e.vimController().View().Searching || !h.e.modal.searchInputFocused {
		t.Fatalf("command line did not autofocus: searching=%v focused=%v", h.e.vimController().View().Searching, h.e.modal.searchInputFocused)
	}
	h.typ("target")
	h.key(0, ui.KeyBackspace)
	if h.e.vimController().View().SearchQuery != "targe" {
		t.Fatalf("input backspace query=%q", h.e.vimController().View().SearchQuery)
	}
	h.typ("t")
	if h.e.vimController().View().SearchQuery != "target" || h.e.Source() != "one target two" {
		t.Fatalf("query/source=%q/%q", h.e.vimController().View().SearchQuery, h.e.Source())
	}
	if len(h.e.vimHighlights) != 1 || h.e.vimCurrent != 0 {
		t.Fatalf("live command-line highlights/current=%d/%d", len(h.e.vimHighlights), h.e.vimCurrent)
	}
	h.shot("vim-search-commandline")
	h.key(0, ui.KeyEnter)
	h.frame()
	if h.e.vimController().View().Searching || h.e.Caret() != 4 || h.e.Source() != "one target two" || !h.e.Focused() {
		t.Fatalf("submit state searching=%v caret=%d source=%q editorFocused=%v", h.e.vimController().View().Searching, h.e.Caret(), h.e.Source(), h.e.Focused())
	}
}

func TestModalSearchCommandLineCancelRestoresEditorSelection(t *testing.T) {
	h := newHarness(t, "alpha beta")
	h.e.SetKeymap("vim")
	h.e.Select(1, 4)
	h.e.SetHighlights([]Range{{Start: 0, End: 5}}, 0)
	origCaret, origAnchor := h.e.Caret(), h.e.Session.Selection().Anchor
	h.typ("?")
	h.frame()
	h.typ("beta")
	if h.e.Source() != "alpha beta" {
		t.Fatalf("query changed source: %q", h.e.Source())
	}
	h.key(0, ui.KeyEscape)
	h.frame()
	if h.e.vimController().View().Searching || h.e.Caret() != origCaret || h.e.Session.Selection().Anchor != origAnchor || h.e.Source() != "alpha beta" || !h.e.Focused() {
		t.Fatalf("cancel searching=%v caret/anchor=%d/%d source=%q editorFocused=%v", h.e.vimController().View().Searching, h.e.Caret(), h.e.Session.Selection().Anchor, h.e.Source(), h.e.Focused())
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
	if h.e.vimController().View().SearchQuery != "世界" || h.e.Source() != "start 世界 end" {
		active, noInput := h.tt.TextInputActive()
		t.Fatalf("IME query/source=%q/%q focused=%v editor=%v inputActive=%v noInput=%v", h.e.vimController().View().SearchQuery, h.e.Source(), h.e.modal.searchInputFocused, h.e.Focused(), active, noInput)
	}
	h.key(0, ui.KeyEnter)
	if h.e.Caret() != len("start ") || h.e.Source() != "start 世界 end" {
		t.Fatalf("IME search caret/source=%d/%q", h.e.Caret(), h.e.Source())
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
	if got := e.modalSearchRanges("k"); len(got) != 3 || e.Source()[got[0].Start:got[0].End] != "K" {
		t.Fatalf("Unicode case-fold ranges=%+v", got)
	}
	if got := e.modalSearchRanges("K"); len(got) != 1 || e.Source()[got[0].Start:got[0].End] != "K" {
		t.Fatalf("smartcase Unicode ranges=%+v", got)
	}
	if at, ok := e.modalSearch("Alpha", 1, 1, 0); !ok || at != 0 {
		t.Fatalf("smartcase wrapped match=%d/%v", at, ok)
	}
}
