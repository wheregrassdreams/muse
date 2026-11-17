// Package settings renders application and editor preferences. It accepts a
// snapshot and returns typed changes; persistence and applying preferences
// to running services remain application responsibilities.
package settings

import (
	"github.com/egoist/mygo/ui"
	"muse/locales"
)

type Page uint8

const (
	AppearancePage Page = iota
	EditorPage
	AssistantPage
)

type ThemeMode uint8

const (
	ThemeSystem ThemeMode = iota
	ThemeLight
	ThemeDark
)

type Accent struct {
	ID    string
	Label string
	Color ui.Color
}

type Appearance struct {
	Theme            ThemeMode
	Accents          []Accent
	AccentID         string
	Locale           string
	LocaleConfigured bool
	HeaderFade       bool
	SmoothPageJumps  bool
}

type Editor struct {
	Keymap              string
	SmartTypography     bool
	FontSize            float64
	LineHeight          float64
	HideMouseCursor     bool
	CursorFade          bool
	SmoothCursor        bool
	HeadingMarkerReveal bool
	VimVirtualImageRows bool
	VimYankFlash        bool
	SearchMatchPulse    bool
}

type Capabilities struct {
	Browse bool
	Edit   bool
	Manage bool
}

type Assistant struct {
	Enabled            bool
	Provider           string
	BaseURL            string
	APIKey             string
	Model              string
	SystemPrompt       string
	Capabilities       Capabilities
	AvailableModels    []string
	ModelsLoading      bool
	ModelsError        string
	SettingsPath       string
	CapabilitiesDetail string
}

type ViewModel struct {
	Appearance Appearance
	Editor     Editor
	Assistant  Assistant
}

type ActionKind uint8

const (
	SelectPage ActionKind = iota + 1
	ThemeChanged
	AccentChanged
	LocaleChanged
	HeaderFadeChanged
	SmoothPageJumpsChanged
	KeymapChanged
	SmartTypographyChanged
	FontSizeChanged
	LineHeightChanged
	HideMouseCursorChanged
	CursorFadeChanged
	SmoothCursorChanged
	HeadingMarkerRevealChanged
	VimVirtualImageRowsChanged
	VimYankFlashChanged
	SearchMatchPulseChanged
	AssistantEnabledChanged
	ProviderChanged
	BaseURLChanged
	APIKeyChanged
	ModelChanged
	SystemPromptChanged
	CapabilityChanged
	FetchModels
	SaveAssistant
	Close
)

type Capability uint8

const (
	BrowseCapability Capability = iota + 1
	EditCapability
	ManageCapability
)

// Action carries values for exactly one ActionKind. Text and Enabled are
// used only by the corresponding field/toggle actions; Page/Theme/Capability
// carry their own closed enums.
type Action struct {
	Kind       ActionKind
	Page       Page
	Theme      ThemeMode
	Capability Capability
	AccentID   string
	Locale     string
	Text       string
	Number     float64
	Enabled    bool
}

func Render(c *ui.Context, handle *Handle, model ViewModel, dispatch func(Action)) {
	if handle == nil {
		return
	}
	state := &handle.state
	t := c.Theme()
	ui.Row(c).Fill().AlignItems(ui.Stretch).Children(func() {
		ui.Column(c).Width(164).Shrink(0).Padding(8, 8, 8, 10).Gap(3).Children(func() {
			pageButton(c, locales.SettingsAppearance(), AppearancePage, state.page, dispatch)
			pageButton(c, locales.SettingsEditor(), EditorPage, state.page, dispatch)
			pageButton(c, locales.SettingsAI(), AssistantPage, state.page, dispatch)
		})
		ui.Column(c).Grow(1).MinWidth(0).FillHeight().Radius(12).Clip().Background(t.Background).Border(1, t.Border).Children(func() {
			title := pageTitle(state.page)
			ui.Row(c).Height(42).Shrink(0).Padding(0, 16).AlignItems(ui.Center).
				BorderWidth(0, 0, 1, 0).BorderColor(t.Border).Children(func() {
				ui.Text(c, title).FontSize(14).FontWeight(650)
				ui.Spacer(c)
				ui.Button(c, locales.CommonClose()).OnClick(func() { emit(dispatch, Action{Kind: Close}) })
			})
			content := func() {
				ui.Column(c).FillWidth().Padding(16).Gap(16).Children(func() {
					switch state.page {
					case EditorPage:
						renderEditor(c, model.Editor, dispatch)
					case AssistantPage:
						renderAssistant(c, model.Assistant, dispatch)
					default:
						renderAppearance(c, model.Appearance, dispatch)
					}
				})
			}
			ui.Scroll(c).Grow(1).TrackScroll(&state.scroll).Children(content)
		})
	})
}

