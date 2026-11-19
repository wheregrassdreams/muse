package nativeui

import (
	"fmt"
	"os"
	"strconv"

	"github.com/egoist/mygo"
	documentsave "muse/application/document"
	"muse/locales"
	searchui "muse/nativeui/search/workspace"
	"muse/nativeui/settings"
	shellui "muse/nativeui/shell"
)

// Run is the native executable's compatibility entry. Runtime lifecycle and
// window policy live in shell.Runtime; this function supplies the product
// composition and startup automation.
func Run(deps Dependencies) error {
	a := NewWithDependencies(deps)
	closeGate := &shellui.CloseGate{}
	runtime := shellui.NewRuntime(shellui.RuntimeOptions{
		Name: "Muse", Title: "Muse", Width: 1200, Height: 800,
		MinWidth: 720, MinHeight: 480, BackgroundColor: "light-dark(#f5f5f5, #1a1a1a)",
		StateKey: "muse-native", Transparent: true, Vibrancy: mygo.VibrancyAcrylic,
		TitleBarStyle: mygo.TitleBarHidden, TitleBarHeight: 44,
		TrafficLightPosition: &mygo.Point{X: 12, Y: 15}, View: a.View,
		InstallMenu:     a.installNativeMenu,
		OnReady:         func() { a.startupFromEnvironment() },
		OnThemeUpdated:  a.invalidate,
		OnLocaleChanged: func(string) { a.invalidate() },
		IsDirty: func() bool {
			commands := a.noteEditor()
			return commands != nil && commands.Snapshot().Dirty
		},
		IsBusy: func() bool { return a.busy },
		CloseDecision: func() shellui.CloseDecision {
			commands := a.noteEditor()
			dirty := commands != nil && commands.Snapshot().Dirty
			return closeGate.Request(dirty, a.busy)
		},
		CompleteClose: closeGate.Finish,
		SaveBeforeClose: func() error {
			if a.documentSave == nil {
				return documentsave.ErrWorkflowUnavailable
			}
			err := a.documentSave.SaveCurrent()
			if err != nil {
				a.notice(fmt.Sprintf("Save failed: %v", err))
			}
			return err
		},
		ConfirmClose: func(done func(shellui.CloseResolution)) {
			go func() {
				result, err := mygo.Dialog.Message(mygo.MessageOptions{
					Parent: a.win, Type: mygo.MessageQuestion, Title: "Unsaved changes",
					Message: "Save changes before closing Muse?",
					Buttons: []string{"Save", "Discard", "Cancel"}, DefaultButton: 0, CancelButton: 2,
				})
				if err != nil {
					done(shellui.CloseCancel)
					return
				}
				switch result.Button {
				case 0:
					done(shellui.CloseSave)
				case 1:
					done(shellui.CloseDiscard)
				default:
					done(shellui.CloseCancel)
				}
			}()
		},
		OnWindowClose: func() {
			if a.workspaceSession != nil {
				a.workspaceRestorer.Persist(a.workspaceSession.Snapshot())
			}
		},
		OnWillQuit: func(*mygo.QuitEvent) {
			if a.agentWorkspaceSubscription != nil {
				a.agentWorkspaceSubscription.Close()
			}
			if a.agentWorkspaceEvents != nil {
				a.agentWorkspaceEvents.Close()
			}
			if a.vault != nil {
				go a.vault.Close()
			}
		},
		CapturePath: os.Getenv("MUSE_CAPTURE"), CaptureDelay: shellui.CaptureDelayFromEnv(),
	})
	return runtime.Run()
}

func (a *App) startupFromEnvironment() {
	if a == nil {
		return
	}
	switch os.Getenv("MUSE_THEME") {
	case "light":
		mygo.Theme.SetSource(mygo.ThemeLight)
	case "dark":
		mygo.Theme.SetSource(mygo.ThemeDark)
	}
	if root := os.Getenv("MUSE_WORKSPACE"); root != "" {
		a.setWorkspace(root)
	}
	if path := os.Getenv("MUSE_OPEN"); path != "" {
		a.requestOpen(path)
	}
	if os.Getenv("MUSE_SETTINGS") == "1" && a.settingsPanel != nil {
		a.settingsPanel.OpenAt(settings.AppearancePage)
	}
	if offset, err := strconv.Atoi(os.Getenv("MUSE_SCROLL")); err == nil && offset >= 0 {
		if commands := a.noteEditor(); commands != nil {
			commands.NavigationCommands().ScrollToOffset(offset)
		}
	}
	switch os.Getenv("MUSE_PANEL") {
	case "properties":
		if a.propertiesPanel != nil {
			a.propertiesPanel.Open()
		}
	case "vault-search":
		a.openWorkspaceSearch()
		if query := os.Getenv("MUSE_SEARCH_QUERY"); query != "" {
			a.workspaceSearch.Apply(searchui.Action{Kind: searchui.QueryChanged, Query: query})
		}
	case "outline":
		a.togglePanel(shellui.SurfaceOutline)
	case "search":
		a.togglePanel(shellui.SurfaceSearch)
	case "ai":
		a.togglePanel(shellui.SurfaceAgent)
	case "closed":
		a.panelOpen, a.panelW = false, 0
	}
	a.invalidate()
}

func (a *App) installNativeMenu(window *mygo.Window) {
	if a == nil || window == nil {
		return
	}
	click := func(fn func()) func(*mygo.MenuItem, *mygo.Window) {
		return func(*mygo.MenuItem, *mygo.Window) {
			fn()
			a.invalidate()
		}
	}
	mygo.App.SetMenu(mygo.NewMenu([]*mygo.MenuItem{
		{Role: mygo.RoleAppMenu},
		{Label: "File", Submenu: []*mygo.MenuItem{
			{Label: "New document", Accelerator: "CmdOrCtrl+N", Click: click(a.newDoc)},
			{Label: "Open file…", Accelerator: "CmdOrCtrl+O", Click: click(a.openFileDialog)},
			{Label: "Open workspace…", Accelerator: "Shift+CmdOrCtrl+O", Click: click(a.openFolderDialog)},
			mygo.Separator(),
			{Label: "Save", Accelerator: "CmdOrCtrl+S", Click: click(a.save)},
			{Label: "Save as…", Accelerator: "Shift+CmdOrCtrl+S", Click: click(func() {
				if a.documentSave != nil {
					a.documentSave.RequestSaveAs()
				}
			})},
			mygo.Separator(), {Role: mygo.RoleClose},
		}},
		{Role: mygo.RoleEditMenu},
		{Label: "View", Submenu: []*mygo.MenuItem{
			{Label: locales.MenuCommandPalette(), Accelerator: "CmdOrCtrl+P", Click: click(a.openCommandPalette)},
		}},
	}))
}
