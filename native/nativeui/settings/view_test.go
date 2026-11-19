package settings

import (
	"testing"

	"github.com/egoist/mygo/ui"
	"muse/locales"
)

func TestThemeChoiceEmitsTypedTheme(t *testing.T) {
	var actions []Action
	state := newController()
	tester := ui.NewTester(func(c *ui.Context) {
		render(c, state, ViewModel{}, func(action Action) { actions = append(actions, action) })
	}, 820, 620)
	tester.Frame()
	if err := tester.Click(locales.ThemeDark()); err != nil {
		t.Fatal(err)
	}
	if len(actions) != 1 || actions[0].Kind != ThemeChanged || actions[0].Theme != ThemeDark {
		t.Fatalf("actions = %+v, want dark theme change", actions)
	}
}

func TestPageNavigationEmitsTypedPage(t *testing.T) {
	var actions []Action
	state := newController()
	tester := ui.NewTester(func(c *ui.Context) {
		render(c, state, ViewModel{}, func(action Action) { actions = append(actions, action) })
	}, 820, 620)
	tester.Frame()
	if err := tester.Click(locales.SettingsEditor()); err != nil {
		t.Fatal(err)
	}
	if len(actions) != 1 || actions[0].Kind != SelectPage || actions[0].Page != EditorPage {
		t.Fatalf("actions = %+v, want editor-page selection", actions)
	}
}

func TestAssistantSaveAndCapabilityActions(t *testing.T) {
	var actions []Action
	state := newController()
	state.SetPage(AssistantPage)
	tester := ui.NewTester(func(c *ui.Context) {
		render(c, state, ViewModel{}, func(action Action) { actions = append(actions, action) })
	}, 820, 620)
	tester.Frame()
	if err := tester.Click(locales.AiSaveModelSettings()); err != nil {
		t.Fatal(err)
	}
	if len(actions) != 1 || actions[0].Kind != SaveAssistant {
		t.Fatalf("actions = %+v, want save-assistant", actions)
	}
}
