package nativeui

import (
	"muse/locales"
	"path/filepath"
	"strings"
	"sync"

	"github.com/egoist/mygo/ui"
	applicationvault "muse/application/vault"
	"muse/nativeui/panels/naviconpicker"
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
	return naviconpicker.FromIcon(icon, parsed), true
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
		return iconFolder
	}
	return iconFile
}

func (a *App) openNavIconPicker(path string) {
	if a.ensureNavIconStore() == nil {
		return
	}
	a.navIconPickerPath = path
	a.navIconPickerQuery = ""
	a.navIconPickerLastQuery = ""
	a.navIconPickerScroll = ui.ScrollState{}
	a.navIconPickerOpen = true
	a.requestNavIconSearch("")
	a.invalidate()
}

func (a *App) requestNavIconSearch(query string) {
	a.navIconPickerMu.Lock()
	a.navIconPickerRequest++
	request := a.navIconPickerRequest
	a.navIconPickerLoading = true
	a.navIconPickerResults = nil
	a.navIconPickerMu.Unlock()
	go func() {
		results := iconlibrary.Search(query)
		a.navIconPickerMu.Lock()
		if request == a.navIconPickerRequest {
			a.navIconPickerResults = results
			a.navIconPickerLoading = false
		}
		a.navIconPickerMu.Unlock()
		a.invalidate()
	}()
}

func (a *App) navIconSearchSnapshot() ([]iconlibrary.Icon, bool) {
	a.navIconPickerMu.Lock()
	defer a.navIconPickerMu.Unlock()
	return a.navIconPickerResults, a.navIconPickerLoading
}

func (a *App) cancelNavIconSearch() {
	a.navIconPickerMu.Lock()
	a.navIconPickerRequest++
	a.navIconPickerLoading = false
	a.navIconPickerMu.Unlock()
}

func (a *App) chooseNavIcon(id string) {
	if store := a.ensureNavIconStore(); store != nil {
		item := a.navItemPath(a.navIconPickerPath)
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
	a.navIconPickerOpen = false
	a.invalidate()
}

func (a *App) navIconPickerPopover(c *ui.Context, anchor ui.Element) {
	if !a.navIconPickerOpen {
		return
	}
	ui.Popover(c, anchor, &a.navIconPickerOpen, func() {
		results, loading := a.navIconSearchSnapshot()
		options := make([]naviconpicker.Option, 0, len(results))
		for _, icon := range results {
			option, ok := navIconOptionFor(icon)
			if ok {
				options = append(options, option)
			}
		}
		model := naviconpicker.ViewModel{
			Path: a.navIconPickerPath, Query: a.navIconPickerQuery, Scroll: a.navIconPickerScroll,
			Results: options, Loading: loading,
		}
		naviconpicker.Render(c, model, func(action naviconpicker.Action) {
			switch action.Kind {
			case naviconpicker.QueryChanged:
				a.navIconPickerQuery = action.Query
				a.navIconPickerScroll.Y = 0
				a.navIconPickerLastQuery = strings.TrimSpace(action.Query)
				a.requestNavIconSearch(a.navIconPickerLastQuery)
			case naviconpicker.ScrollChanged:
				a.navIconPickerScroll = action.Scroll
			case naviconpicker.Choose:
				a.chooseNavIcon(action.ID)
			case naviconpicker.RestoreDefault:
				a.chooseNavIcon("")
			case naviconpicker.Close:
				a.cancelNavIconSearch()
				a.navIconPickerOpen = false
			}
		})
	})
}
