package files

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	vaultfiles "muse/vault/files"
)

type localFileSystem struct {
	trashed            []string
	trashErr           error
	trashErrAt         string
	createFileErr      error
	createDirectoryErr error
}

func (localFileSystem) Lstat(path string) (fs.FileInfo, error) { return os.Lstat(path) }
func (localFileSystem) Rename(oldPath, newPath string) error   { return os.Rename(oldPath, newPath) }
func (f *localFileSystem) CreateFile(path string, content []byte) error {
	if f.createFileErr != nil {
		return f.createFileErr
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	if _, err := file.Write(content); err != nil {
		_ = file.Close()
		_ = os.Remove(path)
		return err
	}
	return file.Close()
}
func (f *localFileSystem) CreateDirectory(path string) error {
	if f.createDirectoryErr != nil {
		return f.createDirectoryErr
	}
	return os.Mkdir(path, 0o755)
}
func (f *localFileSystem) Trash(path string) error {
	if f.trashErr != nil && (f.trashErrAt == "" || filepath.Base(path) == f.trashErrAt) {
		return f.trashErr
	}
	f.trashed = append(f.trashed, path)
	return os.RemoveAll(path)
}

func TestFileOperationsCreateFileAndFolderChooseCollisionFreeVaultPaths(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "Untitled.md"), []byte("preserve"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "New folder"), 0o755); err != nil {
		t.Fatal(err)
	}
	ops := NewFileOperations(&localFileSystem{})
	createdFile, err := ops.CreateFile(root, root, "Untitled", ".md", []byte("new note"))
	if err != nil || createdFile != filepath.Join(root, "Untitled 2.md") {
		t.Fatalf("CreateFile = %q, %v", createdFile, err)
	}
	if body, err := os.ReadFile(filepath.Join(root, "Untitled.md")); err != nil || string(body) != "preserve" {
		t.Fatalf("collision changed existing file: body=%q err=%v", body, err)
	}
	if body, err := os.ReadFile(createdFile); err != nil || string(body) != "new note" {
		t.Fatalf("created file body=%q err=%v", body, err)
	}
	createdFolder, err := ops.CreateFolder(root, root, "New folder")
	if err != nil || createdFolder != filepath.Join(root, "New folder 2") {
		t.Fatalf("CreateFolder = %q, %v", createdFolder, err)
	}
}

func TestFileOperationsCreationRejectsInvalidAndOutsideVaultDestinations(t *testing.T) {
	root, outside := t.TempDir(), t.TempDir()
	ops := NewFileOperations(&localFileSystem{})
	for _, name := range []string{"", ".", "..", "../escape", "nested/name"} {
		if _, err := ops.CreateFile(root, root, name, ".md", nil); err == nil {
			t.Errorf("CreateFile accepted invalid name %q", name)
		}
		if _, err := ops.CreateFolder(root, root, name); err == nil {
			t.Errorf("CreateFolder accepted invalid name %q", name)
		}
	}
	if _, err := ops.CreateFile(root, outside, "note", ".md", nil); err == nil || !strings.Contains(err.Error(), "outside the vault") {
		t.Fatalf("outside-vault file destination error = %v", err)
	}
	if _, err := ops.CreateFolder(root, outside, "folder"); err == nil || !strings.Contains(err.Error(), "outside the vault") {
		t.Fatalf("outside-vault folder destination error = %v", err)
	}
	linked := filepath.Join(root, "linked")
	if err := os.Symlink(outside, linked); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	if _, err := ops.CreateFile(root, linked, "note", ".md", nil); err == nil {
		t.Fatal("CreateFile accepted a symlink destination")
	}
	if _, err := ops.CreateFolder(root, linked, "folder"); err == nil {
		t.Fatal("CreateFolder accepted a symlink destination")
	}
}

