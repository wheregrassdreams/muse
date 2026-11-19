package nativeui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"muse/document/frontmatter"
	"muse/locales"
	propertypanel "muse/nativeui/workbench/features/properties"
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
			root := t.TempDir()
			notePath := filepath.Join(root, "Bresenham.md")
			if err := os.WriteFile(notePath, []byte(referencePage), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(root, "Source.md"), []byte("# Inbound\n\n[[Bresenham]]\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			configureVaultPorts(t, a)
			withDocumentPorts(t, a)
			a.panelOpen, a.panelW = false, 0
			a.flat = true
			a.themeMode = themeLight
			if dark {
				a.themeMode = themeDark
			}
			tt := ui.NewTester(a.View, 1600, 1000)
			a.setWorkspace(root)
			a.openPathNow(notePath)
			waitDocument(t, tt, func() bool { return a.vaultSnapshot.Ready && a.documentSource() == referencePage })
			a.propertiesPanel.Open()
			tt.Frame()
			tt.Advance(time.Second)
			tt.Frame()
			for _, value := range []string{locales.PropertyTitle(), locales.PropertyTypeFieldLabel(), locales.PropertyDefaultNoteType(), locales.PropertyViewBacklinks(), locales.PropertyTagsFieldLabel(), "图形学", locales.PropertyCreationDate()} {
				if !tt.HasText(value) {
					t.Errorf("page inspector lost %q", value)
				}
			}
			if tt.HasText("Object type") || tt.HasText("Legacy title kept") || tt.HasText("created_by") || tt.HasText("internal-data") {
				t.Fatal("page exposed retired title or omitted reference properties")
			}
			if tt.HasText(locales.PropertyModifiedDate()) || tt.HasText(locales.PropertyLastOpenedDate()) {
				t.Fatal("secondary properties should start collapsed")
			}
			runtimeShot(t, tt, "reference-page-"+name)
			a.propertiesPanel.Dispatch(propertypanel.Action{Kind: propertypanel.ToggleHiddenSection})
			tt.Frame()
			if !tt.HasText(locales.PropertyModifiedDate()) || !tt.HasText(locales.PropertyLastOpenedDate()) {
				t.Fatal("expanded section did not expose real document timestamps")
			}
			runtimeShot(t, tt, "reference-properties-expanded-"+name)
			if a.noteEditor().Source() != referencePage {
				t.Fatal("inspector styling or expanding hidden fields rewrote Markdown")
			}
		})
	}
}

func TestReferenceInspectorTypeEditPreservesBodyAndUndo(t *testing.T) {
	t.Setenv("MUSE_CONFIG_DIR", t.TempDir())
	a := New()
	setDocumentPathForTest(a, "/tmp/note.md")
	const src = "# Actual H1\n\nBody text.\n"
	a.loadDoc(src)
	a.propertiesPanel.Open()
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
	d := frontmatter.Parse(a.noteEditor().Source())
	if frontmatter.DocumentType(d) != "Article" || !strings.HasSuffix(a.noteEditor().Source(), src) {
		t.Fatalf("type edit damaged document: %q", a.noteEditor().Source())
	}
	if !a.noteEditor().Undo() || a.noteEditor().Source() != src {
		t.Fatal("type edit did not undo as one native operation")
	}
}
