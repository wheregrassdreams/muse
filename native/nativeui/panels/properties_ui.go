package nativeui

import (
	"encoding/json"
	"fmt"
	"hash/fnv"
	"muse/locales"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"muse/frontmatter"
	"muse/notedate"

	"github.com/egoist/mygo/ui"
)

const propertiesSidebarWidth float32 = 340

type propertyDocumentCache struct {
	source, path string
	doc          frontmatter.Document
}

type systemProperty struct{ key, value string }

const (
	systemPropertyFileName  = "file-name"
	systemPropertyPath      = "path"
	systemPropertyWordCount = "word-count"
	systemPropertyCreatedAt = "created-at"
	systemPropertyFileSize  = "file-size"
	systemPropertyUpdatedAt = "updated-at"
)

func systemPropertyLabel(key string) string {
	switch key {
	case systemPropertyFileName:
		return locales.PropertyFileName()
	case systemPropertyPath:
		return locales.PropertyPath()
	case systemPropertyWordCount:
		return locales.PropertyWordCountLabel()
	case systemPropertyCreatedAt:
		return locales.PropertyCreationDate()
	case systemPropertyFileSize:
		return locales.PropertyFileSize()
	case systemPropertyUpdatedAt:
		return locales.PropertyModifiedDate()
	default:
		return key
	}
}

type fieldDisplayPreference struct {
	Pinned bool `json:"pinned,omitempty"`
	Hidden bool `json:"hidden,omitempty"`
	Order  int  `json:"order,omitempty"`
}

type documentPropertyPrefs struct {
	Fields map[string]fieldDisplayPreference `json:"fields,omitempty"`
}

type propertyPrefsFile map[string]documentPropertyPrefs

func (a *App) propertiesEnabled() bool { return !a.imageDocument.open && codeLanguage(a.path) == "" }

// propertyDocument parses only once for each source/path pair. The frontmatter
// parser itself stops at the closing YAML delimiter and never scans the body.
func (a *App) propertyDocument() frontmatter.Document {
	if a.propertyCache.path != a.path {
		a.propertyPrefs = loadDocumentPropertyPrefs(a.path)
	}
	if a.propertyCache.source != a.source || a.propertyCache.path != a.path {
		a.propertyCache = propertyDocumentCache{source: a.source, path: a.path, doc: frontmatter.Parse(a.source)}
	}
	return a.propertyCache.doc
}

// The page title is the first Markdown H1. The header only hosts metadata;
// existing YAML title values stay intact but have no dedicated title UI.
func (a *App) documentMetadata(c *ui.Context) {
	if !a.propertiesEnabled() {
		return
	}
	ui.Column(c).FillWidth().Children(func() {
		ui.Box(c).Height(100).Shrink(0)
		a.propertySummary(c)
	})
}

func (a *App) propertySummary(c *ui.Context) {
	d := a.propertyDocument()
	if d.Err != nil {
		return
	}
	quiet := propertyQuietColor(c.Theme())
	fields := a.orderedProperties(d.Fields)
	visible := summaryPropertyFields(fields, a.propertyPrefs)
	ui.Row(c).Height(36).Shrink(0).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
		ui.Text(c, propertyDocumentType(d)).FontSize(13).TextColor(quiet)
		separator := func() {
			ui.Text(c, "·").FontSize(13).TextColor(quiet)
		}
		if sources := a.documentBacklinkSources(); len(sources) > 0 {
			separator()
			b := ui.ButtonBase(c).Height(32).Padding(0).Label(locales.PropertyViewBacklinks()).OnClick(a.openLinks)
			b.Children(func() { ui.Text(c, locales.PropertyBacklinkCount(len(sources))).FontSize(13).TextColor(quiet) })
		}
		for i, f := range visible {
			separator()
			if i >= 4 {
				a.propertyButton(c, locales.PropertyMoreCount(len(visible)-i), a.openProperties)
				break
			}
			a.propertySummaryItem(c, f)
		}
	})
}