func pageTitle(page Page) string {
	switch page {
	case EditorPage:
		return locales.SettingsEditor()
	case AssistantPage:
		return locales.SettingsAI()
	default:
		return locales.SettingsAppearance()
	}
}

func pageButton(c *ui.Context, label string, page, selected Page, dispatch func(Action)) {
	t := c.Theme()
	b := ui.ButtonBase(c).Height(32).Padding(0, 10).Radius(7).Label(label)
	switch {
	case page == selected:
		if t.Dark {
			b.Background(ui.RGBA(255, 255, 255, 0.08))
		} else {
			b.Background(t.Background).Border(1, t.Border)
		}
	case b.Hovered():
		b.Background(t.SurfaceHover)
	}
	b.OnClick(func() { emit(dispatch, Action{Kind: SelectPage, Page: page}) }).Children(func() {
		ui.Text(c, label).FontSize(12.5).TextColor(t.Text)
	})
}

func renderAppearance(c *ui.Context, model Appearance, dispatch func(Action)) {
	group(c, locales.Language(), func() {
		row(c, locales.SettingsLanguage(), locales.SettingsLanguageDetail(), func() {
			languageOption(c, locales.LanguageSystem(), "", !model.LocaleConfigured || model.Locale == "", dispatch)
			languageOption(c, locales.LanguageChinese(), "zh-Hans", model.LocaleConfigured && model.Locale == "zh-Hans", dispatch)
			languageOption(c, locales.LanguageEnglish(), "en", model.LocaleConfigured && model.Locale == "en", dispatch)
		})
	})
	group(c, locales.ThemeTitle(), func() {
		row(c, locales.ThemeTitle(), locales.ThemeDescription(), func() {
			themeOption(c, ThemeLight, locales.ThemeLight(), model.Theme, dispatch)
			themeOption(c, ThemeDark, locales.ThemeDark(), model.Theme, dispatch)
			themeOption(c, ThemeSystem, locales.ThemeSystem(), model.Theme, dispatch)
		})
		divider(c)
		row(c, locales.AccentColor(), "", func() {
			for _, option := range model.Accents {
				option := option
				selected := option.ID == model.AccentID
				b := ui.ButtonBase(c).Size(30, 30).Radius(15).Padding(3).Label(locales.AccentNamed(option.Label))
				if selected {
					b.Border(2, c.Theme().Accent)
				} else if b.Hovered() {
					b.Background(c.Theme().SurfaceHover)
				}
				b.OnClick(func() { emit(dispatch, Action{Kind: AccentChanged, AccentID: option.ID}) }).Children(func() {
					ui.Box(c).Size(24, 24).Radius(12).Background(option.Color)
				})
			}
		})
		divider(c)
		toggle(c, locales.MenuHeaderFade(), locales.SettingsHeaderFadeDetail(), model.HeaderFade, func(value bool) {
			emit(dispatch, Action{Kind: HeaderFadeChanged, Enabled: value})
		})
	})
	group(c, locales.SettingsNavigation(), func() {
		toggle(c, locales.SettingsSmoothPageJumps(), locales.SettingsSmoothPageJumpsDetail(), model.SmoothPageJumps, func(value bool) {
			emit(dispatch, Action{Kind: SmoothPageJumpsChanged, Enabled: value})
		})
	})
}

