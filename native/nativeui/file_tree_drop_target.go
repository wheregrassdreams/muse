package nativeui

import (
	"path/filepath"
	"strings"

	"github.com/egoist/mygo/ui"
)

func treePointerWithinRect(p ui.Point, valid bool, r ui.Rect) bool {
	return valid && p.X >= r.X && p.X < r.X+r.W && p.Y >= r.Y && p.Y < r.Y+r.H
}

const (
	treeDropIndentOrigin = float32(12)
	treeDropIndentStep   = float32(14)
	treeDropFolderLabel  = float32(28)
)

type treeDropTarget struct {
	path  string
	start int
	end   int
}

// resolveTreeDropTarget resolves one hovered visible row and the horizontal
// indentation lane into a directory. It intentionally works from the current
// flattened rows, so expanding a folder never leaves a stale row index.
func resolveTreeDropTarget(rows []fileRow, index int, localX float32, root string) (treeDropTarget, bool) {
	if index < 0 || index >= len(rows) || root == "" {
		return treeDropTarget{}, false
	}
	row := rows[index]
	if row.dir {
		return targetForVisiblePath(rows, row.path), true
	}
	ancestors := make([]string, 0, row.depth+2)
	for parent := filepath.Dir(row.path); parent != row.path; parent = filepath.Dir(parent) {
		if parent == root || isPathWithin(root, parent) {
			ancestors = append(ancestors, parent)
		}
		if parent == root {
			break
		}
	}
	if len(ancestors) == 0 || ancestors[len(ancestors)-1] != root {
		ancestors = append(ancestors, root)
	}
	depthLane := int((localX - treeDropIndentOrigin) / treeDropIndentStep)
	if localX < treeDropIndentOrigin {
		depthLane = -1
	}
	chosen := root
	chosenDepth := -2
	for _, path := range ancestors {
		depth := -1
		if path != root {
			for i := range rows {
				if rows[i].path == path {
					depth = rows[i].depth
					break
				}
			}
		}
		if depth <= depthLane && depth > chosenDepth {
			chosen = path
			chosenDepth = depth
		}
	}
	return targetForVisiblePath(rows, chosen), true
}

func defaultTreeDropTarget(rows []fileRow, index int, root string) (treeDropTarget, bool) {
	if index < 0 || index >= len(rows) {
		return treeDropTarget{}, false
	}
	row := rows[index]
	x := treeDropIndentOrigin + float32(row.depth)*treeDropIndentStep
	if row.dir {
		x += treeDropFolderLabel
	}
	return resolveTreeDropTarget(rows, index, x, root)
}

func targetForVisiblePath(rows []fileRow, path string) treeDropTarget {
	if len(rows) > 0 && filepath.Dir(rows[0].path) == path {
		return treeDropTarget{path: path, start: 0, end: len(rows)}
	}
	for i, row := range rows {
		if row.path != path {
			continue
		}
		end := i + 1
		for end < len(rows) && rows[end].depth > row.depth {
			end++
		}
		return treeDropTarget{path: path, start: i, end: end}
	}
	return treeDropTarget{path: path, start: -1, end: -1}
}

func isPathWithin(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	return err == nil && !filepath.IsAbs(rel) && (rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))))
}

func treeDropTargetAtPointer(c *ui.Context, rows []fileRow, bounds map[int]ui.Rect, root string) (treeDropTarget, int, bool) {
	x, y, over := c.Root().PointerPosition()
	if !over {
		return treeDropTarget{}, -1, false
	}
	rootBounds := c.Root().Bounds()
	x += rootBounds.X
	y += rootBounds.Y
	for i, rect := range bounds {
		if x >= rect.X && x < rect.X+rect.W && y >= rect.Y && y < rect.Y+rect.H {
			returnTarget, ok := resolveTreeDropTarget(rows, i, x-rect.X, root)
			return returnTarget, i, ok
		}
	}
	return treeDropTarget{}, -1, false
}

func treeDropTargetAtWindowPoint(rows []fileRow, bounds map[int]ui.Rect, root string, x, y float32) (treeDropTarget, int, bool) {
	for i, rect := range bounds {
		if x >= rect.X && x < rect.X+rect.W && y >= rect.Y && y < rect.Y+rect.H {
			returnTarget, ok := resolveTreeDropTarget(rows, i, x-rect.X, root)
			return returnTarget, i, ok
		}
	}
	return treeDropTarget{}, -1, false
}

func pointerWithinElement(c *ui.Context, element ui.Element) bool {
	x, y, over := c.Root().PointerPosition()
	if !over {
		return false
	}
	rootBounds := c.Root().Bounds()
	x += rootBounds.X
	y += rootBounds.Y
	bound := element.Bounds()
	return x >= bound.X && x < bound.X+bound.W && y >= bound.Y && y < bound.Y+bound.H
}
