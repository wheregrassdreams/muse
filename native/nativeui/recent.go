package nativeui

// recent.go — 最近打开的文件与文件夹：空态的「打开文件或文件夹」按钮
// 下拉里可以直接回到它们。记录持久化在本机配置目录，最多留 10 条。

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// recentEntry 是一条「最近打开」记录。
type recentEntry struct {
	Path  string `json:"path"`
	IsDir bool   `json:"isDir,omitempty"`
}

// recentMax 是最多保留多少条记录。
const recentMax = 10

// recentShow 是菜单里最多展示多少条。
const recentShow = 8

func recentStorePath() string {
	if dir := os.Getenv("MUSE_CONFIG_DIR"); dir != "" {
		return filepath.Join(dir, "recent.json")
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		dir = os.TempDir()
	}
	return filepath.Join(dir, "Muse", "recent.json")
}

func loadRecents() []recentEntry {
	data, err := os.ReadFile(recentStorePath())
	if err != nil {
		return nil
	}
	var rs []recentEntry
	if json.Unmarshal(data, &rs) != nil {
		return nil
	}
	kept := rs[:0]
	for _, r := range rs {
		if r.Path != "" {
			kept = append(kept, r)
		}
	}
	return kept
}

func saveRecents(rs []recentEntry) error {
	path := recentStorePath()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(rs, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

// addRecent 把一条记录挪到最前（去重），截到上限并落盘。
func (a *App) addRecent(path string, isDir bool) {
	if path == "" {
		return
	}
	out := []recentEntry{{Path: path, IsDir: isDir}}
	for _, r := range a.recents {
		if r.Path != path && len(out) < recentMax {
			out = append(out, r)
		}
	}
	a.recents = out
	_ = saveRecents(out)
}

// liveRecents 是此刻还存在的记录（失效的路径不再出现），连同它们的
// 菜单标签。
func (a *App) liveRecents() ([]recentEntry, []string) {
	var live []recentEntry
	for _, r := range a.recents {
		if _, err := os.Stat(r.Path); err == nil {
			live = append(live, r)
			if len(live) >= recentShow {
				break
			}
		}
	}
	return live, recentLabels(live)
}

// recentLabels 给每条记录一个菜单标签：文件/文件夹名，重名的带上
// 父目录名，超长截断——菜单不能撑出布局。
func recentLabels(rs []recentEntry) []string {
	counts := map[string]int{}
	for _, r := range rs {
		counts[strings.ToLower(filepath.Base(r.Path))]++
	}
	labels := make([]string, len(rs))
	for i, r := range rs {
		base := filepath.Base(r.Path)
		if counts[strings.ToLower(base)] > 1 {
			base = filepath.Base(filepath.Dir(r.Path)) + "/" + base
		}
		labels[i] = clipRunes(base, 24)
	}
	return labels
}

// workspacePathLabel shows paths relative to the open vault. Recent entries
// are persisted as absolute paths, while search hits are already relative;
// normalize both before putting them in the workspace search results.
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

// clipRunes 按字符数截断并补省略号。
func clipRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}

// openRecent 回到一条最近记录：文件夹直接进，文件走带未保存确认的
// 打开流程。
func (a *App) openRecent(e recentEntry) {
	if e.IsDir {
		a.setWorkspace(e.Path)
		a.invalidate()
		return
	}
	a.requestOpen(e.Path)
}
