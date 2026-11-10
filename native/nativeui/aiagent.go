package nativeui

// aiagent.go — Muse 的写作 Agent：在流式对话之上加一层工具循环。
//
// 一轮助手回复被拆成若干段（chatPart），对应 Codex 式的三种渲染：
//   - partThink：思考过程（reasoning_content 或 <think> 包裹），流式折叠展示
//   - partTool ：一次工具调用，行内显示状态，可展开参数与结果
//   - partText ：正文，Markdown 流式渲染
//
// 线程约定：Agent 循环跑在流式 goroutine 上，所有 UI 状态的读写都经
// onMain 回到主线程；流解析在 goroutine 上进行，只把增量拷贝回主线程。

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/egoist/mygo/ui"

	"muse/mdoc"
	"muse/notestore"
	"muse/services"
)

// partKind 是助手回复里一段内容的类型。
type partKind string

const (
	partText  partKind = "text"
	partThink partKind = "think"
	partTool  partKind = "tool"
)

// toolState 是一次工具调用的状态。
type toolState string

const (
	toolRunning  toolState = "running"
	toolApproval toolState = "approval" // 等用户确认要不要执行
	toolDone     toolState = "done"
	toolFailed   toolState = "failed"
)

// toolCall 记录一次工具调用及其结果。
type toolCall struct {
	ID     string    `json:"id,omitempty"`
	Name   string    `json:"name,omitempty"`
	Args   string    `json:"args,omitempty"`
	Result string    `json:"result,omitempty"`
	Err    string    `json:"err,omitempty"`
	State  toolState `json:"state,omitempty"`
}

// chatPart 是助手消息里的一段：思考、正文或工具调用。
// open/started 是渲染状态，json 序列化天然跳过非导出字段。
type chatPart struct {
	Kind partKind  `json:"kind"`
	Text string    `json:"text,omitempty"`
	Tool *toolCall `json:"tool,omitempty"`
	Ms   int64     `json:"ms,omitempty"` // 思考段耗时

	open    bool
	started time.Time
}

// chatMessage 是一条对话消息；助手消息由若干 chatPart 组成。
// Role/Content 沿用旧 chat.json 的大写字段名，老文件能直接读进来。
type chatMessage struct {
	Role    string     `json:"Role"`
	Content string     `json:"Content,omitempty"`
	Parts   []chatPart `json:"Parts,omitempty"`
}

// textContent 是发给模型的正文：用户消息即原文，助手消息拼接正文段；
// 没有段落的旧数据回落到 Content。
func (m chatMessage) textContent() string {
	if m.Role != "assistant" {
		return m.Content
	}
	var b strings.Builder
	for _, p := range m.Parts {
		if p.Kind == partText {
			b.WriteString(p.Text)
		}
	}
	if b.Len() > 0 || len(m.Parts) > 0 {
		return b.String()
	}
	return m.Content
}

// toolCalls 按段序取出这条消息的工具调用。
func (m chatMessage) toolCalls() []toolCall {
	var calls []toolCall
	for _, p := range m.Parts {
		if p.Kind == partTool && p.Tool != nil {
			calls = append(calls, *p.Tool)
		}
	}
	return calls
}

// normalizeChatParts 给旧格式的助手消息（只有 Content）补一个正文段，
// 让渲染只需面对 Parts。
func normalizeChatParts(msgs []chatMessage) {
	for i := range msgs {
		m := &msgs[i]
		if m.Role == "assistant" && len(m.Parts) == 0 && strings.TrimSpace(m.Content) != "" {
			m.Parts = []chatPart{{Kind: partText, Text: m.Content}}
		}
	}
}

// ---------------------------------------------------------------------------
// 流解析

const (
	thinkOpen  = "<think>"
	thinkClose = "</think>"
)

// streamTool 是流里正在累积的一次工具调用。
type streamTool struct {
	Index int
	ID    string
	Name  string
	args  strings.Builder
}

// streamToolDelta 告诉 UI 流里新出现了一次工具调用。
type streamToolDelta struct {
	Index int
	ID    string
	Name  string
}

// streamDelta 是一批 SSE 事件新产生的增量，全部是拷贝，可跨线程。
type streamDelta struct {
	Think string
	Text  string
	Tools []streamToolDelta
}

// streamAccumulator 把一轮 SSE 流拼成思考、正文与工具调用。
// 兼容三种形态：delta.reasoning_content（DeepSeek 系）、delta.reasoning
// （OpenRouter 系）与正文里的 <think> 包裹；<think> 标签被切在两个
// 增量之间也能接上。
type streamAccumulator struct {
	text    strings.Builder
	think   strings.Builder
	inThink bool
	pending string
	tools   []*streamTool
}

// feed 消费一个原始 SSE 事件块，返回其中的增量。
func (acc *streamAccumulator) feed(raw string) streamDelta {
	var d streamDelta
	var dText, dThink strings.Builder
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "" || data == "[DONE]" {
			continue
		}
		var payload struct {
			Choices []struct {
				Delta struct {
					Content          *string `json:"content"`
					ReasoningContent *string `json:"reasoning_content"`
					Reasoning        *string `json:"reasoning"`
					ToolCalls        []struct {
						Index    *int   `json:"index"`
						ID       string `json:"id"`
						Function struct {
							Name      string `json:"name"`
							Arguments string `json:"arguments"`
						} `json:"function"`
					} `json:"tool_calls"`
				} `json:"delta"`
			} `json:"choices"`
		}
		if json.Unmarshal([]byte(data), &payload) != nil || len(payload.Choices) == 0 {
			continue
		}
		delta := payload.Choices[0].Delta
		if delta.ReasoningContent != nil {
			acc.emitThink(*delta.ReasoningContent, &dThink)
		}
		if delta.Reasoning != nil {
			acc.emitThink(*delta.Reasoning, &dThink)
		}
		if delta.Content != nil && *delta.Content != "" {
			acc.feedContent(*delta.Content, &dText, &dThink)
		}
		for _, tc := range delta.ToolCalls {
			idx := 0
			if tc.Index != nil {
				idx = *tc.Index
			}
			t := acc.toolAt(idx)
			if tc.ID != "" && t.ID == "" {
				t.ID = tc.ID
			}
			if tc.Function.Name != "" && t.Name == "" {
				t.Name = tc.Function.Name
				d.Tools = append(d.Tools, streamToolDelta{Index: idx, ID: t.ID, Name: t.Name})
			}
			if tc.Function.Arguments != "" {
				t.args.WriteString(tc.Function.Arguments)
			}
		}
	}
	d.Text, d.Think = dText.String(), dThink.String()
	return d
}

// finish 收尾：把扣下的尾巴按当前状态吐完。
func (acc *streamAccumulator) finish() streamDelta {
	var d streamDelta
	var dText, dThink strings.Builder
	if acc.pending != "" {
		if acc.inThink {
			acc.emitThink(acc.pending, &dThink)
		} else {
			// 流结束了，不再等可能的半个标签
			acc.emitText(acc.pending, &dText)
		}
		acc.pending = ""
	}
	d.Text, d.Think = dText.String(), dThink.String()
	return d
}

