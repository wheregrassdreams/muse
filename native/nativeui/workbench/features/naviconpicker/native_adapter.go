package naviconpicker

import (
	"errors"

	vaulticons "muse/application/vault/iconpreferences"
	"muse/locales"
)

// NativePorts are the filesystem and shell capabilities needed by the native
// navigation-icon feature. The feature adapts the application vault service
// to its path-relative preference contract and owns localized notices.
type NativePorts struct {
	Repository   vaulticons.NavigationIconRepository
	VaultRoot    func() string
	RelativePath func(absolutePath string) (itemPath string, ok bool)
	Invalidate   func()
	Notify       func(string)
}

type nativePreferences struct {
	store *vaulticons.IconPreferences
	root  func() string
}

func NewNativeHost(ports NativePorts) *Host {
	var preferences Preferences
	if ports.Repository != nil {
		if store, err := vaulticons.NewIconPreferences(ports.Repository); err == nil {
			preferences = nativePreferences{store: store, root: ports.VaultRoot}
		}
	}
	return NewHost(Ports{
		Preferences: preferences, RelativePath: ports.RelativePath,
		Invalidate: ports.Invalidate,
		Notify: func(notice Notice) {
			if ports.Notify == nil {
				return
			}
			switch notice.Kind {
			case PreferencesUnavailable:
				ports.Notify(locales.CommonOpenIconSettingsError("navigation icon storage is not configured"))
			case PreferenceSaveFailed:
				if notice.Err != nil {
					ports.Notify(locales.CommonSaveIconError(notice.Err.Error()))
				}
			case PreferenceUpdated:
				ports.Notify(locales.NavigationIconUpdated())
			}
		},
	})
}

func (p nativePreferences) Get(path string) string {
	if p.store == nil || p.root == nil {
		return ""
	}
	return p.store.Get(p.root(), path)
}

func (p nativePreferences) Set(path, id string) error {
	if p.store == nil || p.root == nil {
		return errors.New("navigation icon storage is not configured")
	}
	return p.store.Set(p.root(), path, id)
}

func (p nativePreferences) Delete(path string) error {
	if p.store == nil || p.root == nil {
		return errors.New("navigation icon storage is not configured")
	}
	return p.store.Delete(p.root(), path)
}
