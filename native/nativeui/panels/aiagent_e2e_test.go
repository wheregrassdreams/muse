package nativeui

// aiagent_e2e_test.go — 用一个假的 OpenAI 兼容端点走完整个 Agent 循环：
// 思考 → 正文 → 工具调用 → 执行 → 二轮正文，验证段落、工具结果与
// 消息状态都对。

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

// sseChunk 包装一个 delta JSON 为 SSE 事件。
func sseChunk(delta string) string {
	return "data: " + delta + "\n\n"
}

// fakeOpenAI 第 1 轮：思考 + 正文 + read_document 工具调用；
// 第 2 轮：只有正文。请求体里能看到 tools 与回传的 tool 结果。
func fakeOpenAI(t *testing.T, docSource string, got *[][]byte) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Messages []map[string]any `json:"messages"`
			Tools    []map[string]any `json:"tools"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode body: %v", err)
			return
		}
		*got = append(*got, nil) // 占位，下面填原始内容
		raw, _ := json.Marshal(body)
		(*got)[len(*got)-1] = raw

		w.Header().Set("Content-Type", "text/event-stream")
		f := w.(http.Flusher)
		if len(body.Tools) == 0 {
			// 降级通道：不应该在测试里走到
			t.Errorf("endpoint got no tools")
		}
		round := len(*got)
		if round == 1 {
			fmt.Fprint(w, sseChunk(`{"choices":[{"delta":{"reasoning_content":"先读文档"}}]}`))
			f.Flush()
			fmt.Fprint(w, sseChunk(`{"choices":[{"delta":{"content":"我先看看文档。"}}]}`))
			f.Flush()
			fmt.Fprint(w, sseChunk(`{"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"read_document","arguments":"{}"}}]}}]}`))
			f.Flush()
			fmt.Fprint(w, sseChunk(`{"choices":[{"delta":{},"finish_reason":"tool_calls"}]}`))
			f.Flush()
			fmt.Fprint(w, "data: [DONE]\n\n")
			f.Flush()
			return
		}
		fmt.Fprint(w, sseChunk(`{"choices":[{"delta":{"content":"文档读完了，这是一篇测试文档。"}}]}`))
		f.Flush()
		fmt.Fprint(w, "data: [DONE]\n\n")
		f.Flush()
	})
}

func TestAgentLoopEndToEnd(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("MUSE_CONFIG_DIR", dir)

	var got [][]byte
	doc := "# 测试文档\n\nAgent 循环测试。\n"
	srv := httptest.NewServer(fakeOpenAI(t, doc, &got))
	defer srv.Close()

	a := New()
	a.root = dir
	docPath := dir + "/notes.md"
	if err := os.WriteFile(docPath, []byte(doc), 0o644); err != nil {
		t.Fatal(err)
	}
	a.path = docPath
	a.loadDoc(doc)
	a.ai = aiSettings{BaseURL: srv.URL, APIKey: "k", Model: "m", Tools: defaultAITools()}
	a.aiInput = "总结一下这篇文档"
	a.sendAI()

	deadline := time.Now().Add(5 * time.Second)
	for a.aiBusy && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if a.aiBusy {
		t.Fatal("agent loop did not finish in time")
	}
	if a.aiErr != "" {
		t.Fatalf("unexpected error: %q", a.aiErr)
	}

	// 两条请求：第一轮带 tools，第二轮带工具结果。
	if len(got) != 2 {
		t.Fatalf("requests = %d, want 2", len(got))
	}
	var first struct {
		Messages []map[string]any `json:"messages"`
		Tools    []map[string]any `json:"tools"`
	}
	if err := json.Unmarshal(got[0], &first); err != nil {
		t.Fatal(err)
	}
	if len(first.Tools) == 0 {
		t.Fatal("first request should carry tools")
	}
	var second struct {
		Messages []map[string]any `json:"messages"`
	}
	if err := json.Unmarshal(got[1], &second); err != nil {
		t.Fatal(err)
	}
	roles := make([]string, 0, len(second.Messages))
	toolContent := ""
	for _, m := range second.Messages {
		role, _ := m["role"].(string)
		roles = append(roles, role)
		if role == "tool" {
			toolContent, _ = m["content"].(string)
		}
	}
	want := "system,system,user,assistant,tool"
	if strings.Join(roles, ",") != want {
		t.Fatalf("second round roles = %v", roles)
	}
	if !strings.Contains(toolContent, "测试文档") {
		t.Fatalf("tool result missing doc content: %q", toolContent)
	}

	// 界面消息：1 用户 + 2 助手；第一轮有思考、正文与已完成的工具段。
	if len(a.aiMessages) != 3 {
		t.Fatalf("messages = %d", len(a.aiMessages))
	}
	turn1 := a.aiMessages[1]
	if len(turn1.Parts) < 3 {
		t.Fatalf("turn1 parts = %+v", turn1.Parts)
	}
	if turn1.Parts[0].Kind != partThink || turn1.Parts[0].Text != "先读文档" {
		t.Fatalf("think part = %+v", turn1.Parts[0])
	}
	if turn1.Parts[0].Ms < 0 {
		t.Fatalf("think duration missing")
	}
	tool := turn1.Parts[2].Tool
	if tool == nil || tool.Name != "read_document" || tool.State != toolDone {
		t.Fatalf("tool part = %+v", turn1.Parts[2])
	}
	if !strings.Contains(tool.Result, "Agent 循环测试") {
		t.Fatalf("tool result = %q", tool.Result)
	}
	if got := a.aiMessages[2].textContent(); got != "文档读完了，这是一篇测试文档。" {
		t.Fatalf("turn2 text = %q", got)
	}
}