// toolAt 返回第 i 个工具调用的累积状态，没有就建。
func (acc *streamAccumulator) toolAt(i int) *streamTool {
	for _, t := range acc.tools {
		if t.Index == i {
			return t
		}
	}
	t := &streamTool{Index: i}
	acc.tools = append(acc.tools, t)
	return t
}

// feedContent 路由一段正文增量：<think> 包裹进思考，其余进正文。
// 扫描时只扣下可能是半个标签的尾巴（从最后一个 '<' 起），再把切在
// 多字节字符中间的边界往前挪，下一批拼回去。
func (acc *streamAccumulator) feedContent(s string, text, think *strings.Builder) {
	acc.pending += s
	for {
		tag := thinkOpen
		if acc.inThink {
			tag = thinkClose
		}
		if i := strings.Index(acc.pending, tag); i >= 0 {
			if acc.inThink {
				acc.emitThink(acc.pending[:i], think)
			} else {
				acc.emitText(acc.pending[:i], text)
			}
			acc.pending = acc.pending[i+len(tag):]
			acc.inThink = !acc.inThink
			continue
		}
		keep := len(acc.pending) - tagTailLen(acc.pending, tag)
		keep = validPrefixLen(acc.pending, keep)
		if keep > 0 {
			if acc.inThink {
				acc.emitThink(acc.pending[:keep], think)
			} else {
				acc.emitText(acc.pending[:keep], text)
			}
			acc.pending = acc.pending[keep:]
		}
		return
	}
}

// emitThink 把一段思考记入总量与增量。
func (acc *streamAccumulator) emitThink(s string, d *strings.Builder) {
	if s == "" {
		return
	}
	acc.think.WriteString(s)
	d.WriteString(s)
}

// emitText 把一段正文记入总量与增量。
func (acc *streamAccumulator) emitText(s string, d *strings.Builder) {
	if s == "" {
		return
	}
	acc.text.WriteString(s)
	d.WriteString(s)
}

// tagTailLen 返回 s 末尾与 tag 前缀重合的长度（0 到 len(tag)-1）：
// 只有这段可能是被切断的标签，要扣到下一批；其余可以立刻吐出。
func tagTailLen(s, tag string) int {
	n := len(tag) - 1
	if len(s) < n {
		n = len(s)
	}
	for ; n > 0; n-- {
		if strings.HasSuffix(s, tag[:n]) {
			return n
		}
	}
	return 0
}

// validPrefixLen 把切在多字节字符中间的边界往前挪，保证 s[:keep] 是
// 完整的 UTF-8。
func validPrefixLen(s string, keep int) int {
	for keep > 0 && !utf8.ValidString(s[:keep]) {
		keep--
	}
	return keep
}

// ---------------------------------------------------------------------------
// Agent 循环

const agentMaxTurns = 8

const agentSystemPrompt = `你是 Muse 的写作 Agent。Muse 是一款 Markdown 编辑器，工作区里的文档你可以用工具查看、搜索和编辑；修改文档的工具（edit_document / write_document / insert_into_document）需要用户确认才会执行。
编辑文档前先用 read_document 或 search_document 确认原文，再用 edit_document 做精确替换：old_text 必须与文档原文逐字一致且唯一。
回答用简体中文，正文用 Markdown，简洁直接；不要复述你调用过的工具，用户看得到。`

// sendAI 发出一次 Agent 对话：用户消息入列后，后台循环跑
// 流式回复 → 工具调用 → 再流式回复，直到给出正文或到轮数上限。
func (a *App) sendAI() {
	if a.aiBusy || !a.aiConfigured() {
		return
	}
	text := strings.TrimSpace(a.aiInput)
	if text == "" {
		return
	}
	a.aiMessages = append(a.aiMessages, chatMessage{Role: "user", Content: text})
	a.aiInput = ""
	a.aiErr = ""
	a.aiBusy = true
	a.aiPhase = "思考中…"
	a.aiStop.Store(false)
	a.scrollAIBottom()
	a.invalidate()
	go a.agentLoop()
}

// abortAI 停下当前请求与后续工具轮。
func (a *App) abortAI() {
	a.aiStop.Store(true)
	if a.aiRequestID == "" {
		return
	}
	a.aiClient.ChatAbort(struct {
		RequestID string `json:"requestId"`
	}{RequestID: a.aiRequestID})
}

// agentLoop 依次跑每一轮，直到结束、出错、中止或到轮数上限。
func (a *App) agentLoop() {
	more := true
	for turn := 0; turn < agentMaxTurns && more; turn++ {
		more = a.agentTurn()
	}
	if more {
		a.appendAgentNote("已连续调用多轮工具，先停在这里；需要的话让我继续。")
	}
	a.onMain(func() {
		a.aiBusy = false
		a.aiPhase = ""
		a.aiApprovals = nil
		_ = saveAIChat(a.aiMessages)
		a.invalidate()
	})
}

// turnPlan 是一轮请求在主线程取好的全部参数：消息、按能力组过滤后
// 的工具定义、模型与端点。循环 goroutine 之后只读这份拷贝。
type turnPlan struct {
	msgs    []map[string]any
	schemas []map[string]any
	model   string
	baseURL string
	apiKey  string
}

func (a *App) planTurn() turnPlan {
	var p turnPlan
	a.onMain(func() {
		p.msgs = a.wireMessages()
		p.schemas = agentToolSchemas(a.toolGroupEnabled)
		p.model = a.ai.Model
		p.baseURL = a.ai.BaseURL
		p.apiKey = a.ai.APIKey
	})
	return p
}

// toolGroupEnabled 报告一个能力组是否开启（主线程调用）。
func (a *App) toolGroupEnabled(group string) bool {
	switch group {
	case toolGroupBrowse:
		return a.ai.Tools.Browse
	case toolGroupEdit:
		return a.ai.Tools.Edit
	case toolGroupManage:
		return a.ai.Tools.Manage
	}
	return true
}

// toolGroupEnabledOffMain 是循环 goroutine 用的版本。
func (a *App) toolGroupEnabledOffMain(group string) bool {
	var on bool
	a.onMain(func() { on = a.toolGroupEnabled(group) })
	return on
}

