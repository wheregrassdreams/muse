package nativeui

// ai.go — AI 模型设置（服务商、密钥、模型）与对话记录的持久化。
// 对话的渲染在 aichat.go，Agent 循环与工具在 aiagent.go。

import (
	"strings"

	"github.com/egoist/mygo"

	"muse/agent"
	"muse/config"
	"muse/locales"
)

// saveAITools 把能力开关立即落盘（开关切换即生效）。
func (a *App) saveAITools() {
	if err := config.SaveAssistantSettings(a.ai); err != nil {
		a.notice(locales.CommonSaveFailed(err.Error()))
	}
	a.invalidate()
}

func (a *App) setAIService(magpie bool) {
	if a.ai.UsesMagpie() == magpie {
		return
	}
	if magpie {
		a.ai.Provider = config.AssistantProviderMagpie
		a.ai.BaseURL = config.AssistantMagpieBaseURL
	} else {
		a.ai.Provider = config.AssistantProviderCustom
		if a.ai.BaseURL == config.AssistantMagpieBaseURL {
			a.ai.BaseURL = ""
		}
	}
	a.ai.Model = ""
	a.aiModels = nil
	a.aiModelsErr = ""
	a.invalidate()
}

func (a *App) insertAIContent(text string) {
	text = strings.TrimSpace(text)
	if text == "" {
		return
	}
	// 追加到文档末尾，作为编辑器里的一步撤销；光标随之滚到插入处。
	a.activeNote.FormattingCommands().AppendBlock(text)
	a.notice(locales.DocumentInserted())
	a.invalidate()
}

func (a *App) fetchAIModels() {
	if a.aiModelsBusy {
		return
	}
	a.aiModelsBusy = true
	a.aiModelsErr = ""
	a.invalidate()
	req := agent.TestConnRequest{BaseURL: a.ai.BaseURL, APIKey: a.ai.APIKey}
	go func() {
		res := a.aiClient.ListModels(req)
		mygo.RunOnMain(func() {
			a.aiModelsBusy = false
			if res.Ok {
				a.aiModels = res.Models
				if strings.TrimSpace(a.ai.Model) == "" && len(res.Models) > 0 {
					a.ai.Model = res.Models[0].ID
				}
				if len(res.Models) == 0 {
					a.aiModelsErr = locales.AiNoModelsReturned()
				}
			} else {
				a.aiModelsErr = res.Message
			}
			a.invalidate()
		})
	}()
}
