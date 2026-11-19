package findreplace

import (
	"testing"

	editfind "github.com/wheregrassdreams/muse-editor/text/findreplace"
)

type fakeEditor struct {
	source       string
	highlights   []editfind.Match
	selected     int
	navigated    [2]int
	focused      int
	replaceCalls int
	allCalls     int
}

func (e *fakeEditor) Source() string { return e.source }
func (e *fakeEditor) SetHighlights(matches []editfind.Match, selected int) {
	e.highlights = append([]editfind.Match(nil), matches...)
	e.selected = selected
}
func (e *fakeEditor) NavigateToRange(start, end int) { e.navigated = [2]int{start, end} }
func (e *fakeEditor) Focus()                         { e.focused++ }
func (e *fakeEditor) Replace(start, end int, value string) error {
	e.replaceCalls++
	e.source = e.source[:start] + value + e.source[end:]
	return nil
}
func (e *fakeEditor) ReplaceAll(source string) {
	e.allCalls++
	e.source = source
}

func TestHostSyncsMatchesAndRoutesMatchNavigation(t *testing.T) {
	editor := &fakeEditor{source: "cat and cat"}
	host := NewHost(Options{Ports: Ports{Editor: editor}})
	host.Apply(Action{Kind: QueryChanged, Query: "cat"})
	if got := host.Snapshot(); len(got.Matches) != 2 || len(editor.highlights) != 2 {
		t.Fatalf("match projection = %+v, highlights=%+v", got.Matches, editor.highlights)
	}
	host.Apply(Action{Kind: OpenMatch, Index: 1})
	if got := host.Snapshot().Selected; got != 1 || editor.selected != 1 {
		t.Fatalf("selected = %d, editor selected = %d", got, editor.selected)
	}
	if editor.navigated != [2]int{8, 11} || editor.focused != 1 {
		t.Fatalf("navigation = %v, focus count = %d", editor.navigated, editor.focused)
	}
}

func TestHostAppliesSingleAndAllReplacementsThroughEditorPort(t *testing.T) {
	editor := &fakeEditor{source: "cat and cat and Cat"}
	replacedCount := 0
	host := NewHost(Options{Ports: Ports{
		Editor:   editor,
		Replaced: func(count int) { replacedCount = count },
	}})
	host.Apply(Action{Kind: QueryChanged, Query: "cat"})
	host.Apply(Action{Kind: ReplacementChanged, Text: "dog"})
	host.Apply(Action{Kind: OpenMatch, Index: 1})
	host.Apply(Action{Kind: ReplaceOne})
	if editor.source != "cat and dog and Cat" || editor.replaceCalls != 1 {
		t.Fatalf("single replacement = %q, calls=%d", editor.source, editor.replaceCalls)
	}
	host.Apply(Action{Kind: ReplaceAll})
	if editor.source != "dog and dog and dog" || editor.allCalls != 1 || replacedCount != 2 {
		t.Fatalf("replace all = %q, calls=%d, reported=%d", editor.source, editor.allCalls, replacedCount)
	}
}

func TestHostInvalidationRecomputesMatchesForSameSource(t *testing.T) {
	editor := &fakeEditor{source: "same source"}
	host := NewHost(Options{Ports: Ports{Editor: editor}})
	host.Apply(Action{Kind: QueryChanged, Query: "same"})
	if len(host.Snapshot().Matches) != 1 {
		t.Fatal("initial match missing")
	}
	host.Invalidate()
	host.Sync()
	if len(editor.highlights) != 1 {
		t.Fatalf("highlights after invalidation = %+v", editor.highlights)
	}
}
