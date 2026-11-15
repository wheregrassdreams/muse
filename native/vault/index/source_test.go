package index

import (
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

func testIndex(t testing.TB, root string) *Index {
	t.Helper()
	source, err := newTestSource(root)
	if err != nil {
		t.Fatal(err)
	}
	idx := New(source)
	if err := idx.Rebuild(); err != nil {
		t.Fatal(err)
	}
	return idx
}

type testSource struct {
	root    string
	entries []Entry
	files   map[string][]byte
}

func newTestSource(root string) (*testSource, error) {
	source := &testSource{root: root, files: make(map[string][]byte)}
	err := filepath.WalkDir(root, func(abs string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, abs)
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
		rel = filepath.ToSlash(rel)
		source.entries = append(source.entries, Entry{Path: rel, Kind: kind})
		if kind == EntryRegular {
			source.files[rel], err = os.ReadFile(abs)
		}
		return err
	})
	sort.Slice(source.entries, func(i, j int) bool { return source.entries[i].Path < source.entries[j].Path })
	return source, err
}

func (s *testSource) Root() string { return s.root }

func (s *testSource) Walk(visit func(Entry) error) error {
	skipped := make([]string, 0)
	for _, entry := range s.entries {
		ignore := false
		for _, dir := range skipped {
			if strings.HasPrefix(entry.Path, dir+"/") {
				ignore = true
				break
			}
		}
		if ignore {
			continue
		}
		if err := visit(entry); err != nil {
			if err == ErrSkipDir && entry.Kind == EntryDirectory {
				skipped = append(skipped, entry.Path)
				continue
			}
			return err
		}
	}
	return nil
}

func (s *testSource) Stat(path string) (Entry, error) {
	if _, ok := s.files[path]; !ok {
		return Entry{}, fs.ErrNotExist
	}
	return Entry{Path: path, Kind: EntryRegular}, nil
}

func (s *testSource) Read(path string) ([]byte, error) {
	content, ok := s.files[path]
	if !ok {
		return nil, fs.ErrNotExist
	}
	return append([]byte(nil), content...), nil
}