func TestFileOperationsCreationPropagatesPermissionErrors(t *testing.T) {
	root := t.TempDir()
	fileOps := NewFileOperations(&localFileSystem{createFileErr: fs.ErrPermission})
	if _, err := fileOps.CreateFile(root, root, "note", ".md", nil); !errors.Is(err, fs.ErrPermission) {
		t.Fatalf("CreateFile permission error = %v", err)
	}
	folderOps := NewFileOperations(&localFileSystem{createDirectoryErr: fs.ErrPermission})
	if _, err := folderOps.CreateFolder(root, root, "folder"); !errors.Is(err, fs.ErrPermission) {
		t.Fatalf("CreateFolder permission error = %v", err)
	}
}
func (localFileSystem) Copy(source, destination string) error {
	info, err := os.Lstat(source)
	if err != nil {
		return err
	}
	if info.Mode()&fs.ModeSymlink != 0 {
		return errors.New("symlink")
	}
	if !info.IsDir() {
		body, err := os.ReadFile(source)
		if err != nil {
			return err
		}
		return os.WriteFile(destination, body, info.Mode().Perm())
	}
	return filepath.WalkDir(source, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		target := filepath.Join(destination, rel)
		if rel == "." || entry.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		if entry.Type()&fs.ModeSymlink != 0 {
			return nil
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, body, 0o644)
	})
}

func TestFileOperationsKindReturnsTypedPathInformation(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "note.md")
	if err := os.WriteFile(file, []byte("note"), 0o600); err != nil {
		t.Fatal(err)
	}
	ops := NewFileOperations(&localFileSystem{})
	for _, tc := range []struct {
		path string
		want vaultfiles.EntryKind
	}{{root, vaultfiles.EntryDirectory}, {file, vaultfiles.EntryRegular}} {
		got, err := ops.Kind(tc.path)
		if err != nil || got != tc.want {
			t.Errorf("Kind(%q) = %v, %v; want %v", tc.path, got, err, tc.want)
		}
	}
	link := filepath.Join(root, "linked")
	if err := os.Symlink(root, link); err == nil {
		got, err := ops.Kind(link)
		if err != nil || got != vaultfiles.EntrySymlink {
			t.Errorf("Kind(symlink) = %v, %v; want symlink", got, err)
		}
	}
}

