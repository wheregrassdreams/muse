package noteeditor

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"

	editor "github.com/wheregrassdreams/muse-editor/text/session"
	"muse/document"
	editorui "muse/nativeui/workbench/features/editor"
	hosting "muse/nativeui/workbench/framework/hosting"
	"muse/platform/filesystem/documentstore"
	"muse/workspace"
)

type testSurface struct {
	session      *editor.Session
	options      SurfaceOptions
	presentation PresentationSettings
	focused      bool
	keymap       string
	closed       bool
}

var (
	_ hosting.Content          = (*testSurface)(nil)
	_ FocusCommands            = (*testSurface)(nil)
	_ ModalCommands            = (*testSurface)(nil)
	_ NavigationCommands       = (*testSurface)(nil)
	_ SearchCommands           = (*testSurface)(nil)
	_ ClipboardCommands        = (*testSurface)(nil)
	_ FormattingCommands       = (*testSurface)(nil)
	_ PresentationCommands     = (*testSurface)(nil)
	_ DocumentOpenCommands     = (*testSurface)(nil)
	_ SystemEditCommands       = (*testSurface)(nil)
	_ interface{ Undo() bool } = (*testSurface)(nil)
)

func (s *testSurface) Render(*ui.Context) {}
func (s *testSurface) Close()             { s.closed = true }

func (s *testSurface) Focus()                  { s.focused = true }
func (s *testSurface) Focused() bool           { return s.focused }
func (s *testSurface) Keymap() string          { return s.keymap }
func (s *testSurface) SetKeymap(keymap string) { s.keymap = keymap }
func (s *testSurface) ModeLabel() string       { return "" }
func (s *testSurface) ModalAcceptsText() bool  { return s.keymap != "vim" }

func (s *testSurface) Caret() int { return s.session.Selection().Head }
func (s *testSurface) SelectionRange() TextRange {
	selection := s.session.Selection()
	return TextRange{Start: min(selection.Anchor, selection.Head), End: max(selection.Anchor, selection.Head)}
}
func (s *testSurface) Select(start, end int) {
	_ = s.session.SetSelection(editor.Selection{Anchor: start, Head: end})
}
func (s *testSurface) ScrollY() float32                         { return 0 }
func (*testSurface) ScrollToOffset(int)                         {}
func (s *testSurface) NavigateToOffset(offset int)              { s.Select(offset, offset) }
func (s *testSurface) NavigateToRange(start, end int)           { s.Select(start, end) }
func (s *testSurface) Headings() []Heading                      { return nil }
func (s *testSurface) WindowPoint(int) (float32, float32, bool) { return 0, 0, false }

func (s *testSurface) SetHighlights([]TextRange, int) {}
func (s *testSurface) SelectedText() string {
	r := s.SelectionRange()
	source := s.session.Source()
	return source[r.Start:r.End]
}
func (*testSurface) CopyImageObject(string) {}

func (*testSurface) AppendBlock(string)                        {}
func (*testSurface) PrependBlock(string)                       {}
func (*testSurface) ToggleBold()                               {}
func (*testSurface) ToggleItalic()                             {}
func (*testSurface) ToggleUnderline()                          {}
func (*testSurface) ToggleStrike()                             {}
func (*testSurface) ToggleCode()                               {}
func (*testSurface) ToggleHighlight()                          {}
func (*testSurface) ToggleQuote()                              {}
func (*testSurface) ToggleList(string)                         {}
func (*testSurface) SetHeading(int)                            {}
func (*testSurface) InsertCodeBlock()                          {}
func (*testSurface) InsertMathBlock()                          {}
func (*testSurface) InsertTable(int, int)                      {}
func (*testSurface) InsertRule()                               {}
func (*testSurface) InsertLink()                               {}
func (s *testSurface) Configure(settings PresentationSettings) { s.presentation = settings }
func (s *testSurface) Settings() PresentationSettings          { return s.presentation }
func (*testSurface) ResetForDocumentOpen()                     {}
func (s *testSurface) Undo() bool {
	result, err := s.session.Undo()
	return err == nil && result.Changed
}
func (s *testSurface) ReplaceSystemAll(source string) {
	patch, ok := s.session.PlanSystemPatch(source)
	if !ok {
		return
	}
	_, _ = s.session.ApplySystemPatch(patch, nil, nil)
}

