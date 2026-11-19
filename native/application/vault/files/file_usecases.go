package files

import (
	"errors"
	"path/filepath"
	"strings"

	vaultfiles "muse/vault/files"
)

var ErrFileOperationsUnavailable = errors.New("vault file operations are not configured")

// FileOperationPort is the low-level filesystem capability used by vault file
// use cases. The use cases bind it to one active root and own operation choice.
type FileOperationPort interface {
	Kind(path string) (vaultfiles.EntryKind, error)
	EnsureDirectory(root, path string) error
	CreateFile(root, directory, base, extension string, content []byte) (string, error)
	CreateFolder(root, directory, base string) (string, error)
	Move(root string, paths []string, destination string) ([]FileMove, error)
	Copy(root string, paths []string, destination string) ([]FileCopy, error)
	Import(root string, paths []string, destination string) ([]FileCopy, error)
	Rename(root, source, name string) (FileMove, error)
	MoveTo(root, source, target string) (FileMove, error)
	Trash(root string, paths []string) ([]string, error)
}

var _ FileOperationPort = (*FileOperations)(nil)

type DirectoryLookup func(path string) bool

// FileUseCases binds file mutations to one active vault. App supplies the
// current root and directory lookup when composing the use case, and consumes
// typed results to update selection, caches, and the active editor.
type FileUseCases struct {
	root        string
	operations  FileOperationPort
	isDirectory DirectoryLookup
}

func NewFileUseCases(root string, operations FileOperationPort, isDirectory DirectoryLookup) *FileUseCases {
	return &FileUseCases{root: root, operations: operations, isDirectory: isDirectory}
}

// NormalizeTargets removes selected descendants whose selected ancestor is a
// directory. The returned paths preserve the selection order.
func (u *FileUseCases) NormalizeTargets(paths []string) []string {
	if len(paths) == 0 {
		return nil
	}
	selected := make(map[string]struct{}, len(paths))
	for _, path := range paths {
		selected[path] = struct{}{}
	}
	result := make([]string, 0, len(paths))
	for _, path := range paths {
		covered := false
		for parent := filepath.Dir(path); parent != path; {
			if _, ok := selected[parent]; ok && u.isDirectory != nil && u.isDirectory(parent) {
				covered = true
				break
			}
			next := filepath.Dir(parent)
			if next == parent {
				break
			}
			parent = next
		}
		if !covered {
			result = append(result, path)
		}
	}
	return result
}

// DestinationForPath resolves a file-tree item to the directory that should
// receive a create, paste, or drop operation. An empty item targets the vault.
func (u *FileUseCases) DestinationForPath(path string) string {
	if path == "" {
		return u.root
	}
	if u.isDirectory != nil && u.isDirectory(path) {
		return path
	}
	return filepath.Dir(path)
}

type CreateFileCommand struct {
	Directory string
	Name      string
	Extension string
	Content   []byte
}

type CreateFolderCommand struct {
	Directory string
	Name      string
}

type CreateAttachmentCommand struct {
	Directory string
	Name      string
	Extension string
	Content   []byte
}

type DropIntent uint8

const (
	DropCopy DropIntent = iota
	DropMove
)

type DropFilesCommand struct {
	Sources     []string
	Destination string
	Intent      DropIntent
}

type FileMutation struct {
	Kind    FileMutationKind
	Moves   []FileMove
	Copies  []FileCopy
	Trashed []string
}

type FileMutationKind uint8

const (
	FileMutationNone FileMutationKind = iota
	FileMutationMove
	FileMutationCopy
	FileMutationImport
	FileMutationTrash
)

type PasteFilesCommand struct {
	Sources     []string
	Destination string
	Cut         bool
}

func (u *FileUseCases) CreateFile(command CreateFileCommand) (string, error) {
	if err := u.requireOperations(); err != nil {
		return "", err
	}
	return u.operations.CreateFile(u.root, command.Directory, command.Name, command.Extension, command.Content)
}

func (u *FileUseCases) CreateFolder(command CreateFolderCommand) (string, error) {
	if err := u.requireOperations(); err != nil {
		return "", err
	}
	return u.operations.CreateFolder(u.root, command.Directory, command.Name)
}

func (u *FileUseCases) CreateFolderAt(path string) (string, error) {
	return u.CreateFolder(CreateFolderCommand{Directory: filepath.Dir(path), Name: filepath.Base(path)})
}

