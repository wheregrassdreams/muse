// Package atomicfile writes files through a synced temporary file in the
// destination directory, then atomically renames it into place.
package atomicfile

import (
	"io/fs"
	"os"
	"path/filepath"
)

// Stage identifies which step of an atomic write failed. It lets adapters
// retain their established error context without wrapping the underlying error.
type Stage string

const (
	CreateTemp Stage = "create temporary file"
	ChmodTemp  Stage = "set temporary file mode"
	WriteTemp  Stage = "write temporary file"
	SyncTemp   Stage = "sync temporary file"
	CloseTemp  Stage = "close temporary file"
	RenameFile Stage = "replace file"
)

// Write stores data at path using a temporary file matching pattern. The
// temporary file is created beside path to keep the final rename on one volume.
// The caller is responsible for creating the destination directory.
// It returns the failed stage alongside the original error, unwrapped.
func Write(path, pattern string, data []byte, perm fs.FileMode) (Stage, error) {
	tmp, err := os.CreateTemp(filepath.Dir(path), pattern)
	if err != nil {
		return CreateTemp, err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if err := tmp.Chmod(perm); err != nil {
		_ = tmp.Close()
		return ChmodTemp, err
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return WriteTemp, err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return SyncTemp, err
	}
	if err := tmp.Close(); err != nil {
		return CloseTemp, err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return RenameFile, err
	}
	return "", nil
}
