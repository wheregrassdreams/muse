package render

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestProjectHTMLBlockStylesAndMapsSegments(t *testing.T) {
	segments := ProjectHTMLBlock([]HTMLLine{{Offset: 40, Text: `<div><b>Hi</b> there<br>go</div>`}}, Style{Text: ui.RGB(20, 30, 40)}, 16)
	if len(segments) != 5 {
		t.Fatalf("got %d segments: %+v", len(segments), segments)
	}
	want := []struct {
		kind HTMLSegmentKind
		text string
	}{
		{HTMLText, "Hi"},
		{HTMLAtom, " "},
		{HTMLText, "there"},
		{HTMLAtom, "\n"},
		{HTMLText, "go"},
	}
	for i, expected := range want {
		got := segments[i]
		if got.Kind != expected.kind || got.Text != expected.text {
			t.Errorf("segment %d = (%d, %q), want (%d, %q)", i, got.Kind, got.Text, expected.kind, expected.text)
		}
	}
	if segments[0].Style.Weight < 600 {
		t.Errorf("bold tag was not projected into its text style: %+v", segments[0].Style)
	}
	if segments[1].SourceStart != 54 || segments[1].SourceEnd != 55 {
		t.Errorf("collapsed whitespace range = [%d,%d), want [54,55)", segments[1].SourceStart, segments[1].SourceEnd)
	}
}

func TestProjectHTMLBlockPreservesPreAndHidesCommentsAndScripts(t *testing.T) {
	lines := []HTMLLine{
		{Offset: 0, Text: "<pre>one  two"},
		{Offset: 14, Text: "three</pre><script>hidden()"},
		{Offset: 42, Text: "still hidden</script> visible<!-- hidden"},
		{Offset: 83, Text: "hidden too --> done"},
	}
	segments := ProjectHTMLBlock(lines, Style{Mono: "mono"}, 16)
	var got string
	for _, segment := range segments {
		got += segment.Text
	}
	if got != "one  two\nthree\nvisible done" {
		t.Fatalf("projected text = %q", got)
	}
	if len(segments) == 0 || segments[0].Style.Font != "mono" {
		t.Errorf("pre style was not resolved: %+v", segments)
	}
}

func TestProjectHTMLBlockReturnsImageRequestsAsValues(t *testing.T) {
	segments := ProjectHTMLBlock([]HTMLLine{{Offset: 7, Text: `<img src="diagram.png" alt="Diagram" width="120" height="60">`}}, Style{}, 16)
	if len(segments) != 1 {
		t.Fatalf("segments = %+v", segments)
	}
	image := segments[0]
	if image.Kind != HTMLImage || image.ImageSource != "diagram.png" || image.Alt != "Diagram" || image.ImageWidth != 120 || image.ImageHeight != 60 {
		t.Fatalf("image projection = %+v", image)
	}
	if image.SourceStart != 7 || image.SourceEnd != 7+len(`<img src="diagram.png" alt="Diagram" width="120" height="60">`) {
		t.Errorf("image range = [%d,%d)", image.SourceStart, image.SourceEnd)
	}
}
