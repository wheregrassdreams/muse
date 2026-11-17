package vault

import (
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"

	vaultfiles "muse/vault/files"
)

// FileSystem is the platform capability required by vault file operations.
// Concrete disk and OS trash access belongs to platform adapters.
type FileSystem interface {
	Lstat(path string) (fs.FileInfo, error)
	CreateFile(path string, content []byte) error
	CreateDirectory(path string) error
	Rename(oldPath, newPath string) error
	Copy(source, destination string) error
	Trash(path string) error
}

type FileMove struct {
	Source string
	Target string
}

type FileCopy struct {
	Source string
	Target string
}

// CreateFile writes a new file under an existing vault directory. A collision
// selects the next numbered name; exclusive creation closes the race between
// checking a candidate and creating it.
func (o *FileOperations) CreateFile(root, directory, base, extension string, content []byte) (string, error) {
	if o == nil || o.fs == nil {
		return "", errors.New("vault file operations are not configured")
	}
	root, err := absolute(root)
	if err != nil {
		return "", err
	}
	directory, err = absolute(directory)
	if err != nil {
		return "", err
	}
	if err := o.validateCreateLocation(root, directory, base); err != nil {
		return "", err
	}
	if extension != "" && (strings.ContainsAny(extension, "/\\\x00") || !strings.HasPrefix(extension, ".") || filepath.Base(extension) != extension) {
		return "", errors.New("file extension must be a single suffix")
	}
	directoryInfo, err := o.fs.Lstat(directory)
	if err != nil {
		return "", err
	}
	if !directoryInfo.IsDir() || directoryInfo.Mode()&fs.ModeSymlink != 0 {
		return "", errors.New("file destination is not a directory")
	}
	for suffix := 1; ; suffix++ {
		name := base
		if suffix > 1 {
			name = fmt.Sprintf("%s %d", base, suffix)
		}
		candidate := filepath.Join(directory, name+extension)
		if err := o.fs.CreateFile(candidate, content); errors.Is(err, fs.ErrExist) {
			continue
		} else if err != nil {
			return "", err
		}
		return candidate, nil
	}
}

// CreateFolder creates a new directory under an existing vault directory,
// choosing the next numbered name when the preferred name already exists.
func (o *FileOperations) CreateFolder(root, directory, base string) (string, error) {
	if o == nil || o.fs == nil {
		return "", errors.New("vault file operations are not configured")
	}
	root, err := absolute(root)
	if err != nil {
		return "", err
	}
	directory, err = absolute(directory)
	if err != nil {
		return "", err
	}
	if err := o.validateCreateLocation(root, directory, base); err != nil {
		return "", err
	}
	directoryInfo, err := o.fs.Lstat(directory)
	if err != nil {
		return "", err
	}
	if !directoryInfo.IsDir() || directoryInfo.Mode()&fs.ModeSymlink != 0 {
		return "", errors.New("folder destination is not a directory")
	}
	for suffix := 1; ; suffix++ {
		name := base
		if suffix > 1 {
			name = fmt.Sprintf("%s %d", base, suffix)
		}
		candidate := filepath.Join(directory, name)
		if err := o.fs.CreateDirectory(candidate); errors.Is(err, fs.ErrExist) {
			continue
		} else if err != nil {
			return "", err
		}
		return candidate, nil
	}
}

// EnsureDirectory creates one vault directory when missing. Existing regular
// directories are accepted; symlinks and non-directory collisions are not.
func (o *FileOperations) EnsureDirectory(root, path string) error {
	if o == nil || o.fs == nil {
		return errors.New("vault file operations are not configured")
	}
	root, err := absolute(root)
	if err != nil {
		return err
	}
	path, err = absolute(path)
	if err != nil {
		return err
	}
	if !withinRoot(root, path) {
		return fmt.Errorf("directory is outside the vault: %s", path)
	}
	if err := o.ensureNoSymlinkPath(root, path); err != nil {
		return err
	}
	if info, err := o.fs.Lstat(path); err == nil {
		if !info.IsDir() || info.Mode()&fs.ModeSymlink != 0 {
			return fmt.Errorf("path exists and is not a directory: %s", path)
		}
		return nil
	} else if !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	parentInfo, err := o.fs.Lstat(filepath.Dir(path))
	if err != nil {
		return err
	}
	if !parentInfo.IsDir() || parentInfo.Mode()&fs.ModeSymlink != 0 {
		return fmt.Errorf("directory parent is not a directory: %s", filepath.Dir(path))
	}
	if err := o.fs.CreateDirectory(path); err != nil {
		if !errors.Is(err, fs.ErrExist) {
			return err
		}
		info, statErr := o.fs.Lstat(path)
		if statErr != nil {
			return statErr
		}
		if !info.IsDir() || info.Mode()&fs.ModeSymlink != 0 {
			return fmt.Errorf("path exists and is not a directory: %s", path)
		}
	}
	return nil
}