func summaryPropertyFields(fields []frontmatter.Field, prefs documentPropertyPrefs) []frontmatter.Field {
	visible := make([]frontmatter.Field, 0, len(fields))
	for _, f := range fields {
		if propertyOmitted(f.Key) || strings.EqualFold(f.Key, "type") || prefs.Fields[f.Key].Hidden {
			continue
		}
		if strings.TrimSpace(displayPropertyValue(f)) == "" {
			continue
		}
		visible = append(visible, f)
	}
	return visible
}

func (a *App) propertySummaryItem(c *ui.Context, f frontmatter.Field) {
	if !f.Editable {
		t := c.Theme()
		ui.Text(c, displayPropertyValue(f)).FontSize(13).TextColor(propertyQuietColor(t)).Padding(0, 7).SingleLine().Ellipsis("…")
		return
	}
	if a.propertyEditKey == f.Key && a.propertyEditLocation == "summary" {
		a.propertyInlineEditor(c, f, "summary")
		return
	}
	t := c.Theme()
	b := ui.ButtonBase(c).Height(36).MinWidth(0).MaxWidth(260).Padding(0, 1).Radius(5).Label(locales.PropertyEditLabel(f.Key))
	if b.Hovered() {
		b.Background(t.SurfaceHover)
	}
	b.OnClick(func() { a.startPropertyEdit(f, "summary") })
	b.Children(func() {
		val := displayPropertyValue(f)
		switch {
		case strings.EqualFold(f.Key, "tags"):
			a.propertyTags(c, val, 3)

		default:
			ui.Text(c, val).MaxWidth(160).FontSize(13).TextColor(propertyQuietColor(t)).SingleLine().Ellipsis("…")
		}
	})
}

func displayPropertyValue(f frontmatter.Field) string {
	v := strings.TrimSpace(f.Value)
	if f.Kind == "structure" {
		return locales.PropertyStructureSourceOnly()
	}
	if strings.EqualFold(f.Key, "type") && strings.EqualFold(v, "note") {
		return locales.PropertyDefaultNoteType()
	}
	if strings.EqualFold(f.Key, "status") {
		switch strings.ToLower(v) {
		case "draft":
			return locales.PropertyStatusDraft()
		case "done":
			return locales.PropertyStatusDone()
		case "in_progress":
			return locales.PropertyStatusInProgress()
		}
	}
	if f.Kind == "date" || isNaturalDateProperty(f.Key) {
		return naturalPropertyDate(v)
	}
	return v
}

func isNaturalDateProperty(key string) bool {
	switch strings.ToLower(key) {
	case "date", "created", "created_at", "creation date", "updated", "updated_at", "modified", "modified_at", "last_modified", "last modified date", "last_opened", "last opened date":
		return true
	default:
		return false
	}
}

func (a *App) startPropertyEdit(f frontmatter.Field, location string) {
	a.propertyEditKey, a.propertyEditLocation, a.propertyEditDraft = f.Key, location, f.Value
}

func (a *App) propertyInlineEditor(c *ui.Context, f frontmatter.Field, location string) {
	if f.Kind == "bool" {
		ui.Row(c).Gap(4).Children(func() {
			a.propertyButton(c, "true", func() { a.propertySaveField(f.Key, "true") })
			a.propertyButton(c, "false", func() { a.propertySaveField(f.Key, "false") })
			a.propertyButton(c, locales.CommonCancel(), func() { a.propertyEditKey, a.propertyEditLocation = "", "" })
		})
		return
	}
	ui.Row(c).Gap(4).AlignItems(ui.Center).Children(func() {
		input := ui.TextInput(c.Key("property-inline-"+location+"-"+f.Key), &a.propertyEditDraft).
			Width(180).Placeholder(locales.PropertyListPlaceholder()).Label(f.Key).AutoFocus().OnSubmit(func() { a.propertySaveField(f.Key, a.propertyEditDraft) })
		_ = input
		a.propertyButton(c, locales.CommonSave(), func() { a.propertySaveField(f.Key, a.propertyEditDraft) })
		a.propertyButton(c, locales.CommonCancel(), func() { a.propertyEditKey, a.propertyEditLocation = "", "" })
	})
}

