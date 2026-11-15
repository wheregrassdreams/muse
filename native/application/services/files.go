// 文件服务：对应 Electron 版 electron/services/fs.ts。
// 通道（mygo 方法名）：
//
//	Files.Open          弹选择框打开 -> { path, content } | null
//	Files.OpenPath      按路径打开（拖拽 / 最近文件）-> { path, content } | null
//	Files.Save          保存到指定路径 -> path
//	Files.SaveAs        弹另存为框 -> path | ""
//	Files.ReadRecent    读取最近文件列表 -> string[]
//	Files.PickFolder    弹选择框选工作区目录 -> path | ""
//	Files.ListTree      递归列出工作区里的 markdown -> TreeNode[] | null
//	Files.CreateFile    在目录下新建 md（重名自动加序号）-> path | ""
//	Files.Rename        重命名文件 / 目录 -> newPath | ""
//	Files.RenameDocument 按标题改当前文档的文件名（不限工作区）-> newPath | ""
//	Files.Trash         移入废纸篓（可恢复）-> bool
//	Files.WatchFile / Files.UnwatchFile   当前文档监听 -> 变更时推 fs:document-changed
//	Files.UnwatchWorkspace                取消目录监听（listTree 自动换绑）
//	Files.ReadFile                        按路径读取（供外部变更后的无副作用重载）
//	Files.RevealInFolder                  在系统文件管理器中显示
package services

import (
	"context"
	"encoding/json"
	log2 "log"
	"muse/locales"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/egoist/mygo"
	"github.com/fsnotify/fsnotify"
)

// FileContent 是打开文件的返回载荷。
type FileContent struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

// TreeNode 是工作区文件树节点。
type TreeNode struct {
	Name     string     `json:"name"`
	Path     string     `json:"path"`
	Type     string     `json:"type"` // "dir" | "file"
	Children []TreeNode `json:"children,omitempty"`
}

// Draft 是未命名文档草稿。
type Draft struct {
	Content string `json:"content"`
}

var mdExts = map[string]bool{".md": true, ".markdown": true, ".mdx": true}

// textExts 是可直接在 Muse 中打开的文本文件扩展名（拒绝 doc / pdf 等二进制）。
var textExts = map[string]bool{".md": true, ".markdown": true, ".mdx": true, ".txt": true}

var skipDirs = map[string]bool{"node_modules": true, "dist": true, "out": true, "build": true, ".git": true}

const (
	maxDepth   = 8
	maxEntries = 5000
)

// Files 是文件服务对象，同时持有最近文件 / 工作区 / 监听器状态。
type Files struct {
	mu            sync.Mutex
	recent        []string
	workspaceRoot string
	recentFile    string
	draftFile     string

	wsWatcher    *fsnotify.Watcher
	wsTimer      *time.Timer
	docWatcher   *fsnotify.Watcher
	docPath      string
	docTimer     *time.Timer
	saveAsMu     sync.Mutex
	lastSaveAsAt time.Time
}

// NewFiles 初始化文件服务（加载最近文件列表）。
func NewFiles() (*Files, error) {
	dir, err := mygo.App.Path(mygo.PathUserData)
	if err != nil {
		return nil, err
	}
	f := &Files{
		recentFile: filepath.Join(dir, "recent.json"),
		draftFile:  filepath.Join(dir, "draft.json"),
	}
	if data, err := os.ReadFile(f.recentFile); err == nil {
		var list []string
		if json.Unmarshal(data, &list) == nil {
			f.recent = list
		}
	}
	return f, nil
}

// Recent 返回最近文件列表副本（main 建菜单用）。
func (f *Files) Recent() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.recent...)
}

func (f *Files) saveRecentLocked() {
	if data, err := json.Marshal(f.recent); err == nil {
		_ = os.WriteFile(f.recentFile, data, 0o644)
	}
}

func (f *Files) addRecent(path string) {
	f.mu.Lock()
	next := []string{path}
	for _, p := range f.recent {
		if p != path {
			next = append(next, p)
		}
	}
	if len(next) > 12 {
		next = next[:12]
	}
	f.recent = next
	f.saveRecentLocked()
	f.mu.Unlock()
	if RebuildMenu != nil {
		RebuildMenu()
	}
}

