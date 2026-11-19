package nativeui

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"
	vaultfileapp "muse/application/vault/files"
	"muse/locales"
	explorer "muse/nativeui/workbench/features/explorer"
	"muse/platform/filesystem/documentstore"
	"muse/platform/filesystem/temporaryimages"
	"muse/platform/filesystem/vaultfiles"
	"strings"
)

func TestEditorOptionsKeepProductionCapabilitiesAndFrameInsets(t *testing.T) {
	root := t.TempDir()
	note := filepath.Join(root, "note.md")
	target := filepath.Join(root, "target.md")
	for path, source := range map[string]string{note: "", target: "# Dropped"} {
		if err := os.WriteFile(path, []byte(source), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	store := documentstore.New(filepath.Join(t.TempDir(), "recovery"))
	a := NewWithDependencies(Dependencies{
		Documents: store, Recovery: store, History: store,
		FileOperations:  vaultfileapp.NewFileOperations(vaultfiles.New(func(string) error { return nil })),
		TemporaryImages: temporaryimages.NewAt(filepath.Join(t.TempDir(), "temporary-images")),
	})
	a.root = root
	a.requestOpen(note)
	if a.documentPath() != note {
		t.Fatalf("initial note path = %q, want %q", a.documentPath(), note)
	}
	tt := ui.NewTester(a.View, 1200, 800)
	t.Cleanup(func() { a.windows.CloseAll() })

	tt.SetTitleBar(ui.TitleBar{Height: 73})
	tt.Frame()
	opts := a.editorOptions()
	if opts.Placeholder != locales.DocumentStartWriting() || opts.Label != locales.DocumentDefaultLabel() || !opts.PreserveTypedLineBreaks {
		t.Fatalf("editor defaults = placeholder %q label %q preserve-breaks %v", opts.Placeholder, opts.Label, opts.PreserveTypedLineBreaks)
	}
	if got := a.noteEditor().PresentationCommands().Settings().TopInset; got != 73 {
		t.Fatalf("flat editor top inset = %.1f, want current titlebar height 73", got)
	}
	if opts.SaveImage == nil || opts.OnDropFiles == nil || opts.OnVaultItemDragOver == nil {
		t.Fatalf("production editor callbacks missing: save=%v drop=%v drag-over=%v", opts.SaveImage != nil, opts.OnDropFiles != nil, opts.OnVaultItemDragOver != nil)
	}
	imagePath, err := opts.SaveImage([]byte("image bytes"), ".png")
	if err != nil {
		t.Fatal(err)
	}
	imageFile := filepath.Join(root, filepath.FromSlash(imagePath))
	image, err := os.ReadFile(imageFile)
	if err != nil || string(image) != "image bytes" || !strings.HasPrefix(filepath.ToSlash(imagePath), "assets/") {
		t.Fatalf("saved editor image = %q, bytes=%q err=%v", imagePath, image, err)
	}
	opts.OnVaultItemDragOver()
	if got := a.fileTreeState.DragOperationDisplay().Kind; got != explorer.DragOperationInsertIntoEditor {
		t.Fatalf("tree drag-over operation = %v", got)
	}
	opts.OnDropFiles([]string{imageFile, target})
	if a.documentPath() != target {
		t.Fatalf("dropped editable document path = %q, want %q", a.documentPath(), target)
	}
	tt.Frame()

	temporaryRoot := filepath.Join(t.TempDir(), "temporary-images")
	temporaryApp := NewWithDependencies(Dependencies{TemporaryImages: temporaryimages.NewAt(temporaryRoot)})
	t.Cleanup(func() { temporaryApp.windows.CloseAll() })
	temporaryPath, err := temporaryApp.editorOptions().SaveImage([]byte("temporary"), ".png")
	if err != nil {
		t.Fatal(err)
	}
	if !filepath.IsAbs(temporaryPath) {
		t.Fatalf("unsaved editor image path = %q, want absolute temporary path", temporaryPath)
	}
	if data, err := os.ReadFile(filepath.FromSlash(temporaryPath)); err != nil || string(data) != "temporary" {
		t.Fatalf("temporary image bytes = %q, err=%v", data, err)
	}
}
