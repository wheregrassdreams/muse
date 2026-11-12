package nativeui

import (
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"muse/frontmatter"
)

const referencePage = "---\ntitle: Legacy title kept in YAML\ntype: Note\ntags: [图形学]\ncreated: 2026-06-22T23:58:00+08:00\nmodified: 2026-11-12T11:05:00+08:00\ncreated_by: Zane\nlinks: internal-data\n---\n" +
	"# Bresenham 算法推导：从连续直线到离散像素\n\n" +
	"## 公式推导速览\n\n对于直线 $l$：\n\n$$\ny = kx + b\n$$\n\n" +
	"定义 $F(x,y)$，这个函数可以判断点 $(x,y)$ 在直线上还是直线的上方或下方。\n\n" +
	"$$\nF(x,y) = \\Delta y \\cdot x - \\Delta x \\cdot y + c\n$$\n\n" +
	"定义下一个位置的中点 $M_i = (x_i + 1, y_i + \\frac{1}{2})$，代入函数得 $F(M_i)$。\n"

func TestReferencePageLayoutAndHiddenProperties(t *testing.T) {
	for _, dark := range []bool{false, true} {
		name := "light"
		if dark {
			name = "dark"
		}
		t.Run(name, func(t *testing.T) {
			t.Setenv("MUSE_CONFIG_DIR", t.TempDir())
			a := New()
			a.path = "/tmp/Bresenham.md"
			a.panelOpen, a.panelW = false, 0
			a.flat = true
			a.themeMode = themeLight
			if dark {
				a.themeMode = themeDark
			}
			a.loadDoc(referencePage)
			a.openProperties()
			tt := ui.NewTester(a.View, 1100, 1000)
			tt.Frame()
			tt.Advance(time.Second)
			tt.Frame()
			for _, value := range []string{"属性", "Type", "Note", "Backlinks", "Tag", "图形学", "Creation date"} {
				if !tt.HasText(value) {
					t.Errorf("page inspector lost %q", value)
				}
			}
			if tt.HasText("Object type") || tt.HasText("Legacy title kept") || tt.HasText("created_by") || tt.HasText("internal-data") {
				t.Fatal("page exposed retired title or omitted reference properties")
			}
			if tt.HasText("Last modified date") || tt.HasText("Last opened date") {
				t.Fatal("secondary properties should start collapsed")
			}
			runtimeShot(t, tt, "reference-page-"+name)
			if err := tt.Click("隐藏属性"); err != nil {
				t.Fatal(err)
			}
			tt.Frame()
			if !tt.HasText("Last modified date") || !tt.HasText("Last opened date") {
				t.Fatal("expanded section did not expose real document timestamps")
			}
			runtimeShot(t, tt, "reference-properties-expanded-"+name)
			if a.ed.Source() != referencePage {
				t.Fatal("inspector styling or expanding hidden fields rewrote Markdown")
			}
		})
	}
}

func TestReferenceInspectorTypeEditPreservesBodyAndUndo(t *testing.T) {
	t.Setenv("MUSE_CONFIG_DIR", t.TempDir())
	a := New()
	a.path = "/tmp/note.md"
	const src = "# Actual H1\n\nBody text.\n"
	a.loadDoc(src)
	a.openProperties()
	tt := ui.NewTester(a.View, 1200, 800)
	tt.Frame()
	tt.Frame()
	if err := tt.Click("编辑类型"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	tt.Key(ui.Cmd, ui.KeyA)
	tt.Type("Article")
	tt.Key(0, ui.KeyEnter)
	tt.Frame()
	d := frontmatter.Parse(a.ed.Source())
	if propertyDocumentType(d) != "Article" || !strings.HasSuffix(a.ed.Source(), src) {
		t.Fatalf("type edit damaged document: %q", a.ed.Source())
	}
	if !a.ed.Undo() || a.ed.Source() != src {
		t.Fatal("type edit did not undo as one native operation")
	}
}
