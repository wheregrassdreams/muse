package nativeui

// aichat.go — AI 助手面板的界面：左右分列的对话、思考与工具调用的
// 流式渲染（参考 Codex 的呈现），以及底部常驻的输入区。
//
// 布局：用户消息靠右、着色气泡；助手内容靠左、按段展开——思考是
// 可折叠的摘要行，工具调用是带状态的行卡，正文用 mdoc 流式渲染。

import (
	"bytes"
	"encoding/json"
	"fmt"
	"muse/locales"

	"strings"

	"github.com/egoist/mygo/ui"

	mdoc "muse/markdown"
)

func (a *App) aiView(c *ui.Context) {
	if a.assistant != nil {
		a.assistant.render(c, a)
	}
}

// aiEmpty 是没有对话时的空态：说明 Agent 能做什么，并给出示例。
func (a *App) aiEmpty(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).FillWidth().Padding(24, 0, 8, 0).Gap(8).AlignItems(ui.Center).Children(func() {
		ui.Icon(c, iconAI).Size(26, 26).TextColor(iconTint(t))
		ui.Text(c, locales.AiChatWelcomeTitle()).FontSize(13).FontWeight(600)
		ui.Text(c, locales.AiChatWelcomeDetail()).
			FontSize(11.5).TextColor(t.TextMuted).Wrap().TextAlign(ui.Center)
		ui.Box(c).Height(6)
		ui.Row(c).Gap(6).Children(func() {
			a.aiSuggestion(c, locales.AiSuggestionSummarize())
			a.aiSuggestion(c, locales.AiSuggestionFormalize())
			a.aiSuggestion(c, locales.AiSuggestionTitles())
		})
	})
}

// aiSuggestion 是一枚示例气泡，点一下把问题填进输入框。
func (a *App) aiSuggestion(c *ui.Context, text string) {
	t := c.Theme()
	b := ui.ButtonBase(c).Height(24).Padding(0, 10).Radius(12).Label(text).
		AlignItems(ui.Center).Description(text)
	b.Background(t.Background).Border(1, t.Border)
	if b.Hovered() {
		b.Background(t.SurfaceHover)
	}
	b.OnClick(func() {
		if a.aiBusy {
			return
		}
		a.aiInput = text
		a.invalidate()
	})
	b.Children(func() {
		ui.Text(c, text).FontSize(11).TextColor(t.TextMuted).SingleLine()
	})
}

// aiMessage 渲染一条消息：用户靠右，助手靠左。mi 指向 a.aiMessages
// 里的消息，折叠状态直接改在存储的数据上。
func (a *App) aiMessage(c *ui.Context, mi int) {
	m := &a.aiMessages[mi]
	if m.Role == "user" {
		a.aiUserBubble(c, m.Content)
		return
	}
	a.aiAgentBlock(c, m)
}

// aiUserBubble 是靠右的用户气泡，浅浅染一层主题色。
func (a *App) aiUserBubble(c *ui.Context, text string) {
	t := c.Theme()
	ui.Row(c).FillWidth().Justify(ui.End).Children(func() {
		bubble := ui.Column(c).MaxWidthPercent(88).Padding(7, 11).
			Radius(14, 14, 4, 14)
		if t.Dark {
			bubble.Background(t.Accent.Alpha(0.18))
		} else {
			bubble.Background(t.Accent.Alpha(0.10))
		}
		bubble.Selectable().Children(func() {
			ui.Text(c, text).FontSize(12.5).TextColor(t.Text).Wrap().LineHeight(1.55)
		})
	})
}

// aiAgentBlock 是靠左的助手回复：思考、工具、正文各一段。
func (a *App) aiAgentBlock(c *ui.Context, m *chatMessage) {
	ui.Column(c).FillWidth().Gap(8).Children(func() {
		for i := range m.Parts {
			a.aiPart(c, m, i)
		}
	})
}

// aiPart 按类型渲染一段。
func (a *App) aiPart(c *ui.Context, m *chatMessage, i int) {
	p := &m.Parts[i]
	switch p.Kind {
	case partThink:
		a.aiThinkPart(c, p)
	case partTool:
		a.aiToolPart(c, p)
	case partText:
		if strings.TrimSpace(p.Text) == "" {
			return
		}
		a.aiTextPart(c, p.Text)
	}
}

