package nativeui

import (
	"muse/locales"
	"path/filepath"
	"strings"
	"sync"

	"github.com/egoist/mygo/ui"
	applicationvault "muse/application/vault"
	componenticons "muse/nativeui/components/icons"
	"muse/nativeui/workbench/features/naviconpicker"
	"muse/ui/iconlibrary"
)

var navIconSVGCache sync.Map

func navIconFromLibrary(id string) (*ui.SVG, bool) {
	if value, ok := navIconSVGCache.Load(id); ok {
		return value.(*ui.SVG), true
	}
	asset, ok := iconlibrary.Lookup(id)
	if !ok {
		return nil, false
	}
	parsed := ui.MustParseSVG([]byte(asset.SVG))
	value, _ := navIconSVGCache.LoadOrStore(id, parsed)
	return value.(*ui.SVG), true
}

func navIconOptionFor(icon iconlibrary.Icon) (naviconpicker.Option, bool) {
	parsed, ok := navIconFromLibrary(icon.ID)
	if !ok {
		return naviconpicker.Option{}, false
	}
	return naviconpicker.Option{ID: icon.ID, Label: icon.Name, Icon: parsed}, true
}

func (a *App) ensureNavIconStore() *applicationvault.IconPreferences {
	if a.navIconStore == nil {
		a.notice(locales.CommonOpenIconSettingsError("navigation icon storage is not configured"))
		return nil
	}
	return a.navIconStore
}

func (a *App) navItemPath(path string) string {
	if a.root == "" {
		return ""
	}
	rel, err := filepath.Rel(a.root, path)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return ""
	}
	return filepath.ToSlash(rel)
}

func (a *App) treeIconID(path string) string {
	store := a.ensureNavIconStore()
	if store == nil {
		return ""
	}
	return store.Get(a.root, a.navItemPath(path))
}

func (a *App) treeIcon(path string, directory bool) *ui.SVG {
	if id := a.treeIconID(path); id != "" {
		if icon, ok := navIconFromLibrary(id); ok {
			return icon
		}
	}
	if directory {
		return componenticons.Folder()
	}
	return componenticons.File()
}

func (a *App) openNavIconPicker(path string) {
	if a.ensureNavIconStore() == nil {
		return
	}
	a.navIconPicker.Open(path)
	a.requestNavIconSearch("")
	a.invalidate()
}

func (a *App) requestNavIconSearch(query string) {
	request := a.navIconPicker.BeginSearch()
	go func() {
		icons := iconlibrary.Search(query)
		results := make([]naviconpicker.Option, 0, len(icons))
		for _, icon := range icons {
			if option, ok := navIconOptionFor(icon); ok {
				results = append(results, option)
			}
		}
		if a.navIconPicker.CompleteSearch(request, results) {
			a.invalidate()
		}
	}()
}

func (a *App) cancelNavIconSearch() {
	a.navIconPicker.CancelSearch()
}

func (a *App) chooseNavIcon(id string) {
	if store := a.ensureNavIconStore(); store != nil {
		item := a.navItemPath(a.navIconPicker.Snapshot().Path)
		var err error
		if id == "" {
			err = store.Delete(a.root, item)
		} else {
			err = store.Set(a.root, item, id)
		}
		if err != nil {
			a.notice(locales.CommonSaveIconError(err.Error()))
		} else {
			a.notice(locales.NavigationIconUpdated())
		}
	}
	a.cancelNavIconSearch()
	a.navIconPicker.Close()
	a.invalidate()
}

func (a *App) navIconPickerPopover(c *ui.Context, anchor ui.Element) {
	if !a.navIconPicker.IsOpen() {
		return
	}
	naviconpicker.RenderPopover(c, anchor, a.navIconPicker, func(action naviconpicker.Action) {
		switch action.Kind {
		case naviconpicker.QueryChanged:
			query := a.navIconPicker.Apply(action)
			a.requestNavIconSearch(query)
		case naviconpicker.ScrollChanged:
			a.navIconPicker.Apply(action)
		case naviconpicker.Choose:
			a.chooseNavIcon(action.ID)
		case naviconpicker.RestoreDefault:
			a.chooseNavIcon("")
		case naviconpicker.Close:
			a.cancelNavIconSearch()
			a.navIconPicker.Close()
		}
	})
}