func naturalPropertyDate(value string) string {
	parsed, precision, err := notedate.Parse(value)
	if err != nil {
		return strings.TrimSpace(value)
	}
	if precision == notedate.Date {
		year, month, day := parsed.Date()
		return locales.PropertyDate(time.Date(year, month, day, 0, 0, 0, 0, time.Local))
	}
	return locales.PropertyDateTime(parsed.In(time.Local))
}

func (a *App) openProperties() {
	a.propertiesOpen = true
	a.propertySourceMode = false
	a.propertyError = ""
	a.refreshSystemProperties()
	a.invalidate()
}

func (a *App) refreshSystemProperties() {
	chars, _, _ := countText(a.source)
	body := a.source
	if d := a.propertyDocument(); d.Err == nil && d.End > 0 && d.End <= len(body) {
		body = body[d.End:]
	}
	props := []systemProperty{
		{key: systemPropertyFileName, value: filepath.Base(a.path)},
		{key: systemPropertyPath, value: a.path},
		{key: systemPropertyWordCount, value: locales.PropertyWordCount(chars, len(strings.Fields(body)))},
	}
	if st, err := os.Stat(a.path); err == nil {
		if created := fileCreationTime(st); !created.IsZero() {
			props = append(props, systemProperty{key: systemPropertyCreatedAt, value: locales.PropertyDateTime(created.Local())})
		}
		props = append(props,
			systemProperty{key: systemPropertyFileSize, value: formatPropertySize(st.Size())},
			systemProperty{key: systemPropertyUpdatedAt, value: locales.PropertyDateTime(st.ModTime().Local())},
		)
	}
	a.propertySystem = props
}

func formatPropertySize(size int64) string {
	if size < 1024 {
		return fmt.Sprintf("%d B", size)
	}
	return fmt.Sprintf("%.1f KB", float64(size)/1024)
}

func (a *App) propertiesSidebar(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).Key("properties-sidebar").Width(propertiesSidebarWidth).Shrink(0).FillHeight().
		BorderWidth(0, 0, 0, 1).BorderColor(t.Border).Background(t.Background).Children(func() {
		ui.Row(c).Height(70).Shrink(0).Padding(0, 20).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Text(c, locales.PropertyTitle()).FontSize(18).FontWeight(600).TextColor(propertyValueColor(t))
			ui.Spacer(c)
			if a.propertySourceMode {
				a.propertyButton(c, locales.PropertyBackToProperties(), func() { a.propertySourceMode = false; a.invalidate() })
			} else {
				b := ui.ButtonBase(c).Height(34).Padding(0, 14).Radius(17).Background(propertyCardColor(t)).Label(locales.PropertyEditType())
				b.OnClick(func() { a.startPropertyEdit(propertyTypeField(a.propertyDocument()), "sidebar") })
				b.Children(func() {
					ui.Text(c, locales.PropertyEditType()).FontSize(13).FontWeight(500).TextColor(propertyValueColor(t))
				})
			}
			menu := a.smallIconTint(c, iconMore, locales.PropertyMenu(), nil, propertyQuietColor(t))
			menu.Menu(func(m *ui.Menu) {
				if m.Item(locales.PropertyAdd()).Chosen() {
					a.propertyAddOpen = !a.propertyAddOpen
				}
				if m.Item(locales.PropertyMigrationPreviewMenu()).Chosen() {
					a.openCoreMigrationPreview()
				}
				if m.Item(locales.PropertyYAMLSource()).Chosen() {
					a.openPropertySource()
				}
				m.Separator()
				if m.Item(locales.PropertyCloseSidebar()).Chosen() {
					a.propertiesOpen = false
				}
			})
		})
		if a.propertySourceMode {
			a.propertySourceEditor(c)
			return
		}
		if a.propertyError != "" {
			ui.Text(c, a.propertyError).FillWidth().Padding(8, 20).TextColor(ui.Hex("#c34242")).FontSize(12)
		}
		showScroll := a.propertyIndicator.Visible(c.Now(), a.propertyScroll.X, a.propertyScroll.Y)
		if showScroll {
			c.After(a.propertyIndicator.Remaining(c.Now()))
		}
		ui.Scroll(c).Fill().TrackScroll(&a.propertyScroll).Scrollbars(showScroll).Children(func() {
			ui.Column(c).FillWidth().Padding(0, 18, 24, 18).Gap(10).Children(func() {
				d := a.propertyDocument()
				if d.Err != nil {
					ui.Text(c, locales.PropertyYAMLInvalid()).FontWeight(600)
					ui.Text(c, d.Err.Error()).FillWidth().TextColor(propertyQuietColor(t)).FontSize(12)
					a.propertyButton(c, locales.PropertyYAMLSource(), a.openPropertySource)
					return
				}
				// Core schema fields have stable slots in the properties view even
				// before a value has been entered. They are materialized in the
				// document by the schema lifecycle, but remain visible as empty
				// rows while an older note is being migrated.
				for _, f := range propertyCoreFields(d.Fields) {
					a.propertyFieldRow(c, f)
				}
				a.propertyBacklinkCard(c)
				fields := a.orderedProperties(d.Fields)
				var hidden []frontmatter.Field
				for _, f := range fields {
					if propertyOmitted(f.Key) || isCoreProperty(f.Key) {
						continue
					}
					if a.propertyPrefs.Fields[f.Key].Hidden || propertySecondary(f.Key) {
						hidden = append(hidden, f)
						continue
					}
					a.propertyFieldRow(c, f)
				}
				a.propertyHiddenSection(c, hidden)
				if a.propertyAddOpen {
					a.propertyAddForm(c)
				}
			})
		})
	})
}

