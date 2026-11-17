package nativeui

import (
	"path/filepath"
	"strings"

	"muse/application/workspace"
	"muse/locales"
)

type recentEntry = workspace.RecentEntry

func (a *App) addRecent(path string, isDir bool) {
	if a == nil || a.recentItems == nil || path == "" {
		return
	}
	if err := a.recentItems.Add(path, isDir); err != nil {
		a.notice(locales.CommonSaveFailed(err.Error()))
	}
}

func (a *App) workspacePathLabel(path string) string {
	if !filepath.IsAbs(path) {
		return filepath.ToSlash(filepath.Clean(path))
	}
	if a.root != "" {
		if rel, err := filepath.Rel(a.root, path); err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return filepath.ToSlash(rel)
		}
	}
	return filepath.ToSlash(path)
}
