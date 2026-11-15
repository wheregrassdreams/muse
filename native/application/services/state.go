// Package services 实现 Muse 的 Go 后端服务，对应 Electron 版 main 进程
// 的 electron/services/*.ts。服务通过 mygo.Bind 暴露给前端，方法名与
// Electron IPC 通道一一对应（映射表见 src/platform/mygoBridge.ts）。
package services

import (
	"os"
	"sync"

	"github.com/egoist/mygo"
)

// OpenPathItem 是 app:open-paths 的载荷：外部「打开方式」/ 拖 Dock 的路径。
type OpenPathItem struct {
	Path  string `json:"path"`
	IsDir bool   `json:"isDir"`
}

// MenuPayload 是 menu:action 的载荷。
type MenuPayload struct {
	Action string `json:"action"`
	Format string `json:"format,omitempty"`
	Path   string `json:"path,omitempty"`
}

// 主进程 -> 渲染进程事件，通道名与 Electron 版保持一致。
var (
	EvMenuAction   = mygo.NewEvent[MenuPayload]("menu:action")
	EvOpenPaths    = mygo.NewEvent[[]OpenPathItem]("app:open-paths")
	EvRequestClose = mygo.NewEvent[any]("app:request-close")
	EvTreeChanged  = mygo.NewEvent[any]("fs:tree-changed")
	EvDocChanged   = mygo.NewEvent[string]("fs:document-changed")
	EvChatChunk    = mygo.NewEvent[map[string]any]("ai:chat-chunk")
	EvChatEnd      = mygo.NewEvent[map[string]any]("ai:chat-end")
	EvChatError    = mygo.NewEvent[map[string]any]("ai:chat-error")
)

// State 是应用级共享状态（脏标记 / 强制关闭 / 待打开路径 / 主窗口）。
type State struct {
	mu            sync.Mutex
	Dirty         bool
	ForceClose    bool
	RendererReady bool
	PendingOpen   []string
	Win           *mygo.Window
}

// S 是全局共享状态。
var S = &State{}

// RebuildMenu 由 main 注入：最近文件变化时重建应用菜单。
var RebuildMenu func()

// CloseDecision 返回本次关窗的处理方式，并消费强制关闭标记。
func (s *State) CloseDecision() (force, dirty bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	force, dirty = s.ForceClose, s.Dirty
	if force {
		s.ForceClose = false
	}
	return
}

// QueueOpenPath 暂存外部打开路径；渲染进程就绪则立即派发。
func (s *State) QueueOpenPath(path string) {
	s.mu.Lock()
	s.PendingOpen = append(s.PendingOpen, path)
	ready := s.RendererReady
	s.mu.Unlock()
	if ready {
		s.FlushOpenPaths()
	}
}

// TakeOpenPaths 取走并清空暂存路径，附上「是否目录」标记。
func (s *State) TakeOpenPaths() []OpenPathItem {
	s.mu.Lock()
	defer s.mu.Unlock()
	items := make([]OpenPathItem, 0, len(s.PendingOpen))
	for _, p := range s.PendingOpen {
		items = append(items, OpenPathItem{Path: p, IsDir: isDir(p)})
	}
	s.PendingOpen = nil
	return items
}

// FlushOpenPaths 在渲染进程就绪后把暂存路径推给窗口。
func (s *State) FlushOpenPaths() {
	s.mu.Lock()
	ready := s.RendererReady
	empty := len(s.PendingOpen) == 0
	s.mu.Unlock()
	if !ready || empty {
		return
	}
	items := s.TakeOpenPaths()
	if len(items) > 0 {
		_ = EvOpenPaths.Broadcast(items)
	}
}

func isDir(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.IsDir()
}
