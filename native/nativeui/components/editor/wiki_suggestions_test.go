package editor

import (
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
	inlineui "muse/nativeui/components/editor/blocks/inline"
	vaultlinks "muse/vault/links"
)

func TestWikiSuggestionsFilterAndAcceptWithKeyboard(t *testing.T) {
	h := newHarness(t, "")
	h.e.opts.WikiLinkSuggestions = func(string) []string {
		return []string{
			"Projects/Atlas/Overview",
			"Projects/Atlas/Design Notes",
			"Other/Beacon",
		}
	}
	h.typ("[")
	h.want("[]")
	h.typ("[")
	h.want("[[]]")
	if !h.e.wikiPopup.Frame().Open() || h.e.wikiPopup.Frame().Query() != "" || len(h.e.wikiPopup.Frame().Items()) != 3 {
		t.Fatalf("popup after [[ = %+v", h.e.wikiPopup)
	}
	h.typ("at")
	if !h.e.wikiPopup.Frame().Open() || len(h.e.wikiPopup.Frame().Items()) != 2 {
		t.Fatalf("popup did not filter query: %+v", h.e.wikiPopup)
	}
	if h.e.wikiPopup.Frame().Items()[0].Target != "Projects/Atlas/Overview" || h.e.wikiPopup.Frame().Items()[1].Target != "Projects/Atlas/Design Notes" {
		t.Fatalf("filtered candidates = %#v", h.e.wikiPopup.Frame().Items())
	}
	h.key(0, ui.KeyDown)
	h.key(0, ui.KeyEnter)
	h.want("[[Projects/Atlas/Design Notes]]")
	if h.e.Caret() != len("[[Projects/Atlas/Design Notes") {
		t.Fatalf("accepted caret = %d, want just before closing brackets", h.e.Caret())
	}
	h.e.Undo()
	h.frame()
	h.want("[[at]]")
}

func TestWikiSuggestionsMatchAliasesAndKeepCanonicalTarget(t *testing.T) {
	h := newHarness(t, "")
	h.e.opts.WikiLinkSuggestionDetails = func(string) []vaultlinks.Suggestion {
		return []vaultlinks.Suggestion{
			{Target: "Projects/Atlas/Overview", Title: "Project Overview", Aliases: []string{"Atlas launch plan"}},
			{Target: "Notes/Other", Title: "Other note"},
		}
	}
	h.typ("[")
	h.typ("[")
	h.typ("launch")
	if !h.e.wikiPopup.Frame().Open() || len(h.e.wikiPopup.Frame().Items()) != 1 || h.e.wikiPopup.Frame().Items()[0].Target != "Projects/Atlas/Overview" {
		t.Fatalf("alias query did not resolve canonical target: %+v", h.e.wikiPopup)
	}
	h.key(0, ui.KeyEnter)
	h.want("[[Projects/Atlas/Overview]]")
}

func TestWikiSuggestionPopupShowsTitleAndVaultPath(t *testing.T) {
	h := newHarness(t, "")
	h.e.opts.WikiLinkSuggestionDetails = func(string) []vaultlinks.Suggestion {
		return []vaultlinks.Suggestion{{Target: "Projects/Atlas/Overview", Title: "Project Overview"}}
	}
	h.typ("[")
	h.typ("[")
	h.frame()
	if !h.tt.HasText("Project Overview") || !h.tt.HasText("Projects/Atlas/Overview") {
		t.Fatalf("popup did not show indexed title and canonical path: %q", h.tt.Texts())
	}
}

func TestWikiSuggestionsEscapeAndManualCloser(t *testing.T) {
	h := newHarness(t, "")
	h.e.opts.WikiLinkSuggestions = func(string) []string { return []string{"Target"} }
	h.typ("[")
	h.typ("[")
	before := h.e.Source()
	h.key(0, ui.KeyEscape)
	if h.e.wikiPopup.Frame().Open() || h.e.Source() != before {
		t.Fatalf("Escape changed source or left popup open: source=%q popup=%+v", h.e.Source(), h.e.wikiPopup)
	}
	h.typ("x")
	h.key(0, ui.KeyBackspace)
	h.want("[[]]")
	h.typ("x")
	h.typ("]")
	h.want("[[x]]")
	if h.e.Caret() != len("[[x]") {
		t.Fatalf("typed closer did not skip existing bracket: caret=%d", h.e.Caret())
	}
}

func TestWikiSuggestionMouseSelection(t *testing.T) {
	h := newHarness(t, "")
	h.e.opts.WikiLinkSuggestions = func(string) []string { return []string{"First/Target", "Second/Target"} }
	h.typ("[")
	h.typ("[")
	h.frame()
	if err := h.tt.Click("链接建议：Second/Target"); err != nil {
		t.Fatalf("click suggestion: %v; texts=%q", err, h.tt.Texts())
	}
	h.frame()
	if got := h.e.Source(); got != "[[Second/Target]]" {
		t.Fatalf("mouse selection source=%q popup=%+v", got, h.e.wikiPopup)
	}
}

