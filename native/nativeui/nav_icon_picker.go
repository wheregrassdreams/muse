package nativeui

import (
	"fmt"
	"path/filepath"
	"strings"
	"sync"

	"github.com/egoist/mygo/ui"

	"muse/iconlibrary"
	"muse/navicons"
)

const (
	navIconColumns      = 5
	navIconRowHeight    = float32(58)
	navIconVisibleRows  = 6
	navIconPickerHeight = float32(444)
)

type navIconOption struct {
	id, label string
	icon      *ui.SVG
}

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

func navIconOptionFor(icon iconlibrary.Icon) (navIconOption, bool) {
	parsed, ok := navIconFromLibrary(icon.ID)
	if !ok {
		return navIconOption{}, false
	}
	return navIconOption{id: icon.ID, label: icon.Name, icon: parsed}, true
}

func (a *App) ensureNavIconStore() *navicons.Store {
	if a.navIconStore == nil {
		store, err := navicons.New()
		if err != nil {
			a.notice("无法打开图标设置：" + err.Error())
			return nil
		}
		a.navIconStore = store
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
			a.notice("保存图标失败：" + err.Error())
		} else {
			a.notice("已更新导航图标")
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
		a.navIconPickerContent(c)
	})
}

func (a *App) navIconPickerContent(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).Key("nav-icon-picker").Width(336).Height(navIconPickerHeight).Gap(8).Padding(14).Radius(14).
		Background(t.Background).Border(1, t.Border).
		Shadow(0, 14, 34, -4, ui.RGBA(0, 0, 0, 0.22)).Children(func() {
		ui.Row(c).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "选择图标").FontSize(14).FontWeight(650)
			ui.Spacer(c)
			ui.Text(c, filepath.Base(a.navIconPickerPath)).FontSize(11).TextColor(t.TextMuted).SingleLine().Ellipsis("…")
			a.smallIcon(c, iconClose, "关闭", func() {
				a.cancelNavIconSearch()
				a.navIconPickerOpen = false
			})
		})
		ui.TextInput(c, &a.navIconPickerQuery).Placeholder("搜索 2,356 个图标").Label("搜索图标")
		query := strings.TrimSpace(a.navIconPickerQuery)
		if query != a.navIconPickerLastQuery {
			a.navIconPickerScroll.Y = 0
			a.navIconPickerLastQuery = query
			a.requestNavIconSearch(query)
		}
		matches, loading := a.navIconSearchSnapshot()
		countLabel := "全部图标"
		if query != "" {
			countLabel = "搜索结果"
		}
		ui.Text(c, fmt.Sprintf("%s · %d 个图标", countLabel, len(matches))).FontSize(10).TextColor(t.TextMuted)
		rows := (len(matches) + navIconColumns - 1) / navIconColumns
		firstRow := max(int(a.navIconPickerScroll.Y/navIconRowHeight)-1, 0)
		lastRow := min(firstRow+navIconVisibleRows+2, rows)
		ui.Scroll(c).FillWidth().Height(312).TrackScroll(&a.navIconPickerScroll).Scrollbars(a.navIconPickerScroll.MaxY > 0).Children(func() {
			ui.Column(c).FillWidth().Children(func() {
				if firstRow > 0 {
					ui.Box(c).FillWidth().Height(float32(firstRow) * navIconRowHeight)
				}
				for rowIndex := firstRow; rowIndex < lastRow; rowIndex++ {
					start := rowIndex * navIconColumns
					end := min(start+navIconColumns, len(matches))
					ui.Row(c).Height(navIconRowHeight).AlignItems(ui.Center).Gap(4).Children(func() {
						for _, icon := range matches[start:end] {
							option, ok := navIconOptionFor(icon)
							if !ok {
								continue
							}
							button := ui.ButtonBase(c).Size(56, 54).Radius(8).Padding(3).
								Justify(ui.Center).AlignItems(ui.Center).Label(option.label).Tooltip(option.label)
							if button.Hovered() {
								button.Background(t.SurfaceHover)
							}
							button.OnClick(func() { a.chooseNavIcon(option.id) }).Children(func() {
								ui.Column(c).Gap(2).AlignItems(ui.Center).Children(func() {
									ui.Icon(c, option.icon).Size(19, 19).TextColor(t.Text)
									ui.Text(c, option.label).FontSize(8).TextColor(t.TextMuted).SingleLine().Ellipsis("…")
								})
							})
						}
					})
				}
				if lastRow < rows {
					ui.Box(c).FillWidth().Height(float32(rows-lastRow) * navIconRowHeight)
				}
				if loading {
					ui.Text(c, "正在加载图标…").FillWidth().Center().TextColor(t.TextMuted)
				} else if len(matches) == 0 {
					ui.Text(c, "没有匹配的图标").FillWidth().Center().TextColor(t.TextMuted)
				}
			})
		})
		ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
			if ui.Button(c, "恢复默认图标").Clicked() {
				a.chooseNavIcon("")
			}
			if ui.Button(c, "取消").Clicked() {
				a.cancelNavIconSearch()
				a.navIconPickerOpen = false
			}
		})
	})
}
