package filetree

import "path/filepath"

// Row is one visible item in the flattened tree projection.
type Row struct {
	Path      string
	Depth     int
	Directory bool
}

// CatalogPort supplies only the facts needed to project visible rows. The
// explorer owns filtering, flattening, expansion, and cache lifetime.
type CatalogPort struct {
	Children    func(string) []CatalogEntry
	VisibleFile func(string) bool
	Pending     bool
	Revision    uint64
}

// BuildRows returns the cached visible tree projection. A catalog revision
// change invalidates the projection automatically; pending catalogs are never
// cached as empty trees.
func BuildRows(handle *Handle, root string, catalog CatalogPort) []Row {
	if handle == nil || handle.state == nil {
		return nil
	}
	return handle.state.projectRows(root, catalog)
}

func (s *state) projectRows(root string, catalog CatalogPort) []Row {
	if s == nil {
		return nil
	}
	if root == "" {
		s.InvalidateRows()
		return nil
	}
	if catalog.Pending {
		s.InvalidateRows()
		return nil
	}
	if !s.rowsDirty && s.rowsRoot == root && s.rowsRevision == catalog.Revision {
		return s.rows
	}
	rows := make([]Row, 0)
	var walk func(string, int)
	walk = func(directory string, depth int) {
		if catalog.Children == nil {
			return
		}
		rel, err := filepath.Rel(root, directory)
		if err != nil || rel == ".." || filepath.IsAbs(rel) {
			return
		}
		entries := catalog.Children(directory)
		for _, child := range ProjectChildRows(root, entries, catalog.VisibleFile) {
			child.Depth = depth
			childRel, err := filepath.Rel(root, child.Path)
			if err != nil || childRel == ".." || filepath.IsAbs(childRel) || filepath.Dir(childRel) != rel {
				continue
			}
			rows = append(rows, child)
			if child.Directory && s.IsExpanded(child.Path) {
				walk(child.Path, depth+1)
			}
		}
	}
	walk(root, 0)
	s.rows, s.rowsRoot, s.rowsRevision, s.rowsDirty = rows, root, catalog.Revision, false
	return s.rows
}

// InvalidateRows drops the flattened catalog projection after the host
// changes the catalog or its visibility rules.
func (s *state) InvalidateRows() {
	if s == nil {
		return
	}
	s.rows = nil
	s.rowsRoot = ""
	s.rowsRevision = 0
	s.rowsDirty = true
}