func (a *App) propertySourceEditor(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).Fill().MinHeight(0).Padding(12).Gap(8).Children(func() {
		ui.Text(c, locales.PropertyYAMLDetail()).FillWidth().FontSize(11).TextColor(t.TextMuted)
		ui.TextArea(c.Key("properties-yaml-source"), &a.propertySourceDraft).Grow(1).MinHeight(120).FillWidth().Font("monospace").Label(locales.PropertyYAMLSource())
		ui.Row(c).FillWidth().Justify(ui.End).Children(func() {
			a.propertyButton(c, locales.PropertyApplySource(), func() {
				if err := a.applyPropertyYAMLSource(a.propertySourceDraft); err != nil {
					a.propertyError = err.Error()
					return
				}
				a.propertyError = ""
				a.refreshSystemProperties()
				a.invalidate()
			})
		})
	})
}

func (a *App) yamlPropertySource() string {
	d := a.propertyDocument()
	if d.End > 0 {
		return a.source[:d.End]
	}
	return "---\n---\n"
}

func (a *App) applyPropertyYAMLSource(draft string) error {
	end := a.propertyDocument().End
	if strings.HasPrefix(a.source, "---") && end == 0 {
		return fmt.Errorf("%s", locales.PropertyYAMLEndDelimiterMissing())
	}
	d := frontmatter.Parse(draft)
	if d.Err != nil || d.End == 0 {
		return fmt.Errorf("%s", locales.PropertyYAMLCompleteRequired())
	}
	if strings.TrimSpace(draft[d.End:]) != "" {
		return fmt.Errorf("%s", locales.PropertyYAMLOnly())
	}
	prefix := draft[:d.End]
	if !strings.HasSuffix(prefix, "\n") {
		prefix += "\n"
	}
	if end == 0 && !strings.HasSuffix(prefix, "\n\n") {
		prefix = strings.TrimRight(prefix, "\r\n") + "\n\n"
	}
	a.applyPropertySource(prefix + a.source[end:])
	return nil
}