func (o *FileOperations) validateCreateLocation(root, directory, base string) error {
	if strings.TrimSpace(base) == "" || base == "." || base == ".." || strings.ContainsAny(base, "/\\\x00") || filepath.Base(base) != base {
		return errors.New("creation name must be a single file or folder name")
	}
	if !withinRoot(root, directory) {
		return fmt.Errorf("creation destination is outside the vault: %s", directory)
	}
	if err := o.ensureNoSymlinkPath(root, directory); err != nil {
		return err
	}
	return nil
}

// FileOperations applies vault boundaries, collision handling, and copy/trash
// policy. Presentation code only consumes the resulting paths and errors.
type FileOperations struct{ fs FileSystem }

func NewFileOperations(filesystem FileSystem) *FileOperations {
	return &FileOperations{fs: filesystem}
}

// Kind returns a backend-neutral type for a path, including paths outside the
// active vault (for example, an operating-system file drop). It does not
// follow a final symlink.
func (o *FileOperations) Kind(path string) (vaultfiles.EntryKind, error) {
	if o == nil || o.fs == nil {
		return vaultfiles.EntryOther, errors.New("vault file operations are not configured")
	}
	info, err := o.fs.Lstat(path)
	if err != nil {
		return vaultfiles.EntryOther, err
	}
	switch {
	case info.Mode()&fs.ModeSymlink != 0:
		return vaultfiles.EntrySymlink, nil
	case info.IsDir():
		return vaultfiles.EntryDirectory, nil
	case info.Mode().IsRegular():
		return vaultfiles.EntryRegular, nil
	default:
		return vaultfiles.EntryOther, nil
	}
}

// Move moves valid non-symlink vault entries into a vault directory. Completed
// moves are returned even when a later entry fails so callers can remap their
// UI state before reporting the error.
func (o *FileOperations) Move(root string, paths []string, destination string) ([]FileMove, error) {
	if o == nil || o.fs == nil {
		return nil, errors.New("vault file operations are not configured")
	}
	root, err := absolute(root)
	if err != nil {
		return nil, err
	}
	destination, err = absolute(destination)
	if err != nil {
		return nil, err
	}
	if !withinRoot(root, destination) {
		return nil, fmt.Errorf("move destination is outside the vault: %s", destination)
	}
	if err := o.ensureNoSymlinkPath(root, destination); err != nil {
		return nil, err
	}
	destinationInfo, err := o.fs.Lstat(destination)
	if err != nil {
		return nil, err
	}
	if !destinationInfo.IsDir() || destinationInfo.Mode()&fs.ModeSymlink != 0 {
		return nil, errors.New("move destination is not a directory")
	}
	moves := make([]FileMove, 0, len(paths))
	for _, source := range paths {
		if source == "" {
			continue
		}
		source, err = absolute(source)
		if err != nil {
			return moves, err
		}
		if !withinRoot(root, source) {
			return moves, fmt.Errorf("source is outside the vault: %s", source)
		}
		if err := o.ensureNoSymlinkParents(root, source); err != nil {
			return moves, err
		}
		info, err := o.fs.Lstat(source)
		if err != nil {
			return moves, err
		}
		if info.Mode()&fs.ModeSymlink != 0 || source == destination || filepath.Dir(source) == destination || isDescendant(destination, source) {
			continue
		}
		target, err := o.availableTarget(destination, filepath.Base(source))
		if err != nil {
			return moves, err
		}
		if err := o.fs.Rename(source, target); err != nil {
			return moves, err
		}
		moves = append(moves, FileMove{Source: source, Target: target})
	}
	return moves, nil
}

// Copy duplicates internal vault entries without overwriting existing names.
// Symlinks and stale clipboard entries are skipped.
func (o *FileOperations) Copy(root string, paths []string, destination string) ([]FileCopy, error) {
	return o.copy(root, paths, destination, false)
}

