package nativeui

// ai.go — AI 模型设置（服务商、密钥、模型）与对话记录的持久化。
// 对话的渲染在 aichat.go，Agent 循环与工具在 aiagent.go。

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/egoist/mygo"

	applicationagent "muse/application/agent"
	applicationconfig "muse/application/config"
	"muse/config"
	"muse/locales"
)

func (a *App) saveAssistantSettings() error {
	return applicationconfig.NewService(a.configRepository).SaveAssistantSettings(a.ai)
}

// saveAITools 把能力开关立即落盘（开关切换即生效）。
func (a *App) saveAITools() {
	if err := a.saveAssistantSettings(); err != nil {
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
	a.aiModelCatalog.Reset()
	a.invalidate()
}

func (a *App) insertAIContent(text string) {
	text = strings.TrimSpace(text)
	if text == "" {
		return
	}
	// 追加到文档末尾，作为编辑器里的一步撤销；光标随之滚到插入处。
	a.noteEditor().FormattingCommands().AppendBlock(text)
	a.notice(locales.DocumentInserted())
	a.invalidate()
}

func (a *App) fetchAIModels() {
	generation, begun := a.aiModelCatalog.Begin()
	if !begun {
		return
	}
	a.invalidate()
	if a.assistantProvider == nil {
		a.aiModelCatalog.Complete(generation, nil, locales.AiServiceRequestFailed(0))
		a.invalidate()
		return
	}
	if a.assistantModelCancel != nil {
		a.assistantModelCancel()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	a.assistantModelCancel = cancel
	a.assistantModelGeneration = generation
	req := applicationagent.ProviderCredentials{BaseURL: a.ai.BaseURL, APIKey: a.ai.APIKey}
	provider := a.assistantProvider
	go func() {
		res, providerErr := provider.ListModels(ctx, req)
		mygo.RunOnMain(func() {
			if a.assistantModelGeneration == generation {
				cancel()
				a.assistantModelCancel = nil
			}
			var models []applicationagent.ProviderModel
			var err string
			if providerErr == nil {
				models = res.Models
				if strings.TrimSpace(a.ai.Model) == "" && len(res.Models) > 0 {
					a.ai.Model = res.Models[0].ID
				}
				if len(res.Models) == 0 {
					err = locales.AiNoModelsReturned()
				}
			} else {
				err = assistantProviderError(providerErr)
			}
			if !a.aiModelCatalog.Complete(generation, models, err) {
				return
			}
			a.invalidate()
		})
	}()
}

func assistantProviderError(err error) string {
	var providerErr *applicationagent.ProviderError
	if !errors.As(err, &providerErr) {
		return err.Error()
	}
	switch providerErr.Kind {
	case applicationagent.ProviderErrorInvalidConfiguration:
		return locales.AiServiceFillBaseURL()
	case applicationagent.ProviderErrorTimeout:
		return locales.AiServiceTimeout(10)
	case applicationagent.ProviderErrorNetwork:
		return locales.AiServiceUnreachable()
	case applicationagent.ProviderErrorUnauthorized:
		return locales.AiServiceAuthFailure()
	case applicationagent.ProviderErrorRequest:
		if providerErr.Detail != "" {
			return providerErr.Detail
		}
		return locales.AiServiceRequestFailed(providerErr.StatusCode)
	case applicationagent.ProviderErrorReadResponse:
		return locales.AiServiceReadModelsFailed()
	case applicationagent.ProviderErrorInvalidModelList:
		return locales.AiServiceInvalidModelList()
	default:
		return err.Error()
	}
}