func languageOption(c *ui.Context, label, locale string, selected bool, dispatch func(Action)) {
	optionButton(c, label, selected, func() { emit(dispatch, Action{Kind: LocaleChanged, Locale: locale}) })
}

func themeOption(c *ui.Context, mode ThemeMode, label string, selected ThemeMode, dispatch func(Action)) {
	optionButton(c, label, mode == selected, func() { emit(dispatch, Action{Kind: ThemeChanged, Theme: mode}) })
}

func renderEditor(c *ui.Context, model Editor, dispatch func(Action)) {
	group(c, locales.SettingsKeymap(), func() {
		row(c, locales.SettingsKeymap(), locales.SettingsKeymapDetail(), func() {
			for _, option := range []struct{ mode, label string }{
				{"default", locales.KeymapDefault()}, {"helix", locales.KeymapHelix()}, {"vim", locales.KeymapVim()},
			} {
				option := option
				optionButton(c, option.label, model.Keymap == option.mode, func() {
					emit(dispatch, Action{Kind: KeymapChanged, Text: option.mode})
				})
			}
		})
	})
	group(c, locales.SettingsTypography(), func() {
		toggle(c, locales.SettingsSmartTypography(), locales.SettingsSmartTypographyDetail(), model.SmartTypography, func(value bool) {
			emit(dispatch, Action{Kind: SmartTypographyChanged, Enabled: value})
		})
		divider(c)
		sliderRow(c, locales.SettingsFontSize(), locales.SettingsFontSizeDetail(), model.FontSize, 12, 22, "%.0fpx", func(value float64) {
			emit(dispatch, Action{Kind: FontSizeChanged, Number: value})
		})
		divider(c)
		sliderRow(c, locales.SettingsLineHeight(), locales.SettingsLineHeightDetail(), model.LineHeight, 1.4, 2.2, "%.1f", func(value float64) {
			emit(dispatch, Action{Kind: LineHeightChanged, Number: value})
		})
	})
	group(c, locales.SettingsCursor(), func() {
		toggle(c, locales.SettingsHideMouseCursor(), locales.SettingsHideMouseCursorDetail(), model.HideMouseCursor, func(value bool) {
			emit(dispatch, Action{Kind: HideMouseCursorChanged, Enabled: value})
		})
		divider(c)
		toggle(c, locales.SettingsCursorFade(), locales.SettingsCursorFadeDetail(), model.CursorFade, func(value bool) {
			emit(dispatch, Action{Kind: CursorFadeChanged, Enabled: value})
		})
		if model.Keymap == "vim" || model.Keymap == "helix" {
			divider(c)
			toggle(c, locales.SettingsSmoothCursor(), locales.SettingsSmoothCursorDetail(), model.SmoothCursor, func(value bool) {
				emit(dispatch, Action{Kind: SmoothCursorChanged, Enabled: value})
			})
		}
	})
	group(c, locales.SettingsEditorGroup(), func() {
		toggle(c, locales.SettingsHeadingMarkerReveal(), locales.SettingsHeadingMarkerRevealDetail(), model.HeadingMarkerReveal, func(value bool) {
			emit(dispatch, Action{Kind: HeadingMarkerRevealChanged, Enabled: value})
		})
	})
	if model.Keymap == "vim" {
		group(c, "Vim", func() {
			toggle(c, locales.SettingsVimImageRows(), locales.SettingsVimImageRowsDetail(), model.VimVirtualImageRows, func(value bool) {
				emit(dispatch, Action{Kind: VimVirtualImageRowsChanged, Enabled: value})
			})
			divider(c)
			toggle(c, locales.SettingsVimYankFlash(), locales.SettingsVimYankFlashDetail(), model.VimYankFlash, func(value bool) {
				emit(dispatch, Action{Kind: VimYankFlashChanged, Enabled: value})
			})
		})
	}
	group(c, locales.SettingsSearch(), func() {
		toggle(c, locales.SettingsSearchMatchPulse(), locales.SettingsSearchMatchPulseDetail(), model.SearchMatchPulse, func(value bool) {
			emit(dispatch, Action{Kind: SearchMatchPulseChanged, Enabled: value})
		})
	})
}