// agentTurn 跑一轮流式请求：思考与正文增量实时进 UI；流结束时若有
// 工具调用就依次执行并返回 true 继续下一轮。
func (a *App) agentTurn() bool {
	useTools := !a.aiToolsUnsupported
	acc := &streamAccumulator{}
	plan := a.planTurn()
	if plan.msgs == nil {
		return false
	}
	idx := a.beginAssistantTurn()
	if idx < 0 {
		return false
	}
	reqID := fmt.Sprintf("agent-%d", time.Now().UnixNano())
	a.setRequestID(reqID)
	body := map[string]any{
		"model":    plan.model,
		"stream":   true,
		"messages": plan.msgs,
	}
	if useTools {
		body["tools"] = plan.schemas
	}
	res := a.chatStream(reqID, plan, body, idx, acc)

	// 端点可能不认识 function calling（llama.cpp、旧网关会报 4xx 或
	// "tools" 字样）：这轮去掉工具重试一次，之后整个会话都纯对话。
	if !res.Ok && !res.Aborted && useTools && toolsLookUnsupported(a.aiErrText()) {
		a.aiToolsUnsupported = true
		a.dropAssistantTurn(idx)
		a.clearAIErr()
		return a.agentTurn()
	}

	tail := acc.finish()
	tools := acc.tools
	a.onMain(func() {
		if tail.Text != "" || tail.Think != "" {
			a.applyStreamDelta(idx, tail)
		}
		a.tidyAssistant(idx)
		for _, t := range tools {
			a.setToolArgs(idx, t.Index, t.ID, t.Name, t.args.String())
		}
	})
	if res.Aborted || a.aiStop.Load() {
		return false
	}
	if !res.Ok {
		return false // 错误已经落到 a.aiErr
	}
	if len(tools) == 0 {
		return false // 正文给出，对话结束
	}
	for _, t := range tools {
		if a.aiStop.Load() {
			return false
		}
		// 能力组没开：直接拒绝，别让模型硬试。
		if group := toolGroup(t.Name); group != "" && !a.toolGroupEnabledOffMain(group) {
			a.setToolOutcome(idx, t.Index, t.ID,
				toolFail("当前设置没有开启「%s」能力；请告诉用户可以在 设置 → 模型 → Agent 能力 里开启", toolGroupLabel(group)))
			continue
		}
		if toolNeedsApproval(t.Name) {
			// 权限预设决定怎么走：只读直接拒绝，完全访问直接执行，
			// 每次询问弹审批卡。
			switch perm := a.currentPerm(); perm {
			case permReadonly:
				a.setToolOutcome(idx, t.Index, t.ID,
					toolFail("当前权限预设是只读，不能修改文档；请把想改的内容直接写给用户，或让用户把权限切换到「每次询问」或「完全访问」"))
				continue
			case permAsk:
				dec, aborted := a.requestApproval(idx, t)
				if aborted {
					a.setToolOutcome(idx, t.Index, t.ID, toolFail("已中止"))
					return false
				}
				if dec == approvalDeny {
					// 拒绝也回传给模型，让它改道而不是硬试。
					a.setToolOutcome(idx, t.Index, t.ID,
						toolFail("用户拒绝了这次修改；不要原样重试，先问用户想怎么改"))
					continue
				}
			}
		}
		out := a.execTool(t.Name, t.args.String())
		a.setToolOutcome(idx, t.Index, t.ID, out)
	}
	return !a.aiStop.Load()
}

// toolNeedsApproval 报告一个工具是否要用户确认后才能执行：
// 凡是会改工作区的都要确认，只读的直接放行。
func toolNeedsApproval(name string) bool {
	switch name {
	case "edit_document", "write_document", "insert_into_document",
		"rename_document", "delete_document", "create_folder":
		return true
	}
	return false
}

// aiPerm 是修改类工具的权限预设。
type aiPerm string

const (
	permAsk      aiPerm = "ask"      // 每次询问（默认）
	permFull     aiPerm = "full"     // 完全访问：免确认
	permReadonly aiPerm = "readonly" // 只读：修改类工具直接拒绝
)

var aiPerms = []aiPerm{permAsk, permFull, permReadonly}

// label 是权限预设的界面名。
func (p aiPerm) label() string {
	switch p {
	case permFull:
		return "完全访问"
	case permReadonly:
		return "只读"
	default:
		return "每次询问"
	}
}

// currentPerm 读一次权限预设（循环 goroutine 安全）。
func (a *App) currentPerm() aiPerm {
	var p aiPerm
	a.onMain(func() { p = a.aiPerm })
	if p == "" {
		return permAsk
	}
	return p
}

// approvalDecision 是审批卡上用户能给的三种决定。
type approvalDecision int

const (
	approvalDeny   approvalDecision = iota // 这次不行
	approvalAllow                          // 这次可以
	approvalAlways                         // 这次可以，之后本会话都免确认
)

// requestApproval 把一次工具调用挂成「待确认」，阻塞等用户决定。
// 返回 (决定, 是否中止)。
func (a *App) requestApproval(msgIdx int, t *streamTool) (dec approvalDecision, aborted bool) {
	decided := make(chan approvalDecision, 1)
	a.onMain(func() {
		if a.aiApprovals == nil {
			a.aiApprovals = map[string]chan approvalDecision{}
		}
		a.aiApprovals[t.ID] = decided
		if p := a.toolPart(msgIdx, t.Index, t.ID); p != nil && p.Tool != nil {
			p.Tool.State = toolApproval
		}
		a.aiPhase = "等待确认…"
		a.scrollAIBottom()
		a.invalidate()
	})
	for {
		select {
		case dec := <-decided:
			a.onMain(func() {
				delete(a.aiApprovals, t.ID)
				if dec == approvalAlways {
					// 卡上的「总是允许」= 本会话切到完全访问。
					a.aiPerm = permFull
				}
				a.aiPhase = ""
				a.invalidate()
			})
			return dec, false
		case <-time.After(120 * time.Millisecond):
			if a.aiStop.Load() {
				a.onMain(func() {
					delete(a.aiApprovals, t.ID)
					a.aiPhase = ""
					a.invalidate()
				})
				return approvalDeny, true
			}
		}
	}
}

// decideApproval 是审批按钮的落点：把用户的决定递给等待中的循环。
func (a *App) decideApproval(id string, dec approvalDecision) {
	a.onMain(func() {
		if ch := a.aiApprovals[id]; ch != nil {
			select {
			case ch <- dec:
			default:
			}
		}
	})
}

// chatStream 发起一轮流式请求，把增量落到 idx 这条助手消息上。
func (a *App) chatStream(reqID string, plan turnPlan, body map[string]any, idx int, acc *streamAccumulator) services.ChatResult {
	return a.aiClient.ChatStreamWith(services.ChatStreamRequest{
		RequestID: reqID,
		BaseURL:   plan.baseURL,
		APIKey:    plan.apiKey,
		Body:      body,
	}, services.ChatSink{
		Chunk: func(raw string) {
			d := acc.feed(raw)
			if d.Think != "" || d.Text != "" || len(d.Tools) > 0 {
				a.onMain(func() { a.applyStreamDelta(idx, d) })
			}
		},
		Error: func(message string) {
			a.onMain(func() { a.aiErr = message })
		},
	})
}

// aiErrText 读一次当前错误文本。
func (a *App) aiErrText() string {
	var s string
	a.onMain(func() { s = a.aiErr })
	return s
}

// clearAIErr 清掉上一轮留下的错误文本（重试前）。
func (a *App) clearAIErr() {
	a.onMain(func() { a.aiErr = "" })
}

// toolsLookUnsupported 猜测这次失败是不是端点不支持 function calling。
func toolsLookUnsupported(message string) bool {
	m := strings.ToLower(message)
	return strings.Contains(m, "http 4") || strings.Contains(m, "tool")
}

// dropAssistantTurn 撤掉一条还没有任何内容的助手消息（重试前清理）。
func (a *App) dropAssistantTurn(idx int) {
	a.onMain(func() {
		if idx < 0 || idx >= len(a.aiMessages) {
			return
		}
		if m := a.aiMessages[idx]; m.Role == "assistant" && len(m.Parts) == 0 {
			a.aiMessages = append(a.aiMessages[:idx], a.aiMessages[idx+1:]...)
		}
		a.invalidate()
	})
}