func (a *App) propertyFieldRow(c *ui.Context, f frontmatter.Field) {
	t := c.Theme()
	editing := a.propertyEditKey == f.Key && a.propertyEditLocation == "sidebar"
	ui.Column(c).Key("property-card-"+f.Key).FillWidth().MinHeight(64).Gap(8).Padding(18, 22).Radius(20).Background(propertyCardColor(t)).Children(func() {
		if editing {
			ui.Text(c, propertyLabel(f.Key)).FontSize(14).TextColor(propertyQuietColor(t))
			a.propertyInlineEditor(c, f, "sidebar")
			return
		}
		stacked := f.Kind == "date" || isNaturalDateProperty(f.Key) || len([]rune(displayPropertyValue(f))) > 26 || f.Kind == "structure"
		content := func() {
			ui.Text(c, propertyLabel(f.Key)).FontSize(14).TextColor(propertyQuietColor(t)).Shrink(0)
			if f.Editable {
				value := ui.ButtonBase(c).Key("property-value-" + f.Key).Grow(1).MinWidth(0).MinHeight(28).Padding(0).Justify(ui.Start).Label(locales.PropertyEditLabel(f.Key))
				value.OnClick(func() { a.startPropertyEdit(f, "sidebar") })
				value.ContextMenu(func(m *ui.Menu) { a.propertyFieldMenu(m, f) })
				value.Children(func() { a.propertyFieldValue(c, f, t) })
			} else {
				a.propertyFieldValue(c, f, t)
			}
		}
		if stacked {
			ui.Column(c).FillWidth().Gap(8).Children(content)
		} else {
			ui.Row(c).FillWidth().Gap(12).AlignItems(ui.Center).Children(content)
		}
	})
}

// propertyCoreFields projects the required properties into a stable display
// order. Timestamps are app-managed and therefore never editable here.
func propertyCoreFields(fields []frontmatter.Field) []frontmatter.Field {
	keys := []struct {
		key, kind, fallback string
	}{
		{"type", "text", "note"},
		{"tags", "list", ""},
		{"aliases", "list", ""},
		{"created_at", "date", ""},
		{"updated_at", "date", ""},
	}
	out := make([]frontmatter.Field, 0, len(keys))
	for _, core := range keys {
		field := frontmatter.Field{Key: core.key, Kind: core.kind, Editable: core.key != "created_at" && core.key != "updated_at"}
		for _, existing := range fields {
			if strings.EqualFold(existing.Key, core.key) {
				field = existing
				break
			}
		}
		if strings.TrimSpace(field.Value) == "" && core.fallback != "" {
			field.Value = core.fallback
		}
		if core.key == "created_at" || core.key == "updated_at" {
			field.Editable = false
		}
		out = append(out, field)
	}
	return out
}

func isCoreProperty(key string) bool {
	switch strings.ToLower(key) {
	case "type", "tags", "aliases", "created_at", "updated_at":
		return true
	default:
		return false
	}
}

func (a *App) propertyFieldValue(c *ui.Context, f frontmatter.Field, t *ui.Theme) {
	val := displayPropertyValue(f)
	if val == "" {
		ui.Text(c, locales.PropertyNoValue()).FontSize(14).TextColor(propertyEmptyColor(t))
		return
	}
	if strings.EqualFold(f.Key, "tags") {
		a.propertyTags(c, val, 0)
		return
	}
	ui.Text(c, val).FillWidth().FontSize(15).TextColor(propertyValueColor(t)).Wrap()
}

func (a *App) propertyAddForm(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).FillWidth().Gap(6).Padding(8, 0).Children(func() {
		ui.Text(c, locales.PropertyAdd()).FontSize(12).FontWeight(600)
		ui.TextInput(c, &a.propertyAddKey).FillWidth().Placeholder(locales.PropertyName()).Label(locales.PropertyName())
		ui.Row(c).FillWidth().Gap(5).AlignItems(ui.Center).Children(func() {
			kind := a.propertyAddKind
			if kind == "" {
				kind = "text"
			}
			k := ui.ButtonBase(c).Height(28).Padding(0, 8).Radius(6).Background(t.Surface).Label(locales.PropertyTypeLabel(propertyTypeName(kind)))
			k.Menu(func(m *ui.Menu) {
				for _, typ := range []string{"text", "list", "bool", "number", "date"} {
					if m.Item(propertyTypeName(typ)).Chosen() {
						a.propertyAddKind = typ
					}
				}
			})
			k.Children(func() { ui.Text(c, locales.PropertyTypeLabel(propertyTypeName(kind))).FontSize(11) })
			ui.Spacer(c)
		})
		kind := a.propertyAddKind
		if kind == "" {
			kind = "text"
		}
		if kind == "bool" {
			ui.Row(c).Gap(6).Children(func() {
				a.propertyButton(c, locales.PropertyAddFalse(), func() { a.propertyAdd("false") })
				a.propertyButton(c, locales.PropertyAddTrue(), func() { a.propertyAdd("true") })
			})
		} else {
			placeholder := locales.PropertyValuePlaceholder()
			if kind == "list" {
				placeholder = locales.PropertyListValuePlaceholder()
			}
			if kind == "date" {
				placeholder = locales.PropertyDateValuePlaceholder()
			}
			ui.TextInput(c, &a.propertyAddValue).FillWidth().Placeholder(placeholder).Label(locales.PropertyValue()).OnSubmit(func() { a.propertyAdd(a.propertyAddValue) })
			a.propertyButton(c, locales.CommonAdd(), func() { a.propertyAdd(a.propertyAddValue) })
		}
	})
}