func renderAssistant(c *ui.Context, model Assistant, dispatch func(Action)) {
	group(c, locales.AssistantGroup(), func() {
		toggle(c, locales.AiAssistant(), locales.AiAssistantDetail(), model.Enabled, func(value bool) {
			emit(dispatch, Action{Kind: AssistantEnabledChanged, Enabled: value})
		})
	})
	group(c, locales.AiService(), func() {
		row(c, locales.AiService(), "", func() {
			optionButton(c, locales.AiGateway(), model.Provider == "magpie", func() { emit(dispatch, Action{Kind: ProviderChanged, Text: "magpie"}) })
			optionButton(c, locales.AiCustom(), model.Provider != "magpie", func() { emit(dispatch, Action{Kind: ProviderChanged, Text: "custom"}) })
		})
		divider(c)
		textField(c, locales.AiEndpoint(), locales.AiURLDetail(), model.BaseURL, "https://api.openai.com/v1", false, BaseURLChanged, dispatch)
		divider(c)
		textField(c, locales.AiApiKey(), locales.AiKeyDetail(), model.APIKey, "sk-…", true, APIKeyChanged, dispatch)
		divider(c)
		textField(c, locales.AiModel(), locales.AiModelDetail(), model.Model, "gpt-4.1", false, ModelChanged, dispatch)
		divider(c)
		textAreaField(c, locales.AiSystemPrompt(), locales.AiSystemPromptDetail(), model.SystemPrompt, SystemPromptChanged, dispatch)
		ui.Row(c).Gap(8).AlignItems(ui.Center).Padding(0, 3).Children(func() {
			label := locales.AiFetchModels()
			if model.ModelsLoading {
				label = locales.AiFetchModelsLoading()
			} else if len(model.AvailableModels) > 0 {
				label = locales.AiRefreshModels()
			}
			ui.Button(c, label).OnClick(func() { emit(dispatch, Action{Kind: FetchModels}) })
			if len(model.AvailableModels) > 0 {
				ui.Text(c, locales.AiAvailableModels(len(model.AvailableModels))).FontSize(11).TextColor(c.Theme().TextMuted)
			}
			if model.ModelsError != "" {
				ui.Text(c, model.ModelsError).FontSize(10.5).TextColor(c.Theme().Danger).Wrap()
			}
		})
		ui.Row(c).Justify(ui.End).Children(func() {
			ui.Button(c, locales.AiSaveModelSettings()).OnClick(func() { emit(dispatch, Action{Kind: SaveAssistant}) })
		})
	})
	group(c, locales.AiAgentCapabilities(), func() {
		toggle(c, locales.AiBrowseSearch(), locales.AiBrowseSearchDetail(), model.Capabilities.Browse, func(value bool) {
			emit(dispatch, Action{Kind: CapabilityChanged, Capability: BrowseCapability, Enabled: value})
		})
		divider(c)
		toggle(c, locales.AiEditDocuments(), locales.AiEditDocumentsDetail(), model.Capabilities.Edit, func(value bool) {
			emit(dispatch, Action{Kind: CapabilityChanged, Capability: EditCapability, Enabled: value})
		})
		divider(c)
		toggle(c, locales.AiManageFiles(), locales.AiManageFilesDetail(), model.Capabilities.Manage, func(value bool) {
			emit(dispatch, Action{Kind: CapabilityChanged, Capability: ManageCapability, Enabled: value})
		})
	})
	if model.SettingsPath != "" {
		ui.Text(c, locales.AiConfigLocation(model.SettingsPath)).FontSize(10.5).TextColor(c.Theme().TextMuted).SingleLine().Ellipsis("…")
	}
	if model.CapabilitiesDetail != "" {
		ui.Text(c, model.CapabilitiesDetail).FontSize(11.5).TextColor(c.Theme().TextMuted).Padding(0, 4).Wrap()
	}
}