// aiThinkPart 是思考段：一行摘要，点开看过程，参考 Codex 的折叠式
// "Thought for …"。
func (a *App) aiThinkPart(c *ui.Context, p *chatPart) {
	t := c.Theme()
	thinking := p.Ms == 0 && p.open // 流式进行中
	label := locales.AiThinkProcess()
	switch {
	case thinking:
		label = locales.AiThinkingLong()
	case p.Ms > 0:
		label = locales.AiThinkingDuration(fmt.Sprintf("%.1f", float64(p.Ms)/1000))
	}
	hasBody := strings.TrimSpace(p.Text) != ""

	ui.Column(c).FillWidth().Gap(4).Children(func() {
		if !hasBody {
			ui.Row(c).Gap(5).AlignItems(ui.Center).Children(func() {
				ui.Icon(c, iconAI).Size(11, 11).TextColor(iconTint(t))
				ui.Text(c, label).FontSize(10.5).TextColor(t.TextMuted)
			})
			return
		}
		btn := ui.ButtonBase(c).Height(20).Padding(4, 2).Radius(5).Gap(5).
			AlignItems(ui.Center).Label(label)
		if btn.Hovered() {
			btn.Background(t.SurfaceHover)
		}
		btn.OnClick(func() {
			p.open = !p.open
			a.invalidate()
		})
		btn.Children(func() {
			ui.Icon(c, iconAI).Size(11, 11).TextColor(iconTint(t))
			ui.Text(c, label).FontSize(10.5).TextColor(t.TextMuted)
			chevron := iconChevron
			if p.open {
				chevron = iconChevronDown
			}
			ui.Icon(c, chevron).Size(10, 10).TextColor(iconTint(t))
		})
		if p.open {
			ui.Column(c).FillWidth().Padding(2, 0, 0, 9).
				BorderWidth(0, 0, 0, 1).BorderColor(t.Border).Children(func() {
				ui.Text(c, p.Text).FontSize(10.5).Italic().TextColor(t.TextMuted).
					Wrap().LineHeight(1.55).Selectable()
			})
		}
	})
}

// aiToolPart 是一次工具调用。只读工具是一行状态；要改文档的工具
// 走审批卡（参考 beUI 的 tool-approval）：待确认时亮出要做的修改和
// 同意/拒绝按钮，决定之后收起成普通的一行结果。
func (a *App) aiToolPart(c *ui.Context, p *chatPart) {
	t := c.Theme()
	tool := p.Tool
	if tool == nil {
		return
	}
	if tool.State == toolApproval {
		a.aiApprovalCard(c, p)
		return
	}
	label := toolLabel(tool.Name)
	target := toolTarget(tool.Args)

	open := p.open
	running := tool.State == toolRunning

	detail := ""
	if tool.Err != "" {
		detail = tool.Err
	} else if tool.Result != "" {
		detail = tool.Result
	}

	ui.Column(c).FillWidth().Gap(3).Children(func() {
		btn := ui.ButtonBase(c).Height(24).Padding(4, 7).Radius(7).Gap(6).
			FillWidth().AlignItems(ui.Center).Label(label)
		btn.Background(t.Background).Border(1, t.Border)
		if btn.Hovered() {
			btn.Background(t.SurfaceHover)
		}
		btn.OnClick(func() {
			p.open = !p.open
			a.invalidate()
		})
		btn.Children(func() {
			ui.Icon(c, toolIcon(tool.Name)).Size(12, 12).TextColor(iconTint(t))
			ui.Text(c, label).FontSize(11).TextColor(t.Text).SingleLine()
			if target != "" {
				ui.Text(c, target).FontSize(10.5).TextColor(t.TextMuted).Grow(1).
					SingleLine().Ellipsis("…")
			} else {
				ui.Spacer(c)
			}
			switch {
			case running:
				ui.Spinner(c).TextColor(t.TextMuted)
			case tool.State == toolFailed:
				ui.Text(c, "✕").FontSize(10.5).TextColor(t.Danger)
			default:
				ui.Text(c, "✓").FontSize(10.5).TextColor(t.TextMuted)
			}
		})
		if open && detail != "" {
			body := detail
			if len(body) > 700 {
				body = body[:700] + "\n…"
			}
			ui.Column(c).FillWidth().Padding(4, 0, 0, 9).
				BorderWidth(0, 0, 0, 1).BorderColor(t.Border).Gap(3).Children(func() {
				if args := prettyJSON(tool.Args); args != "" {
					ui.Text(c, args).FontSize(10).TextColor(t.TextMuted).Wrap().LineHeight(1.45)
				}
				ui.Text(c, body).FontSize(10).TextColor(t.TextMuted).Wrap().LineHeight(1.45)
			})
		}
	})
}

