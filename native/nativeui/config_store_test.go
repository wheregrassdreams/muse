package nativeui

import "muse/platform/filesystem/configstore"

func newConfigTestApp() *App {
	store := configstore.New()
	app := New()
	app.SetDependencies(Dependencies{
		Configuration:             store,
		AssistantSettingsLocation: store.AssistantSettingsLocation(),
	})
	return app
}
