package nativeui

import (
	"testing"

	"github.com/egoist/mygo/ui"
	"muse/document/frontmatter"
	"muse/locales"
	propertypanel "muse/nativeui/workbench/features/properties"
)

func TestPropertiesInspectorKeepsFileSidebarAndSource(t *testing.T) {
	a := newConfigTestApp()
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
	if !a.propertiesPanel.IsOpen() {
		t.Fatal("Info did not open right property sidebar")
	}
	if a.panel != beforePanel || a.panelOpen != beforeOpen {
		t.Fatal("opening properties replaced the left file sidebar")
	}
	if a.noteEditor().Source() != src {
		t.Fatal("opening properties rewrote Markdown")
	}
	if frontmatter.Title(src, a.documentPath()) != "Body" {
		t.Fatal("canonical H1 title was not selected")
	}
}

func TestPropertyTitleOverrideUndoAndUserFields(t *testing.T) {
	a := newConfigTestApp()
	setDocumentPathForTest(a, "/tmp/default-name.md")
	src := "---\ntags: [one]\npath: custom-user-value\n---\nbody text\n"
	a.loadDoc(src)
	tt := ui.NewTester(a.View, 1200, 800)
	tt.Frame()
	tt.Frame()
	caret := a.noteEditor().NavigationCommands().Caret()
	if err := a.propertiesPanel.SetProperty("title", "Manual title"); err != nil {
		t.Fatal(err)
	}
	if frontmatter.Title(a.noteEditor().Source(), a.documentPath()) != "default-name" || a.documentPath() != "/tmp/default-name.md" {
		t.Fatal("legacy YAML title incorrectly overrode the canonical title")
	}
	if !a.noteEditor().Undo() || a.noteEditor().Source() != src || a.noteEditor().NavigationCommands().Caret() != caret {
		t.Fatal("property edit did not undo with original body caret")
	}
	if err := a.propertiesPanel.SetProperty("path", "new user value"); err != nil {
		t.Fatal("user YAML property incorrectly treated as system metadata", err)
	}
	if err := a.propertiesPanel.SetProperty("title", "Override"); err != nil {
		t.Fatal(err)
	}
	if err := a.propertiesPanel.DeleteProperty("title"); err != nil {
		t.Fatal(err)
	}
	if frontmatter.Title(a.noteEditor().Source(), a.documentPath()) != "default-name" {
		t.Fatal("removing title did not restore filename fallback")
	}
}

func TestPropertySummaryEditsInPlaceAndCanCancel(t *testing.T) {
	a := newConfigTestApp()
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
	if a.propertiesPanel.IsOpen() {
		t.Fatal("summary edit opened full form")
	}
	tt.Key(ui.Cmd, ui.KeyA)
	tt.Type("done")
	tt.Key(0, ui.KeyEnter)
	tt.Frame()
	found := false
	for _, f := range frontmatter.Parse(a.noteEditor().Source()).Fields {
		if f.Key == "status" && f.Value == "done" {
			found = true
		}
	}
	if !found {
		t.Fatalf("inline edit not saved: %q", a.noteEditor().Source())
	}
	saved := a.noteEditor().Source()
	if err := tt.Click("编辑属性 status"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	tt.Key(ui.Cmd, ui.KeyA)
	tt.Type("Cancelled")
	tt.Key(0, ui.KeyEscape)
	tt.Frame()
	if a.propertiesPanel.EditingKey() != "" || a.noteEditor().Source() != saved {
		t.Fatalf("Escape did not cancel metadata editing: key=%q source=%q want=%q", a.propertiesPanel.EditingKey(), a.noteEditor().Source(), saved)
	}
}

func TestDocumentUsesBodyH1AndPreservesLegacyTitle(t *testing.T) {
	t.Setenv("MUSE_CONFIG_DIR", t.TempDir())
	a := newConfigTestApp()
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
	if got := a.noteEditor().NavigationCommands().Headings(); len(got) != 1 || got[0].Text != "Body H1" {
		t.Fatalf("body H1 was changed: %+v", got)
	}
	a.propertiesPanel.Open()
	tt.Frame()
	tt.Frame()
	if tt.HasText("Legacy metadata title") || !tt.HasText(locales.PropertyTypeFieldLabel()) || !tt.HasText(locales.PropertyDefaultNoteType()) {
		t.Fatalf("inspector has a legacy title or lacks the localized default Type: %q", tt.Texts())
	}
	if a.noteEditor().Source() != src {
		t.Fatal("hiding title UI changed existing YAML or body")
	}
	runtimeShot(t, tt, "page-h1-without-title-field")
}

func TestPropertyDisplayPreferencesPersistOutsideMarkdown(t *testing.T) {
	t.Setenv("MUSE_CONFIG_DIR", t.TempDir())
	a := newConfigTestApp()
	setDocumentPathForTest(a, "/tmp/preferences-note.md")
	src := "---\ntags: [one]\nstatus: draft\n---\nBody\n"
	a.loadDoc(src)
	frontmatter.Parse(a.noteEditor().Source())
	a.propertiesPanel.Dispatch(propertypanel.Action{Kind: propertypanel.ToggleHidden, FieldKey: "tags"})
	a.propertiesPanel.MoveProperty("status", -1)
	if a.noteEditor().Source() != src {
		t.Fatal("display preferences changed YAML")
	}
	b := newConfigTestApp()
	setDocumentPathForTest(b, a.documentPath())
	b.loadDoc(src)
	d := frontmatter.Parse(b.noteEditor().Source())
	if !b.propertiesPanel.Preference("tags").Hidden {
		t.Fatal("hidden preference not restored on document open")
	}
	preferences := make(map[string]frontmatter.FieldPreference)
	for key, preference := range b.propertiesPanel.Preferences() {
		preferences[key] = frontmatter.FieldPreference{Pinned: preference.Pinned, Hidden: preference.Hidden, Order: preference.Order}
	}
	projection := frontmatter.ProjectInspector(d.Fields, preferences)
	if len(projection.Ordered) < 2 || projection.Ordered[0].Key != "status" {
		t.Fatalf("order was not restored: %+v", projection.Ordered)
	}
}

func TestPropertyYAMLSourceCannotOverwriteBody(t *testing.T) {
	a := newConfigTestApp()
	setDocumentPathForTest(a, "/tmp/source.md")
	a.loadDoc("---\ntitle: Old\n---\n# Keep body\n")
	if err := a.propertiesPanel.ApplyYAMLSource("---\ntitle: New\n---"); err != nil {
		t.Fatal(err)
	}
	want := "---\ntitle: New\n---\n# Keep body\n"
	if a.noteEditor().Source() != want {
		t.Fatalf("YAML source merged incorrectly: %q", a.noteEditor().Source())
	}
	if err := a.propertiesPanel.ApplyYAMLSource("---\ntitle: Bad\n---\nreplacing body"); err == nil {
		t.Fatal("source form accepted body content")
	}
	if a.noteEditor().Source() != want {
		t.Fatal("rejected YAML source changed document")
	}
}
