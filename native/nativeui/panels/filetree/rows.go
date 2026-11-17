package filetree

// Row is one visible item in the flattened tree projection.
type Row struct {
	Path      string
	Depth     int
	Directory bool
}

// Rows returns the cached visible tree projection for root. Catalog access is
// supplied by the host; expansion and projection cache lifetime belong to
// this panel state.
func (s *state) Rows(root string, children func(string) []string, isDirectory func(string) bool) []Row {
	if s == nil {
		return nil
	}
	if root == "" {
		s.InvalidateRows()
		return nil
	}
	if !s.rowsDirty && s.rowsRoot == root {
		return s.rows
	}
	rows := make([]Row, 0)
	var walk func(string, int)
	walk = func(directory string, depth int) {
		if children == nil {
			return
		}
		for _, path := range children(directory) {
			isDir := isDirectory != nil && isDirectory(path)
			rows = append(rows, Row{Path: path, Depth: depth, Directory: isDir})
			if isDir && s.IsExpanded(path) {
				walk(path, depth+1)
			}
		}
	}
	walk(root, 0)
	s.rows, s.rowsRoot, s.rowsDirty = rows, root, false
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
	s.rowsDirty = true
}
