package nativeui

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/egoist/mygo/ui"
)

// sse 把一段 JSON 包装成一个 OpenAI 风格的 SSE 事件。
func sse(payload string) string {
	return "data: " + payload + "\n\n"
}

func TestStreamAccumulatorText(t *testing.T) {
	acc := &streamAccumulator{}
	d1 := acc.feed(sse(`{"choices":[{"delta":{"content":"Hel"}}]}`))
	d2 := acc.feed(sse(`{"choices":[{"delta":{"content":"lo"}}]}`))
	d3 := acc.feed(sse("data: [DONE]\n\n"))
	tail := acc.finish()
	var got strings.Builder
	for _, d := range []streamDelta{d1, d2, d3, tail} {
		got.WriteString(d.Text)
	}
	if got.String() != "Hello" {
		t.Fatalf("text = %q", got.String())
	}
	if got.String() != acc.text.String() {
		t.Fatalf("delta 与总量不一致: %q vs %q", got.String(), acc.text.String())
	}
}

func TestStreamAccumulatorReasoningContent(t *testing.T) {
	acc := &streamAccumulator{}
	d1 := acc.feed(sse(`{"choices":[{"delta":{"reasoning_content":"先想"}}]}`))
	d2 := acc.feed(sse(`{"choices":[{"delta":{"content":"答案"}}]}`))
	tail := acc.finish()
	if d1.Think != "先想" || d1.Text != "" {
		t.Fatalf("d1 = %+v", d1)
	}
	if d2.Text != "答案" || d2.Think != "" {
		t.Fatalf("d2 = %+v", d2)
	}
	if tail.Think != "" || tail.Text != "" {
		t.Fatalf("tail = %+v", tail)
	}
	if acc.think.String() != "先想" || acc.text.String() != "答案" {
		t.Fatalf("totals think=%q text=%q", acc.think.String(), acc.text.String())
	}
}

func TestStreamAccumulatorThinkTags(t *testing.T) {
	acc := &streamAccumulator{}
	// <think> 被切在两个增量之间，</think> 也是。
	feeds := []string{
		sse(`{"choices":[{"delta":{"content":"让我想"}}]}`),
		sse(`{"choices":[{"delta":{"content":"<thi"}}]}`),
		sse(`{"choices":[{"delta":{"content":"nk>推理</th"}}]}`),
		sse(`{"choices":[{"delta":{"content":"ink>结论"}}]}`),
	}
	var text, think strings.Builder
	for _, f := range feeds {
		d := acc.feed(f)
		text.WriteString(d.Text)
		think.WriteString(d.Think)
	}
	tail := acc.finish()
	text.WriteString(tail.Text)
	think.WriteString(tail.Think)
	if text.String() != "让我想结论" {
		t.Fatalf("text = %q", text.String())
	}
	if think.String() != "推理" {
		t.Fatalf("think = %q", think.String())
	}
}

func TestStreamAccumulatorToolCalls(t *testing.T) {
	acc := &streamAccumulator{}
	d1 := acc.feed(sse(`{"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"read_document","arguments":""}}]}}]}`))
	d2 := acc.feed(sse(`{"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"pa"}}]}}]}`))
	d3 := acc.feed(sse(`{"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"th\":\"a.md\"}"}}]}}]}`))
	d4 := acc.feed(sse(`{"choices":[{"delta":{"tool_calls":[{"index":1,"id":"call_2","type":"function","function":{"name":"search_document","arguments":"{}"}}]}}]}`))
	if len(d1.Tools) != 1 || d1.Tools[0].Name != "read_document" {
		t.Fatalf("d1.Tools = %+v", d1.Tools)
	}
	if len(d2.Tools) != 0 || len(d3.Tools) != 0 {
		t.Fatalf("参数增量不该触发新工具: %+v %+v", d2.Tools, d3.Tools)
	}
	if len(d4.Tools) != 1 || d4.Tools[0].Name != "search_document" {
		t.Fatalf("d4.Tools = %+v", d4.Tools)
	}
	acc.finish()
	if len(acc.tools) != 2 {
		t.Fatalf("tools = %+v", acc.tools)
	}
	if acc.tools[0].args.String() != `{"path":"a.md"}` {
		t.Fatalf("args = %q", acc.tools[0].args.String())
	}
	if acc.tools[0].ID != "call_1" || acc.tools[1].ID != "call_2" {
		t.Fatalf("ids = %q %q", acc.tools[0].ID, acc.tools[1].ID)
	}
}