// TestAgentLoopDegradesWithoutTools 端点不认 tools 时退回纯对话。
func TestAgentLoopDegradesWithoutTools(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("MUSE_CONFIG_DIR", dir)

	var sawTools []bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Tools []map[string]any `json:"tools"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		sawTools = append(sawTools, len(body.Tools) > 0)
		if len(body.Tools) > 0 {
			http.Error(w, `{"error":{"message":"server does not support tools"}}`, http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, sseChunk(`{"choices":[{"delta":{"content":"好的。"}}]}`))
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer srv.Close()

	a := New()
	a.ai = aiSettings{BaseURL: srv.URL, Model: "m", Tools: defaultAITools()}
	a.aiInput = "在吗"
	a.sendAI()

	deadline := time.Now().Add(5 * time.Second)
	for a.aiBusy && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if a.aiBusy {
		t.Fatal("agent loop did not finish in time")
	}
	if a.aiErr != "" {
		t.Fatalf("unexpected error: %q", a.aiErr)
	}
	if len(sawTools) != 2 || !sawTools[0] || sawTools[1] {
		t.Fatalf("tool attempts = %v", sawTools)
	}
	if !a.aiToolsUnsupported {
		t.Fatal("degrade flag not set")
	}
	if got := a.aiMessages[1].textContent(); got != "好的。" {
		t.Fatalf("answer = %q", got)
	}
}

// approvalFixture 搭一个两轮的假端点：第一轮发起 edit_document，
// 第二轮把模型收到的 tool 结果回显成正文，方便断言。
func approvalFixture(t *testing.T, secondRound func(toolContent string) string) (srv *httptest.Server, requests *[][]byte) {
	requests = &[][]byte{}
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Messages []map[string]any `json:"messages"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		*requests = append(*requests, nil)
		raw, _ := json.Marshal(body)
		(*requests)[len(*requests)-1] = raw
		w.Header().Set("Content-Type", "text/event-stream")
		if len(body.Messages) < 5 { // 第一轮：system,system,user,assistant,tool 还没齐
			fmt.Fprint(w, sseChunk(`{"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"edit_document","arguments":"{\"old_text\":\"旧标题\",\"new_text\":\"新标题\"}"}}]}}]}`))
			fmt.Fprint(w, sseChunk(`{"choices":[{"delta":{},"finish_reason":"tool_calls"}]}`))
			fmt.Fprint(w, "data: [DONE]\n\n")
			return
		}
		toolContent := ""
		for _, m := range body.Messages {
			if m["role"] == "tool" {
				toolContent, _ = m["content"].(string)
			}
		}
		answer := secondRound(toolContent)
		fmt.Fprint(w, sseChunk(`{"choices":[{"delta":{"content":`+jsonQuote(answer)+`}}]}`))
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	return srv, requests
}

