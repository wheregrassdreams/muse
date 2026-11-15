package files

import (
	"errors"
	"fmt"
	pathpkg "path"
	"strings"
)

// ErrSkipDir tells a Source.Walk implementation to omit descendants of the
// directory passed to its callback.
var ErrSkipDir = errors.New("vault/files: skip directory")

// EntryKind describes a catalog source entry without exposing OS metadata.
type EntryKind uint8

const (
	EntryOther EntryKind = iota
	EntryRegular
	EntryDirectory
	EntrySymlink
)

// Entry is a vault-relative path using slash separators on all platforms.
type Entry struct {
	Path string
	Kind EntryKind
}

// Source is the consumer-owned filesystem capability required by Catalog.
// Walk reports entries in deterministic path order and honors ErrSkipDir.
// Stat accepts only normalized vault-relative paths.
type Source interface {
	Root() string
	Walk(visit func(Entry) error) error
	Stat(path string) (Entry, error)
}

func validateSource(source Source) error {
	if source == nil {
		return errors.New("vault/files: catalog source is required")
	}
	if source.Root() == "" {
		return errors.New("vault/files: catalog source root is empty")
	}
	return nil
}

func validateEntryPath(value string) (string, error) {
	clean := pathpkg.Clean(value)
	if value == "" || strings.Contains(value, "\\") || pathpkg.IsAbs(value) || clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
		return "", fmt.Errorf("vault/files: source returned invalid relative path %q", value)
	}
	return clean, nil
}
