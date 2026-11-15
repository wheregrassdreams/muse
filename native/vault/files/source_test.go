package files

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type testSource struct{ root string }

func catalogForTest(t testing.TB, root string) *Catalog {
	t.Helper()
	absolute, err := filepath.Abs(root)
	if err != nil {
		t.Fatal(err)
	}
	catalog := New(&testSource{root: absolute})
	if err := catalog.Rebuild(); err != nil {
		t.Fatal(err)
	}
	return catalog
}

func (s *testSource) Root() string { return s.root }

func (s *testSource) Walk(visit func(Entry) error) error {
	return filepath.WalkDir(s.root, func(abs string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(s.root, abs)
		if err != nil || rel == "." {
			return err
		}
		kind := EntryOther
		switch {
		case entry.Type()&fs.ModeSymlink != 0:
			kind = EntrySymlink
		case entry.IsDir():
			kind = EntryDirectory
		case entry.Type().IsRegular():
			kind = EntryRegular
		}
		projected := Entry{Path: filepath.ToSlash(rel), Kind: kind}
		if err := visit(projected); err != nil {
			if errors.Is(err, ErrSkipDir) && kind == EntryDirectory {
				return filepath.SkipDir
			}
			return err
		}
		return nil
	})
}

func (s *testSource) Stat(path string) (Entry, error) {
	if path == "" || filepath.IsAbs(path) {
		return Entry{}, errors.New("expected relative path")
	}
	rel := filepath.Clean(filepath.FromSlash(path))
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return Entry{}, errors.New("path escapes test source")
	}
	current := s.root
	parts := strings.Split(filepath.ToSlash(rel), "/")
	for n, part := range parts {
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if err != nil {
			return Entry{}, err
		}
		if n < len(parts)-1 && info.Mode()&fs.ModeSymlink != 0 {
			return Entry{}, errors.New("path traverses symlink")
		}
		if n == len(parts)-1 {
			kind := EntryOther
			switch {
			case info.Mode()&fs.ModeSymlink != 0:
				kind = EntrySymlink
			case info.IsDir():
				kind = EntryDirectory
			case info.Mode().IsRegular():
				kind = EntryRegular
			}
			return Entry{Path: filepath.ToSlash(rel), Kind: kind}, nil
		}
	}
	return Entry{}, fs.ErrNotExist
}