// approvalApp 起一个带文档的 app，按给定权限预设发出会触发审批的请求。
func approvalApp(t *testing.T, srv *httptest.Server, perm aiPerm, setup ...func(*App)) (*App, *ui.Tester) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("MUSE_CONFIG_DIR", dir)
	doc := dir + "/notes.md"
	if err := os.WriteFile(doc, []byte("# 旧标题\n\n正文\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	a := New()
	a.root = dir
	a.path = doc
	a.loadDoc("# 旧标题\n\n正文\n")
	a.panel = panelAI
	a.panelWidth = a.panelTargetWidth()
	a.aiPerm = perm // 必须在 sendAI 前设好，循环会立刻读到
	a.ai = aiSettings{BaseURL: srv.URL, Model: "m", Tools: defaultAITools()}
	a.aiInput = "把标题改掉"
	for _, fn := range setup {
		fn(a) // 设置要在 sendAI 前定好：循环一开始就读它们
	}
	a.sendAI()
	tt := ui.NewTester(a.View, 348, 800)
	return a, tt
}

// waitApproval 等到出现待确认的工具段。
func waitApproval(t *testing.T, a *App, tt *ui.Tester) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		for i := range a.aiMessages {
			for j := range a.aiMessages[i].Parts {
				p := &a.aiMessages[i].Parts[j]
				if p.Kind == partTool && p.Tool != nil && p.Tool.State == toolApproval {
					tt.Frame()
					return
				}
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("没有等到审批卡")
}

// waitIdle 等循环结束。
func waitIdle(t *testing.T, a *App) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for a.aiBusy && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if a.aiBusy {
		t.Fatal("agent loop did not finish in time")
	}
}

func TestAgentApprovalReject(t *testing.T) {
	var answer string
	srv, _ := approvalFixture(t, func(toolContent string) string {
		answer = toolContent
		return "知道了。"
	})
	defer srv.Close()
	a, tt := approvalApp(t, srv, permAsk)

	waitApproval(t, a, tt)
	if a.source != "# 旧标题\n\n正文\n" {
		t.Fatalf("确认前文档就被改了: %q", a.source)
	}
	if err := tt.Click("拒绝"); err != nil {
		t.Fatalf("click reject: %v", err)
	}
	waitIdle(t, a)

	if strings.Contains(a.source, "新标题") {
		t.Fatalf("拒绝后文档不应该被修改: %q", a.source)
	}
	if !strings.Contains(answer, "拒绝") {
		t.Fatalf("模型没收到拒绝信息: %q", answer)
	}
	if got := a.aiMessages[1].Parts[0].Tool; got == nil || got.State != toolFailed {
		t.Fatalf("rejected tool part = %+v", a.aiMessages[1].Parts[0])
	}
}

func TestAgentApprovalAccept(t *testing.T) {
	srv, _ := approvalFixture(t, func(string) string { return "改好了。" })
	defer srv.Close()
	a, tt := approvalApp(t, srv, permAsk)

	waitApproval(t, a, tt)
	if err := tt.Click("同意修改"); err != nil {
		t.Fatalf("click approve: %v", err)
	}
	waitIdle(t, a)

	if !strings.Contains(a.source, "# 新标题") {
		t.Fatalf("同意后文档应该更新: %q", a.source)
	}
	if !a.dirty {
		t.Fatal("同意后的修改应标记未保存")
	}
	if got := a.aiMessages[1].Parts[0].Tool; got == nil || got.State != toolDone {
		t.Fatalf("approved tool part = %+v", a.aiMessages[1].Parts[0])
	}
}

// TestAgentApprovalAlways 点一次「总是允许」后，本轮会话的后续修改
// 不再询问，连着的两次编辑都直接执行。
func TestAgentApprovalAlways(t *testing.T) {
	var rounds []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Messages []map[string]any `json:"messages"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		w.Header().Set("Content-Type", "text/event-stream")
		n := len(body.Messages)
		switch {
		case n < 5: // 第一轮：发起第一处修改
			rounds = append(rounds, "edit1")
			fmt.Fprint(w, sseChunk(`{"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"edit_document","arguments":"{\"old_text\":\"旧标题\",\"new_text\":\"新标题\"}"}}]}}]}`))
			fmt.Fprint(w, sseChunk(`{"choices":[{"delta":{},"finish_reason":"tool_calls"}]}`))
			fmt.Fprint(w, "data: [DONE]\n\n")
		case n < 7: // 第二轮：再发起一处修改，这时应该已免确认
			rounds = append(rounds, "edit2")
			fmt.Fprint(w, sseChunk(`{"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_2","type":"function","function":{"name":"edit_document","arguments":"{\"old_text\":\"正文\",\"new_text\":\"正文改\"}"}}]}}]}`))
			fmt.Fprint(w, sseChunk(`{"choices":[{"delta":{},"finish_reason":"tool_calls"}]}`))
			fmt.Fprint(w, "data: [DONE]\n\n")
		default:
			rounds = append(rounds, "final")
			fmt.Fprint(w, sseChunk(`{"choices":[{"delta":{"content":"两处都改好了。"}}]}`))
			fmt.Fprint(w, "data: [DONE]\n\n")
		}
	}))
	defer srv.Close()
	a, tt := approvalApp(t, srv, permAsk)

	waitApproval(t, a, tt)
	if err := tt.Click("总是允许"); err != nil {
		t.Fatalf("click always: %v", err)
	}
	waitIdle(t, a)

	if a.aiPerm != permFull {
		t.Fatalf("总是允许应切到完全访问: %q", a.aiPerm)
	}
	if !strings.Contains(a.source, "# 新标题") || !strings.Contains(a.source, "正文改") {
		t.Fatalf("两处修改都应该执行: %q", a.source)
	}
	if len(rounds) != 3 {
		t.Fatalf("应走满两轮工具加一轮正文: %v", rounds)
	}
	for i := range a.aiMessages {
		for j := range a.aiMessages[i].Parts {
			p := &a.aiMessages[i].Parts[j]
			if p.Kind == partTool && p.Tool != nil && p.Tool.State == toolApproval {
				t.Fatalf("第二处修改不应再询问: %+v", p.Tool)
			}
		}
	}
}

// TestAgentReadonlyRejectsEdits 只读预设下修改类工具直接拒绝，
// 拒绝原因回传给模型，文档不动。
func TestAgentReadonlyRejectsEdits(t *testing.T) {
	var answer string
	srv, _ := approvalFixture(t, func(toolContent string) string {
		answer = toolContent
		return "好的，只读模式我不改文档。"
	})
	defer srv.Close()

	a, _ := approvalApp(t, srv, permReadonly)
	waitIdle(t, a)

	if strings.Contains(a.source, "新标题") {
		t.Fatalf("只读模式下文档不应被修改: %q", a.source)
	}
	if !strings.Contains(answer, "只读") {
		t.Fatalf("模型没收到只读说明: %q", answer)
	}
	tool := a.aiMessages[1].Parts[0].Tool
	if tool == nil || tool.State != toolFailed {
		t.Fatalf("readonly tool part = %+v", a.aiMessages[1].Parts[0])
	}
	if !strings.Contains(tool.Err, "只读") {
		t.Fatalf("tool err = %q", tool.Err)
	}
}

// TestAgentManageDisabled 设置里没开「管理文件」时，删除调用被
// 能力门禁拒绝，文件完好，拒绝原因回传给模型。
func TestAgentManageDisabled(t *testing.T) {
	var answer string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Messages []map[string]any `json:"messages"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		w.Header().Set("Content-Type", "text/event-stream")
		if len(body.Messages) < 5 {
			fmt.Fprint(w, sseChunk(`{"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"delete_document","arguments":"{\"path\":\"notes.md\"}"}}]}}]}`))
			fmt.Fprint(w, sseChunk(`{"choices":[{"delta":{},"finish_reason":"tool_calls"}]}`))
			fmt.Fprint(w, "data: [DONE]\n\n")
			return
		}
		for _, m := range body.Messages {
			if m["role"] == "tool" {
				answer, _ = m["content"].(string)
			}
		}
		fmt.Fprint(w, sseChunk(`{"choices":[{"delta":{"content":"好的，不动文件。"}}]}`))
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer srv.Close()

	a, _ := approvalApp(t, srv, permAsk, func(a *App) {
		a.ai.Tools = aiTools{Browse: true, Edit: true, Manage: false}
	})
	waitIdle(t, a)

	if _, err := os.Stat(a.path); err != nil {
		t.Fatalf("文件应该还在: %v", err)
	}
	if !strings.Contains(answer, "管理文件") {
		t.Fatalf("模型没收到能力关闭说明: %q", answer)
	}
	tool := a.aiMessages[1].Parts[0].Tool
	if tool == nil || tool.State != toolFailed {
		t.Fatalf("gated tool part = %+v", a.aiMessages[1].Parts[0])
	}
}
