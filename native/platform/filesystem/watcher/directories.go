package watcher

import (
	"os"
	"path/filepath"
)

// DirectoryAccess provides the filesystem queries needed to maintain
// recursive watch coverage. The vault watcher owns the policy for which
// directories to watch; this adapter owns the concrete filesystem calls.
type DirectoryAccess struct{}

func NewDirectoryAccess() DirectoryAccess { return DirectoryAccess{} }

func (DirectoryAccess) IsDirectory(path string) (bool, error) {
	info, err := os.Stat(path)
	if err != nil {
		return false, err
	}
	return info.IsDir(), nil
}

func (DirectoryAccess) WalkDirectories(root string, visit func(path string, walkErr error) error) error {
	return filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return visit(path, walkErr)
		}
		if entry.IsDir() {
			return visit(path, nil)
		}
		return nil
	})
}