// wireMessages 组装 OpenAI 兼容的消息列表：系统提示、文档上下文，
// 以及由段落重建的 assistant/tool 消息。
func (a *App) wireMessages() []map[string]any {
	msgs := make([]map[string]any, 0, len(a.aiMessages)+4)
	if sys := strings.TrimSpace(a.ai.System); sys != "" {
		msgs = append(msgs, map[string]any{"role": "system", "content": sys})
	} else {
		msgs = append(msgs, map[string]any{"role": "system", "content": agentSystemPrompt})
	}
	if ctx := a.agentDocContext(); ctx != "" {
		msgs = append(msgs, map[string]any{"role": "system", "content": ctx})
	}
	for _, m := range a.aiMessages {
		if m.Role == "user" {
			if strings.TrimSpace(m.Content) != "" {
				msgs = append(msgs, map[string]any{"role": "user", "content": m.Content})
			}
			continue
		}
		text := m.textContent()
		calls := m.toolCalls()
		if text == "" && len(calls) == 0 {
			continue
		}
		am := map[string]any{"role": "assistant"}
		if text != "" {
			am["content"] = text
		}
		if len(calls) > 0 {
			tcs := make([]map[string]any, 0, len(calls))
			for _, c := range calls {
				tcs = append(tcs, map[string]any{
					"id":   c.ID,
					"type": "function",
					"function": map[string]any{
						"name":      c.Name,
						"arguments": c.Args,
					},
				})
			}
			am["tool_calls"] = tcs
		}
		msgs = append(msgs, am)
		for _, c := range calls {
			// Err/Result 都是执行时生成的 JSON，直接回传。
			content := c.Result
			if c.Err != "" {
				content = c.Err
			}
			msgs = append(msgs, map[string]any{"role": "tool", "tool_call_id": c.ID, "content": content})
		}
	}
	return msgs
}

// agentDocContext 是随每轮发送的文档上下文，让模型知道它面对什么。
func (a *App) agentDocContext() string {
	var b strings.Builder
	fmt.Fprintf(&b, "当前时间：%s\n", time.Now().Format("2006-01-02 15:04"))
	if a.path != "" {
		fmt.Fprintf(&b, "当前文档：%s\n", a.displayName())
		fmt.Fprintf(&b, "文档路径：%s\n", a.path)
	} else {
		b.WriteString("当前文档：未命名（尚未保存）\n")
	}
	if a.root != "" {
		fmt.Fprintf(&b, "工作区根目录：%s\n", a.root)
	}
	if a.dirty {
		b.WriteString("文档有未保存的修改；read_document/edit_document 看到的就是编辑器里的最新内容。\n")
	}
	if len(a.headings) > 0 {
		b.WriteString("文档大纲：\n")
		for _, h := range a.headings {
			fmt.Fprintf(&b, "%s- %s\n", strings.Repeat("  ", max(h.level-1, 0)), h.text)
		}
	}
	if off := a.disabledGroupLabels(); len(off) > 0 {
		fmt.Fprintf(&b, "以下能力未开启，对应工具不可用，不要尝试调用：%s。\n", strings.Join(off, "、"))
	}
	return b.String()
}

// disabledGroupLabels 是设置里没开启的能力组名（主线程调用）。
func (a *App) disabledGroupLabels() []string {
	var off []string
	for _, g := range []string{toolGroupBrowse, toolGroupEdit, toolGroupManage} {
		if !a.toolGroupEnabled(g) {
			off = append(off, toolGroupLabel(g))
		}
	}
	return off
}

// beginAssistantTurn 在主线程追加一条空的助手消息，返回它的下标。
func (a *App) beginAssistantTurn() int {
	idx := -1
	a.onMain(func() {
		a.aiMessages = append(a.aiMessages, chatMessage{Role: "assistant"})
		idx = len(a.aiMessages) - 1
		a.scrollAIBottom()
		a.invalidate()
	})
	return idx
}

func (a *App) setRequestID(id string) {
	a.onMain(func() {
		a.aiRequestID = id
		a.invalidate()
	})
}

// applyStreamDelta 把一批流式增量落到消息段落上（主线程）。
func (a *App) applyStreamDelta(idx int, d streamDelta) {
	if idx < 0 || idx >= len(a.aiMessages) {
		return
	}
	m := &a.aiMessages[idx]
	if d.Think != "" {
		p := a.appendPart(m, partThink)
		p.Text += d.Think
		if p.started.IsZero() {
			p.started = time.Now()
		}
		p.open = true // 思考流式期间保持展开
		a.aiPhase = "深度思考中…"
	}
	if d.Text != "" {
		a.closeThinking(m)
		p := a.appendPart(m, partText)
		p.Text += d.Text
		a.aiPhase = "正在生成…"
	}
	for _, td := range d.Tools {
		a.closeThinking(m)
		m.Parts = append(m.Parts, chatPart{Kind: partTool, Tool: &toolCall{
			ID:    td.ID,
			Name:  td.Name,
			State: toolRunning,
		}})
		if td.Name != "" {
			a.aiPhase = "调用 " + toolLabel(td.Name) + "…"
		}
	}
	a.scrollAIBottom()
	a.invalidate()
}

// appendPart 返回消息末尾可续写的同类段：只有末段同类型才续写，
// 否则新开一段，保证段落顺序与流的到达顺序一致。
func (a *App) appendPart(m *chatMessage, k partKind) *chatPart {
	if n := len(m.Parts); n > 0 && m.Parts[n-1].Kind == k {
		return &m.Parts[n-1]
	}
	m.Parts = append(m.Parts, chatPart{Kind: k})
	return &m.Parts[len(m.Parts)-1]
}

// closeThinking 收束还在流式中的思考段，记下耗时并折叠。
func (a *App) closeThinking(m *chatMessage) {
	for i := range m.Parts {
		p := &m.Parts[i]
		if p.Kind == partThink && p.Ms == 0 && !p.started.IsZero() {
			p.Ms = time.Since(p.started).Milliseconds()
			p.open = false
		}
	}
}

// tidyAssistant 去掉流结束时仍无内容的空段。
func (a *App) tidyAssistant(idx int) {
	if idx < 0 || idx >= len(a.aiMessages) {
		return
	}
	m := &a.aiMessages[idx]
	kept := make([]chatPart, 0, len(m.Parts))
	for _, p := range m.Parts {
		empty := p.Text == "" && (p.Tool == nil || p.Tool.Name == "")
		if !empty {
			kept = append(kept, p)
		}
	}
	m.Parts = kept
	a.invalidate()
}

// appendAgentNote 在最后一条助手消息后补一段说明文字。
func (a *App) appendAgentNote(text string) {
	a.onMain(func() {
		for i := len(a.aiMessages) - 1; i >= 0; i-- {
			if a.aiMessages[i].Role == "assistant" {
				a.appendPart(&a.aiMessages[i], partText).Text += "\n\n" + text
				a.invalidate()
				return
			}
		}
	})
}