func TestWikiSuggestionsEmacsNavigationAndTypeIcons(t *testing.T) {
	h := newHarness(t, "")
	h.e.opts.WikiLinkSuggestions = func(string) []string {
		return []string{"Projects/Target.md", "Attachments/diagram.webp"}
	}
	h.typ("[")
	h.typ("[")
	h.key(ui.Ctrl, ui.KeyN)
	if h.e.wikiPopup.Frame().Selected() != 1 || h.e.Source() != "[[]]" {
		t.Fatalf("Ctrl+N failed to select next without inserting text: source=%q popup=%+v", h.e.Source(), h.e.wikiPopup)
	}
	h.key(ui.Ctrl, ui.KeyP)
	if h.e.wikiPopup.Frame().Selected() != 0 || h.e.Source() != "[[]]" {
		t.Fatalf("Ctrl+P failed to select previous without inserting text: source=%q popup=%+v", h.e.Source(), h.e.wikiPopup)
	}
	note, noteColor := inlineui.SuggestionIcon("Projects/Target.md", ui.Hex("#8848dd"), ui.Hex("#777777"))
	image, imageColor := inlineui.SuggestionIcon("Attachments/diagram.webp", ui.Hex("#8848dd"), ui.Hex("#777777"))
	if note == nil || image == nil || note == image {
		t.Fatalf("note and image suggestions must have distinct SVG icons: note=%p image=%p", note, image)
	}
	if noteColor == imageColor {
		t.Fatalf("note and image suggestions share an icon color: note=%v image=%v", noteColor, imageColor)
	}
	h.key(0, ui.KeyDown)
	h.key(0, ui.KeyEnter)
	h.want("[[Attachments/diagram.webp]]")
}

func TestSquareBracketPairInsertSkipAndBackspace(t *testing.T) {
	h := newHarness(t, "")
	h.typ("[")
	h.want("[]")
	if h.e.Caret() != 1 {
		t.Fatalf("caret inside empty pair = %d", h.e.Caret())
	}
	h.typ("x")
	h.typ("]")
	h.want("[x]")
	if h.e.Caret() != len("[x]") {
		t.Fatalf("closer skip caret = %d", h.e.Caret())
	}
	h.e.SetSource("")
	h.frame()
	h.typ("[")
	h.key(0, ui.KeyBackspace)
	h.want("")
	h.e.Undo()
	h.frame()
	h.want("[]")
}

func TestSquareBracketPairIsDisabledInCode(t *testing.T) {
	for _, src := range []string{"```txt\nx\n```", "`x`"} {
		h := newHarness(t, src)
		off := strings.Index(src, "x") + 1
		h.caretAt(off)
		h.typ("[")
		if got := h.e.Source(); !strings.Contains(got, "[") || strings.Contains(got, "[]") {
			t.Errorf("typing [ in code %q produced %q", src, got)
		}
	}
}

func TestWikiHoverReportsOnlyNonEmbeddedTargets(t *testing.T) {
	h := newHarness(t, "[[Target]] and ![[Embedded]]")
	var events []string
	h.e.opts.OnWikiLinkHover = func(target string, _, _ float32, _ ui.Rect) { events = append(events, target) }
	from := strings.Index(h.e.Source(), "Target") + 2
	x, y, ok := h.e.WindowPoint(from)
	if !ok {
		t.Fatal("could not locate visible wiki target")
	}
	h.tt.Move(x, y)
	h.frame()
	if len(events) != 1 || events[0] != "Target" {
		t.Fatalf("hover events = %#v, want Target", events)
	}
	embed := strings.Index(h.e.Source(), "Embedded") + 2
	x, y, ok = h.e.WindowPoint(embed)
	if !ok {
		t.Fatal("could not locate embedded note")
	}
	h.tt.Move(x, y)
	h.frame()
	if len(events) != 2 || events[1] != "" {
		t.Fatalf("embed hover events = %#v, want empty close event", events)
	}
}

func TestWikiHoverCallbackIncludesVisibleLinkBounds(t *testing.T) {
	const src = "Before [[Target|visible label]] after."
	h := newHarness(t, src)
	var gotTarget string
	var gotBounds ui.Rect
	h.e.opts.OnWikiLinkHover = func(target string, _, _ float32, bounds ui.Rect) {
		gotTarget, gotBounds = target, bounds
	}
	x, y := pointForSource(t, h.e, strings.Index(src, "visible label")+3)
	h.e.hover(x, y)
	h.frame()
	if gotTarget != "Target" {
		t.Fatalf("hover target = %q, want Target", gotTarget)
	}
	if gotBounds.W <= 0 || gotBounds.H <= 0 || !gotBounds.Contains(x, y) {
		t.Fatalf("hover callback bounds %+v do not contain link pointer (%v,%v)", gotBounds, x, y)
	}
}

func TestWikiLinkOpensOnOrdinaryClick(t *testing.T) {
	h := newHarness(t, "open [[Target]] now")
	var opened string
	h.e.opts.OnLink = func(link string) { opened = link }
	off := strings.Index(h.e.Source(), "Target") + 2
	x, y, ok := h.e.WindowPoint(off)
	if !ok {
		t.Fatal("could not locate wiki target")
	}
	h.tt.ClickAt(x, y)
	h.frame()
	if opened != "wiki:Target" {
		t.Fatalf("ordinary click opened %q, want wiki:Target", opened)
	}
}

func TestWikiSuggestionPopupScreenshot(t *testing.T) {
	h := newHarness(t, "")
	h.e.opts.WikiLinkSuggestions = func(string) []string {
		return []string{"Projects/Atlas/Overview.md", "Attachments/diagram.png"}
	}
	h.typ("[")
	h.typ("[")
	h.shot("wiki-suggestions")
	if !h.tt.HasText("Projects/Atlas/Overview") {
		t.Fatalf("popup candidates are not in the UI tree: %q", h.tt.Texts())
	}
}