func isTextFile(path string) bool {
	return textExts[strings.ToLower(filepath.Ext(path))]
}

// warnUnsupported 弹一个温和的提示：只能打开文本类文件。
func warnUnsupported(parent *mygo.Window, path string) {
	_, _ = mygo.Dialog.Message(mygo.MessageOptions{
		Parent:  parent,
		Type:    mygo.MessageWarning,
		Message: locales.CommonOpenUnsupportedFileMessage(filepath.Base(path)),
		Detail:  locales.CommonOpenUnsupportedFileDetail(),
		Buttons: []string{locales.CommonAcknowledge()},
	})
}

func (f *Files) readPath(path string) *FileContent {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	f.addRecent(path)
	return &FileContent{Path: path, Content: string(data)}
}

// Open 弹选择框打开 Markdown。
func (f *Files) Open(ctx context.Context) (*FileContent, error) {
	parent := mygo.CallerWindow(ctx)
	paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
		Parent:  parent,
		Title:   locales.CommonOpenMarkdown(),
		Filters: []mygo.FileFilter{{Name: "Markdown", Extensions: []string{"md", "markdown", "mdx", "txt"}}},
	})
	if err != nil || len(paths) == 0 {
		return nil, err
	}
	if !isTextFile(paths[0]) {
		warnUnsupported(parent, paths[0])
		return nil, nil
	}
	return f.readPath(paths[0]), nil
}

// OpenPath 按路径打开（拖拽 / 最近文件 / 外部「打开方式」），同样只接受文本类文件。
func (f *Files) OpenPath(path string) *FileContent {
	if !isTextFile(path) {
		return nil
	}
	return f.readPath(path)
}

// ReadFile 按路径读取，不更新最近文件（外部程序改写当前文档后重载用）。
func (f *Files) ReadFile(path string) *FileContent {
	log2.Printf("[muse] Files.ReadFile %s", path)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	return &FileContent{Path: path, Content: string(data)}
}

// IsDir 判断路径是否为目录（窗口内拖入文件夹时区分「工作区」与「文档」）。
func (f *Files) IsDir(path string) bool {
	return isDir(path)
}

// Save 保存到指定路径。
func (f *Files) Save(path, content string) (string, error) {
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return "", err
	}
	f.addRecent(path)
	return path, nil
}

// SaveAs 弹另存为框。互斥 + 冷却：渲染层连发、长按 Cmd+S 全部只弹 1 次。
func (f *Files) SaveAs(ctx context.Context, content string) (string, error) {
	f.saveAsMu.Lock()
	defer f.saveAsMu.Unlock()
	if time.Since(f.lastSaveAsAt) < 1500*time.Millisecond {
		return "", nil
	}
	f.lastSaveAsAt = time.Now()
	path, err := mygo.Dialog.Save(mygo.SaveDialogOptions{
		Parent:      mygo.CallerWindow(ctx),
		Title:       locales.CommonSaveFile(),
		DefaultPath: "Untitled.md",
		Filters:     []mygo.FileFilter{{Name: "Markdown", Extensions: []string{"md", "markdown", "mdx"}}},
	})
	if err != nil || path == "" {
		return "", err
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return "", err
	}
	f.addRecent(path)
	return path, nil
}

// CreateDefault：启动即在固定目录打开同一文件（可配置，不自动递增）。
func (f *Files) CreateDefault(dir, name string) *FileContent {
	log2.Printf("[muse] Files.CreateDefault dir=%q name=%q", dir, name)
	baseDir := strings.TrimSpace(dir)
	if baseDir == "" || !isDir(baseDir) {
		if docs, err := mygo.App.Path(mygo.PathDocuments); err == nil && docs != "" {
			baseDir = docs
		} else if tmp, err := mygo.App.Path(mygo.PathTemp); err == nil {
			baseDir = tmp
		}
	}
	fileName := strings.TrimSpace(name)
	if fileName == "" {
		fileName = "Untitled.md"
	}
	if !strings.HasSuffix(strings.ToLower(fileName), ".md") {
		fileName += ".md"
	}
	fileName = filepath.Base(fileName)
	if err := os.MkdirAll(baseDir, 0o755); err != nil {
		return nil
	}
	p := filepath.Join(baseDir, fileName)
	if data, err := os.ReadFile(p); err == nil {
		f.addRecent(p)
		return &FileContent{Path: p, Content: string(data)}
	}
	if err := os.WriteFile(p, nil, 0o644); err != nil {
		return nil
	}
	f.addRecent(p)
	return &FileContent{Path: p, Content: ""}
}