// setToolArgs 把流结束时确定的最终参数落到对应工具段（主线程）。
func (a *App) setToolArgs(msgIdx, deltaIndex int, id, name, args string) {
	if p := a.toolPart(msgIdx, deltaIndex, id); p != nil {
		if name != "" {
			p.Tool.Name = name
		}
		p.Tool.ID = id
		p.Tool.Args = args
		a.invalidate()
	}
}

// setToolOutcome 记录一次工具调用的结果（主线程）。
func (a *App) setToolOutcome(msgIdx, deltaIndex int, id string, out toolOutcome) {
	a.onMain(func() {
		p := a.toolPart(msgIdx, deltaIndex, id)
		if p == nil || p.Tool == nil {
			return
		}
		if out.OK {
			p.Tool.State = toolDone
			p.Tool.Result = out.Content
		} else {
			p.Tool.State = toolFailed
			p.Tool.Err = out.Content
		}
		a.aiPhase = ""
		a.invalidate()
	})
}

// toolPart 按 ID（缺省时按出现序）找到消息里对应的工具段。
func (a *App) toolPart(msgIdx, deltaIndex int, id string) *chatPart {
	if msgIdx < 0 || msgIdx >= len(a.aiMessages) {
		return nil
	}
	m := &a.aiMessages[msgIdx]
	ordinal := 0
	for i := range m.Parts {
		p := &m.Parts[i]
		if p.Kind != partTool || p.Tool == nil {
			continue
		}
		if id != "" && p.Tool.ID == id {
			return p
		}
		if id == "" && ordinal == deltaIndex {
			return p
		}
		ordinal++
	}
	return nil
}

// ---------------------------------------------------------------------------
// 工具

// lineFeed 换行符，几个工具拼多行输出用。
const lineFeed = "\n"

// toolOutcome 是一次工具执行的结果：Content 是回传给模型的 JSON。
type toolOutcome struct {
	OK      bool
	Content string
}

func toolOK(v map[string]any) toolOutcome {
	if v == nil {
		v = map[string]any{}
	}
	v["ok"] = true
	data, _ := json.Marshal(v)
	return toolOutcome{OK: true, Content: string(data)}
}

func toolErrorJSON(msg string) string {
	data, _ := json.Marshal(map[string]any{"ok": false, "error": msg})
	return string(data)
}

func toolFail(format string, args ...any) toolOutcome {
	return toolOutcome{OK: false, Content: toolErrorJSON(fmt.Sprintf(format, args...))}
}

// execTool 在主线程执行一次工具调用（流式 goroutine 阻塞等待）。
func (a *App) execTool(name, argsJSON string) toolOutcome {
	var out toolOutcome
	a.onMain(func() { out = a.execToolOnMain(name, argsJSON) })
	return out
}

// 工具按能力组划分，设置里可以按组开关；"" 表示始终可用。
const (
	toolGroupBrowse = "browse"
	toolGroupEdit   = "edit"
	toolGroupManage = "manage"
)

// toolGroup 是一个工具所属的能力组。
func toolGroup(name string) string {
	switch name {
	case "list_documents", "read_document", "search_document", "read_outline",
		"workspace_tree", "document_stats", "open_document":
		return toolGroupBrowse
	case "edit_document", "insert_into_document":
		return toolGroupEdit
	case "write_document", "rename_document", "delete_document", "create_folder":
		return toolGroupManage
	}
	return ""
}

// toolGroupLabel 是能力组的界面名。
func toolGroupLabel(group string) string {
	switch group {
	case toolGroupBrowse:
		return "浏览与搜索"
	case toolGroupEdit:
		return "编辑文档内容"
	case toolGroupManage:
		return "管理文件"
	}
	return group
}

// agentToolDef 是一条工具定义：定义本体 + 所属能力组。
type agentToolDef struct {
	group  string
	schema map[string]any
}

// agentToolDefs 是全部工具的定义表。
func agentToolDefs() []agentToolDef {
	str := func(desc string) map[string]any {
		return map[string]any{"type": "string", "description": desc}
	}
	fn := func(name, desc string, params map[string]any) map[string]any {
		return map[string]any{
			"type": "function",
			"function": map[string]any{
				"name":        name,
				"description": desc,
				"parameters": map[string]any{
					"type":       "object",
					"properties": params,
				},
			},
		}
	}
	return []agentToolDef{
		{toolGroupBrowse, fn("list_documents", "列出工作区里的文件与子目录，用来找文档。", map[string]any{
			"path": str("相对工作区根目录的子目录；缺省列出根目录"),
		})},
		{toolGroupBrowse, fn("read_document", "读一个文档的内容（带行号）。不传 path 读当前文档；文件较大时会被截断，可用 offset 续读。", map[string]any{
			"path":   str("相对工作区根目录的路径；缺省读当前文档"),
			"offset": map[string]any{"type": "integer", "description": "起始行号，从 1 开始"},
			"limit":  map[string]any{"type": "integer", "description": "最多读多少行，缺省 300"},
		})},
		{toolGroupBrowse, fn("search_document", "在一个文档里按行搜索文本，返回行号与该行内容。", map[string]any{
			"query":    str("要找的文本"),
			"path":     str("相对工作区根目录的路径；缺省搜当前文档"),
			"is_regex": map[string]any{"type": "boolean", "description": "把 query 当正则；缺省按普通文本、忽略大小写"},
		})},
		{toolGroupBrowse, fn("read_outline", "读一个文档的标题大纲（各级 Heading）。", map[string]any{
			"path": str("相对工作区根目录的路径；缺省读当前文档"),
		})},
		{toolGroupBrowse, fn("workspace_tree", "递归列出工作区的目录结构，用来了解全貌。", map[string]any{
			"path":  str("起点目录；缺省为工作区根目录"),
			"depth": map[string]any{"type": "integer", "description": "递归层数，缺省 2，最大 4"},
		})},
		{toolGroupBrowse, fn("document_stats", "统计一个文档的字符数、行数与标题数。", map[string]any{
			"path": str("相对工作区根目录的路径；缺省统计当前文档"),
		})},
		{toolGroupBrowse, fn("open_document", "把一个文档打开到编辑器里，方便用户查看。", map[string]any{
			"path": str("相对工作区根目录的路径"),
		})},
		{toolGroupEdit, fn("edit_document", "编辑文档：把 old_text 精确替换为 new_text。old_text 必须与原文逐字一致且只出现一次；可以先 read_document 核对。", map[string]any{
			"path":     str("相对工作区根目录的路径；缺省编辑当前文档"),
			"old_text": str("要被替换的原文片段，逐字一致且唯一"),
			"new_text": str("替换成的内容，可以为空表示删除"),
		})},
		{toolGroupEdit, fn("insert_into_document", "把一段 Markdown 插入当前文档的开头或结尾。", map[string]any{
			"text":     str("要插入的 Markdown"),
			"position": map[string]any{"type": "string", "enum": []string{"start", "end"}, "description": "缺省插到结尾"},
		})},
		{toolGroupManage, fn("write_document", "在工作区新建或整体改写一个 Markdown 文档。", map[string]any{
			"path":    str("相对工作区根目录的路径"),
			"content": str("完整的文档内容"),
		})},
		{toolGroupManage, fn("rename_document", "重命名或移动一个文件（可以移到别的子目录）。", map[string]any{
			"path":     str("现在的相对路径"),
			"new_path": str("新的相对路径"),
		})},
		{toolGroupManage, fn("delete_document", "把一个文件移到系统回收站（不是永久删除）。", map[string]any{
			"path": str("相对工作区根目录的路径"),
		})},
		{toolGroupManage, fn("create_folder", "在工作区新建一个文件夹（可含子目录）。", map[string]any{
			"path": str("相对工作区根目录的文件夹路径"),
		})},
	}
}

