package noteeditor

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"

	"muse/document"
	"muse/editor"
	"muse/nativeui/window"
	"muse/platform/filesystem/documentstore"
	"muse/workspace"
)

type testSurface struct {
	session *editor.Session
	options SurfaceOptions
	closed  bool
}

func (s *testSurface) Render(*ui.Context) {}
func (s *testSurface) Close()             { s.closed = true }

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
	}, func(session *editor.Session, options SurfaceOptions) window.Content {
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

func TestContentCloseClosesOwnerOnce(t *testing.T) {
	store := documentstore.New(filepath.Join(t.TempDir(), "recovery"))
	var actions []Action
	var hosted *testSurface
	content, err := NewUntitled("pane-b", "", Dependencies{Documents: store, Recovery: store, History: store}, func(session *editor.Session, options SurfaceOptions) window.Content {
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
	content, err := NewUntitled("pane-c", "", Dependencies{Documents: store, Recovery: store, History: store}, func(session *editor.Session, options SurfaceOptions) window.Content {
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