// aiApprovalCard 是待确认的修改卡：标题、要做的修改、同意/拒绝。
func (a *App) aiApprovalCard(c *ui.Context, p *chatPart) {
	t := c.Theme()
	tool := p.Tool
	target := toolTarget(tool.Args)

	title := locales.AiApprovalEditDocument()
	if tool.Name == "write_document" {
		title = locales.AiApprovalCreateDocument()
	} else if tool.Name == "insert_into_document" {
		title = locales.AiApprovalInsertContent()
	}

	card := ui.Column(c).FillWidth().Radius(10).Padding(9, 10).Gap(7)
	if t.Dark {
		card.Background(t.Accent.Alpha(0.12))
	} else {
		card.Background(t.Accent.Alpha(0.07))
	}
	card.Children(func() {
		ui.Row(c).Gap(6).AlignItems(ui.Center).FillWidth().Children(func() {
			ui.Icon(c, toolIcon(tool.Name)).Size(13, 13).TextColor(t.Accent)
			ui.Text(c, title).FontSize(11.5).FontWeight(600).TextColor(t.Text)
			ui.Spacer(c)
			if target != "" {
				ui.Text(c, target).FontSize(10).TextColor(t.TextMuted).
					MaxWidth(120).SingleLine().Ellipsis("…")
			}
		})
		if desc := toolDescription(tool); desc != "" {
			ui.Text(c, desc).FontSize(10).TextColor(t.TextMuted).Wrap().LineHeight(1.5)
		}
		ui.Row(c).Gap(6).AlignItems(ui.Center).FillWidth().Wrap().Children(func() {
			allow := ui.ButtonBase(c).Height(24).Padding(0, 11).Radius(7).
				Label(locales.AiApproveChange()).Description(locales.AiApproveChange())
			allow.Background(t.Accent)
			if allow.Hovered() {
				allow.Background(t.AccentHover)
			}
			allow.OnClick(func() { a.decideApproval(tool.ID, approvalAllow) })
			allow.Children(func() {
				ui.Text(c, locales.AiApproveChange()).FontSize(10.5).FontWeight(600).TextColor(t.AccentText)
			})
			always := ui.ButtonBase(c).Height(24).Padding(0, 11).Radius(7).
				Label(locales.AiAlwaysAllow()).Description(locales.AiAlwaysAllowSession())
			always.Background(t.Background).Border(1, t.Border)
			if always.Hovered() {
				always.Background(t.SurfaceHover)
			}
			always.OnClick(func() { a.decideApproval(tool.ID, approvalAlways) })
			always.Children(func() {
				ui.Text(c, locales.AiAlwaysAllow()).FontSize(10.5).TextColor(t.Text)
			})
			deny := ui.ButtonBase(c).Height(24).Padding(0, 11).Radius(7).
				Label(locales.CommonReject()).Description(locales.CommonReject())
			deny.Background(t.Background).Border(1, t.Border)
			if deny.Hovered() {
				deny.Background(t.SurfaceHover)
			}
			deny.OnClick(func() { a.decideApproval(tool.ID, approvalDeny) })
			deny.Children(func() {
				ui.Text(c, locales.CommonReject()).FontSize(10.5).TextColor(t.TextMuted)
			})
		})
	})
}