func TestContentOwnsEditorAndDocumentLifecycle(t *testing.T) {
	path := filepath.Join(t.TempDir(), "note.md")
	diskSource := "one\r\ntwo\r\n"
	if err := os.WriteFile(path, []byte(diskSource), 0o600); err != nil {
		t.Fatal(err)
	}
	store := documentstore.New(filepath.Join(t.TempDir(), "recovery"))
	var hosted *testSurface
	var actions []Action
	content, err := NewContent("pane-a", document.NewNote(path, diskSource, document.RevisionOf(diskSource)), 5, Dependencies{
		Documents: store, Recovery: store, History: store,
	}, func(session *editor.Session, options SurfaceOptions) hosting.Content {
		hosted = &testSurface{session: session, options: options}
		return hosted
	}, func(action Action) { actions = append(actions, action) })
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(content.Close)

	if got := content.SessionFrame().Source; got != "one\ntwo\n" {
		t.Fatalf("normalized editor source = %q", got)
	}
	if snap := content.Snapshot(); snap.PaneID != workspace.PaneID("pane-a") || snap.DiskSource != diskSource || snap.CursorOffset != 5 || snap.Dirty {
		t.Fatalf("initial pane snapshot = %+v", snap)
	}
	if hosted == nil || hosted.session != content.session {
		t.Fatal("factory did not receive the pane-owned editor session")
	}
	if err := content.Replace(0, len("one\ntwo\n"), "one\ntwo\n"); err != nil {
		t.Fatal(err)
	}
	if content.Snapshot().Dirty || len(actions) != 0 {
		t.Fatalf("no-op replacement changed document state: snapshot=%+v actions=%+v", content.Snapshot(), actions)
	}

	if err := content.ReplaceUser("changed\n"); err != nil {
		t.Fatal(err)
	}
	if got := content.Snapshot(); !got.Dirty || got.Source != "changed\n" || got.DiskSource != "changed\r\n" {
		t.Fatalf("edited pane snapshot = %+v", got)
	}
	if len(actions) != 1 || actions[0].Kind != DocumentChanged || actions[0].PaneID != "pane-a" {
		t.Fatalf("change actions = %+v", actions)
	}

	snapshot := content.Snapshot()
	if _, err := content.SaveNow(snapshot.Path, snapshot.Revision); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "changed\r\n" || content.Snapshot().Dirty {
		t.Fatalf("saved pane source = %q, dirty=%v", got, content.Snapshot().Dirty)
	}
}

func TestHostedVimInsertSurvivesACommittedCharacter(t *testing.T) {
	content, err := NewNativeContent("pane-vim", document.NewNote("", "abc", document.RevisionOf("abc")), 0,
		Dependencies{}, NativeEditorOptions{}, func(Action) {})
	if err != nil {
		t.Fatal(err)
	}
	defer content.Close()
	modal := content.ModalCommands()
	modal.SetKeymap("vim")
	tester := ui.NewTester(func(c *ui.Context) { content.Render(c) }, 640, 480)
	tester.Frame()
	content.FocusCommands().Focus()
	tester.Frame()
	tester.TypeKey(0, ui.KeyL, "l")
	tester.TypeKey(0, ui.KeyI, "i")
	tester.TypeKey(0, ui.KeyX, "X")
	// App settings may be reapplied while the hosted pane is invalidated. The
	// same keymap must not reset Vim's live mode or insert transaction.
	modal.SetKeymap("vim")
	if got := modal.ModeLabel(); got != "vim Insert" && got != "Insert" {
		t.Fatalf("mode after committed character = %q, want Insert", got)
	}
	if got := content.Snapshot().Source; got != "aXbc" {
		t.Fatalf("source after committed character = %q, want aXbc", got)
	}
}