func TestValidPrefixLenKeepsRuneWhole(t *testing.T) {
	s := "读文档"
	// 边界切在「文」的三个字节之间时，都要往前挪到字符开头。
	if got := validPrefixLen(s, 4); got != 3 {
		t.Fatalf("keep=4 -> %d, want 3", got)
	}
	if got := validPrefixLen(s, 5); got != 3 {
		t.Fatalf("keep=5 -> %d, want 3", got)
	}
	if got := validPrefixLen(s, 6); got != 6 {
		t.Fatalf("keep=6 -> %d, want 6", got)
	}
}

func TestStreamAccumulatorSplitRune(t *testing.T) {
	// 多字节字符被 SSE 块切开：encoding/json 解码时会把非法字节替换成
	// U+FFFD，解析器要做的只是不恐慌、总量与增量保持一致。
	acc := &streamAccumulator{}
	half1 := string([]byte("文档")[:2])
	half2 := string([]byte("文档")[2:])
	d1 := acc.feed(sse(`{"choices":[{"delta":{"content":"读` + half1 + `"}}]}`))
	d2 := acc.feed(sse(`{"choices":[{"delta":{"content":` + jsonQuote(half2) + `}}]}`))
	tail := acc.finish()
	full := d1.Text + d2.Text + tail.Text
	if full != acc.text.String() {
		t.Fatalf("delta 拼接 %q != 总量 %q", full, acc.text.String())
	}
	if !utf8.ValidString(full) {
		t.Fatalf("产出必须是合法 UTF-8: %q", full)
	}
}

// jsonQuote 生成 JSON 字符串字面量，测试里拼增量用。
func jsonQuote(s string) string {
	data, _ := json.Marshal(s)
	return string(data)
}

func TestWireMessagesCarriesToolRound(t *testing.T) {
	a := New()
	a.ai = aiSettings{BaseURL: "https://example.test/v1", Model: "m"}
	a.aiMessages = []chatMessage{
		{Role: "user", Content: "帮我改标题"},
		{Role: "assistant", Parts: []chatPart{
			{Kind: partThink, Text: "先读一下"},
			{Kind: partText, Text: "我先看看文档。"},
			{Kind: partTool, Tool: &toolCall{ID: "call_1", Name: "read_document", Args: "{}", Result: `{"ok":true}`, State: toolDone}},
		}},
		{Role: "assistant", Parts: []chatPart{{Kind: partText, Text: "改好了。"}}},
	}
	msgs := a.wireMessages()
	roles := make([]string, 0, len(msgs))
	for _, m := range msgs {
		role, _ := m["role"].(string)
		roles = append(roles, role)
	}
	want := []string{"system", "system", "user", "assistant", "tool", "assistant"}
	if strings.Join(roles, ",") != strings.Join(want, ",") {
		t.Fatalf("roles = %v", roles)
	}
	assistant := msgs[3]
	calls, ok := assistant["tool_calls"].([]map[string]any)
	if !ok || len(calls) != 1 {
		t.Fatalf("tool_calls = %+v", assistant["tool_calls"])
	}
	if calls[0]["id"] != "call_1" {
		t.Fatalf("call id = %+v", calls[0])
	}
	fn, _ := calls[0]["function"].(map[string]any)
	if fn["name"] != "read_document" {
		t.Fatalf("function = %+v", fn)
	}
	toolMsg := msgs[4]
	if toolMsg["tool_call_id"] != "call_1" || toolMsg["content"] != `{"ok":true}` {
		t.Fatalf("tool message = %+v", toolMsg)
	}
}

