package markdown

import "testing"

func TestInlineRunsHideMarkers(t *testing.T) {
	src := "a **bold** *it* `code` ~~s~~ [link](https://x) $x^2$"
	runs := InlineRuns(src)
	var visible string
	styles := map[string]bool{}
	for _, r := range runs {
		visible += r.Text
		if r.Bold {
			styles["bold"] = true
		}
		if r.Italic {
			styles["it"] = true
		}
		if r.Code {
			styles["code"] = true
		}
		if r.Strike {
			styles["s"] = true
		}
		if r.Link == "https://x" {
			styles["link"] = true
		}
		if r.Math {
			styles["math"] = true
		}
	}
	if visible != "a bold it code s link x^2" {
		t.Fatalf("visible = %q", visible)
	}
	for _, want := range []string{"bold", "it", "code", "s", "link", "math"} {
		if !styles[want] {
			t.Fatalf("missing style %q in %+v", want, runs)
		}
	}
	for _, r := range runs {
		if r.Text == "bold" && src[r.SourceStart:r.SourceEnd] != "bold" {
			t.Fatalf("source range = %q", src[r.SourceStart:r.SourceEnd])
		}
	}
}