// toolDescription 是审批卡上给用户看的「要做什么」：从参数里挑一句
// 人话，而不是整个 JSON。
func toolDescription(tool *toolCall) string {
	var args struct {
		Path    string `json:"path"`
		OldText string `json:"old_text"`
		NewText string `json:"new_text"`
		Content string `json:"content"`
		Text    string `json:"text"`
	}
	if json.Unmarshal([]byte(tool.Args), &args) != nil {
		return strings.TrimSpace(tool.Args)
	}
	clip := func(s string, n int) string {
		s = strings.TrimSpace(s)
		if len(s) > n {
			return s[:n] + "…"
		}
		return s
	}
	switch tool.Name {
	case "edit_document":
		return locales.AiToolDescriptionEdit(clip(args.OldText, 80), clip(args.NewText, 80))
	case "write_document":
		return locales.AiToolDescriptionWrite(baseName(args.Path), len([]rune(args.Content)))
	case "insert_into_document":
		return locales.AiToolDescriptionInsert(clip(args.Text, 120))
	}
	return strings.TrimSpace(tool.Args)
}

// aiMdEntry is a Markdown text and the blocks it parsed into.
type aiMdEntry struct {
	text   string
	blocks []mdoc.Block
}

// aiMdBlocks returns the blocks of a Markdown part, from the cache when
// the text was parsed already. The assistant's text is parsed as it
// streams, and every frame in between draws the same part again.
func (a *App) aiMdBlocks(text string) []mdoc.Block {
	for i := range a.aiMdCache {
		if e := &a.aiMdCache[i]; e.blocks != nil && e.text == text {
			return e.blocks
		}
	}
	blocks := mdoc.Parse(text).Blocks
	a.aiMdCache[a.aiMdNext%len(a.aiMdCache)] = aiMdEntry{text: text, blocks: blocks}
	a.aiMdNext++
	return blocks
}

// aiTextPart 是正文段：mdoc 流式渲染。
func (a *App) aiTextPart(c *ui.Context, text string) {
	mdoc.BlocksView(c, a.aiMdBlocks(text), mdoc.Options{
		BodyFontSize:       12.5,
		SoftBreaksAsSpaces: true,
		ContentWidth:       a.aiContentWidth(),
		BaseDir:            a.docDir(),
		AssetRoot:          a.root,
		OnImagePreview:     a.openImagePreview,
		Padding:            0,
		IndentStep:         14,
	})
}

// aiMiniButton 是一行里的浅色小按钮。
func (a *App) aiMiniButton(c *ui.Context, label string, fn func()) {
	t := c.Theme()
	b := ui.ButtonBase(c).Height(22).Padding(0, 9).Radius(6).Label(label).
		AlignItems(ui.Center).Description(label)
	b.Background(t.Background).Border(1, t.Border)
	if b.Hovered() {
		b.Background(t.SurfaceHover)
	}
	b.OnClick(fn)
	b.Children(func() {
		ui.Text(c, label).FontSize(10.5).TextColor(t.TextMuted)
	})
}

// aiBusyRow 是流式进行中的状态行。
func (a *App) aiBusyRow(c *ui.Context) {
	t := c.Theme()
	ui.Row(c).Gap(7).AlignItems(ui.Center).Padding(0, 2).Children(func() {
		ui.Spinner(c).TextColor(t.TextMuted)
		phase := a.aiPhase
		if phase == "" {
			phase = locales.AiThinkingShort()
		}
		ui.Text(c, phase).FontSize(11).TextColor(t.TextMuted)
	})
}

// aiErrorRow 是错误行。
func (a *App) aiErrorRow(c *ui.Context) {
	t := c.Theme()
	ui.Row(c).Gap(6).AlignItems(ui.Center).FillWidth().Children(func() {
		ui.Text(c, a.aiErr).FontSize(11).TextColor(t.Danger).Wrap().Grow(1)
	})
}

