package services

import (
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/plugins/updater"
)

type UpdatePreferences struct {
	Version            string `json:"version"`
	Enabled            bool   `json:"enabled"`
	AutomaticChecks    bool   `json:"automaticChecks"`
	AutomaticDownloads bool   `json:"automaticDownloads"`
	LastCheck          string `json:"lastCheck"`
}

func (a *App) UpdatePreferences() UpdatePreferences {
	lastCheck := ""
	if last := updater.LastCheck(); !last.IsZero() {
		lastCheck = last.Format("2006-01-02T15:04:05Z07:00")
	}
	return UpdatePreferences{Version: mygo.App.Version(), Enabled: mygo.Updater.Enabled(),
		AutomaticChecks: updater.AutomaticChecks(), AutomaticDownloads: updater.AutomaticDownloads(), LastCheck: lastCheck}
}

func (a *App) SetUpdatePreferences(req struct {
	AutomaticChecks    bool `json:"automaticChecks"`
	AutomaticDownloads bool `json:"automaticDownloads"`
}) UpdatePreferences {
	updater.SetAutomaticChecks(req.AutomaticChecks)
	updater.SetAutomaticDownloads(req.AutomaticDownloads)
	return a.UpdatePreferences()
}

func (a *App) CheckForUpdates() {
	updater.CheckForUpdates()
}