// agentToolSchemas 按开启的能力组过滤出这轮要发给模型的工具定义。
func agentToolSchemas(enabled func(group string) bool) []map[string]any {
	out := []map[string]any{}
	for _, d := range agentToolDefs() {
		if d.group == "" || enabled(d.group) {
			out = append(out, d.schema)
		}
	}
	return out
}

// toolLabel 是工具在界面上的中文名。
func toolLabel(name string) string {
	switch name {
	case "list_documents":
		return "列出文件"
	case "read_document":
		return "读取文档"
	case "search_document":
		return "搜索文档"
	case "read_outline":
		return "读取大纲"
	case "workspace_tree":
		return "目录结构"
	case "document_stats":
		return "文档统计"
	case "open_document":
		return "打开文档"
	case "edit_document":
		return "编辑文档"
	case "write_document":
		return "写入文档"
	case "insert_into_document":
		return "插入内容"
	case "rename_document":
		return "重命名文档"
	case "delete_document":
		return "删除文档"
	case "create_folder":
		return "新建文件夹"
	default:
		return name
	}
}

// toolIcon 是工具行用的图标。
func toolIcon(name string) *ui.SVG {
	switch name {
	case "list_documents":
		return iconFiles
	case "read_document":
		return iconEye
	case "search_document":
		return iconSearch
	case "read_outline":
		return iconOutline
	case "edit_document":
		return iconEdit
	case "write_document":
		return iconFilePlus
	case "insert_into_document":
		return iconNew
	default:
		return iconEdit
	}
}

// execToolOnMain 分发并执行工具；只会在主线程被调。
func (a *App) execToolOnMain(name, argsJSON string) toolOutcome {
	args := map[string]any{}
	if s := strings.TrimSpace(argsJSON); s != "" {
		if err := json.Unmarshal([]byte(s), &args); err != nil {
			return toolFail("参数不是合法 JSON：%v", err)
		}
	}
	argStr := func(key string) string {
		v, _ := args[key].(string)
		return v
	}
	argInt := func(key string, fallback int) int {
		f, ok := args[key].(float64)
		if !ok || f <= 0 {
			return fallback
		}
		return int(f)
	}
	switch name {
	case "list_documents":
		return a.toolListDocuments(argStr("path"))
	case "read_document":
		return a.toolReadDocument(argStr("path"), argInt("offset", 1), argInt("limit", 300))
	case "search_document":
		regex := false
		if v, ok := args["is_regex"].(bool); ok {
			regex = v
		}
		return a.toolSearchDocument(argStr("query"), argStr("path"), regex)
	case "read_outline":
		return a.toolReadOutline(argStr("path"))
	case "edit_document":
		return a.toolEditDocument(argStr("path"), argStr("old_text"), argStr("new_text"))
	case "write_document":
		return a.toolWriteDocument(argStr("path"), argStr("content"))
	case "insert_into_document":
		pos := strings.TrimSpace(argStr("position"))
		return a.toolInsertIntoDocument(argStr("text"), pos)
	case "rename_document":
		return a.toolRenameDocument(argStr("path"), argStr("new_path"))
	case "delete_document":
		return a.toolDeleteDocument(argStr("path"))
	case "create_folder":
		return a.toolCreateFolder(argStr("path"))
	case "workspace_tree":
		return a.toolWorkspaceTree(argStr("path"), argInt("depth", 2))
	case "open_document":
		return a.toolOpenDocument(argStr("path"))
	case "document_stats":
		return a.toolDocumentStats(argStr("path"))
	default:
		return toolFail("未知工具 %q", name)
	}
}

// dropTreeCache 让某个目录的列表缓存失效（并发安全）。
func (a *App) dropTreeCache(dir string) {
	a.dirMu.Lock()
	delete(a.dirs, dir)
	a.dirMu.Unlock()
}

// toolRenameDocument 重命名或移动一个文件；若它是当前文档，
// 编辑器跟着切到新路径。
func (a *App) toolRenameDocument(rel, relNew string) toolOutcome {
	if strings.TrimSpace(relNew) == "" {
		return toolFail("new_path 不能为空")
	}
	path, err := a.resolveToolPath(rel)
	if err != nil {
		return toolFail("%v", err)
	}
	newPath, err := a.resolveToolPath(relNew)
	if err != nil {
		return toolFail("%v", err)
	}
	if newPath == path {
		return toolOK(map[string]any{"path": newPath, "moved": false})
	}
	if !isEditableFile(newPath) {
		return toolFail("新路径必须是可编辑的文本文件（md / markdown / txt 等）")
	}
	if _, err := os.Stat(newPath); err == nil {
		return toolFail("目标路径已存在：%s", relNew)
	}
	if _, err := os.Stat(path); err != nil {
		return toolFail("%v", err)
	}
	if err := os.Rename(path, newPath); err != nil {
		return toolFail("重命名失败：%v", err)
	}
	a.dropTreeCache(filepath.Dir(path))
	a.dropTreeCache(filepath.Dir(newPath))
	if path == a.path {
		a.path = newPath
		a.selectedPath = newPath
		a.updateTitle()
	}
	a.addRecent(newPath, false)
	return toolOK(map[string]any{"path": newPath, "moved": true})
}

// toolDeleteDocument 把文件移进系统回收站；删当前文档时先落空文档。
func (a *App) toolDeleteDocument(rel string) toolOutcome {
	path, err := a.resolveToolPath(rel)
	if err != nil {
		return toolFail("%v", err)
	}
	if _, err := os.Stat(path); err != nil {
		return toolFail("路径不存在：%v", err)
	}
	if path == a.path && a.dirty {
		return toolFail("当前文档有未保存的修改；请先保存或放弃，再删除它")
	}
	if err := trashItem(path); err != nil {
		return toolFail("删除失败：%v", err)
	}
	a.dropTreeCache(filepath.Dir(path))
	if path == a.path {
		a.resetDoc()
	}
	return toolOK(map[string]any{"path": path, "trashed": true})
}

// toolCreateFolder 新建一个文件夹（可含子目录）。
func (a *App) toolCreateFolder(rel string) toolOutcome {
	if strings.TrimSpace(rel) == "" {
		return toolFail("path 不能为空")
	}
	path, err := a.resolveToolPath(rel)
	if err != nil {
		return toolFail("%v", err)
	}
	if _, err := os.Stat(path); err == nil {
		return toolFail("目录已存在：%s", rel)
	}
	if err := os.MkdirAll(path, 0o755); err != nil {
		return toolFail("创建失败：%v", err)
	}
	a.dropTreeCache(filepath.Dir(path))
	return toolOK(map[string]any{"path": path, "created": true})
}

