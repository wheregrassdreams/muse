package nativeui

import (
	"github.com/egoist/mygo/ui"
	"muse/frontmatter"
	"testing"
)

func TestPropertiesInspectorKeepsFileSidebarAndSource(t *testing.T) {
	a := New()
	a.path = "/tmp/filename-title.md"
	src := "---\ntags: [work, 回归]\nstatus: draft\ncustom: keep\n---\n# Body\n"
	a.loadDoc(src)
	tt := ui.NewTester(a.View, 1200, 800)
	tt.Frame()
	tt.Frame()
	beforePanel, beforeOpen := a.panel, a.panelOpen
	if err := tt.Click("信息"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !a.propertiesOpen {
		t.Fatal("Info did not open right property sidebar")
	}
	if a.panel != beforePanel || a.panelOpen != beforeOpen {
		t.Fatal("opening properties replaced the left file sidebar")
	}
	if a.ed.Source() != src {
		t.Fatal("opening properties rewrote Markdown")
	}
	if frontmatter.Title(src, a.path) != "filename-title" {
		t.Fatal("default title not derived from filename")
	}
}

func TestPropertyTitleOverrideUndoAndUserFields(t *testing.T) {
	a := New()
	a.path = "/tmp/default-name.md"
	src := "---\ntags: [one]\npath: custom-user-value\n---\nbody text\n"
	a.loadDoc(src)
	tt := ui.NewTester(a.View, 1200, 800)
	tt.Frame()
	tt.Frame()
	caret := a.ed.Caret()
	if err := a.propertySet("title", "Manual title"); err != nil {
		t.Fatal(err)
	}
	if frontmatter.Title(a.ed.Source(), a.path) != "Manual title" || a.path != "/tmp/default-name.md" {
		t.Fatal("title edit renamed file or failed")
	}
	if !a.ed.Undo() || a.ed.Source() != src || a.ed.Caret() != caret {
		t.Fatal("property edit did not undo with original body caret")
	}
	if err := a.propertySet("path", "new user value"); err != nil {
		t.Fatal("user YAML property incorrectly treated as system metadata", err)
	}
	if err := a.propertySet("title", "Override"); err != nil {
		t.Fatal(err)
	}
	if err := a.propertyDelete("title"); err != nil {
		t.Fatal(err)
	}
	if frontmatter.Title(a.ed.Source(), a.path) != "default-name" {
		t.Fatal("removing title did not restore filename fallback")
	}
}

func TestPropertySummaryEditsInPlaceAndTitleCanCancel(t *testing.T) {
	a := New()
	a.path = "/tmp/Filename.md"
	src := "---\nstatus: draft\ntags: [one]\n---\nBody\n"
	a.loadDoc(src)
	tt := ui.NewTester(a.View, 1200, 800)
	tt.Frame()
	tt.Frame()
	if err := tt.Click("编辑属性 status"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.propertiesOpen {
		t.Fatal("summary edit opened full form")
	}
	tt.Key(ui.Cmd, ui.KeyA)
	tt.Type("done")
	tt.Key(0, ui.KeyEnter)
	tt.Frame()
	found := false
	for _, f := range frontmatter.Parse(a.ed.Source()).Fields {
		if f.Key == "status" && f.Value == "done" {
			found = true
		}
	}
	if !found {
		t.Fatalf("inline edit not saved: %q", a.ed.Source())
	}
	saved := a.ed.Source()
	if err := tt.Click("编辑文档标题"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	tt.Key(ui.Cmd, ui.KeyA)
	tt.Type("Cancelled")
	tt.Key(0, ui.KeyEscape)
	tt.Frame()
	if a.propertyTitleEditing || a.ed.Source() != saved {
		t.Fatal("Escape did not cancel title editing")
	}
	if err := tt.Click("编辑文档标题"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	tt.Key(ui.Cmd, ui.KeyA)
	tt.Type("Custom title")
	tt.Key(0, ui.KeyEnter)
	tt.Frame()
	if frontmatter.Title(a.ed.Source(), a.path) != "Custom title" {
		t.Fatalf("title UI edit failed: %q", a.ed.Source())
	}
}

func TestDocumentTitleCommitsOnBlur(t *testing.T) {
	a := New()
	a.path = "/tmp/Filename.md"
	a.loadDoc("Body\n")
	tt := ui.NewTester(a.View, 1200, 800)
	tt.Frame()
	tt.Frame()
	if err := tt.Click("编辑文档标题"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	tt.Key(ui.Cmd, ui.KeyA)
	tt.Type("Edited in place")
	tt.Frame()
	runtimeShot(t, tt, "title-editing-borderless")
	if err := tt.Click("信息"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	tt.Frame()
	if a.propertyTitleEditing {
		t.Fatal("title stayed in edit mode after losing focus")
	}
	if got := frontmatter.Title(a.ed.Source(), a.path); got != "Edited in place" {
		t.Fatalf("blur did not save title: %q", got)
	}
	runtimeShot(t, tt, "title-after-blur")
}

func TestPropertyDisplayPreferencesPersistOutsideMarkdown(t *testing.T) {
	t.Setenv("MUSE_CONFIG_DIR", t.TempDir())
	a := New()
	a.path = "/tmp/preferences-note.md"
	src := "---\ntags: [one]\nstatus: draft\n---\nBody\n"
	a.loadDoc(src)
	a.propertyDocument()
	a.updatePropertyPreference("tags", func(p *fieldDisplayPreference) { p.Hidden = true })
	a.moveProperty("status", -1)
	if a.ed.Source() != src {
		t.Fatal("display preferences changed YAML")
	}
	b := New()
	b.path = a.path
	b.loadDoc(src)
	d := b.propertyDocument()
	if !b.propertyPrefs.Fields["tags"].Hidden {
		t.Fatal("hidden preference not restored on document open")
	}
	fields := b.orderedProperties(d.Fields)
	if len(fields) < 2 || fields[0].Key != "status" {
		t.Fatalf("order was not restored: %+v", fields)
	}
}

func TestPropertyYAMLSourceCannotOverwriteBody(t *testing.T) {
	a := New()
	a.path = "/tmp/source.md"
	a.loadDoc("---\ntitle: Old\n---\n# Keep body\n")
	if err := a.applyPropertyYAMLSource("---\ntitle: New\n---"); err != nil {
		t.Fatal(err)
	}
	want := "---\ntitle: New\n---\n# Keep body\n"
	if a.ed.Source() != want {
		t.Fatalf("YAML source merged incorrectly: %q", a.ed.Source())
	}
	if err := a.applyPropertyYAMLSource("---\ntitle: Bad\n---\nreplacing body"); err == nil {
		t.Fatal("source form accepted body content")
	}
	if a.ed.Source() != want {
		t.Fatal("rejected YAML source changed document")
	}
}