func propertyTypeName(kind string) string {
	switch kind {
	case "list":
		return locales.PropertyTypeList()
	case "bool":
		return locales.PropertyTypeBoolean()
	case "number":
		return locales.PropertyTypeNumber()
	case "date":
		return locales.PropertyTypeDate()
	case "structure":
		return locales.PropertyTypeStructure()
	default:
		return locales.PropertyTypeText()
	}
}

func (a *App) propertyAdd(value string) {
	key := strings.TrimSpace(a.propertyAddKey)
	kind := a.propertyAddKind
	if kind == "" {
		kind = "text"
	}
	if strings.EqualFold(key, "tags") || strings.EqualFold(key, "aliases") {
		kind = "list"
	}
	next, err := frontmatter.SetTyped(a.source, key, value, kind)
	if err != nil {
		a.propertyError = err.Error()
		a.invalidate()
		return
	}
	a.applyPropertySource(next)
	a.propertyAddKey, a.propertyAddValue, a.propertyError = "", "", ""
	a.refreshSystemProperties()
}

func (a *App) propertySet(key, value string) error {
	if key == "title" && strings.TrimSpace(value) == "" {
		return a.propertyDelete(key)
	}
	next, err := frontmatter.Set(a.source, key, value)
	if err != nil {
		return err
	}
	a.applyPropertySource(next)
	a.refreshSystemProperties()
	return nil
}

func (a *App) propertyDelete(key string) error {
	if isCoreProperty(key) {
		return fmt.Errorf("%s", locales.PropertyCoreCannotDelete(key))
	}
	next, err := frontmatter.Delete(a.source, key)
	if err != nil {
		return err
	}
	a.applyPropertySource(next)
	a.refreshSystemProperties()
	return nil
}

func (a *App) applyPropertySource(next string) {
	if next != a.source {
		a.ed.ReplaceAll(next)
	}
	a.propertyCache = propertyDocumentCache{}
	a.invalidate()
}

func (a *App) propertySaveField(key, value string) {
	if err := a.propertySet(key, value); err != nil {
		a.propertyError = err.Error()
		return
	}
	a.propertyEditKey, a.propertyEditLocation, a.propertyError = "", "", ""
	a.invalidate()
}

func (a *App) propertyButton(c *ui.Context, label string, fn func()) {
	b := ui.ButtonBase(c).Height(25).Padding(0, 7).Radius(5).Label(label).OnClick(fn)
	if b.Hovered() {
		b.Background(c.Theme().SurfaceHover)
	}
	b.Children(func() { ui.Text(c, label).FontSize(10).TextColor(c.Theme().TextMuted) })
}

func (a *App) orderedProperties(fields []frontmatter.Field) []frontmatter.Field {
	out := append([]frontmatter.Field(nil), fields...)
	rank := func(key string) int {
		switch strings.ToLower(key) {
		case "tags":
			return 0
		case "status":
			return 1
		case "date":
			return 2
		case "aliases":
			return 3
		default:
			return 10
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		pi, pj := a.propertyPrefs.Fields[out[i].Key], a.propertyPrefs.Fields[out[j].Key]
		if pi.Pinned != pj.Pinned {
			return pi.Pinned
		}
		if pi.Order > 0 || pj.Order > 0 {
			oi, oj := pi.Order, pj.Order
			if oi == 0 {
				oi = int(^uint(0) >> 1)
			}
			if oj == 0 {
				oj = int(^uint(0) >> 1)
			}
			if oi != oj {
				return oi < oj
			}
		}
		if rank(out[i].Key) != rank(out[j].Key) {
			return rank(out[i].Key) < rank(out[j].Key)
		}
		return false
	})
	return out
}

