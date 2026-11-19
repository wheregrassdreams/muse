package settings

import (
	"testing"

	"github.com/egoist/mygo/ui"
	"muse/locales"
)

func TestHostKeepsNavigationLocalAndRoutesTypedPreferenceRequests(t *testing.T) {
	var received []Action
	host := NewHost(Ports{
		Snapshot: func() ViewModel { return ViewModel{} },
		Dispatch: func(action Action) { received = append(received, action) },
	})
	host.OpenAt(AppearancePage)
	host.Dispatch(Action{Kind: SelectPage, Page: AssistantPage})
	if host.Page() != AssistantPage || len(received) != 0 {
		t.Fatalf("local page transition = %v, forwarded=%+v", host.Page(), received)
	}
	host.Dispatch(Action{Kind: ThemeChanged, Theme: ThemeDark})
	if len(received) != 1 || received[0].Kind != ThemeChanged || received[0].Theme != ThemeDark {
		t.Fatalf("routed preference actions = %+v", received)
	}
	host.Dispatch(Action{Kind: Close})
	if host.IsOpen() || len(received) != 1 {
		t.Fatalf("close state=%t, forwarded=%+v", host.IsOpen(), received)
	}
}

func TestHostRendersSnapshotAndRoutesViewActionsThroughPorts(t *testing.T) {
	var received []Action
	host := NewHost(Ports{
		Snapshot: func() ViewModel {
			return ViewModel{Appearance: Appearance{Theme: ThemeSystem}}
		},
		Dispatch: func(action Action) { received = append(received, action) },
	})
	host.OpenAt(AppearancePage)
	tester := ui.NewTester(func(c *ui.Context) { host.RenderDialog(c) }, 820, 620)
	tester.Frame()
	if !tester.HasText(locales.SettingsAppearance()) {
		t.Fatal("host did not render the feature-owned settings dialog")
	}
	if err := tester.Click(locales.ThemeDark()); err != nil {
		t.Fatal(err)
	}
	if len(received) != 1 || received[0].Kind != ThemeChanged || received[0].Theme != ThemeDark {
		t.Fatalf("view action port received %+v", received)
	}
}