func TestAgentToolsOnWorkspace(t *testing.T) {
	dir := t.TempDir()
	doc := filepath.Join(dir, "notes.md")
	if err := os.WriteFile(doc, []byte("# 标题\n\n第一段\n\n第二段 提到 Muse\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	a := New()
	a.root = dir
	a.path = doc
	if data, err := os.ReadFile(doc); err != nil {
		t.Fatal(err)
	} else {
		a.loadDoc(string(data))
	}

	// list_documents
	out := a.execToolOnMain("list_documents", "{}")
	if !out.OK || !strings.Contains(out.Content, "notes.md") {
		t.Fatalf("list = %+v", out)
	}

	// read_document 默认读当前文档
	out = a.execToolOnMain("read_document", "{}")
	if !out.OK || !strings.Contains(out.Content, "第二段 提到 Muse") {
		t.Fatalf("read = %+v", out)
	}
	if !strings.Contains(out.Content, "1|") {
		t.Fatalf("read 应带行号: %+v", out)
	}

	// search_document
	out = a.execToolOnMain("search_document", `{"query":"muse"}`)
	if !out.OK || !strings.Contains(out.Content, `"line":5`) {
		t.Fatalf("search = %+v", out)
	}

	// read_outline
	out = a.execToolOnMain("read_outline", "{}")
	if !out.OK || !strings.Contains(out.Content, "标题") {
		t.Fatalf("outline = %+v", out)
	}

	// edit_document：当前文档，替换后编辑器与磁盘同步
	out = a.execToolOnMain("edit_document", `{"old_text":"第二段","new_text":"第二段（已改）"}`)
	if !out.OK {
		t.Fatalf("edit = %+v", out)
	}
	if !strings.Contains(a.source, "第二段（已改）") || !a.dirty {
		t.Fatalf("编辑器内容没有更新: %q dirty=%v", a.source, a.dirty)
	}
	disk, _ := os.ReadFile(doc)
	if strings.Contains(string(disk), "第二段（已改）") {
		t.Fatal("未保存的文档不应该被直接写盘")
	}

	// edit_document：old_text 不唯一要报错
	out = a.execToolOnMain("edit_document", `{"old_text":"段","new_text":"x"}`)
	if out.OK || !strings.Contains(out.Content, "2 次") {
		t.Fatalf("重复片段应报错: %+v", out)
	}

	// write_document 新建文件
	out = a.execToolOnMain("write_document", `{"path":"docs/todo.md","content":"# 待办\n"}`)
	if !out.OK {
		t.Fatalf("write = %+v", out)
	}
	if _, err := os.Stat(filepath.Join(dir, "docs", "todo.md")); err != nil {
		t.Fatalf("新文件不存在: %v", err)
	}

	// read_document 读别的文件
	out = a.execToolOnMain("read_document", `{"path":"docs/todo.md"}`)
	if !out.OK || !strings.Contains(out.Content, "待办") {
		t.Fatalf("read other = %+v", out)
	}

	// insert_into_document
	out = a.execToolOnMain("insert_into_document", `{"text":"## 结尾","position":"end"}`)
	if !out.OK || !strings.Contains(a.source, "## 结尾") {
		t.Fatalf("insert end = %+v source=%q", out, a.source)
	}
	out = a.execToolOnMain("insert_into_document", `{"text":"开头的话","position":"start"}`)
	if !out.OK || !strings.HasPrefix(a.source, "开头的话") {
		t.Fatalf("insert start = %+v source=%q", out, a.source)
	}

	// 越界路径要被拒绝
	out = a.execToolOnMain("read_document", `{"path":"../outside.md"}`)
	if out.OK {
		t.Fatalf("越界路径应被拒绝: %+v", out)
	}
}

func TestAgentToolsUnsavedDoc(t *testing.T) {
	a := New()
	a.loadDoc("# 草稿\n\n还没保存\n")
	out := a.execToolOnMain("read_document", "{}")
	if !out.OK || !strings.Contains(out.Content, "还没保存") {
		t.Fatalf("read unsaved = %+v", out)
	}
	out = a.execToolOnMain("read_document", `{"path":"x.md"}`)
	if out.OK {
		t.Fatalf("没有工作区时相对路径应报错: %+v", out)
	}
}

func TestNormalizeChatParts(t *testing.T) {
	msgs := []chatMessage{{Role: "user", Content: "hi"}, {Role: "assistant", Content: "hello"}}
	normalizeChatParts(msgs)
	if len(msgs[1].Parts) != 1 || msgs[1].Parts[0].Kind != partText || msgs[1].Parts[0].Text != "hello" {
		t.Fatalf("parts = %+v", msgs[1].Parts)
	}
	if msgs[1].textContent() != "hello" {
		t.Fatalf("textContent = %q", msgs[1].textContent())
	}
}

func TestAIChatPersistence(t *testing.T) {
	dir := t.TempDir()
	old := os.Getenv("MUSE_CONFIG_DIR")
	t.Setenv("MUSE_CONFIG_DIR", dir)
	defer os.Setenv("MUSE_CONFIG_DIR", old)

	msgs := []chatMessage{
		{Role: "user", Content: "hi"},
		{Role: "assistant", Parts: []chatPart{
			{Kind: partThink, Text: "想一想", Ms: 3200},
			{Kind: partTool, Tool: &toolCall{ID: "c1", Name: "read_document", Args: "{}", State: toolDone, Result: `{"ok":true}`}},
			{Kind: partText, Text: "done"},
		}},
	}
	if err := saveAIChat(msgs); err != nil {
		t.Fatal(err)
	}
	got := loadAIChat()
	if len(got) != 2 {
		t.Fatalf("chat = %+v", got)
	}
	if len(got[1].Parts) != 3 || got[1].Parts[0].Kind != partThink || got[1].Parts[0].Ms != 3200 {
		t.Fatalf("think part = %+v", got[1].Parts)
	}
	if got[1].Parts[1].Tool == nil || got[1].Parts[1].Tool.Name != "read_document" {
		t.Fatalf("tool part = %+v", got[1].Parts[1])
	}
	if got[1].textContent() != "done" {
		t.Fatalf("textContent = %q", got[1].textContent())
	}
}

func TestAISettingsRoundTrip(t *testing.T) {
	dir := t.TempDir()
	old := os.Getenv("MUSE_CONFIG_DIR")
	t.Setenv("MUSE_CONFIG_DIR", dir)
	defer os.Setenv("MUSE_CONFIG_DIR", old)

	want := aiSettings{BaseURL: "https://example.test/v1", APIKey: "sk-test", Model: "m", System: "sys"}
	if err := saveAISettings(want); err != nil {
		t.Fatal(err)
	}
	fi, err := os.Stat(filepath.Join(dir, "ai.json"))
	if err != nil {
		t.Fatal(err)
	}
	// Windows does not carry Unix permission bits: os.WriteFile(0600) shows
	// as 0666 there, so only the Unix builds can assert the mode.
	if runtime.GOOS != "windows" && fi.Mode().Perm() != 0o600 {
		t.Fatalf("config mode = %v, want 0600", fi.Mode().Perm())
	}
	got := loadAISettings()
	if got != want {
		t.Fatalf("loaded = %+v, want %+v", got, want)
	}
}

func TestAIServiceMagpiePreset(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("MUSE_CONFIG_DIR", dir)
	a := New()
	a.openSettings()
	a.settingsTab = 2
	tt := ui.NewTester(a.View, 1200, 800)
	tt.Frame()
	if !tt.HasText("Magpie 本地网关") {
		t.Fatalf("provider switch missing; texts=%q", tt.Texts())
	}
	if err := tt.Click("Magpie 本地网关"); err != nil {
		t.Fatalf("choose magpie: %v", err)
	}
	tt.Frame()
	if !a.ai.usesMagpie() || a.ai.BaseURL != magpieGatewayBaseURL {
		t.Fatalf("magpie preset = %+v", a.ai)
	}
	if !tt.HasText(magpieGatewayBaseURL) {
		t.Fatalf("gateway address not shown; texts=%q", tt.Texts())
	}
	if err := tt.Click("保存模型设置"); err != nil {
		t.Fatalf("save: %v", err)
	}
	got := loadAISettings()
	if got.Provider != aiProviderMagpie || got.BaseURL != magpieGatewayBaseURL {
		t.Fatalf("saved = %+v", got)
	}

	if err := tt.Click("自定义"); err != nil {
		t.Fatalf("choose custom: %v", err)
	}
	tt.Frame()
	if a.ai.usesMagpie() || a.ai.BaseURL != "" {
		t.Fatalf("custom switch left %+v", a.ai)
	}
}

func TestAIViewShowsSetupWhenUnconfigured(t *testing.T) {
	a := New()
	a.panel = panelAI
	a.panelOpen = true
	a.panelWidth = a.panelTargetWidth()
	a.ai = aiSettings{}
	tt := ui.NewTester(a.View, 1200, 800)
	tt.Frame()
	if !tt.HasText("设置模型") {
		t.Fatalf("setup prompt missing; texts=%q", tt.Texts())
	}
}

func TestAIViewShowsUserMessage(t *testing.T) {
	a := New()
	a.panel = panelAI
	a.panelOpen = true
	a.panelWidth = a.panelTargetWidth()
	a.ai = aiSettings{BaseURL: "https://example.test/v1", Model: "m"}
	a.aiMessages = []chatMessage{{Role: "user", Content: "hello"}}
	tt := ui.NewTester(a.View, 1200, 800)
	tt.Frame()
	if !tt.HasText("hello") {
		t.Fatalf("message missing; texts=%q", tt.Texts())
	}
}

// TestAIViewRendersAgentParts 检查思考摘要、工具行与正文同时渲染。
func TestAIViewRendersAgentParts(t *testing.T) {
	a := New()
	a.panel = panelAI
	a.panelOpen = true
	a.panelWidth = a.panelTargetWidth()
	a.ai = aiSettings{BaseURL: "https://example.test/v1", Model: "m"}
	a.aiMessages = []chatMessage{
		{Role: "user", Content: "帮我看看"},
		{Role: "assistant", Parts: []chatPart{
			{Kind: partThink, Text: "先读文档再总结", Ms: 3200},
			{Kind: partTool, Tool: &toolCall{ID: "c1", Name: "read_document", Args: `{"path":"notes.md"}`, State: toolDone, Result: `{"ok":true}`}},
			{Kind: partText, Text: "这篇文档讲的是……"},
		}},
	}
	tt := ui.NewTester(a.View, 1200, 800)
	tt.Frame()
	for _, want := range []string{"已思考 3.2 秒", "读取文档", "notes.md", "帮我看看"} {
		if !tt.HasText(want) {
			t.Fatalf("missing %q; texts=%q", want, tt.Texts())
		}
	}
	// 工具行点开能看到参数
	if err := tt.Click("读取文档"); err != nil {
		t.Fatalf("click tool row: %v", err)
	}
	tt.Frame()
	if !tt.HasText(`"path"`) {
		t.Fatalf("tool args not shown after click; texts=%q", tt.Texts())
	}
}

// TestAIViewEmptyShowsSuggestions 空态给示例问题。
func TestAIViewEmptyShowsSuggestions(t *testing.T) {
	a := New()
	a.panel = panelAI
	a.panelOpen = true
	a.panelWidth = a.panelTargetWidth()
	a.ai = aiSettings{BaseURL: "https://example.test/v1", Model: "m"}
	tt := ui.NewTester(a.View, 1200, 800)
	tt.Frame()
	if !tt.HasText("总结这篇文档") {
		t.Fatalf("suggestions missing; texts=%q", tt.Texts())
	}
	if err := tt.Click("总结这篇文档"); err != nil {
		t.Fatalf("click suggestion: %v", err)
	}
	if a.aiInput != "总结这篇文档" {
		t.Fatalf("aiInput = %q", a.aiInput)
	}
}

// TestAIComposerStaysPinned 内容再长，输入区也要钉在面板底部。
func TestAIComposerStaysPinned(t *testing.T) {
	probe := func(contentLen int) (modelY float64, ok bool) {
		a := New()
		a.panel = panelAI
		a.panelOpen = true
		a.panelWidth = a.panelTargetWidth()
		a.ai = aiSettings{BaseURL: "https://example.test/v1", Model: "very-long-model-name"}
		if contentLen > 0 {
			long := strings.Repeat("这是一段很长的内容，用来把对话撑出面板可视区。\n\n", 120)
			a.aiMessages = []chatMessage{
				{Role: "user", Content: "你好"},
				{Role: "assistant", Parts: []chatPart{{Kind: partText, Text: long}}},
			}
		}
		tt := ui.NewTester(a.View, 348, 800)
		tt.Frame()
		r, found := tt.Find("very-long-model-name")
		return float64(r.Y), found
	}
	shortY, ok1 := probe(0)
	longY, ok2 := probe(120)
	if !ok1 || !ok2 {
		t.Fatalf("模型名没渲染出来: %v %v", ok1, ok2)
	}
	if longY > 800 {
		t.Fatalf("输入区被挤出面板: y=%.1f", longY)
	}
	if shortY != longY {
		t.Fatalf("输入区位置随内容变化: %.1f → %.1f", shortY, longY)
	}
}

// TestAIViewShowsApprovalCard 修改类工具待确认时渲染审批卡：
// 标题、要做的修改、同意/拒绝。
func TestAIViewShowsApprovalCard(t *testing.T) {
	a := New()
	a.panel = panelAI
	a.panelOpen = true
	a.panelWidth = a.panelTargetWidth()
	a.ai = aiSettings{BaseURL: "https://example.test/v1", Model: "m"}
	a.aiMessages = []chatMessage{
		{Role: "user", Content: "帮我改一下"},
		{Role: "assistant", Parts: []chatPart{
			{Kind: partTool, Tool: &toolCall{
				ID:    "call_1",
				Name:  "edit_document",
				Args:  `{"old_text":"第一段","new_text":"第一段（改）"}`,
				State: toolApproval,
			}},
		}},
	}
	tt := ui.NewTester(a.View, 348, 800)
	tt.Frame()
	for _, want := range []string{"请求修改文档", "把「第一段」改成「第一段（改）」。", "同意修改", "总是允许", "拒绝"} {
		if !tt.HasText(want) {
			t.Fatalf("审批卡缺 %q; texts=%q", want, tt.Texts())
		}
	}
}

func TestInsertAIContentAppendsToDocument(t *testing.T) {
	a := New()
	a.loadDoc("# 无标题\n\n旧内容\n")
	a.insertAIContent("## AI 回答\n\n新段落")
	if !strings.Contains(a.source, "新段落") || !strings.Contains(a.source, "旧内容") {
		t.Fatalf("source = %q", a.source)
	}
	if !a.dirty {
		t.Fatal("document not marked dirty")
	}
}

// TestAIComposerPermMenu 底栏有权限预设下拉，选择即生效。
func TestAIComposerPermMenu(t *testing.T) {
	a := New()
	a.panel = panelAI
	a.panelOpen = true
	a.panelWidth = a.panelTargetWidth()
	a.ai = aiSettings{BaseURL: "https://example.test/v1", Model: "m"}
	tt := ui.NewTester(a.View, 348, 800)
	tt.Frame()
	if !tt.HasText("每次询问") {
		t.Fatalf("权限按钮缺省应显示每次询问; texts=%q", tt.Texts())
	}
	if err := tt.Click("每次询问"); err != nil {
		t.Fatalf("open perm menu: %v", err)
	}
	tt.Frame()
	if err := tt.ChooseMenuItem("完全访问"); err != nil {
		t.Fatalf("choose full access: %v", err)
	}
	tt.Frame()
	if a.aiPerm != permFull {
		t.Fatalf("aiPerm = %q, want full", a.aiPerm)
	}
	if !tt.HasText("完全访问") {
		t.Fatalf("按钮应显示完全访问; texts=%q", tt.Texts())
	}
}

// TestAgentToolsMoreOnWorkspace 覆盖新增的六类工具。
func TestAgentToolsMoreOnWorkspace(t *testing.T) {
	dir := t.TempDir()
	doc := filepath.Join(dir, "notes.md")
	if err := os.WriteFile(doc, []byte("# 笔记\n\n正文内容\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	a := New()
	a.root = dir
	a.path = doc
	a.loadDoc("# 笔记\n\n正文内容\n")

	// create_folder
	out := a.execToolOnMain("create_folder", `{"path":"archive/2024"}`)
	if !out.OK {
		t.Fatalf("create_folder = %+v", out)
	}
	if fi, err := os.Stat(filepath.Join(dir, "archive", "2024")); err != nil || !fi.IsDir() {
		t.Fatalf("文件夹不存在: %v", err)
	}

	// workspace_tree：能看到子目录（斜杠结尾）与文件
	out = a.execToolOnMain("workspace_tree", "{}")
	if !out.OK || !strings.Contains(out.Content, "archive/") || !strings.Contains(out.Content, "notes.md") {
		t.Fatalf("tree = %+v", out)
	}

	// document_stats
	out = a.execToolOnMain("document_stats", "{}")
	if !out.OK || !strings.Contains(out.Content, `"headings":1`) {
		t.Fatalf("stats = %+v", out)
	}

	// rename_document：改的不是当前文档
	if err := os.WriteFile(filepath.Join(dir, "old.md"), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out = a.execToolOnMain("rename_document", `{"path":"old.md","new_path":"archive/renamed.md"}`)
	if !out.OK {
		t.Fatalf("rename = %+v", out)
	}
	if _, err := os.Stat(filepath.Join(dir, "old.md")); !os.IsNotExist(err) {
		t.Fatal("旧文件应该已经不在了")
	}
	if _, err := os.Stat(filepath.Join(dir, "archive", "renamed.md")); err != nil {
		t.Fatalf("新文件不存在: %v", err)
	}

	// rename 当前文档：编辑器跟着换路径
	out = a.execToolOnMain("rename_document", `{"path":"notes.md","new_path":"notes-renamed.md"}`)
	if !out.OK {
		t.Fatalf("rename current = %+v", out)
	}
	if a.path != filepath.Join(dir, "notes-renamed.md") {
		t.Fatalf("当前文档路径没有跟随: %q", a.path)
	}

	// open_document：切到另一个已保存文档
	other := filepath.Join(dir, "other.md")
	if err := os.WriteFile(other, []byte("# Other\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out = a.execToolOnMain("open_document", `{"path":"other.md"}`)
	if !out.OK {
		t.Fatalf("open_document = %+v", out)
	}
	if a.path != other || a.source != "# Other\n" {
		t.Fatalf("open_document 没有切过去: %q %q", a.path, a.source)
	}

	// delete_document：走回收站桩，不真删
	trashed := ""
	old := trashItem
	trashItem = func(p string) error { trashed = p; return nil }
	defer func() { trashItem = old }()
	out = a.execToolOnMain("delete_document", `{"path":"other.md"}`)
	if !out.OK || trashed != other {
		t.Fatalf("delete = %+v trashed=%q", out, trashed)
	}

	// 删除当前文档且有未保存修改：拒绝（上一步 resetDoc 清了当前文档，
	// 重新指回一个再标记脏）
	a.path = filepath.Join(dir, "notes-renamed.md")
	a.source = "# 笔记\n\n未保存\n"
	a.dirty = true
	out = a.execToolOnMain("delete_document", "{}")
	if out.OK || !strings.Contains(out.Content, "未保存") {
		t.Fatalf("脏文档应拒绝删除: %+v", out)
	}
}

// TestAgentToolSchemasFilter 没开启的能力组不进工具定义表。
func TestAgentToolSchemasFilter(t *testing.T) {
	all := agentToolSchemas(func(string) bool { return true })
	if len(all) != len(agentToolDefs()) {
		t.Fatalf("全开时应有全部工具: %d", len(all))
	}
	browseOnly := agentToolSchemas(func(g string) bool { return g == toolGroupBrowse })
	for _, s := range browseOnly {
		fn := s["function"].(map[string]any)
		if toolGroup(fn["name"].(string)) != toolGroupBrowse {
			t.Fatalf("过滤漏了 %v", fn["name"])
		}
	}
}

// TestAgentToolsSettingsToggles 设置页的能力开关：点击即翻转并落盘。
func TestAgentToolsSettingsToggles(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("MUSE_CONFIG_DIR", dir)
	a := New()
	a.openSettings()
	a.settingsTab = 2
	tt := ui.NewTester(a.View, 1200, 800)
	tt.Frame()
	// 能力组在模型页末尾，先滚动到它
	tt.Scroll(600, 400, 0, 600)
	tt.Frame()
	if err := tt.Click("允许管理文件"); err != nil {
		t.Fatalf("click switch: %v", err)
	}
	if a.ai.Tools.Manage {
		t.Fatal("开关应翻为关闭")
	}
	got := loadAISettings()
	if got.Tools.Manage {
		t.Fatal("翻转应立即落盘")
	}
	if err := tt.Click("允许管理文件"); err != nil {
		t.Fatalf("click switch again: %v", err)
	}
	if !loadAISettings().Tools.Manage {
		t.Fatal("再次翻转应恢复开启")
	}
}

// TestAIMdBlocksCache checks that a Markdown part drawn again with the
// same text is not parsed again, and that another text is.
func TestAIMdBlocksCache(t *testing.T) {
	a := &App{}
	first := a.aiMdBlocks("**hello**")
	if len(first) == 0 {
		t.Fatal("no blocks parsed")
	}
	again := a.aiMdBlocks("**hello**")
	if len(again) == 0 || &again[0] != &first[0] {
		t.Fatal("the same text was parsed again")
	}
	other := a.aiMdBlocks("**bye**")
	if len(other) == 0 || &other[0] == &first[0] {
		t.Fatal("another text reused the blocks of the first")
	}
	if a.aiMdNext != 2 {
		t.Fatalf("aiMdNext = %d, want 2 parses", a.aiMdNext)
	}
}