// ReadRecent 读取最近文件列表。
func (f *Files) ReadRecent() []string {
	return f.Recent()
}

// PickFolder 弹选择框选工作区目录。
func (f *Files) PickFolder(ctx context.Context) (string, error) {
	paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
		Parent:            mygo.CallerWindow(ctx),
		Title:             locales.CommonOpenFolderDialog(),
		ButtonLabel:       locales.CommonOpenButton(),
		Directory:         true,
		CreateDirectories: true,
	})
	if err != nil || len(paths) == 0 {
		return "", err
	}
	return paths[0], nil
}

// ListTree 列树，顺带把 root 记为当前工作区（写类操作的越界校验基准）并挂上监听。
func (f *Files) ListTree(root string) []TreeNode {
	f.mu.Lock()
	if root == "" || !isDir(root) {
		if f.workspaceRoot == root {
			f.workspaceRoot = ""
			f.stopWatchLocked()
		}
		f.mu.Unlock()
		return nil
	}
	if f.workspaceRoot != root {
		f.workspaceRoot = root
		f.startWatchLocked(root)
	}
	f.mu.Unlock()
	budget := 0
	return walk(root, 0, &budget)
}

// 递归收集 markdown 文件树：目录在前、同级按名称排序；跳过隐藏目录与
// 依赖/产物目录；不含任何 markdown 的目录会被剪掉。
func walk(dir string, depth int, budget *int) []TreeNode {
	if depth > maxDepth || *budget >= maxEntries {
		return nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil // 无权限 / 已被删除
	}
	var dirs, files []TreeNode
	for _, e := range entries {
		if *budget >= maxEntries {
			break
		}
		name := e.Name()
		if strings.HasPrefix(name, ".") || skipDirs[name] {
			continue
		}
		full := filepath.Join(dir, name)
		if e.IsDir() {
			children := walk(full, depth+1, budget)
			if len(children) == 0 {
				continue // 空目录（无 md）不展示
			}
			*budget++
			dirs = append(dirs, TreeNode{Name: name, Path: full, Type: "dir", Children: children})
		} else if e.Type().IsRegular() && mdExts[strings.ToLower(filepath.Ext(name))] {
			*budget++
			files = append(files, TreeNode{Name: name, Path: full, Type: "file"})
		}
	}
	byName := func(list []TreeNode) {
		sort.Slice(list, func(i, j int) bool { return list[i].Name < list[j].Name })
	}
	byName(dirs)
	byName(files)
	return append(dirs, files...)
}

// insideWorkspace 校验目标是否位于工作区之内（防止渲染层传来越界路径）。
func (f *Files) insideWorkspace(target string) bool {
	f.mu.Lock()
	root := f.workspaceRoot
	f.mu.Unlock()
	if root == "" {
		return false
	}
	root = filepath.Clean(root)
	p := filepath.Clean(target)
	return p == root || strings.HasPrefix(p, root+string(os.PathSeparator))
}

// uniquePath 在 dir 下找一个未被占用的文件名：未命名.md / 未命名 2.md / …
func uniquePath(dir, base, ext string) string {
	p := filepath.Join(dir, base+ext)
	for i := 2; ; i++ {
		if _, err := os.Stat(p); os.IsNotExist(err) {
			return p
		}
		p = filepath.Join(dir, base+" "+strconv.Itoa(i)+ext)
	}
}

// CreateFile 在目录下新建 md（重名自动加序号）。
func (f *Files) CreateFile(dir, base string) string {
	if base == "" {
		base = locales.FilesUntitled()
	}
	if !f.insideWorkspace(dir) || !isDir(dir) {
		return ""
	}
	p := uniquePath(dir, base, ".md")
	if err := os.WriteFile(p, nil, 0o644); err != nil {
		return ""
	}
	return p
}