func textField(c *ui.Context, label, detail, value, placeholder string, password bool, kind ActionKind, dispatch func(Action)) {
	row(c, label, detail, func() {
		next := value
		field := ui.TextInput(c, &next).Width(250).Placeholder(placeholder)
		if password {
			field.Password()
		}
		if next != value {
			emit(dispatch, Action{Kind: kind, Text: next})
		}
	})
}

func textAreaField(c *ui.Context, label, detail, value string, kind ActionKind, dispatch func(Action)) {
	row(c, label, detail, func() {
		next := value
		ui.TextAreaBase(c, &next).Width(250).Lines(2, 5).Placeholder(locales.AiSystemPromptPlaceholder())
		if next != value {
			emit(dispatch, Action{Kind: kind, Text: next})
		}
	})
}

func group(c *ui.Context, title string, content func()) {
	t := c.Theme()
	ui.Column(c).FillWidth().Gap(7).Children(func() {
		if title != "" {
			ui.Text(c, title).FontSize(12).FontWeight(600).TextColor(t.TextMuted).Padding(0, 4)
		}
		ui.Column(c).FillWidth().Radius(12).Background(t.Surface).Padding(4, 16).Children(content)
	})
}

func row(c *ui.Context, label, detail string, control func()) {
	t := c.Theme()
	ui.Row(c).FillWidth().Padding(11, 0).Gap(16).AlignItems(ui.Center).Children(func() {
		ui.Column(c).Grow(1).MinWidth(0).Gap(3).Children(func() {
			ui.Text(c, label).FontSize(13.5).TextColor(t.Text)
			if detail != "" {
				ui.Text(c, detail).FontSize(12).TextColor(t.TextMuted).Wrap()
			}
		})
		ui.Row(c).Shrink(0).Gap(8).AlignItems(ui.Center).Children(control)
	})
}

func toggle(c *ui.Context, label, detail string, checked bool, changed func(bool)) {
	t := c.Theme()
	ui.Row(c).FillWidth().Padding(11, 0).Gap(16).AlignItems(ui.Center).Children(func() {
		ui.Column(c).Grow(1).MinWidth(0).Gap(3).Children(func() {
			ui.Text(c, label).FontSize(13.5).TextColor(t.Text)
			if detail != "" {
				ui.Text(c, detail).FontSize(12).TextColor(t.TextMuted).Wrap()
			}
		})
		value := checked
		if ui.Switch(c, &value).Label(locales.SettingsToggleStateLabel(label)).Changed() {
			changed(value)
		}
	})
}

func sliderRow(c *ui.Context, label, detail string, value, minValue, maxValue float64, format string, changed func(float64)) {
	row(c, label, detail, func() {
		next := value
		ui.Textf(c, format, value).FontSize(12).TextColor(c.Theme().TextMuted).Width(38)
		ui.Slider(c, &next, minValue, maxValue).Width(190).Label(label)
		if next != value {
			changed(next)
		}
	})
}

func optionButton(c *ui.Context, label string, selected bool, onClick func()) {
	t := c.Theme()
	b := ui.ButtonBase(c).Height(28).Padding(0, 9).Radius(6).Label(label)
	if selected {
		b.Background(t.Background).Border(1, t.Border)
	} else if b.Hovered() {
		b.Background(t.SurfaceHover)
	}
	b.OnClick(onClick).Children(func() { ui.Text(c, label).FontSize(11.5).TextColor(t.Text) })
}

func divider(c *ui.Context) {
	ui.Box(c).FillWidth().Height(1).Background(c.Theme().Border)
}

func emit(dispatch func(Action), action Action) {
	if dispatch != nil {
		dispatch(action)
	}
}
