package nativeui

import (
	"github.com/egoist/mygo/ui"
	"muse/document/frontmatter"
	"testing"
)

func TestPropertiesInspectorKeepsFileSidebarAndSource(t *testing.T) {
	a := New()
	setDocumentPathForTest(a, "/tmp/filename-title.md")
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
	if a.activeNote.Source() != src {
		t.Fatal("opening properties rewrote Markdown")
	}
	if frontmatter.Title(src, a.documentPath()) != "Body" {
		t.Fatal("canonical H1 title was not selected")
	}
}

func TestPropertyTitleOverrideUndoAndUserFields(t *testing.T) {
	a := New()
	setDocumentPathForTest(a, "/tmp/default-name.md")
	src := "---\ntags: [one]\npath: custom-user-value\n---\nbody text\n"
	a.loadDoc(src)
	tt := ui.NewTester(a.View, 1200, 800)
	tt.Frame()
	tt.Frame()
	caret := a.activeNote.NavigationCommands().Caret()
	if err := a.propertySet("title", "Manual title"); err != nil {
		t.Fatal(err)
	}
	if frontmatter.Title(a.activeNote.Source(), a.documentPath()) != "default-name" || a.documentPath() != "/tmp/default-name.md" {
		t.Fatal("legacy YAML title incorrectly overrode the canonical title")
	}
	if !a.activeNote.Undo() || a.activeNote.Source() != src || a.activeNote.NavigationCommands().Caret() != caret {
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
	if frontmatter.Title(a.activeNote.Source(), a.documentPath()) != "default-name" {
		t.Fatal("removing title did not restore filename fallback")
	}
}

func TestPropertySummaryEditsInPlaceAndCanCancel(t *testing.T) {
	a := New()
	setDocumentPathForTest(a, "/tmp/Filename.md")
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
	for _, f := range frontmatter.Parse(a.activeNote.Source()).Fields {
		if f.Key == "status" && f.Value == "done" {
			found = true
		}
	}
	if !found {
		t.Fatalf("inline edit not saved: %q", a.activeNote.Source())
	}
	saved := a.activeNote.Source()
	if err := tt.Click("编辑属性 status"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	tt.Key(ui.Cmd, ui.KeyA)
	tt.Type("Cancelled")
	tt.Key(0, ui.KeyEscape)
	tt.Frame()
	if a.propertyEditKey != "" || a.activeNote.Source() != saved {
		t.Fatal("Escape did not cancel metadata editing")
	}
}

func TestDocumentUsesBodyH1AndPreservesLegacyTitle(t *testing.T) {
	t.Setenv("MUSE_CONFIG_DIR", t.TempDir())
	a := New()
	setDocumentPathForTest(a, "/tmp/Filename.md")
	const src = "---\ntitle: Legacy metadata title\ntags: [one]\n---\n# Body H1\n\nBody\n"
	a.loadDoc(src)
	tt := ui.NewTester(a.View, 1200, 800)
	tt.Frame()
	tt.Frame()
	if tt.HasText("Legacy metadata title") {
		t.Fatal("page still renders a second title outside the body H1")
	}
	if err := tt.Click("编辑文档标题"); err == nil {
		t.Fatal("dedicated title editing is still available")
	}
	if got := a.activeNote.NavigationCommands().Headings(); len(got) != 1 || got[0].Text != "Body H1" {
		t.Fatalf("body H1 was changed: %+v", got)
	}
	a.openProperties()
	tt.Frame()
	tt.Frame()
	if tt.HasText("Legacy metadata title") || !tt.HasText("Type") || !tt.HasText("Note") {
		t.Fatal("inspector has a legacy title or lacks the default Type")
	}
	if a.activeNote.Source() != src {
		t.Fatal("hiding title UI changed existing YAML or body")
	}
	runtimeShot(t, tt, "page-h1-without-title-field")
}

func TestPropertyDisplayPreferencesPersistOutsideMarkdown(t *testing.T) {
	t.Setenv("MUSE_CONFIG_DIR", t.TempDir())
	a := New()
	setDocumentPathForTest(a, "/tmp/preferences-note.md")
	src := "---\ntags: [one]\nstatus: draft\n---\nBody\n"
	a.loadDoc(src)
	a.propertyDocument()
	a.updatePropertyPreference("tags", func(p *fieldDisplayPreference) { p.Hidden = true })
	a.moveProperty("status", -1)
	if a.activeNote.Source() != src {
		t.Fatal("display preferences changed YAML")
	}
	b := New()
	setDocumentPathForTest(b, a.documentPath())
	b.loadDoc(src)
	d := b.propertyDocument()
	if !b.propertyPrefs.Fields["tags"].Hidden {
		t.Fatal("hidden preference not restored on document open")
	}
	fields := b.propertyProjection(d.Fields).Ordered
	if len(fields) < 2 || fields[0].Key != "status" {
		t.Fatalf("order was not restored: %+v", fields)
	}
}

func TestPropertyYAMLSourceCannotOverwriteBody(t *testing.T) {
	a := New()
	setDocumentPathForTest(a, "/tmp/source.md")
	a.loadDoc("---\ntitle: Old\n---\n# Keep body\n")
	if err := a.applyPropertyYAMLSource("---\ntitle: New\n---"); err != nil {
		t.Fatal(err)
	}
	want := "---\ntitle: New\n---\n# Keep body\n"
	if a.activeNote.Source() != want {
		t.Fatalf("YAML source merged incorrectly: %q", a.activeNote.Source())
	}
	if err := a.applyPropertyYAMLSource("---\ntitle: Bad\n---\nreplacing body"); err == nil {
		t.Fatal("source form accepted body content")
	}
	if a.activeNote.Source() != want {
		t.Fatal("rejected YAML source changed document")
	}
}
