// Package agent renders the assistant conversation. It owns presentation
// state such as expanded parts and translates user input into typed actions;
// the caller owns the agent session, tool execution, and approval policy.
package agent

import (
	"fmt"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"muse/locales"
)

type Role uint8

const (
	User Role = iota + 1
	Assistant
)

type PartKind uint8

const (
	TextPart PartKind = iota + 1
	ThoughtPart
	ToolPart
)

type ToolStatus uint8

const (
	ToolRunning ToolStatus = iota + 1
	ToolNeedsApproval
	ToolSucceeded
	ToolFailed
)

// Tool is display data only. The panel never executes a tool or decides
// whether its result is trusted.
type Tool struct {
	ID, Name, Target string
	Status           ToolStatus
	Summary          string
	Details          string
}

type Part struct {
	Kind     PartKind
	Text     string
	Thinking time.Duration
	Expanded bool
	Tool     Tool
}

type Message struct {
	Role  Role
	Text  string
	Parts []Part
}

type ViewModel struct {
	Messages       []Message
	Draft          string
	Busy           bool
	Phase          string
	Error          string
	Scroll         *ui.ScrollState
	Icon           *ui.SVG
	ContentWidth   float32
	RenderMarkdown func(c *ui.Context, source string, width float32)
}

type ActionKind uint8

const (
	DraftChanged ActionKind = iota + 1
	Send
	Stop
	ClearConversation
	TogglePart
	ApproveTool
	AlwaysAllowTool
	RejectTool
)

// Action is the complete output of the panel. Part indexes address the
// immutable message snapshot supplied in ViewModel for the current frame.
type Action struct {
	Kind         ActionKind
	Text         string
	MessageIndex int
	PartIndex    int
	ToolID       string
}

func Render(c *ui.Context, model ViewModel, dispatch func(Action)) {
	t := c.Theme()
	ui.Column(c).Fill().Children(func() {
		if model.Scroll != nil {
			ui.Scroll(c).Grow(1).TrackScroll(model.Scroll).Children(func() {
				conversation(c, model, dispatch)
			})
		} else {
			ui.Column(c).Grow(1).Children(func() { conversation(c, model, dispatch) })
		}
		if model.Busy {
			ui.Row(c).Gap(7).AlignItems(ui.Center).Padding(8, 12).Children(func() {
				ui.Spinner(c).TextColor(t.TextMuted)
				phase := model.Phase
				if phase == "" {
					phase = locales.AiThinkingShort()
				}
				ui.Text(c, phase).FontSize(11).TextColor(t.TextMuted).Grow(1)
				button(c, locales.CommonStop(), func() { emit(dispatch, Action{Kind: Stop}) })
			})
		}
		if model.Error != "" {
			ui.Text(c, model.Error).Padding(8, 12).FontSize(11).TextColor(t.Danger).Wrap()
		}
		composer(c, model, dispatch)
	})
}

func conversation(c *ui.Context, model ViewModel, dispatch func(Action)) {
	if len(model.Messages) == 0 {
		t := c.Theme()
		ui.Column(c).FillWidth().Padding(24, 12).Gap(8).AlignItems(ui.Center).Children(func() {
			if model.Icon != nil {
				ui.Icon(c, model.Icon).Size(25, 25).TextColor(t.TextMuted)
			}
			ui.Text(c, locales.AiChatWelcomeTitle()).FontSize(13).FontWeight(600)
			ui.Text(c, locales.AiChatWelcomeDetail()).FontSize(11.5).TextColor(t.TextMuted).Wrap().TextAlign(ui.Center)
		})
		return
	}
	ui.Column(c).FillWidth().Padding(10, 12, 12, 12).Gap(12).Children(func() {
		for mi, message := range model.Messages {
			message := message
			switch message.Role {
			case User:
				userBubble(c, message.Text)
			case Assistant:
				assistantMessage(c, message, mi, model, dispatch)
			}
		}
		if !model.Busy {
			ui.Row(c).Justify(ui.End).Padding(2, 0).Children(func() {
				button(c, locales.AiClearConversation(), func() { emit(dispatch, Action{Kind: ClearConversation}) })
			})
		}
	})
}