// CreateFolder 在目录下新建文件夹。
func (f *Files) CreateFolder(dir, base string) string {
	if base == "" {
		base = locales.FilesNewFolderDefaultName()
	}
	if !f.insideWorkspace(dir) || !isDir(dir) {
		return ""
	}
	p := filepath.Join(dir, base)
	for i := 2; ; i++ {
		if _, err := os.Stat(p); os.IsNotExist(err) {
			break
		}
		p = filepath.Join(dir, base+" "+strconv.Itoa(i))
	}
	if err := os.Mkdir(p, 0o755); err != nil {
		return ""
	}
	return p
}

// Rename 重命名：只接受纯文件名（不含分隔符），且新旧路径都必须在工作区内。
func (f *Files) Rename(path, newName string) string {
	name := strings.TrimSpace(newName)
	if name == "" || strings.ContainsAny(name, "/\\") || name == "." || name == ".." {
		return ""
	}
	if !f.insideWorkspace(path) {
		return ""
	}
	if _, err := os.Stat(path); err != nil {
		return ""
	}
	next := filepath.Join(filepath.Dir(path), name)
	if !f.insideWorkspace(next) || pathExists(next) {
		return ""
	}
	if err := os.Rename(path, next); err != nil {
		return ""
	}
	// 最近文件里的旧路径同步跟上，避免菜单里留死链接
	f.mu.Lock()
	for i, p := range f.recent {
		if p == path {
			f.recent[i] = next
		}
	}
	f.saveRecentLocked()
	f.mu.Unlock()
	if RebuildMenu != nil {
		RebuildMenu()
	}
	return next
}