func (u *FileUseCases) CreateAttachment(command CreateAttachmentCommand) (string, error) {
	if err := u.requireOperations(); err != nil {
		return "", err
	}
	if err := u.operations.EnsureDirectory(u.root, command.Directory); err != nil {
		return "", err
	}
	return u.CreateFile(CreateFileCommand{
		Directory: command.Directory, Name: command.Name, Extension: command.Extension, Content: command.Content,
	})
}

func (u *FileUseCases) MoveFiles(sources []string, destination string) (FileMutation, error) {
	if err := u.requireOperations(); err != nil {
		return FileMutation{}, err
	}
	moves, err := u.operations.Move(u.root, sources, destination)
	return FileMutation{Kind: FileMutationMove, Moves: moves}, err
}

func (u *FileUseCases) DropFiles(command DropFilesCommand) (FileMutation, error) {
	if err := u.requireOperations(); err != nil {
		return FileMutation{}, err
	}
	if command.Intent == DropMove && len(command.Sources) > 0 && u.OwnsPath(command.Sources[0]) {
		owned := make([]string, 0, len(command.Sources))
		for _, source := range command.Sources {
			if source != "" && u.OwnsPath(source) {
				owned = append(owned, source)
			}
		}
		moves, err := u.operations.Move(u.root, owned, command.Destination)
		return FileMutation{Kind: FileMutationMove, Moves: moves}, err
	}
	copies, err := u.operations.Import(u.root, command.Sources, command.Destination)
	return FileMutation{Kind: FileMutationImport, Copies: copies}, err
}

func (u *FileUseCases) PasteFiles(command PasteFilesCommand) (FileMutation, error) {
	if err := u.requireOperations(); err != nil {
		return FileMutation{}, err
	}
	destination := command.Destination
	if destination == "" {
		destination = u.root
	}
	if u.isDirectory != nil && !u.isDirectory(destination) {
		destination = filepath.Dir(destination)
	}
	if command.Cut {
		moves, err := u.operations.Move(u.root, command.Sources, destination)
		return FileMutation{Kind: FileMutationMove, Moves: moves}, err
	}
	copies, err := u.operations.Copy(u.root, command.Sources, destination)
	return FileMutation{Kind: FileMutationCopy, Copies: copies}, err
}

func (u *FileUseCases) ImportFiles(sources []string, destination string) (FileMutation, error) {
	if err := u.requireOperations(); err != nil {
		return FileMutation{}, err
	}
	copies, err := u.operations.Import(u.root, sources, destination)
	return FileMutation{Kind: FileMutationImport, Copies: copies}, err
}

func (u *FileUseCases) RenameFile(source, name string) (FileMutation, error) {
	if err := u.requireOperations(); err != nil {
		return FileMutation{}, err
	}
	move, err := u.operations.Rename(u.root, source, name)
	mutation := FileMutation{Kind: FileMutationMove}
	if err == nil {
		mutation.Moves = []FileMove{move}
	}
	return mutation, err
}

func (u *FileUseCases) MoveFileTo(source, target string) (FileMutation, error) {
	if err := u.requireOperations(); err != nil {
		return FileMutation{}, err
	}
	move, err := u.operations.MoveTo(u.root, source, target)
	mutation := FileMutation{Kind: FileMutationMove}
	if err == nil {
		mutation.Moves = []FileMove{move}
	}
	return mutation, err
}

func (u *FileUseCases) TrashFiles(paths []string) (FileMutation, error) {
	if err := u.requireOperations(); err != nil {
		return FileMutation{}, err
	}
	trashed, err := u.operations.Trash(u.root, paths)
	return FileMutation{Kind: FileMutationTrash, Trashed: trashed}, err
}

// OwnsPath decides whether a path belongs to this use case's active vault.
func (u *FileUseCases) OwnsPath(path string) bool {
	if u == nil || u.root == "" || path == "" {
		return false
	}
	rel, err := filepath.Rel(filepath.Clean(u.root), filepath.Clean(path))
	return err == nil && rel != ".." && !filepath.IsAbs(rel) && !hasParentPrefix(rel)
}

func hasParentPrefix(path string) bool {
	return path == ".." || strings.HasPrefix(path, ".."+string(filepath.Separator))
}

func (u *FileUseCases) requireOperations() error {
	if u == nil || u.root == "" || u.operations == nil {
		return ErrFileOperationsUnavailable
	}
	return nil
}