func userBubble(c *ui.Context, text string) {
	t := c.Theme()
	ui.Row(c).FillWidth().Justify(ui.End).Children(func() {
		bubble := ui.Column(c).MaxWidthPercent(88).Padding(7, 11).Radius(14, 14, 4, 14)
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

func assistantMessage(c *ui.Context, message Message, messageIndex int, model ViewModel, dispatch func(Action)) {
	ui.Column(c).FillWidth().Gap(8).Children(func() {
		if len(message.Parts) == 0 {
			markdown(c, message.Text, model)
			return
		}
		for pi, part := range message.Parts {
			part := part
			switch part.Kind {
			case TextPart:
				if strings.TrimSpace(part.Text) != "" {
					markdown(c, part.Text, model)
				}
			case ThoughtPart:
				thought(c, part, messageIndex, pi, dispatch)
			case ToolPart:
				tool(c, part.Tool, part.Expanded, messageIndex, pi, dispatch)
			}
		}
	})
}

func markdown(c *ui.Context, text string, model ViewModel) {
	width := model.ContentWidth
	if width <= 0 {
		width = 360
	}
	if model.RenderMarkdown != nil {
		model.RenderMarkdown(c, text, width)
		return
	}
	ui.Text(c, text).FontSize(12.5).TextColor(c.Theme().Text).Wrap().LineHeight(1.55).Selectable()
}

func thought(c *ui.Context, part Part, messageIndex, partIndex int, dispatch func(Action)) {
	t := c.Theme()
	label := locales.AiThinkProcess()
	if part.Thinking > 0 {
		label = locales.AiThinkingDuration(formatSeconds(part.Thinking))
	} else if strings.TrimSpace(part.Text) == "" {
		label = locales.AiThinkingLong()
	}
	ui.Column(c).FillWidth().Gap(4).Children(func() {
		if strings.TrimSpace(part.Text) == "" {
			ui.Row(c).Gap(5).AlignItems(ui.Center).Children(func() {
				ui.Text(c, label).FontSize(10.5).TextColor(t.TextMuted)
			})
			return
		}
		b := ui.ButtonBase(c).Height(22).Padding(4, 5).Radius(5).Label(label)
		if b.Hovered() {
			b.Background(t.SurfaceHover)
		}
		b.OnClick(func() { emit(dispatch, Action{Kind: TogglePart, MessageIndex: messageIndex, PartIndex: partIndex}) })
		b.Children(func() { ui.Text(c, label).FontSize(10.5).TextColor(t.TextMuted) })
		if part.Expanded {
			ui.Text(c, part.Text).Padding(2, 0, 0, 9).FontSize(10.5).Italic().TextColor(t.TextMuted).Wrap().LineHeight(1.55).Selectable()
		}
	})
}

func formatSeconds(d time.Duration) string {
	return fmt.Sprintf("%.1f", d.Seconds())
}

func tool(c *ui.Context, value Tool, expanded bool, messageIndex, partIndex int, dispatch func(Action)) {
	t := c.Theme()
	if value.Status == ToolNeedsApproval {
		approval(c, value, dispatch)
		return
	}
	label := value.Name
	if label == "" {
		label = locales.AiToolCallPrefix("tool")
	}
	ui.Column(c).FillWidth().Gap(3).Children(func() {
		b := ui.ButtonBase(c).Height(26).Padding(4, 8).Radius(7).FillWidth().AlignItems(ui.Center).Label(label)
		b.Background(t.Background).Border(1, t.Border)
		if b.Hovered() {
			b.Background(t.SurfaceHover)
		}
		b.OnClick(func() { emit(dispatch, Action{Kind: TogglePart, MessageIndex: messageIndex, PartIndex: partIndex}) })
		b.Children(func() {
			ui.Text(c, label).FontSize(11).TextColor(t.Text).SingleLine()
			if value.Target != "" {
				ui.Text(c, value.Target).FontSize(10.5).TextColor(t.TextMuted).Grow(1).SingleLine().Ellipsis("…")
			} else {
				ui.Spacer(c)
			}
			switch value.Status {
			case ToolRunning:
				ui.Spinner(c).TextColor(t.TextMuted)
			case ToolFailed:
				ui.Text(c, "✕").FontSize(10.5).TextColor(t.Danger)
			default:
				ui.Text(c, "✓").FontSize(10.5).TextColor(t.TextMuted)
			}
		})
		if expanded && value.Details != "" {
			ui.Text(c, value.Details).Padding(4, 0, 0, 9).FontSize(10).TextColor(t.TextMuted).Wrap().LineHeight(1.45).Selectable()
		}
	})
}

func approval(c *ui.Context, value Tool, dispatch func(Action)) {
	t := c.Theme()
	card := ui.Column(c).FillWidth().Radius(10).Padding(9, 10).Gap(7)
	if t.Dark {
		card.Background(t.Accent.Alpha(0.12))
	} else {
		card.Background(t.Accent.Alpha(0.07))
	}
	card.Children(func() {
		ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
			title := value.Name
			if title == "" {
				title = locales.AiApprovalEditDocument()
			}
			ui.Text(c, title).FontSize(11.5).FontWeight(600).TextColor(t.Text)
			ui.Spacer(c)
			if value.Target != "" {
				ui.Text(c, value.Target).FontSize(10).TextColor(t.TextMuted).MaxWidth(120).SingleLine().Ellipsis("…")
			}
		})
		if value.Summary != "" {
			ui.Text(c, value.Summary).FontSize(10).TextColor(t.TextMuted).Wrap().LineHeight(1.5)
		}
		ui.Row(c).Gap(6).AlignItems(ui.Center).Wrap().Children(func() {
			approvalButton(c, locales.AiApproveChange(), true, func() { emit(dispatch, Action{Kind: ApproveTool, ToolID: value.ID}) })
			approvalButton(c, locales.AiAlwaysAllow(), false, func() { emit(dispatch, Action{Kind: AlwaysAllowTool, ToolID: value.ID}) })
			approvalButton(c, locales.CommonReject(), false, func() { emit(dispatch, Action{Kind: RejectTool, ToolID: value.ID}) })
		})
	})
}

func approvalButton(c *ui.Context, label string, primary bool, onClick func()) {
	t := c.Theme()
	b := ui.ButtonBase(c).Height(24).Padding(0, 10).Radius(7).Label(label).Description(label)
	if primary {
		b.Background(t.Accent)
		if b.Hovered() {
			b.Background(t.AccentHover)
		}
	} else {
		b.Background(t.Background).Border(1, t.Border)
		if b.Hovered() {
			b.Background(t.SurfaceHover)
		}
	}
	b.OnClick(onClick).Children(func() {
		color := t.Text
		if primary {
			color = t.AccentText
		}
		ui.Text(c, label).FontSize(10.5).TextColor(color)
	})
}

func composer(c *ui.Context, model ViewModel, dispatch func(Action)) {
	t := c.Theme()
	ui.Column(c).Shrink(0).Padding(2, 10, 10, 10).Gap(5).Children(func() {
		card := ui.Column(c).FillWidth().Radius(12).Background(t.Surface).Padding(6, 6, 6, 10)
		card.Children(func() {
			ui.Row(c).Gap(8).AlignItems(ui.End).Children(func() {
				draft := model.Draft
				ta := ui.TextAreaBase(c, &draft).Grow(1).Placeholder(locales.AiPromptPlaceholder()).Lines(1, 7).
					FontSize(12.5).Padding(4, 2).LineHeight(1.5)
				if ta.Shortcut(ui.Cmd, ui.KeyEnter) && !model.Busy {
					emit(dispatch, Action{Kind: Send, Text: strings.TrimSpace(draft)})
				}
				if draft != model.Draft {
					emit(dispatch, Action{Kind: DraftChanged, Text: draft})
				}
				if model.Busy {
					button(c, locales.CommonStop(), func() { emit(dispatch, Action{Kind: Stop}) })
				} else {
					button(c, locales.CommonSend(), func() {
						if strings.TrimSpace(draft) != "" {
							emit(dispatch, Action{Kind: Send, Text: strings.TrimSpace(draft)})
						}
					})
				}
			})
		})
		if card.FocusWithin() {
			card.Border(1, t.Accent)
		} else {
			card.Border(1, t.Border)
		}
	})
}

func button(c *ui.Context, label string, onClick func()) {
	t := c.Theme()
	b := ui.ButtonBase(c).Height(24).Padding(0, 9).Radius(6).Label(label)
	if b.Hovered() {
		b.Background(t.SurfaceHover)
	}
	b.OnClick(onClick).Children(func() { ui.Text(c, label).FontSize(10.5).TextColor(t.TextMuted) })
}

func emit(dispatch func(Action), action Action) {
	if dispatch != nil {
		dispatch(action)
	}
}
