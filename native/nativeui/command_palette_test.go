package nativeui

import (
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

// ⌘P 命令面板：打开 → 模糊过滤 → 运行选中命令。
func TestCommandPalette(t *testing.T) {
	a := New()
	a.setWorkspace(t.TempDir())

	a.openCommandPalette()
	if !a.cmdPalette {
		t.Fatal("面板未打开")
	}
	tt := ui.NewTester(a.View, 1200, 800)
	tt.Frame()

	for _, cmd := range a.paletteCommands() {
		if strings.Contains(cmd.name, "主题") || strings.Contains(cmd.name, "AI 助手") {
			t.Fatalf("disabled AI or theme control is still a command: %q", cmd.name)
		}
	}
	a.aiEnabled = true
	foundAI := false
	for _, cmd := range a.paletteCommands() {
		foundAI = foundAI || cmd.name == "AI 助手面板"
	}
	if !foundAI {
		t.Fatal("enabling AI did not expose its palette command")
	}

	// 无匹配提示空候选，Esc 路径关闭
	a.openCommandPalette()
	a.cmdValue = "不存在的命令xyz"
	if len(a.paletteCandidates()) != 0 {
		t.Fatal("无匹配时应为空候选")
	}
	a.closeCommandPalette()
	if a.cmdPalette {
		t.Fatal("关闭失败")
	}

	// 默认第一项是「新建文档」：运行后 dirty 文档状态建立
	a.openCommandPalette()
	a.applyCommandPalette()
	if a.cmdPalette {
		t.Fatal("运行后面板未关闭")
	}
	// 新建文档 = 重置为空文档（path/source 清空）
	if a.documentPath() != "" || a.documentSource() != "" {
		t.Fatalf("新建文档命令未执行: path=%q source=%q", a.documentPath(), a.documentSource())
	}
}