// aiComposer 是底部的输入区：输入卡片在上，权限预设 / 模型 / 清空
// 这排状态放卡片下面的底栏。
func (a *App) aiComposer(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).Shrink(0).Padding(2, 10, 10, 10).Gap(5).Children(func() {
		card := ui.Column(c).FillWidth().Radius(12).Background(t.Surface).
			Padding(6, 6, 6, 10)
		card.Children(func() {
			ui.Row(c).Gap(8).AlignItems(ui.End).Children(func() {
				ta := ui.TextAreaBase(c, &a.aiInput).Grow(1).
					Placeholder(locales.AiPromptPlaceholder()).
					Lines(1, 7).FontSize(12.5).Padding(4, 2).LineHeight(1.5)
				if ta.Shortcut(ui.Cmd, ui.KeyEnter) && !a.aiBusy {
					a.sendAI()
				}
				a.aiSendButton(c)
			})
		})
		if card.FocusWithin() {
			card.Border(1, t.Accent)
		} else {
			card.Border(1, t.Border)
		}
		ui.Row(c).Gap(6).AlignItems(ui.Center).Padding(0, 4).Children(func() {
			a.aiPermButton(c)
			// 模型名可能很长：占满剩余宽度并省略，别把清空挤走。
			ui.Text(c, a.ai.Model).FontSize(10).TextColor(t.TextMuted).
				Grow(1).MinWidth(0).SingleLine().Ellipsis("…")
			if !a.aiBusy {
				a.aiMiniButton(c, locales.AiClearConversation(), a.clearAIChat)
			}
		})
	})
}

// aiPermButton 是底栏上的权限预设：当前预设名 + 下拉三档。
func (a *App) aiPermButton(c *ui.Context) {

	perm := a.aiPerm
	if perm == "" {
		perm = permAsk
	}
	ui.MenuButton(c, perm.label(), func(m *ui.Menu) {
		for _, p := range aiPerms {
			if m.Item(p.label()).Checked(p == perm).Chosen() {
				a.aiPerm = p
				a.invalidate()
			}
		}
	}).Height(22).FontSize(10.5).Radius(6).Padding(0, 4)
}

// aiSendButton 是输入框右侧的圆形动作键：空闲时发送，进行中停止。
func (a *App) aiSendButton(c *ui.Context) {
	t := c.Theme()
	b := ui.ButtonBase(c).Width(28).Height(28).Radius(14).Padding(0).
		Justify(ui.Center).AlignItems(ui.Center).Label(locales.CommonSend()).Description(locales.CommonSend())
	switch {
	case a.aiBusy:
		b.Label(locales.CommonStop()).Description(locales.CommonStop())
		b.Background(t.SurfaceHover)
		b.OnClick(a.abortAI)
		b.Children(func() {
			ui.Icon(c, iconStop).Size(10, 10).TextColor(t.Text)
		})
	case strings.TrimSpace(a.aiInput) == "":
		b.Background(t.SurfaceHover)
		b.Children(func() {
			ui.Icon(c, iconArrowUp).Size(14, 14).TextColor(iconTint(t))
		})
	default:
		b.Background(t.Accent)
		if b.Hovered() {
			b.Background(t.AccentHover)
		}
		b.OnClick(a.sendAI)
		b.Children(func() {
			ui.Icon(c, iconArrowUp).Size(14, 14).TextColor(t.AccentText)
		})
	}
}

// aiContentWidth 是正文渲染的栏宽：面板宽减去左右留白。
func (a *App) aiContentWidth() float32 {
	w := a.panelWidth - 28
	if w < 180 {
		return 180
	}
	return w
}

// toolTarget 从参数里挑一个给界面看的对象名（通常是文件名）。
func toolTarget(argsJSON string) string {
	var args struct {
		Path string `json:"path"`
	}
	if json.Unmarshal([]byte(argsJSON), &args) != nil || args.Path == "" {
		return ""
	}
	return baseName(args.Path)
}

// baseName 兼容 / 与 \ 的取文件名。
func baseName(p string) string {
	if i := strings.LastIndexAny(p, `/\`); i >= 0 {
		return p[i+1:]
	}
	return p
}

// prettyJSON 把工具参数缩进成可读的几行；解析失败就原样给出。
func prettyJSON(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	var indented bytes.Buffer
	if json.Indent(&indented, []byte(raw), "", "  ") != nil {
		return raw
	}
	return indented.String()
}

// scrollAIBottom 把对话滚到最底。
func (a *App) scrollAIBottom() {
	a.aiScroll.Y = 1e9
}