// Import duplicates external files into a vault directory. Invalid, missing,
// and symlink top-level entries are ignored, matching desktop drop behavior.
func (o *FileOperations) Import(root string, paths []string, destination string) ([]FileCopy, error) {
	return o.copy(root, paths, destination, true)
}

func (o *FileOperations) copy(root string, paths []string, destination string, ignoreInvalid bool) ([]FileCopy, error) {
	if o == nil || o.fs == nil {
		return nil, errors.New("vault file operations are not configured")
	}
	root, err := absolute(root)
	if err != nil {
		return nil, err
	}
	destination, err = absolute(destination)
	if err != nil {
		return nil, err
	}
	if !withinRoot(root, destination) {
		return nil, fmt.Errorf("copy destination is outside the vault: %s", destination)
	}
	if err := o.ensureNoSymlinkPath(root, destination); err != nil {
		return nil, err
	}
	destinationInfo, err := o.fs.Lstat(destination)
	if err != nil {
		return nil, err
	}
	if !destinationInfo.IsDir() || destinationInfo.Mode()&fs.ModeSymlink != 0 {
		return nil, errors.New("copy destination is not a directory")
	}
	copies := make([]FileCopy, 0, len(paths))
	for _, source := range paths {
		if source == "" {
			continue
		}
		source, err = absolute(source)
		if err != nil {
			if ignoreInvalid {
				continue
			}
			return copies, err
		}
		if !ignoreInvalid && !withinRoot(root, source) {
			return copies, fmt.Errorf("source is outside the vault: %s", source)
		}
		if !ignoreInvalid {
			if err := o.ensureNoSymlinkParents(root, source); err != nil {
				return copies, err
			}
		}
		info, err := o.fs.Lstat(source)
		if err != nil {
			if ignoreInvalid || errors.Is(err, fs.ErrNotExist) {
				continue
			}
			return copies, err
		}
		if info.Mode()&fs.ModeSymlink != 0 || source == destination || (info.IsDir() && isDescendant(destination, source)) {
			continue
		}
		target, err := o.availableTarget(destination, filepath.Base(source))
		if err != nil {
			return copies, err
		}
		if err := o.fs.Copy(source, target); err != nil {
			return copies, err
		}
		copies = append(copies, FileCopy{Source: source, Target: target})
	}
	return copies, nil
}

// Rename renames one vault entry without replacing an existing sibling.
func (o *FileOperations) Rename(root, source, name string) (FileMove, error) {
	if o == nil || o.fs == nil {
		return FileMove{}, errors.New("vault file operations are not configured")
	}
	root, err := absolute(root)
	if err != nil {
		return FileMove{}, err
	}
	source, err = absolute(source)
	if err != nil {
		return FileMove{}, err
	}
	name = strings.TrimSpace(name)
	if name == "" || name == "." || name == ".." || filepath.Base(name) != name || strings.ContainsAny(name, `/\\`) {
		return FileMove{}, errors.New("rename requires a single file or folder name")
	}
	if !withinRoot(root, source) {
		return FileMove{}, fmt.Errorf("source is outside the vault: %s", source)
	}
	if err := o.ensureNoSymlinkParents(root, source); err != nil {
		return FileMove{}, err
	}
	info, err := o.fs.Lstat(source)
	if err != nil {
		return FileMove{}, err
	}
	if info.Mode()&fs.ModeSymlink != 0 {
		return FileMove{}, errors.New("cannot rename a symbolic link")
	}
	target := filepath.Join(filepath.Dir(source), name)
	return o.MoveTo(root, source, target)
}

