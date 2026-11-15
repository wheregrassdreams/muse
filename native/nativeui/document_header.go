package nativeui

import "github.com/egoist/mygo/ui"

func (a *App) propertyHeader() func(*ui.Context) {
	if !a.propertiesEnabled() {
		return nil
	}
	return a.documentMetadata
}
func (a *App) propertyHeaderHeight() float32 {
	if !a.propertiesEnabled() {
		return 0
	}
	return 160
}
