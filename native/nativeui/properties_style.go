package nativeui

import (
	"muse/locales"
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
	switch propertyCanonicalKey(key) {
	case "type":
		return locales.PropertyTypeFieldLabel()
	case "tags":
		return locales.PropertyTagsFieldLabel()
	case "created_at":
		return locales.PropertyCreationDate()
	case "updated_at":
		return locales.PropertyModifiedDate()
	case "last_opened":
		return locales.PropertyLastOpenedDate()
	case "status":
		return locales.PropertyStatusFieldLabel()
	case "aliases":
		return locales.PropertyAliasesFieldLabel()
	case "date":
		return locales.PropertyDateFieldLabel()
	}
	return key
}

func propertyCanonicalKey(key string) string {
	switch strings.ToLower(strings.TrimSpace(key)) {
	case "type":
		return "type"
	case "tags":
		return "tags"
	case "created", "created_at", "creation date":
		return "created_at"
	case "updated", "modified", "modified_at", "last_modified", "last modified date":
		return "updated_at"
	case "last_opened", "last opened date":
		return "last_opened"
	case "status":
		return "status"
	case "aliases":
		return "aliases"
	case "date":
		return "date"
	default:
		return strings.ToLower(strings.TrimSpace(key))
	}
}

func propertyHasField(d frontmatter.Document, key string) bool {
	for _, f := range d.Fields {
		if propertyCanonicalKey(f.Key) == propertyCanonicalKey(key) {
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
	return frontmatter.Field{Key: "type", Value: locales.PropertyDefaultNoteType(), Kind: "text", Editable: true}
}

func propertyDocumentType(d frontmatter.Document) string {
	if v := strings.TrimSpace(propertyTypeField(d).Value); v != "" {
		return v
	}
	return locales.PropertyDefaultNoteType()
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

// documentBacklinkSources returns the shared index's reverse-link projection. The
// index owns grouping and counts, so rendering does not rebuild or cache a
// second copy of the graph in the UI layer.
func (a *App) documentBacklinkSources() []vaultindex.BacklinkSource {
	v := a.vault
	path := a.path
	if a.imageDocument.open && a.imageDocument.path != "" {
		path = a.imageDocument.path
	}
	if v == nil || v.index == nil || path == "" {
		return nil
	}
	return v.index.BacklinkSources(relativeVaultPath(v.root, path))
}

func (a *App) propertyBacklinkCard(c *ui.Context) {
	t := c.Theme()
	sources := a.documentBacklinkSources()
	ui.Row(c).Key("property-card-backlinks").FillWidth().MinHeight(64).Padding(18, 22).Gap(12).AlignItems(ui.Center).
		Radius(20).Background(propertyCardColor(t)).Children(func() {
		ui.Text(c, locales.VaultBacklinksTitle()).FontSize(14).TextColor(propertyQuietColor(t)).Shrink(0)
		if len(sources) == 0 {
			ui.Text(c, locales.PropertyNoValue()).FontSize(14).TextColor(propertyEmptyColor(t))
			return
		}
		ui.Column(c).Grow(1).MinWidth(0).Gap(6).Children(func() {
			for _, source := range sources {
				s := source
				label := strings.TrimSuffix(filepath.Base(s.Source), filepath.Ext(s.Source))
				if s.Count > 1 {
					label += locales.VaultBacklinkCountSuffix(s.Count)
				}
				b := ui.ButtonBase(c).FillWidth().MinHeight(28).Padding(0).Justify(ui.Start).Label(locales.CorePropertyBacklinkLabel(s.Source))
				b.OnClick(func() { a.navigateIndexed(s.Source, s.Start) })
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
	pin, hide := locales.PropertyPin(), locales.PropertyHideFromSummary()
	if pref.Pinned {
		pin = locales.PropertyUnpin()
	}
	if pref.Hidden {
		hide = locales.PropertyShow()
	}
	if m.Item(pin).Chosen() {
		a.updatePropertyPreference(f.Key, func(p *fieldDisplayPreference) { p.Pinned = !p.Pinned })
	}
	if m.Item(hide).Chosen() {
		a.updatePropertyPreference(f.Key, func(p *fieldDisplayPreference) { p.Hidden = !p.Hidden })
	}
	if m.Item(locales.PropertyMoveUp()).Chosen() {
		a.moveProperty(f.Key, -1)
	}
	if m.Item(locales.PropertyMoveDown()).Chosen() {
		a.moveProperty(f.Key, 1)
	}
	m.Separator()
	if m.Item(locales.PropertyDelete()).Chosen() {
		if err := a.propertyDelete(f.Key); err != nil {
			a.propertyError = err.Error()
		}
	}
}

func (a *App) propertyHiddenSection(c *ui.Context, fields []frontmatter.Field) {
	t := c.Theme()
	b := ui.ButtonBase(c).FillWidth().Height(38).Padding(0, 8).Gap(7).Justify(ui.Start).Label(locales.PropertyHide())
	b.OnClick(func() { a.propertyHiddenOpen = !a.propertyHiddenOpen; a.invalidate() })
	b.Children(func() {
		icon := iconChevronDown
		if !a.propertyHiddenOpen {
			icon = iconChevron
		}
		ui.Icon(c, icon).Size(16, 16).TextColor(propertyQuietColor(t))
		ui.Text(c, locales.PropertyHidden()).FontSize(14).TextColor(propertyQuietColor(t))
	})
	if !a.propertyHiddenOpen {
		return
	}
	for _, f := range fields {
		a.propertyFieldRow(c, f)
	}
	d := a.propertyDocument()
	for _, p := range a.propertySystem {
		if p.key == systemPropertyCreatedAt || p.key == systemPropertyUpdatedAt {
			continue
		}
		a.propertyReadonlyCard(c, systemPropertyLabel(p.key), p.value)
	}
	if !a.propertyOpenedAt.IsZero() && !propertyHasField(d, "last_opened") {
		a.propertyReadonlyCard(c, locales.PropertyLastOpenedDate(), locales.PropertyDateTime(a.propertyOpenedAt.Local()))
	}
}

func (a *App) propertyReadonlyCard(c *ui.Context, key, value string) {
	t := c.Theme()
	ui.Column(c).FillWidth().Gap(8).Padding(18, 22).Radius(20).Background(propertyCardColor(t)).Children(func() {
		ui.Text(c, key).FontSize(14).TextColor(propertyQuietColor(t))
		ui.Text(c, value).FillWidth().FontSize(15).TextColor(propertyValueColor(t)).Wrap()
	})
}
