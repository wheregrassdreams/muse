// 应用级服务：对应 Electron 版 main.ts 里的应用通道。
//
//	App.GetOpenPaths    渲染进程启动握手：取走暂存的外部打开路径
//	App.SetDirty        渲染进程同步脏标记（关窗判断用）
//	App.Close           渲染进程确认后请求强制关闭
//	App.ConfirmUnsaved  未保存确认框 -> "save" | "discard" | "cancel"
//	App.CopyText        写系统剪贴板（文件树 / 卡片头的复制路径）
package services

import (
	"log"
	"muse/locales"

	"github.com/egoist/mygo"
)

// App 是应用级服务对象。
type App struct{}

// GetOpenPaths 渲染进程启动握手：标记就绪并返回暂存路径。
func (a *App) GetOpenPaths() []OpenPathItem {
	S.mu.Lock()
	S.RendererReady = true
	S.mu.Unlock()
	return S.TakeOpenPaths()
}

// SetDirty 同步脏标记（main 关窗时用）。
func (a *App) SetDirty(v bool) {
	S.mu.Lock()
	S.Dirty = v
	S.mu.Unlock()
}

// Close 渲染进程完成确认后请求关闭（绕过脏检查）。
func (a *App) Close() {
	S.mu.Lock()
	S.ForceClose = true
	win := S.Win
	S.mu.Unlock()
	if win != nil {
		win.Close()
	}
}

// CopyText 写系统剪贴板，供渲染进程复制文件 / 目录的相对或绝对路径。
// 返回是否写入成功（空字符串不覆盖剪贴板）。
func (a *App) CopyText(text string) bool {
	if text == "" {
		return false
	}
	mygo.Clipboard.WriteText(text)
	return true
}

// ConfirmUnsaved 未保存确认框。
func (a *App) ConfirmUnsaved(name string) string {
	log.Printf("[muse] App.ConfirmUnsaved name=%q", name)
	S.mu.Lock()
	win := S.Win
	S.mu.Unlock()
	res, err := mygo.Dialog.Message(mygo.MessageOptions{
		Parent:        win,
		Type:          mygo.MessageWarning,
		Message:       locales.CommonUnsavedMessage(name),
		Detail:        locales.CommonUnsavedDetail(),
		Buttons:       []string{locales.CommonSaveUnsaved(), locales.CommonDiscardUnsaved(), locales.CommonCancelUnsaved()},
		DefaultButton: 0,
		CancelButton:  2,
	})
	if err != nil {
		return "cancel"
	}
	switch res.Button {
	case 0:
		return "save"
	case 1:
		return "discard"
	default:
		return "cancel"
	}
}