// MoveTo moves an entry to an exact vault-relative target path. Unlike Move,
// it rejects collisions instead of generating a numbered name; this is used
// by explicit rename tools that must not silently choose a different path.
func (o *FileOperations) MoveTo(root, source, target string) (FileMove, error) {
	if o == nil || o.fs == nil {
		return FileMove{}, errors.New("vault file operations are not configured")
	}
	root, err := absolute(root)
	if err != nil {
		return FileMove{}, err
	}
	source, err = absolute(source)
	if err != nil {
		return FileMove{}, err
	}
	target, err = absolute(target)
	if err != nil {
		return FileMove{}, err
	}
	if !withinRoot(root, source) || !withinRoot(root, target) {
		return FileMove{}, errors.New("move source and target must stay inside the vault")
	}
	if err := o.ensureNoSymlinkParents(root, source); err != nil {
		return FileMove{}, err
	}
	if err := o.ensureNoSymlinkPath(root, filepath.Dir(target)); err != nil {
		return FileMove{}, err
	}
	if source == target {
		return FileMove{Source: source, Target: target}, nil
	}
	info, err := o.fs.Lstat(source)
	if err != nil {
		return FileMove{}, err
	}
	if info.Mode()&fs.ModeSymlink != 0 {
		return FileMove{}, errors.New("cannot move a symbolic link")
	}
	parent := filepath.Dir(target)
	parentInfo, err := o.fs.Lstat(parent)
	if err != nil {
		return FileMove{}, err
	}
	if !parentInfo.IsDir() || parentInfo.Mode()&fs.ModeSymlink != 0 {
		return FileMove{}, errors.New("move target parent is not a directory")
	}
	if info.IsDir() && isDescendant(parent, source) {
		return FileMove{}, errors.New("cannot move a directory into itself")
	}
	if _, err := o.fs.Lstat(target); err == nil {
		return FileMove{}, fmt.Errorf("move target already exists: %s", target)
	} else if !errors.Is(err, fs.ErrNotExist) {
		return FileMove{}, err
	}
	if err := o.fs.Rename(source, target); err != nil {
		return FileMove{}, err
	}
	return FileMove{Source: source, Target: target}, nil
}

// Trash moves vault entries to the operating system's recoverable trash.
// Successfully trashed paths are returned if a later item fails.
func (o *FileOperations) Trash(root string, paths []string) ([]string, error) {
	if o == nil || o.fs == nil {
		return nil, errors.New("vault file operations are not configured")
	}
	root, err := absolute(root)
	if err != nil {
		return nil, err
	}
	trashed := make([]string, 0, len(paths))
	for _, path := range paths {
		if path == "" {
			continue
		}
		path, err = absolute(path)
		if err != nil {
			return trashed, err
		}
		if !withinRoot(root, path) {
			return trashed, fmt.Errorf("source is outside the vault: %s", path)
		}
		if err := o.ensureNoSymlinkParents(root, path); err != nil {
			return trashed, err
		}
		if _, err := o.fs.Lstat(path); err != nil {
			return trashed, err
		}
		if err := o.fs.Trash(path); err != nil {
			return trashed, err
		}
		trashed = append(trashed, path)
	}
	return trashed, nil
}

func (o *FileOperations) availableTarget(directory, name string) (string, error) {
	target := filepath.Join(directory, name)
	if _, err := o.fs.Lstat(target); errors.Is(err, fs.ErrNotExist) {
		return target, nil
	} else if err != nil {
		return "", err
	}
	ext := filepath.Ext(name)
	base := strings.TrimSuffix(name, ext)
	for suffix := 2; ; suffix++ {
		target = filepath.Join(directory, fmt.Sprintf("%s %d%s", base, suffix, ext))
		if _, err := o.fs.Lstat(target); errors.Is(err, fs.ErrNotExist) {
			return target, nil
		} else if err != nil {
			return "", err
		}
	}
}

func (o *FileOperations) ensureNoSymlinkPath(root, path string) error {
	root, err := absolute(root)
	if err != nil {
		return err
	}
	path, err = absolute(path)
	if err != nil {
		return err
	}
	if !withinRoot(root, path) {
		return fmt.Errorf("path is outside the vault: %s", path)
	}
	if path == root {
		return nil
	}
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return err
	}
	current := root
	for _, component := range strings.Split(rel, string(filepath.Separator)) {
		current = filepath.Join(current, component)
		info, err := o.fs.Lstat(current)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return nil
			}
			return err
		}
		if info.Mode()&fs.ModeSymlink != 0 {
			return fmt.Errorf("path crosses a symbolic link: %s", current)
		}
	}
	return nil
}

func (o *FileOperations) ensureNoSymlinkParents(root, path string) error {
	root, err := absolute(root)
	if err != nil {
		return err
	}
	path, err = absolute(path)
	if err != nil {
		return err
	}
	if !withinRoot(root, path) {
		return fmt.Errorf("path is outside the vault: %s", path)
	}
	parent := filepath.Dir(path)
	return o.ensureNoSymlinkPath(root, parent)
}

func absolute(path string) (string, error) {
	if path == "" {
		return "", errors.New("path is required")
	}
	return filepath.Abs(path)
}

func withinRoot(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	return err == nil && rel != ".." && !filepath.IsAbs(rel) && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func isDescendant(path, parent string) bool {
	rel, err := filepath.Rel(parent, path)
	return err == nil && rel != "." && rel != ".." && !filepath.IsAbs(rel) && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
