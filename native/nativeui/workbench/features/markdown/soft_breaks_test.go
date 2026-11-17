package markdown

import (
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestSoftBreakProjectionPreservesPlainTextAndHardBreaks(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{"one\ntwo\n", "one two"},
		{"one  \ntwo\n", "one\ntwo"},
		{"one\\\ntwo\n", "one\ntwo"},
	} {
		d := Parse(tc.src)
		if d.PlainText() != "one\ntwo\n" {
			t.Fatal("plain text source semantics changed")
		}
		var got string
		tt := ui.NewTester(func(c *ui.Context) {
			var b strings.Builder
			for _, sp := range uiSpans(c, d.Blocks[0].Inlines, rstyle{o: Options{SoftBreaksAsSpaces: true}}) {
				b.WriteString(sp.Text)
			}
			got = b.String()
		}, 300, 200)
		tt.Frame()
		if got != tc.want {
			t.Fatalf("%q rendered %q want %q", tc.src, got, tc.want)
		}
	}
}

func TestQuotedSoftBreakProjectionKeepsSourceRows(t *testing.T) {
	for _, src := range []string{
		"> 普通引用。\n> 第二行。\n",
		"> [!NOTE] 提示\n> 正文第一行。\n> 正文第二行。\n",
	} {
		t.Run(src, func(t *testing.T) {
			d := Parse(src)
			if len(d.Blocks) != 1 || d.Blocks[0].Kind != Quote || len(d.Blocks[0].Children) != 1 {
				t.Fatalf("unexpected quote parse: %#v", d.Blocks)
			}
			spans := d.Blocks[0].Children[0].Inlines
			var got string
			tt := ui.NewTester(func(c *ui.Context) {
				got = ""
				for _, sp := range uiSpans(c, spans, rstyle{o: Options{SoftBreaksAsSpaces: true}, preserveSoftBreaks: true}) {
					got += sp.Text
				}
			}, 400, 200)
			tt.Frame()
			want := "普通引用。\n第二行。"
			if strings.Contains(src, "[!NOTE]") {
				want = "[!NOTE] 提示\n正文第一行。\n正文第二行。"
			}
			if got != want {
				t.Fatalf("quoted display=%q want %q", got, want)
			}
			if got := d.PlainText(); !strings.Contains(got, "\n") {
				t.Fatalf("plain text lost source newline: %q", got)
			}
		})
	}
}
