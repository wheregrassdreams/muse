package index

import (
	"errors"
	"fmt"
	pathpkg "path"
	"strings"
)

// ErrSkipDir tells a Source.Walk implementation not to visit the descendants
// of the directory passed to its callback.
var ErrSkipDir = errors.New("vault/index: skip directory")

// EntryKind describes a source entry without exposing filesystem metadata.
type EntryKind uint8

const (
	EntryOther EntryKind = iota
	EntryRegular
	EntryDirectory
	EntrySymlink
)

// Entry is a path relative to one vault root. Paths use slash separators on
// every platform. Source implementations must not follow symlinks.
type Entry struct {
	Path string
	Kind EntryKind
}

// Source is the consumer-owned read-only file capability used by the index.
// Walk reports entries in a deterministic order and accepts ErrSkipDir from
// the callback. Stat and Read accept only vault-relative slash paths.
type Source interface {
	Root() string
	Walk(visit func(Entry) error) error
	Stat(path string) (Entry, error)
	Read(path string) ([]byte, error)
}

func validateSource(source Source) error {
	if source == nil {
		return errors.New("vault/index: file source is required")
	}
	if source.Root() == "" {
		return errors.New("vault/index: file source root is empty")
	}
	return nil
}

func validateEntryPath(path string) (string, error) {
	clean := pathpkg.Clean(path)
	if path == "" || strings.Contains(path, "\\") || pathpkg.IsAbs(path) || clean == "" || clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
		return "", fmt.Errorf("vault/index: source returned invalid relative path %q", path)
	}
	return clean, nil
}