func TestOpenPlacesCaretAtFirstVisibleLineAfterHiddenFrontmatter(t *testing.T) {
	source := "---\ntype: note\ntags: []\n---\n\n# Body\ntext\n"
	store := documentstore.New(filepath.Join(t.TempDir(), "recovery"))
	var hosted *testSurface
	content, err := NewContent("pane-frontmatter", document.NewNote("note.md", source, document.RevisionOf(source)), 0, Dependencies{
		Documents: store, Recovery: store, History: store,
	}, func(session *editor.Session, _ SurfaceOptions) hosting.Content {
		hosted = &testSurface{session: session}
		return hosted
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(content.Close)
	hosted.Configure(PresentationSettings{HideFrontmatter: true})
	if err := content.Open(document.NewNote("note.md", source, document.RevisionOf(source)), 0); err != nil {
		t.Fatal(err)
	}
	want := strings.Index(source, "# Body")
	if got := content.NavigationCommands().Caret(); got != want {
		t.Fatalf("open caret = %d, want first visible body offset %d", got, want)
	}
	if got := content.Snapshot().Source; got != source {
		t.Fatalf("opening changed source: %q", got)
	}
}

func TestNewContentPlacesCaretAtFirstVisibleLineAfterHiddenFrontmatter(t *testing.T) {
	source := "---\ntype: note\ntags: []\n---\n\n# Body\ntext\n"
	store := documentstore.New(filepath.Join(t.TempDir(), "recovery"))
	var hosted *testSurface
	content, err := NewContent("pane-frontmatter", document.NewNote("note.md", source, document.RevisionOf(source)), 0, Dependencies{
		Documents: store, Recovery: store, History: store,
	}, func(session *editor.Session, options SurfaceOptions) hosting.Content {
		hosted = &testSurface{session: session, options: options, presentation: PresentationSettings{HideFrontmatter: true}}
		return hosted
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(content.Close)
	want := strings.Index(source, "# Body")
	if got := content.NavigationCommands().Caret(); got != want {
		t.Fatalf("initial pane caret = %d, want first visible body offset %d", got, want)
	}
	if got := content.Snapshot().Source; got != source {
		t.Fatalf("initial mount changed source: %q", got)
	}
}

func TestNewNativeContentPlacesDefaultCaretAtFirstEditableText(t *testing.T) {
	for _, tc := range []struct {
		name   string
		source string
		opts   editorui.Options
		text   string
	}{
		{name: "heading", source: "# Title\n\nBody\n", text: "Title"},
		{name: "list item", source: "- Item\n", text: "Item"},
		{
			name:   "hidden frontmatter heading",
			source: "---\ntype: note\n---\n\n# Title\nBody\n",
			opts:   editorui.Options{HideFrontmatter: true},
			text:   "Title",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			content, err := NewNativeContent("pane-initial-caret", document.NewNote("note.md", tc.source, document.RevisionOf(tc.source)), 0,
				Dependencies{}, NativeEditorOptions{Editor: tc.opts}, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer content.Close()
			want := strings.Index(tc.source, tc.text)
			if got := content.NavigationCommands().Caret(); got != want {
				t.Fatalf("initial caret = %d, want first editable text at %d", got, want)
			}
		})
	}
}

func TestContentCloseClosesOwnerOnce(t *testing.T) {
	store := documentstore.New(filepath.Join(t.TempDir(), "recovery"))
	var actions []Action
	var hosted *testSurface
	content, err := NewUntitled("pane-b", "", Dependencies{Documents: store, Recovery: store, History: store}, func(session *editor.Session, options SurfaceOptions) hosting.Content {
		hosted = &testSurface{session: session, options: options}
		return hosted
	}, func(action Action) { actions = append(actions, action) })
	if err != nil {
		t.Fatal(err)
	}
	content.Close()
	content.Close()
	if !hosted.closed || len(actions) != 1 || actions[0].Kind != ContentClosed || actions[0].PaneID != "pane-b" {
		t.Fatalf("close lifecycle: closed=%v actions=%+v", hosted.closed, actions)
	}
}

func TestContentMutationConsolidatesRendererChangeCallback(t *testing.T) {
	store := documentstore.New(filepath.Join(t.TempDir(), "recovery"))
	var actions []Action
	var hosted *testSurface
	content, err := NewUntitled("pane-c", "", Dependencies{Documents: store, Recovery: store, History: store}, func(session *editor.Session, options SurfaceOptions) hosting.Content {
		hosted = &testSurface{session: session, options: options}
		return hosted
	}, func(action Action) { actions = append(actions, action) })
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(content.Close)

	content.mutateSession(func() { hosted.options.OnChange() })
	if len(actions) != 0 {
		t.Fatalf("renderer callback escaped semantic mutation: %+v", actions)
	}
	content.editorChanged()
	if len(actions) != 1 || actions[0].Kind != DocumentChanged {
		t.Fatalf("semantic mutation emitted %d actions, want one DocumentChanged: %+v", len(actions), actions)
	}
}
