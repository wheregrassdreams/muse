package explorer

import (
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
	"muse/nativeui/commands"
)

func TestControllersOwnIndependentPresentationState(t *testing.T) {
	first := NewController()
	second := NewController()
	first.SetExpanded("/vault/notes", true)

	if !first.IsExpanded("/vault/notes") {
		t.Fatal("first controller lost its expanded row")
	}
	if second.IsExpanded("/vault/notes") {
		t.Fatal("separate Explorer controllers shared transient tree state")
	}
}

func TestExplorerBoundaryDoesNotCaptureAppOrOtherFeatures(t *testing.T) {
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("could not locate Explorer package")
	}
	dir := filepath.Dir(source)
	entries, err := filepath.Glob(filepath.Join(dir, "*.go"))
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range entries {
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ParseComments)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		if file.Name.Name != "explorer" {
			t.Errorf("%s declares package %q, want explorer", filepath.Base(path), file.Name.Name)
		}
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Recv == nil {
				continue
			}
			if isAppReceiver(fn.Recv) {
				t.Errorf("%s captures *App in %s", filepath.Base(path), fn.Name.Name)
			}
		}
		for _, spec := range file.Imports {
			importPath := strings.Trim(spec.Path.Value, "\"")
			if strings.Contains(importPath, "/features/") && importPath != "muse/nativeui/explorer/filetree" {
				t.Errorf("%s imports another feature implementation: %s", filepath.Base(path), importPath)
			}
		}
	}
}

func isAppReceiver(receiver *ast.FieldList) bool {
	for _, field := range receiver.List {
		star, ok := field.Type.(*ast.StarExpr)
		if !ok {
			continue
		}
		if ident, ok := star.X.(*ast.Ident); ok && ident.Name == "App" {
			return true
		}
	}
	return false
}

func TestDragItemIsAFeatureOwnedValue(t *testing.T) {
	item := DragItem{Path: "/vault/notes/a.md", Name: "a.md"}
	if item.Path == "" || item.Directory {
		t.Fatalf("unexpected drag value: %+v", item)
	}
}

func TestControllerReturnsTypedKeyIntent(t *testing.T) {
	controller := NewController()
	binding, handled := controller.HandleKey(0, ui.KeyJ, "vim")
	if !handled || binding.Action != commands.ActionNavigate || binding.Args.Delta != 1 {
		t.Fatalf("vim j did not become a typed navigation intent: handled=%v binding=%+v", handled, binding)
	}
}

func TestActionForKeyBindingReturnsOpenAndCreateIntents(t *testing.T) {
	controller := NewController()
	controller.SetFocusID("/vault/notes/a.md")
	open := controller.ActionForKeyBinding(TreeBinding{Action: commands.ActionOpen}, "/vault", "", nil)
	if open.Kind != OpenEntry || open.ID != "/vault/notes/a.md" {
		t.Fatalf("open key action = %+v", open)
	}
	create := controller.ActionForKeyBinding(TreeBinding{Action: commands.ActionCreateFolder}, "/vault", "/vault/notes",
		func(path string) bool { return path == "/vault/notes" })
	if create.Kind != CreateFolder || create.Directory != "/vault/notes" {
		t.Fatalf("create key action = %+v", create)
	}
}

func TestCreateActionResolvesSelectionIntoTypedRequest(t *testing.T) {
	controller := NewController()

	file := controller.CreateAction(CreateFile, "/vault", "/vault/notes", func(path string) bool { return path == "/vault/notes" })
	if file.Kind != CreateFile || file.Directory != "/vault/notes" {
		t.Fatalf("directory selection should be the create target: %+v", file)
	}

	folder := controller.CreateAction(CreateFolder, "/vault", "/vault/readme.md", func(string) bool { return false })
	if folder.Kind != CreateFolder || folder.Directory != "/vault" {
		t.Fatalf("file selection should target vault root: %+v", folder)
	}

	if action := controller.CreateAction(InvokeCommand, "/vault", "", nil); action.Kind != 0 {
		t.Fatalf("non-create kind produced a create request: %+v", action)
	}
}

func TestHandleActionUsesCreateCapabilitiesAndRoutesResults(t *testing.T) {
	var opened, createdAt, folderAt string
	invalidated := 0
	noteAction := Action{Kind: CreateFile, Directory: "/vault/notes"}
	if !HandleAction(noteAction, ActionPorts{
		CreateNote: func(directory string) (string, error) {
			createdAt = directory
			return directory + "/Untitled.md", nil
		},
		Open:       func(path string) { opened = path },
		Invalidate: func() { invalidated++ },
	}) {
		t.Fatal("create note action was not consumed")
	}
	if createdAt != "/vault/notes" || opened != "/vault/notes/Untitled.md" || invalidated != 1 {
		t.Fatalf("create note routing: directory=%q open=%q invalidations=%d", createdAt, opened, invalidated)
	}

	if !HandleAction(Action{Kind: CreateFolder, Directory: "/vault"}, ActionPorts{
		CreateFolder: func(directory string) error { folderAt = directory; return nil },
		Invalidate:   func() { invalidated++ },
	}) {
		t.Fatal("create folder action was not consumed")
	}
	if folderAt != "/vault" || invalidated != 2 {
		t.Fatalf("create folder routing: directory=%q invalidations=%d", folderAt, invalidated)
	}
}

func TestHandleActionReportsApplicationErrors(t *testing.T) {
	wantErr := errors.New("creation failed")
	var gotErr error
	HandleAction(Action{Kind: CreateFile, Directory: "/vault"}, ActionPorts{
		CreateNote: func(string) (string, error) { return "", wantErr },
		OnError:    func(err error) { gotErr = err },
	})
	if !errors.Is(gotErr, wantErr) {
		t.Fatalf("creation error = %v, want %v", gotErr, wantErr)
	}
}

func TestTrashActionCarriesSelectedPathsAsTypedIntent(t *testing.T) {
	controller := NewController()
	controller.SetSelection([]string{"/vault/a.md", "/vault/b.md"})
	action := controller.TrashAction(DragItem{Path: "/vault/a.md", Name: "a.md"})
	if action.Kind != TrashEntries || action.Payload.Path != "/vault/a.md" {
		t.Fatalf("unexpected trash action: %+v", action)
	}
	if !reflect.DeepEqual(action.ItemIDs, []string{"/vault/a.md", "/vault/b.md"}) {
		t.Fatalf("trash selection = %v", action.ItemIDs)
	}

	single := controller.TrashAction(DragItem{Path: "/vault/other.md"})
	if !reflect.DeepEqual(single.ItemIDs, []string{"/vault/other.md"}) {
		t.Fatalf("unselected drag should trash only itself: %v", single.ItemIDs)
	}
}