// RenameDocument 把当前文档改成新的文件名（标题即文件名）。
// 与 Rename 的区别是不要求文件位于工作区内——文档可以从任何位置打开；
// 仍然只接受纯文件名，且只对已存在的文本类文件生效，目标已占用时直接失败（不静默覆盖）。
func (f *Files) RenameDocument(path, newName string) string {
	name := strings.TrimSpace(newName)
	if name == "" || strings.ContainsAny(name, `/\`) || name == "." || name == ".." {
		return ""
	}
	if !isTextFile(path) {
		return ""
	}
	if _, err := os.Stat(path); err != nil {
		return ""
	}
	next := filepath.Join(filepath.Dir(path), name)
	if next == path {
		return path
	}
	if pathExists(next) {
		return ""
	}
	if err := os.Rename(path, next); err != nil {
		return ""
	}
	// 最近文件里的旧路径同步跟上，避免菜单里留死链接
	f.mu.Lock()
	for i, p := range f.recent {
		if p == path {
			f.recent[i] = next
		}
	}
	f.saveRecentLocked()
	f.mu.Unlock()
	if RebuildMenu != nil {
		RebuildMenu()
	}
	return next
}

// Trash 删除走废纸篓而非直接 unlink：误删可恢复。
func (f *Files) Trash(path string) bool {
	if !f.insideWorkspace(path) || !pathExists(path) {
		return false
	}
	if err := mygo.Shell.TrashItem(path); err != nil {
		return false
	}
	f.mu.Lock()
	next := f.recent[:0]
	for _, p := range f.recent {
		if p != path {
			next = append(next, p)
		}
	}
	f.recent = next
	f.saveRecentLocked()
	f.mu.Unlock()
	if RebuildMenu != nil {
		RebuildMenu()
	}
	return true
}

// RevealInFolder 在系统文件管理器中显示。
func (f *Files) RevealInFolder(path string) {
	if path != "" && pathExists(path) {
		mygo.Shell.ShowItemInFolder(path)
	}
}

// UnwatchWorkspace 取消工作区监听。
func (f *Files) UnwatchWorkspace() {
	f.mu.Lock()
	f.workspaceRoot = ""
	f.stopWatchLocked()
	f.mu.Unlock()
}

// ---- 未命名文档草稿：自动保存 / 启动恢复 ----

// ReadDraft 读取草稿。
func (f *Files) ReadDraft() *Draft {
	data, err := os.ReadFile(f.draftFile)
	if err != nil {
		return nil
	}
	var d Draft
	if json.Unmarshal(data, &d) != nil {
		return nil
	}
	return &d
}

// WriteDraft 写入草稿。
func (f *Files) WriteDraft(content string) {
	data, _ := json.Marshal(map[string]any{"content": content, "ts": time.Now().UnixMilli()})
	_ = os.WriteFile(f.draftFile, data, 0o644)
}

// ClearDraft 清除草稿。
func (f *Files) ClearDraft() {
	_ = os.Remove(f.draftFile)
}

// ---- 工作区目录监听（fsnotify 手动递归，等价 Node recursive watch） ----

// startWatchLocked 监听整棵目录树；f.mu 已持有。
func (f *Files) startWatchLocked(root string) {
	f.stopWatchLocked()
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return
	}
	_ = filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil || !d.IsDir() {
			return nil
		}
		name := d.Name()
		if p != root && (strings.HasPrefix(name, ".") || skipDirs[name]) {
			return filepath.SkipDir
		}
		_ = w.Add(p)
		return nil
	})
	f.wsWatcher = w
	go f.wsLoop(w)
}

func (f *Files) stopWatchLocked() {
	if f.wsWatcher != nil {
		_ = f.wsWatcher.Close()
		f.wsWatcher = nil
	}
	if f.wsTimer != nil {
		f.wsTimer.Stop()
		f.wsTimer = nil
	}
}

func (f *Files) wsLoop(w *fsnotify.Watcher) {
	for {
		select {
		case ev, ok := <-w.Events:
			if !ok {
				return
			}
			// 新建目录纳入监听，否则其下的变化会丢
			if ev.Op&fsnotify.Create != 0 && isDir(ev.Name) {
				_ = w.Add(ev.Name)
			}
			f.debounceTree()
		case _, ok := <-w.Errors:
			if !ok {
				return
			}
		}
	}
}

// 目录变更 -> 防抖后通知渲染层刷新树。
func (f *Files) debounceTree() {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.wsTimer != nil {
		f.wsTimer.Stop()
	}
	f.wsTimer = time.AfterFunc(300*time.Millisecond, func() {
		_ = EvTreeChanged.Broadcast(nil)
	})
}

// ---- 当前文档监听（外部编辑器改写后自动重载） ----
// 监听父目录而非文件本身：VS Code 等程序常以「写临时文件后 rename 替换」
// 的方式保存，文件级 watcher 会在替换后失效。

// WatchFile 监听指定文档。
func (f *Files) WatchFile(path string) {
	if path == "" {
		return
	}
	log2.Printf("[muse] Files.WatchFile %s", path)
	target := filepath.Clean(path)
	parent := filepath.Dir(target)
	base := filepath.Base(target)

	f.mu.Lock()
	f.stopDocWatchLocked()
	w, err := fsnotify.NewWatcher()
	if err != nil || w.Add(parent) != nil {
		if w != nil {
			_ = w.Close()
		}
		f.mu.Unlock()
		return
	}
	f.docWatcher = w
	f.docPath = target
	f.mu.Unlock()

	go func() {
		for {
			select {
			case ev, ok := <-w.Events:
				if !ok {
					return
				}
				if filepath.Base(ev.Name) != base {
					continue
				}
				f.mu.Lock()
				if f.docTimer != nil {
					f.docTimer.Stop()
				}
				t := target
				f.docTimer = time.AfterFunc(180*time.Millisecond, func() {
					log2.Printf("[muse] broadcast fs:document-changed %s", t)
					_ = EvDocChanged.Broadcast(t)
				})
				f.mu.Unlock()
			case _, ok := <-w.Errors:
				if !ok {
					return
				}
			}
		}
	}()
}

// UnwatchFile 取消文档监听。
func (f *Files) UnwatchFile() {
	f.mu.Lock()
	f.stopDocWatchLocked()
	f.mu.Unlock()
}

func (f *Files) stopDocWatchLocked() {
	if f.docWatcher != nil {
		_ = f.docWatcher.Close()
		f.docWatcher = nil
	}
	if f.docTimer != nil {
		f.docTimer.Stop()
		f.docTimer = nil
	}
	f.docPath = ""
}

func pathExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}
