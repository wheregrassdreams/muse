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
	a.recents = a.recentItems.Entries()
}

func (a *App) liveRecents() ([]recentEntry, []string) {
	if a == nil || a.recentItems == nil {
		return nil, nil
	}
	live := a.recentItems.Live(workspace.MaxVisibleRecent)
	return live, recentLabels(live)
}

func recentLabels(entries []recentEntry) []string {
	counts := map[string]int{}
	for _, entry := range entries {
		counts[strings.ToLower(filepath.Base(entry.Path))]++
	}
	labels := make([]string, len(entries))
	for i, entry := range entries {
		label := filepath.Base(entry.Path)
		if counts[strings.ToLower(label)] > 1 {
			label = filepath.Base(filepath.Dir(entry.Path)) + "/" + label
		}
		labels[i] = clipRunes(label, 24)
	}
	return labels
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

func clipRunes(value string, limit int) string {
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit-1]) + "…"
}

func (a *App) openRecent(entry recentEntry) {
	if entry.IsDir {
		a.setWorkspace(entry.Path)
		a.invalidate()
		return
	}
	a.requestOpen(entry.Path)
}
