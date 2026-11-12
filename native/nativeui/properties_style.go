package nativeui

import (
	"path/filepath"
	"strings"

	"github.com/egoist/mygo/ui"
	"muse/frontmatter"
	"muse/vaultindex"
)

// Metadata has its own quiet palette: document prose remains more prominent.
func propertyQuietColor(t *ui.Theme) ui.Color {
	if t.Dark {
		return ui.Hex("#949492")
	}
	return ui.Hex("#929290")
}

func propertyValueColor(t *ui.Theme) ui.Color {
	if t.Dark {
		return ui.Hex("#dededb")
	}
	return ui.Hex("#323231")
}

func propertyEmptyColor(t *ui.Theme) ui.Color {
	if t.Dark {
		return ui.Hex("#666664")
	}
	return ui.Hex("#bcbcb9")
}

func propertyCardColor(t *ui.Theme) ui.Color {
	if t.Dark {
		return ui.Hex("#2d2d30")
	}
	return ui.Hex("#f7f7f7")
}

func propertyOmitted(key string) bool {
	switch strings.ToLower(key) {
	case "title", "created by", "created_by", "createdby", "last modified by", "modified_by", "links":
		return true
	}
	return false
}

func propertySecondary(key string) bool {
	switch strings.ToLower(key) {
	case "updated", "modified", "modified_at", "last_modified", "last modified date", "last_opened", "last opened date":
		return true
	}
	return false
}

func propertyLabel(key string) string {
	switch strings.ToLower(key) {
	case "type":
		return "Type"
	case "tags":
		return "Tag"
	case "created", "created_at", "creation date":
		return "Creation date"
	case "updated", "modified", "modified_at", "last_modified":
		return "Last modified date"
	case "last_opened":
		return "Last opened date"
	case "status":
		return "Status"
	case "aliases":
		return "Aliases"
	case "date":
		return "Date"
	}
	return key
}

func propertyHasField(d frontmatter.Document, key string) bool {
	for _, f := range d.Fields {
		if strings.EqualFold(f.Key, key) || propertyLabel(f.Key) == propertyLabel(key) {
			return true
		}
	}
	return false
}

func propertyTypeField(d frontmatter.Document) frontmatter.Field {
	for _, f := range d.Fields {
		if strings.EqualFold(f.Key, "type") {
			return f
		}
	}
	return frontmatter.Field{Key: "type", Value: "Note", Kind: "text", Editable: true}
}

func propertyDocumentType(d frontmatter.Document) string {
	if v := strings.TrimSpace(propertyTypeField(d).Value); v != "" {
		return v
	}
	return "Note"
}

func (a *App) propertyTags(c *ui.Context, value string, limit int) {
	t := c.Theme()
	row := ui.Row(c).MinWidth(0).Gap(5).AlignItems(ui.Center)
	if limit == 0 {
		row.FillWidth().Wrap()
	}
	row.Children(func() {
		shown := 0
		for _, tag := range strings.Split(value, ",") {
			tag = strings.TrimSpace(tag)
			if tag == "" {
				continue
			}
			if limit > 0 && shown >= limit {
				ui.Text(c, "…").FontSize(13).TextColor(propertyQuietColor(t))
				break
			}
			bg, fg := propertyTagColors(tag, t.Dark)
			ui.Text(c, tag).Key("property-tag-"+tag).MaxWidth(150).FontSize(13).TextColor(fg).
				Padding(6, 10).Radius(18).Background(bg).SingleLine().Ellipsis("…")
			shown++
		}
	})
}

type propertyBacklinkCache struct {
	context  string
	revision uint64
	vault    *vaultRuntime
	links    []vaultindex.Link
}

func (a *App) documentBacklinks() []vaultindex.Link {
	v := a.vault
	if v == nil || v.index == nil || a.path == "" {
		return nil
	}
	context := a.editorLinkContext()
	cache := &a.propertyBacklinks
	if cache.context != context || cache.vault != v || cache.revision != v.linkRevision {
		cache.context, cache.vault, cache.revision = context, v, v.linkRevision
		cache.links = nil
		seen := make(map[string]bool)
		for _, link := range v.index.Backlinks(relativeVaultPath(v.root, a.path)) {
			if !seen[link.Source] {
				cache.links = append(cache.links, link)
				seen[link.Source] = true
			}
		}
	}
	return cache.links
}

