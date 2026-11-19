// Package vaultfiles provides the filesystem adapter for vault file operations.
package vaultfiles

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"

	vaultfilesapp "muse/application/vault/files"
)

type FileSystem struct {
	trash func(string) error
}

func New(trash func(string) error) *FileSystem {
	return &FileSystem{trash: trash}
}

func (f *FileSystem) Stat(path string) (fs.FileInfo, error) { return os.Stat(path) }

func (f *FileSystem) Lstat(path string) (fs.FileInfo, error) { return os.Lstat(path) }

func (f *FileSystem) CreateFile(path string, content []byte) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	if _, err := file.Write(content); err != nil {
		_ = file.Close()
		_ = os.Remove(path)
		return err
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(path)
		return err
	}
	return nil
}

func (f *FileSystem) CreateDirectory(path string) error { return os.Mkdir(path, 0o755) }

func (f *FileSystem) Rename(oldPath, newPath string) error { return renameNoReplace(oldPath, newPath) }

func (f *FileSystem) Copy(source, destination string) error {
	info, err := os.Lstat(source)
	if err != nil {
		return err
	}
	if info.Mode()&fs.ModeSymlink != 0 {
		return errors.New("cannot copy a symbolic link")
	}
	if !info.IsDir() {
		return copyFile(source, destination, info.Mode().Perm())
	}
	if err := os.Mkdir(destination, info.Mode().Perm()); err != nil {
		return err
	}
	err = filepath.WalkDir(source, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		target := filepath.Join(destination, rel)
		if rel == "." {
			return nil
		}
		if entry.Type()&fs.ModeSymlink != 0 {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.IsDir() {
			childInfo, err := os.Lstat(path)
			if err != nil {
				return err
			}
			return os.Mkdir(target, childInfo.Mode().Perm())
		}
		childInfo, err := os.Lstat(path)
		if err != nil {
			return err
		}
		if childInfo.Mode()&fs.ModeSymlink != 0 {
			return nil
		}
		return copyFile(path, target, childInfo.Mode().Perm())
	})
	if err != nil {
		_ = os.RemoveAll(destination)
	}
	return err
}

func (f *FileSystem) Trash(path string) error {
	if f == nil || f.trash == nil {
		return errors.New("trash adapter is not configured")
	}
	return f.trash(path)
}

func copyFile(source, destination string, mode fs.FileMode) error {
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()
	output, err := os.OpenFile(destination, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		return err
	}
	_, copyErr := output.ReadFrom(input)
	closeErr := output.Close()
	if copyErr != nil {
		_ = os.Remove(destination)
		return copyErr
	}
	if closeErr != nil {
		_ = os.Remove(destination)
		return closeErr
	}
	return nil
}

var _ vaultfilesapp.FileSystem = (*FileSystem)(nil)