// toolWorkspaceTree 递归列出目录结构。
func (a *App) toolWorkspaceTree(rel string, depth int) toolOutcome {
	root := a.root
	if strings.TrimSpace(rel) != "" {
		resolved, err := a.resolveToolPath(rel)
		if err != nil {
			return toolFail("%v", err)
		}
		root = resolved
	}
	if root == "" {
		return toolFail("没有打开的工作区")
	}
	if depth <= 0 || depth > 4 {
		depth = 2
	}
	var b strings.Builder
	count := 0
	truncated := false
	var walk func(dir, prefix string, d int)
	walk = func(dir, prefix string, d int) {
		des, err := os.ReadDir(dir)
		if err != nil {
			return
		}
		sort.SliceStable(des, func(i, j int) bool {
			di, dj := des[i].IsDir(), des[j].IsDir()
			if di != dj {
				return di
			}
			return strings.ToLower(des[i].Name()) < strings.ToLower(des[j].Name())
		})
		for _, de := range des {
			name := de.Name()
			if strings.HasPrefix(name, ".") || name == "node_modules" {
				continue
			}
			if count >= 200 {
				truncated = true
				return
			}
			count++
			if de.IsDir() {
				fmt.Fprintf(&b, "%s%s/"+lineFeed, prefix, name)
				if d < depth {
					walk(filepath.Join(dir, name), prefix+"  ", d+1)
				}
			} else {
				fmt.Fprintf(&b, "%s%s"+lineFeed, prefix, name)
			}
		}
	}
	walk(root, "", 1)
	return toolOK(map[string]any{
		"root":      root,
		"tree":      strings.TrimRight(b.String(), lineFeed),
		"entries":   count,
		"truncated": truncated,
	})
}

// toolOpenDocument 把一个文档打开到编辑器里。
func (a *App) toolOpenDocument(rel string) toolOutcome {
	path, err := a.resolveToolPath(rel)
	if err != nil {
		return toolFail("%v", err)
	}
	if !isEditableFile(path) {
		return toolFail("不是可编辑的文本文件")
	}
	if path == a.path {
		return toolOK(map[string]any{"path": path, "opened": true, "note": "已经是当前文档"})
	}
	a.requestOpen(path)
	return toolOK(map[string]any{"path": path, "opened": true})
}

// toolDocumentStats 统计字符数、行数与标题数。
func (a *App) toolDocumentStats(rel string) toolOutcome {
	path, content, err := a.toolTargetDoc(rel)
	if err != nil {
		return toolFail("%v", err)
	}
	headings := 0
	for _, block := range mdoc.Parse(content).Blocks {
		if block.Kind == mdoc.Heading {
			headings++
		}
	}
	return toolOK(map[string]any{
		"path":     path,
		"chars":    utf8.RuneCountInString(content),
		"lines":    strings.Count(content, lineFeed) + 1,
		"headings": headings,
	})
}

// resolveToolPath 把工具参数里的路径解析成绝对路径：空值指当前文档，
// 相对路径相对工作区根目录，结果必须落在工作区之内。
func (a *App) resolveToolPath(p string) (string, error) {
	p = strings.TrimSpace(p)
	if p == "" || p == "current" || p == "当前文档" {
		if a.path == "" {
			return "", errors.New("当前文档还没保存；请先保存，或给出工作区内的相对路径")
		}
		return a.path, nil
	}
	if filepath.IsAbs(p) {
		if a.root != "" && underDir(p, a.root) {
			return filepath.Clean(p), nil
		}
		return "", errors.New("只允许访问工作区内的文件")
	}
	root := a.root
	if root == "" {
		if a.path == "" {
			return "", errors.New("没有打开的工作区，也没有已保存的文档")
		}
		root = filepath.Dir(a.path)
	}
	full := filepath.Clean(filepath.Join(root, p))
	if !underDir(full, root) {
		return "", fmt.Errorf("路径 %q 越出了工作区", p)
	}
	return full, nil
}

// toolTargetDoc 解析工具的 path 参数，返回路径与内容。空路径指当前
// 文档：已保存或有草稿时直接用编辑器里的最新内容（未保存也能读）。
func (a *App) toolTargetDoc(rel string) (string, string, error) {
	p := strings.TrimSpace(rel)
	if p == "" || p == "current" || p == "当前文档" {
		if a.path == "" && a.source == "" {
			return "", "", errors.New("当前文档是空的；先写点内容，或给出工作区内的路径")
		}
		if a.path != "" {
			return a.path, a.source, nil
		}
		return "(未命名)", a.source, nil
	}
	path, err := a.resolveToolPath(p)
	if err != nil {
		return "", "", err
	}
	content, err := a.toolDocContent(path)
	if err != nil {
		return "", "", err
	}
	return path, content, nil
}