func (a *App) propertyBacklinkCard(c *ui.Context) {
	t := c.Theme()
	links := a.documentBacklinks()
	ui.Row(c).Key("property-card-backlinks").FillWidth().MinHeight(64).Padding(18, 22).Gap(12).AlignItems(ui.Center).
		Radius(20).Background(propertyCardColor(t)).Children(func() {
		ui.Text(c, "Backlinks").FontSize(14).TextColor(propertyQuietColor(t)).Shrink(0)
		if len(links) == 0 {
			ui.Text(c, "无数值").FontSize(14).TextColor(propertyEmptyColor(t))
			return
		}
		ui.Column(c).Grow(1).MinWidth(0).Gap(6).Children(func() {
			for _, link := range links {
				l := link
				label := strings.TrimSuffix(filepath.Base(l.Source), filepath.Ext(l.Source))
				b := ui.ButtonBase(c).FillWidth().MinHeight(28).Padding(0).Justify(ui.Start).Label("属性反向链接：" + l.Source)
				b.OnClick(func() { a.navigateIndexed(l.Source, l.Start) })
				b.Children(func() { ui.Text(c, label).FontSize(15).TextColor(propertyValueColor(t)).SingleLine().Ellipsis("…") })
			}
		})
	})
}

func (a *App) openPropertySource() {
	a.propertySourceMode = true
	a.propertySourceDraft = a.yamlPropertySource()
	a.invalidate()
}

func (a *App) propertyFieldMenu(m *ui.Menu, f frontmatter.Field) {
	pref := a.propertyPrefs.Fields[f.Key]
	pin, hide := "置顶", "隐藏"
	if pref.Pinned {
		pin = "取消置顶"
	}
	if pref.Hidden {
		hide = "显示"
	}
	if m.Item(pin).Chosen() {
		a.updatePropertyPreference(f.Key, func(p *fieldDisplayPreference) { p.Pinned = !p.Pinned })
	}
	if m.Item(hide).Chosen() {
		a.updatePropertyPreference(f.Key, func(p *fieldDisplayPreference) { p.Hidden = !p.Hidden })
	}
	if m.Item("上移").Chosen() {
		a.moveProperty(f.Key, -1)
	}
	if m.Item("下移").Chosen() {
		a.moveProperty(f.Key, 1)
	}
	m.Separator()
	if m.Item("删除属性").Chosen() {
		if err := a.propertyDelete(f.Key); err != nil {
			a.propertyError = err.Error()
		}
	}
}

func (a *App) propertyHiddenSection(c *ui.Context, fields []frontmatter.Field) {
	t := c.Theme()
	b := ui.ButtonBase(c).FillWidth().Height(38).Padding(0, 8).Gap(7).Justify(ui.Start).Label("隐藏属性")
	b.OnClick(func() { a.propertyHiddenOpen = !a.propertyHiddenOpen; a.invalidate() })
	b.Children(func() {
		icon := iconChevronDown
		if !a.propertyHiddenOpen {
			icon = iconChevron
		}
		ui.Icon(c, icon).Size(16, 16).TextColor(propertyQuietColor(t))
		ui.Text(c, "隐藏").FontSize(14).TextColor(propertyQuietColor(t))
	})
	if !a.propertyHiddenOpen {
		return
	}
	for _, f := range fields {
		a.propertyFieldRow(c, f)
	}
	d := a.propertyDocument()
	for _, p := range a.propertySystem {
		if p.key == "Creation date" || propertyHasField(d, p.key) {
			continue
		}
		a.propertyReadonlyCard(c, p.key, p.value)
	}
	if !a.propertyOpenedAt.IsZero() && !propertyHasField(d, "last_opened") {
		a.propertyReadonlyCard(c, "Last opened date", a.propertyOpenedAt.Local().Format("2006年1月2日 15:04"))
	}
}

func (a *App) propertyReadonlyCard(c *ui.Context, key, value string) {
	t := c.Theme()
	ui.Column(c).FillWidth().Gap(8).Padding(18, 22).Radius(20).Background(propertyCardColor(t)).Children(func() {
		ui.Text(c, key).FontSize(14).TextColor(propertyQuietColor(t))
		ui.Text(c, value).FillWidth().FontSize(15).TextColor(propertyValueColor(t)).Wrap()
	})
}
