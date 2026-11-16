package nativeui

// ai.go — AI 模型设置（服务商、密钥、模型）与对话记录的持久化。
// 对话的渲染在 aichat.go，Agent 循环与工具在 aiagent.go。

import (
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

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

func (a *App) aiSettings(c *ui.Context) {
	settingsGroup(c, locales.AssistantGroup(), func() {
		settingsRowToggleAction(c, locales.AiAssistant(), locales.AiAssistantDetail(), &a.aiEnabled, func() {
			a.setAIEnabled(!a.aiEnabled)
		})
	})
	a.modelSettings(c)
}

func (a *App) modelSettings(c *ui.Context) {
	t := c.Theme()
	magpie := a.ai.UsesMagpie()
	settingsGroup(c, "", func() {
		serviceDetail := locales.AiServiceAnyDetail()
		if magpie {
			serviceDetail = locales.AiServiceMagpieDetail()
		}
		settingsRow(c, locales.AiService(), serviceDetail, func() {
			a.aiServiceButton(c, locales.AiGateway(), true)
			a.aiServiceButton(c, locales.AiCustom(), false)
		})
		a.settingsDivider(c, true)
		urlDetail := locales.AiURLDetail()
		urlPlaceholder := "https://api.openai.com/v1"
		if magpie {
			urlDetail = locales.AiURLMagpieDetail()
			urlPlaceholder = config.AssistantMagpieBaseURL
		}
		settingsRow(c, locales.AiEndpoint(), urlDetail, func() {
			ui.TextInput(c, &a.ai.BaseURL).Width(250).Placeholder(urlPlaceholder)
		})
		a.settingsDivider(c, true)
		keyDetail := locales.AiKeyDetail()
		keyPlaceholder := "sk-…"
		if magpie {
			keyDetail = locales.AiKeyMagpieDetail()
			keyPlaceholder = locales.AiOptional()
		}
		settingsRow(c, locales.AiApiKey(), keyDetail, func() {
			ui.TextInput(c, &a.ai.APIKey).Password().Width(250).Placeholder(keyPlaceholder)
		})
		a.settingsDivider(c, true)
		modelDetail := locales.AiModelDetail()
		modelPlaceholder := "gpt-4.1"
		if magpie {
			modelDetail = locales.AiModelMagpieDetail()
			modelPlaceholder = "opencode-go/deepseek-v4.1-flash"
		}
		settingsRow(c, locales.AiModel(), modelDetail, func() {
			if len(a.aiModels) > 0 {
				opts := make([]string, 0, len(a.aiModels))
				for _, m := range a.aiModels {
					opts = append(opts, m.ID)
				}
				ui.Combobox(c, &a.ai.Model, opts).Width(250)
			} else {
				ui.TextInput(c, &a.ai.Model).Width(250).Placeholder(modelPlaceholder)
			}
		})
		ui.Row(c).Gap(8).AlignItems(ui.Center).Padding(0, 2).Children(func() {
			if a.aiModelsBusy {
				ui.Text(c, locales.AiFetchModelsLoading()).FontSize(11).TextColor(t.TextMuted)
			} else {
				label := locales.AiFetchModels()
				if len(a.aiModels) > 0 {
					label = locales.AiRefreshModels()
				}
				if ui.Button(c, label).Clicked() {
					a.fetchAIModels()
				}
			}
			if len(a.aiModels) > 0 {
				ui.Text(c, locales.AiAvailableModels(len(a.aiModels))).FontSize(11).TextColor(t.TextMuted)
			}
			if a.aiModelsErr != "" {
				ui.Text(c, a.aiModelsErr).FontSize(10.5).TextColor(t.Danger).Wrap()
			}
		})
		a.settingsDivider(c, true)
		settingsRow(c, locales.AiSystemPrompt(), locales.AiSystemPromptDetail(), func() {
			ui.TextInput(c, &a.ai.System).Width(250).Placeholder(locales.AiSystemPromptPlaceholder())
		})
	})
	// 保存行保持在原位；Agent 能力组放页面末尾，切换即存，不用点保存。
	ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
		ui.Text(c, locales.AiConfigLocation(config.AssistantSettingsPath())).FontSize(10.5).TextColor(t.TextMuted).Grow(1).SingleLine().Ellipsis("…")
		if modelSaveButton(c, locales.AiSaveModelSettings()).Clicked() {
			if err := config.SaveAssistantSettings(a.ai); err != nil {
				a.notice(locales.CommonSaveFailed(err.Error()))
			} else {
				a.notice(locales.AiModelSettingsSaved())
			}
		}
	})
	a.settingsDivider(c, true)
	settingsGroup(c, locales.AiAgentCapabilities(), func() {
		settingsRow(c, locales.AiBrowseSearch(), locales.AiBrowseSearchDetail(), func() {
			if ui.Switch(c, &a.ai.Tools.Browse).Label(locales.AiAllowBrowseSearch()).Changed() {
				a.saveAITools()
			}
		})
		a.settingsDivider(c, true)
		settingsRow(c, locales.AiEditDocuments(), locales.AiEditDocumentsDetail(), func() {
			if ui.Switch(c, &a.ai.Tools.Edit).Label(locales.AiAllowEdit()).Changed() {
				a.saveAITools()
			}
		})
		a.settingsDivider(c, true)
		settingsRow(c, locales.AiManageFiles(), locales.AiManageFilesDetail(), func() {
			if ui.Switch(c, &a.ai.Tools.Manage).Label(locales.AiAllowManageFiles()).Changed() {
				a.saveAITools()
			}
		})
	})
	ui.Text(c, locales.AiCapabilitiesDetail()).
		FontSize(11.5).TextColor(t.TextMuted).Padding(0, 4).Wrap()
}

// aiServiceButton is one half of the model page's 服务 switch. The chosen
// half is a raised card in the theme's own colors, as the settings nav and
// the rail are; the accent stays for links and focus only.
func (a *App) aiServiceButton(c *ui.Context, label string, magpie bool) {
	t := c.Theme()
	selected := a.ai.UsesMagpie() == magpie
	b := ui.ButtonBase(c).Height(28).Padding(0, 11).Radius(7).Label(label)
	switch {
	case selected && t.Dark:
		b.Background(ui.RGBA(255, 255, 255, 0.10))
	case selected:
		b.Background(t.Background)
		b.Border(1, t.Border)
		b.Shadow(0, 0.5, 2, 0, ui.RGBA(0, 0, 0, 0.08))
	case b.Hovered():
		b.Background(t.SurfaceHover)
	}
	b.OnClick(func() { a.setAIService(magpie) })
	b.Children(func() {
		color, fontWeight := t.TextMuted, 400
		if selected {
			color, fontWeight = t.Text, 600
		}
		ui.Text(c, label).FontSize(12).FontWeight(fontWeight).TextColor(color)
	})
}

// modelSaveButton is the page's primary action in neutral colors: the
// theme's text color fills it and the background labels it, so the model
// page never borrows the accent for a button.
func modelSaveButton(c *ui.Context, label string) ui.Element {
	t := c.Theme()
	b := ui.ButtonBase(c).Height(30).Padding(0, 14).Radius(7).Label(label)
	b.Background(t.Text)
	if b.Hovered() {
		b.Background(t.TextMuted)
	}
	b.Children(func() {
		ui.Text(c, label).FontSize(12).FontWeight(600).TextColor(t.Background)
	})
	return b
}

// setAIService switches between the local magpie gateway and a custom
// endpoint. The model name belongs to the endpoint it came from, so it is
// dropped and re-fetched rather than sent to the other one.
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
	a.ed.AppendBlock(text)
	a.docChanged()
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