func (a *App) moveProperty(key string, delta int) {
	d := a.propertyDocument()
	fields := a.orderedProperties(d.Fields)
	index := -1
	for i, f := range fields {
		if f.Key == key {
			index = i
			break
		}
	}
	to := index + delta
	if index < 0 || to < 0 || to >= len(fields) {
		return
	}
	if a.propertyPrefs.Fields[fields[index].Key].Pinned != a.propertyPrefs.Fields[fields[to].Key].Pinned {
		return
	}
	fields[index], fields[to] = fields[to], fields[index]
	if a.propertyPrefs.Fields == nil {
		a.propertyPrefs.Fields = map[string]fieldDisplayPreference{}
	}
	for i, f := range fields {
		p := a.propertyPrefs.Fields[f.Key]
		if p.Pinned == a.propertyPrefs.Fields[key].Pinned {
			p.Order = i + 1
		}
		a.propertyPrefs.Fields[f.Key] = p
	}
	if err := saveDocumentPropertyPrefs(a.path, a.propertyPrefs); err != nil {
		a.propertyError = err.Error()
	}
	a.invalidate()
}

func (a *App) updatePropertyPreference(key string, change func(*fieldDisplayPreference)) {
	if a.propertyPrefs.Fields == nil {
		a.propertyPrefs.Fields = map[string]fieldDisplayPreference{}
	}
	p := a.propertyPrefs.Fields[key]
	change(&p)
	a.propertyPrefs.Fields[key] = p
	if err := saveDocumentPropertyPrefs(a.path, a.propertyPrefs); err != nil {
		a.propertyError = err.Error()
	}
	a.invalidate()
}

func documentPropertyPrefsPath() string {
	if dir := os.Getenv("MUSE_CONFIG_DIR"); dir != "" {
		return filepath.Join(dir, "document-properties.json")
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		dir = os.TempDir()
	}
	return filepath.Join(dir, "Muse", "document-properties.json")
}

func loadDocumentPropertyPrefs(path string) documentPropertyPrefs {
	var all propertyPrefsFile
	b, err := os.ReadFile(documentPropertyPrefsPath())
	if err == nil {
		_ = json.Unmarshal(b, &all)
	}
	if all == nil {
		return documentPropertyPrefs{}
	}
	return all[path]
}

func saveDocumentPropertyPrefs(path string, prefs documentPropertyPrefs) error {
	filename := documentPropertyPrefsPath()
	var all propertyPrefsFile
	if b, err := os.ReadFile(filename); err == nil {
		_ = json.Unmarshal(b, &all)
	}
	if all == nil {
		all = propertyPrefsFile{}
	}
	all[path] = prefs
	if err := os.MkdirAll(filepath.Dir(filename), 0o700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(all, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filename, b, 0o600)
}

// Stable tag colors keep the same label recognizable across notes and themes.
func propertyTagColors(tag string, dark bool) (ui.Color, ui.Color) {
	light := [][2]string{{"#F0E7B1", "#665713"}, {"#F7DCE8", "#862D54"}, {"#D8EEE7", "#285F52"}, {"#DEE8F6", "#315A89"}, {"#EBDFF2", "#68447C"}}
	night := [][2]string{{"#665D14", "#EEE19B"}, {"#540A2D", "#F0ABC8"}, {"#194C49", "#A4D9CF"}, {"#213C62", "#B5CDF1"}, {"#463255", "#DCC0EE"}}
	h := fnv.New32a()
	_, _ = h.Write([]byte(tag))
	p := light[h.Sum32()%uint32(len(light))]
	if dark {
		p = night[h.Sum32()%uint32(len(night))]
	}
	return ui.Hex(p[0]), ui.Hex(p[1])
}
