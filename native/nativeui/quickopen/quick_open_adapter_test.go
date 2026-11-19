package quickopen

import (
	"errors"
	"testing"

	"github.com/egoist/mygo/ui"
	workspaceui "muse/nativeui/workspace"
)

type testCandidates struct {
	revision Revision
	byQuery  map[string]Rows
	queries  []string
}

func (s *testCandidates) Rows(query string) Rows {
	s.queries = append(s.queries, query)
	return s.byQuery[query]
}

func (s *testCandidates) Revision() Revision { return s.revision }

type testWindowHost struct {
	manager   *workspaceui.Manager
	content   workspaceui.Content
	handle    workspaceui.Handle
	openErr   error
	placement workspaceui.Placement
}

func newTestWindowHost() *testWindowHost {
	return &testWindowHost{manager: workspaceui.NewManager(nil)}
}

func (h *testWindowHost) Open(content workspaceui.Content, placement workspaceui.Placement) (workspaceui.Handle, error) {
	if h.openErr != nil {
		return workspaceui.Handle{}, h.openErr
	}
	handle, err := h.manager.Open(content, placement)
	if err != nil {
		return workspaceui.Handle{}, err
	}
	h.content, h.handle, h.placement = content, handle, placement
	return handle, nil
}

func (h *testWindowHost) Close(handle workspaceui.Handle) error   { return h.manager.Close(handle) }
func (h *testWindowHost) Contains(handle workspaceui.Handle) bool { return h.manager.Contains(handle) }

func TestControllerOpensSelectsAndClosesHostedPalette(t *testing.T) {
	candidates := &testCandidates{
		revision: Revision{Catalog: 1, Index: 1},
		byQuery: map[string]Rows{
			"": testRows{
				{Path: "/vault/aardvark.txt", Title: "aardvark", PathText: "aardvark.txt"},
				{Path: "/vault/note.md", Title: "note", PathText: "note.md"},
			},
			"B": testRows{{Path: "/vault/子/笔记B.md", Title: "笔记B", PathText: "子/笔记B.md"}},
		},
	}
	host := newTestWindowHost()
	var opened []string
	closed := 0
	controller := NewController(ControllerOptions{
		Candidates: candidates,
		Windows:    host,
		Placement:  workspaceui.Placement{Host: "main", Layer: workspaceui.ModalOverlay},
		OnOpen:     func(path string) { opened = append(opened, path) },
		OnClose:    func() { closed++ },
	})
	if err := controller.Open(); err != nil {
		t.Fatal(err)
	}
	if !controller.IsOpen() || host.content == nil || host.placement.Layer != workspaceui.ModalOverlay {
		t.Fatalf("opened state=%v content=%T placement=%+v", controller.IsOpen(), host.content, host.placement)
	}

	tester := ui.NewTester(func(c *ui.Context) { host.content.Render(c) }, 800, 600)
	tester.Frame()
	if !tester.HasText("aardvark") {
		t.Fatalf("empty-query candidate missing: %q", tester.Texts())
	}
	tester.Key(0, ui.KeyEnter)
	tester.Frame()
	if len(opened) != 1 || opened[0] != "/vault/aardvark.txt" || controller.IsOpen() || closed != 1 {
		t.Fatalf("opened=%v open=%v close-callbacks=%d", opened, controller.IsOpen(), closed)
	}

	if err := controller.Open(); err != nil {
		t.Fatal(err)
	}
	tester = ui.NewTester(func(c *ui.Context) { host.content.Render(c) }, 800, 600)
	tester.Frame()
	tester.Type("B")
	tester.Frame()
	if !tester.HasText("笔记B") || len(candidates.queries) == 0 || candidates.queries[len(candidates.queries)-1] != "B" {
		t.Fatalf("query projection not refreshed: texts=%q queries=%v", tester.Texts(), candidates.queries)
	}
	tester.Key(0, ui.KeyEnter)
	tester.Frame()
	if len(opened) != 2 || opened[1] != "/vault/子/笔记B.md" || controller.IsOpen() || closed != 2 {
		t.Fatalf("second open=%v open=%v close-callbacks=%d", opened, controller.IsOpen(), closed)
	}
}

func TestControllerRefreshesCandidateRowsOnRevisionChange(t *testing.T) {
	candidates := &testCandidates{byQuery: map[string]Rows{"": testRows{{Path: "/a.md", Title: "Alpha"}}}}
	host := newTestWindowHost()
	controller := NewController(ControllerOptions{
		Candidates: candidates,
		Windows:    host,
		Placement:  workspaceui.Placement{Host: "main", Layer: workspaceui.ModalOverlay},
	})
	if err := controller.Open(); err != nil {
		t.Fatal(err)
	}
	tester := ui.NewTester(func(c *ui.Context) { host.content.Render(c) }, 800, 600)
	tester.Frame()
	if !tester.HasText("Alpha") {
		t.Fatalf("initial rows missing: %q", tester.Texts())
	}

	candidates.byQuery[""] = testRows{{Path: "/b.md", Title: "Beta"}}
	candidates.revision.Catalog++
	tester.Frame()
	if !tester.HasText("Beta") || tester.HasText("Alpha") {
		t.Fatalf("revision did not refresh rows: %q", tester.Texts())
	}
	if err := controller.Close(); err != nil {
		t.Fatal(err)
	}
	if controller.IsOpen() {
		t.Fatal("explicit close left hosted palette open")
	}
}

func TestControllerReportsHostFailureWithoutOpening(t *testing.T) {
	host := newTestWindowHost()
	host.openErr = errors.New("host unavailable")
	closed := 0
	controller := NewController(ControllerOptions{Windows: host, OnClose: func() { closed++ }})
	if err := controller.Open(); err == nil || err.Error() != "host unavailable" {
		t.Fatalf("open error = %v", err)
	}
	if controller.IsOpen() || closed != 0 {
		t.Fatalf("failed open left state open=%v close-callbacks=%d", controller.IsOpen(), closed)
	}
	if err := NewController(ControllerOptions{}).Open(); !errors.Is(err, ErrWindowHostUnavailable) {
		t.Fatalf("missing-host error = %v", err)
	}
}