func TestFileOperationsMoveCollisionAndRejectsOutsideRoot(t *testing.T) {
	root := t.TempDir()
	destination := filepath.Join(root, "Destination")
	if err := os.Mkdir(destination, 0o755); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(root, "note.md")
	if err := os.WriteFile(source, []byte("source"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(destination, "note.md"), []byte("existing"), 0o644); err != nil {
		t.Fatal(err)
	}
	ops := NewFileOperations(&localFileSystem{})
	moves, err := ops.Move(root, []string{source}, destination)
	if err != nil || len(moves) != 1 || moves[0].Target != filepath.Join(destination, "note 2.md") {
		t.Fatalf("Move = %+v, %v", moves, err)
	}
	if _, err := ops.Move(root, []string{filepath.Join(t.TempDir(), "outside.md")}, destination); err == nil || !strings.Contains(err.Error(), "outside the vault") {
		t.Fatalf("outside-vault move error = %v", err)
	}
}

func TestFileOperationsMoveSkipsSymlinksAndSelfDescendant(t *testing.T) {
	root := t.TempDir()
	sourceDir := filepath.Join(root, "Folder")
	if err := os.Mkdir(sourceDir, 0o755); err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(sourceDir, "Child")
	if err := os.Mkdir(destination, 0o755); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(root, "real.md")
	if err := os.WriteFile(file, []byte("real"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "link.md")
	if err := os.Symlink(file, link); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	moves, err := NewFileOperations(&localFileSystem{}).Move(root, []string{sourceDir, link}, destination)
	if err != nil || len(moves) != 0 {
		t.Fatalf("self-descendant or symlink move = %+v, %v", moves, err)
	}
}

func TestFileOperationsRejectsVaultParentSymlinks(t *testing.T) {
	root, outside := t.TempDir(), t.TempDir()
	link := filepath.Join(root, "linked")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	inside := filepath.Join(root, "note.md")
	if err := os.WriteFile(inside, []byte("inside"), 0o644); err != nil {
		t.Fatal(err)
	}
	external := filepath.Join(outside, "external.md")
	if err := os.WriteFile(external, []byte("outside"), 0o644); err != nil {
		t.Fatal(err)
	}
	throughLink := filepath.Join(link, "external.md")
	ops := NewFileOperations(&localFileSystem{})
	if _, err := ops.Move(root, []string{inside}, link); err == nil {
		t.Fatal("Move accepted a destination beneath a parent symlink")
	}
	if _, err := ops.Copy(root, []string{throughLink}, root); err == nil {
		t.Fatal("Copy accepted a source beneath a parent symlink")
	}
	if _, err := ops.Import(root, []string{external}, link); err == nil {
		t.Fatal("Import accepted a destination beneath a parent symlink")
	}
	if _, err := ops.MoveTo(root, inside, throughLink); err == nil {
		t.Fatal("MoveTo accepted a target beneath a parent symlink")
	}
	if _, err := ops.Trash(root, []string{throughLink}); err == nil {
		t.Fatal("Trash accepted a source beneath a parent symlink")
	}
}

func TestFileOperationsCopyAndImportDoNotOverwrite(t *testing.T) {
	root := t.TempDir()
	destination := filepath.Join(root, "vault")
	if err := os.Mkdir(destination, 0o755); err != nil {
		t.Fatal(err)
	}
	sourceDir := filepath.Join(root, "external")
	if err := os.Mkdir(sourceDir, 0o755); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(sourceDir, "note.md")
	if err := os.WriteFile(source, []byte("incoming"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(destination, "note.md"), []byte("existing"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(sourceDir, "linked.md")
	if err := os.Symlink(source, link); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	ops := NewFileOperations(&localFileSystem{})
	copies, err := ops.Import(destination, []string{source, link, filepath.Join(sourceDir, "missing")}, destination)
	if err != nil || len(copies) != 1 || copies[0].Target != filepath.Join(destination, "note 2.md") {
		t.Fatalf("Import = %+v, %v", copies, err)
	}
	body, err := os.ReadFile(copies[0].Target)
	if err != nil || string(body) != "incoming" {
		t.Fatalf("imported body = %q, %v", body, err)
	}
	copies, err = ops.Copy(destination, []string{copies[0].Target}, destination)
	if err != nil || len(copies) != 1 || copies[0].Target != filepath.Join(destination, "note 2 2.md") {
		t.Fatalf("Copy = %+v, %v", copies, err)
	}
	if _, err := ops.Copy(destination, []string{source}, destination); err == nil || !strings.Contains(err.Error(), "outside the vault") {
		t.Fatalf("outside-vault source copy error = %v", err)
	}
	if _, err := ops.Copy(destination, []string{copies[0].Target}, sourceDir); err == nil || !strings.Contains(err.Error(), "destination is outside") {
		t.Fatalf("outside-vault destination copy error = %v", err)
	}
	if _, err := ops.Import(destination, []string{source}, sourceDir); err == nil || !strings.Contains(err.Error(), "destination is outside") {
		t.Fatalf("outside-vault import destination error = %v", err)
	}
	linkDestination := filepath.Join(destination, "linked-destination")
	if err := os.Symlink(sourceDir, linkDestination); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	if _, err := ops.Import(destination, []string{source}, linkDestination); err == nil {
		t.Fatal("import accepted symlink destination")
	}
}

func TestFileOperationsRenameValidatesNameAndNeverOverwrites(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "old.md")
	existing := filepath.Join(root, "taken.md")
	for _, path := range []string{source, existing} {
		if err := os.WriteFile(path, []byte(filepath.Base(path)), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	ops := NewFileOperations(&localFileSystem{})
	if _, err := ops.Rename(root, source, "../escape.md"); err == nil {
		t.Fatal("accepted traversal name")
	}
	if _, err := ops.Rename(root, source, "taken.md"); err == nil {
		t.Fatal("overwrote existing sibling")
	}
	if _, err := ops.Rename(root, existing, "../escape.md"); err == nil {
		t.Fatal("rename accepted a target outside the vault")
	}
	moved, err := ops.Rename(root, source, "new.md")
	if err != nil || moved.Target != filepath.Join(root, "new.md") {
		t.Fatalf("Rename = %+v, %v", moved, err)
	}
}

func TestFileOperationsTrashReturnsPartialSuccess(t *testing.T) {
	root := t.TempDir()
	first, second := filepath.Join(root, "first.md"), filepath.Join(root, "second.md")
	for _, path := range []string{first, second} {
		if err := os.WriteFile(path, nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	filesystem := &localFileSystem{trashErr: errors.New("trash failed"), trashErrAt: filepath.Base(second)}
	trashed, err := NewFileOperations(filesystem).Trash(root, []string{first, second})
	if err == nil || len(trashed) != 1 || trashed[0] != first || len(filesystem.trashed) != 1 {
		t.Fatalf("Trash = %v, %v; calls=%v", trashed, err, filesystem.trashed)
	}
	filesystem.trashErr = nil
	trashed, err = NewFileOperations(filesystem).Trash(root, []string{second})
	if err != nil || len(trashed) != 1 || trashed[0] != second {
		t.Fatalf("Trash success = %v, %v", trashed, err)
	}
}
