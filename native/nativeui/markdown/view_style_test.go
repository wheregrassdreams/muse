package markdown

import (
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestPreviewSoftBreakOption(t *testing.T) {
	// Exercise parser output so an explicit hard break cannot accidentally
	// acquire the same presentation as an ordinary source newline.
	doc := Parse("ordinary\ncontinuation  \nhard break\n\nnext paragraph\n")
	for _, spaces := range []bool{false, true} {
		var soft, hard bool
		tt := ui.NewTester(func(c *ui.Context) {
			s := rstyle{o: Options{SoftBreaksAsSpaces: spaces}.withDefaults()}
			for _, b := range doc.Blocks {
				runs := uiSpans(c, b.Inlines, s)
				for i, span := range b.Inlines {
					want := span.Text
					if span.SoftBreak {
						soft = true
						if spaces {
							want = " "
						}
					} else if strings.Contains(span.Text, "\n") {
						hard = true
					}
					if runs[i].Text != want {
						t.Errorf("spaces=%v: span %q became %q, want %q", spaces, span.Text, runs[i].Text, want)
					}
				}
			}
		}, 320, 200)
		tt.Frame()
		if !soft || !hard || len(doc.Blocks) != 2 {
			t.Fatal("fixture must retain a soft break, an explicit hard break and two paragraphs")
		}
	}
}