// underDir 报告 path 是否就在 dir 之内（dir 本身也算）。
func underDir(path, dir string) bool {
	path, dir = filepath.Clean(path), filepath.Clean(dir)
	if path == dir {
		return true
	}
	rel, err := filepath.Rel(dir, path)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// toolDocContent 取一个工具要读的文档内容：当前文档用编辑器里的
// 最新源码，其他文件读盘。返回内容与路径。
func (a *App) toolDocContent(path string) (string, error) {
	if path == a.path {
		return a.source, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	if len(data) > 2<<20 {
		return "", errors.New("文件超过 2MB，太大了")
	}
	if !isTextData(data) {
		return "", errors.New("不是文本文件")
	}
	return string(data), nil
}

func (a *App) toolListDocuments(rel string) toolOutcome {
	dir := a.root
	if rel != "" {
		resolved, err := a.resolveToolPath(rel)
		if err != nil {
			return toolFail("%v", err)
		}
		dir = resolved
	}
	if dir == "" {
		return toolFail("没有打开的工作区")
	}
	des, err := os.ReadDir(dir)
	if err != nil {
		return toolFail("读目录失败：%v", err)
	}
	type entry struct {
		Name string `json:"name"`
		Type string `json:"type"`
	}
	entries := make([]entry, 0, len(des))
	truncated := false
	for _, de := range des {
		name := de.Name()
		if strings.HasPrefix(name, ".") || name == "node_modules" {
			continue
		}
		if len(entries) >= 200 {
			truncated = true
			break
		}
		kind := "file"
		if de.IsDir() {
			kind = "dir"
		}
		entries = append(entries, entry{Name: name, Type: kind})
	}
	return toolOK(map[string]any{
		"path":      dir,
		"entries":   entries,
		"truncated": truncated,
	})
}

func (a *App) toolReadDocument(rel string, offset, limit int) toolOutcome {
	path, content, err := a.toolTargetDoc(rel)
	if err != nil {
		return toolFail("%v", err)
	}
	lines := strings.Split(content, "\n")
	total := len(lines)
	if offset < 1 {
		offset = 1
	}
	if offset > total {
		return toolFail("offset %d 超出了总行数 %d", offset, total)
	}
	if limit <= 0 || limit > 800 {
		limit = 800
	}
	end := min(offset+limit-1, total)
	truncated := end < total
	// 行数之外再按字节兜底，避免超长行撑爆上下文。
	var b strings.Builder
	bytes := 0
	for i := offset; i <= end; i++ {
		line := fmt.Sprintf("%5d| %s", i, lines[i-1])
		if bytes+len(line) > 20000 {
			truncated = true
			end = i - 1
			break
		}
		bytes += len(line)
		b.WriteString(line)
		b.WriteByte('\n')
	}
	return toolOK(map[string]any{
		"path":        path,
		"total_lines": total,
		"start_line":  offset,
		"end_line":    end,
		"truncated":   truncated,
		"content":     b.String(),
	})
}

func (a *App) toolSearchDocument(query, rel string, useRegex bool) toolOutcome {
	query = strings.TrimSpace(query)
	if query == "" {
		return toolFail("query 不能为空")
	}
	path, content, err := a.toolTargetDoc(rel)
	if err != nil {
		return toolFail("%v", err)
	}
	match := func(line string) bool {
		return strings.Contains(strings.ToLower(line), strings.ToLower(query))
	}
	if useRegex {
		re, err := regexp.Compile(query)
		if err != nil {
			return toolFail("正则无效：%v", err)
		}
		match = re.MatchString
	}
	type hit struct {
		Line int    `json:"line"`
		Text string `json:"text"`
	}
	hits := []hit{}
	for i, line := range strings.Split(content, "\n") {
		if !match(line) {
			continue
		}
		if len(hits) < 50 {
			text := strings.TrimSpace(line)
			if len(text) > 200 {
				text = text[:200] + "…"
			}
			hits = append(hits, hit{Line: i + 1, Text: text})
		} else {
			return toolOK(map[string]any{
				"path": path, "matches": len(hits), "truncated": true, "hits": hits,
			})
		}
	}
	return toolOK(map[string]any{
		"path": path, "matches": len(hits), "hits": hits,
	})
}

func (a *App) toolReadOutline(rel string) toolOutcome {
	path, content, err := a.toolTargetDoc(rel)
	if err != nil {
		return toolFail("%v", err)
	}
	var b strings.Builder
	count := 0
	for _, block := range mdoc.Parse(content).Blocks {
		if block.Kind != mdoc.Heading {
			continue
		}
		if count >= 100 {
			b.WriteString("…\n")
			break
		}
		fmt.Fprintf(&b, "H%d %s\n", block.Level, strings.TrimSpace(plainSpans(block.Inlines)))
		count++
	}
	if count == 0 {
		return toolOK(map[string]any{"path": path, "outline": "(没有标题)"})
	}
	return toolOK(map[string]any{"path": path, "outline": strings.TrimRight(b.String(), "\n")})
}

func (a *App) toolEditDocument(rel, oldText, newText string) toolOutcome {
	if oldText == "" {
		return toolFail("old_text 不能为空；先 read_document 核对原文，再逐字复制要替换的片段")
	}
	// 未保存的当前文档：直接改编辑器里的内容。
	p := strings.TrimSpace(rel)
	if p == "" || p == "current" || p == "当前文档" {
		updated, err := replaceOnce(a.source, oldText, newText)
		if err != nil {
			return toolFail("%v", err)
		}
		a.applyToolSourceEdit(updated)
		return toolOK(map[string]any{"path": "(当前文档)", "replaced": true, "new_chars": len(updated)})
	}
	path, err := a.resolveToolPath(p)
	if err != nil {
		return toolFail("%v", err)
	}
	content, err := a.toolDocContent(path)
	if err != nil {
		return toolFail("%v", err)
	}
	updated, err := replaceOnce(content, oldText, newText)
	if err != nil {
		return toolFail("%v", err)
	}
	if err := a.applyToolFileEdit(path, updated, notestore.Revision(content)); err != nil {
		return toolFail("%v", err)
	}
	return toolOK(map[string]any{
		"path":      path,
		"replaced":  true,
		"new_chars": len(updated),
	})
}

// replaceOnce 把 oldText 唯一地替换为 newText；找不到或出现多次都算错。
func replaceOnce(content, oldText, newText string) (string, error) {
	switch n := strings.Count(content, oldText); n {
	case 0:
		return "", errors.New("old_text 在文档里没有找到；请先 read_document 核对原文（注意空格与换行）")
	case 1:
		return strings.Replace(content, oldText, newText, 1), nil
	default:
		return "", fmt.Errorf("old_text 出现了 %d 次，请加长片段让它唯一", n)
	}
}

func (a *App) toolWriteDocument(rel, content string) toolOutcome {
	if strings.TrimSpace(rel) == "" {
		return toolFail("path 不能为空；相对工作区根目录，例如 notes/meeting.md")
	}
	path, err := a.resolveToolPath(rel)
	if err != nil {
		return toolFail("%v", err)
	}
	if !strings.HasSuffix(strings.ToLower(path), ".md") &&
		!strings.HasSuffix(strings.ToLower(path), ".markdown") &&
		!strings.HasSuffix(strings.ToLower(path), ".txt") {
		return toolFail("只支持 .md / .markdown / .txt 文件")
	}
	created := true
	if _, err := os.Stat(path); err == nil {
		created = false
	}
	if path == a.path {
		a.applyToolSourceEdit(content)
	} else {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return toolFail("创建目录失败：%v", err)
		}
		expected, err := a.destinationRevision(path, "", "")
		if err == nil {
			err = a.writeDocument(path, content, expected)
		}
		if err != nil {
			return toolFail("写入失败：%v", err)
		}
		a.dirMu.Lock()
		delete(a.dirs, filepath.Dir(path))
		a.dirMu.Unlock()
		a.invalidate()
	}
	return toolOK(map[string]any{
		"path":      path,
		"created":   created,
		"new_chars": len(content),
	})
}

func (a *App) toolInsertIntoDocument(text, position string) toolOutcome {
	if strings.TrimSpace(text) == "" {
		return toolFail("text 不能为空")
	}
	if a.path == "" && a.source == "" && a.root == "" {
		return toolFail("没有可写入的文档；先新建或打开一个")
	}
	if position == "start" {
		a.insertTextAtStart(text)
		return toolOK(map[string]any{"position": "start"})
	}
	a.insertAIContent(text)
	return toolOK(map[string]any{"position": "end"})
}

// applyToolSourceEdit 把工具改出的新内容落到当前文档：经编辑器作为一步
// 可撤销的修改写入，用户按 ⌘Z 就能退回 AI 的改动。
func (a *App) applyToolSourceEdit(content string) {
	a.editDoc(content)
}

// applyToolFileEdit 把工具改出的内容写到磁盘上的其他文件。
func (a *App) applyToolFileEdit(path, content, revision string) error {
	if err := a.writeDocument(path, content, revision); err != nil {
		return err
	}
	a.dirMu.Lock()
	delete(a.dirs, filepath.Dir(path))
	a.dirMu.Unlock()
	a.invalidate()
	return nil
}

// insertTextAtStart 把一段 Markdown 放到当前文档最前面。
func (a *App) insertTextAtStart(text string) {
	a.ed.PrependBlock(text)
	a.docChanged()
	a.notice("已插入到文档开头")
	a.invalidate()
}
