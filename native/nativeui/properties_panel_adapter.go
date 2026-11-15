package nativeui

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/egoist/mygo/ui"
	"muse/config"
	"muse/document/frontmatter"
	"muse/locales"
	propertypanel "muse/nativeui/panels/properties"
	vaultlinks "muse/vault/links"
)

// propertiesSidebar preserves the app composition root's existing panel hook.
// All widget rendering lives in the properties package; this edge supplies a
// snapshot and translates returned actions to current app/use-case APIs.
func (a *App) propertiesSidebar(c *ui.Context) {
	if !a.propertiesOpen {
		return
	}
	t := c.Theme()
	ui.Column(c).Key("properties-sidebar").Width(propertiesSidebarWidth).Shrink(0).FillHeight().
		BorderWidth(0, 0, 0, 1).BorderColor(t.Border).Background(t.Background).Children(func() {
		propertypanel.Render(c, a.propertiesViewModel(), a.dispatchPropertyAction)
	})
}

func (a *App) propertiesViewModel() propertypanel.ViewModel {
	d := a.propertyDocument()
	model := propertypanel.ViewModel{
		Title:       locales.PropertyTitle(),
		Error:       a.propertyError,
		EditingKey:  a.propertyEditKey,
		Draft:       a.propertyEditDraft,
		AddOpen:     a.propertyAddOpen,
		AddDraft:    propertypanel.AddDraft{Key: a.propertyAddKey, Value: a.propertyAddValue, Kind: panelFieldKind(a.propertyAddKind)},
		HiddenOpen:  a.propertyHiddenOpen,
		SourceMode:  a.propertySourceMode,
		SourceDraft: a.propertySourceDraft,
		Scroll: propertypanel.ScrollPort{
			State:     &a.propertyScroll,
			Indicator: &a.propertyIndicator,
		},
	}
	if d.Err != nil {
		model.Error = d.Err.Error()
		return model
	}
	model.DocumentType = propertyDocumentType(d)
	for _, field := range propertyCoreFields(d.Fields) {
		model.Fields = append(model.Fields, a.panelField(field, true))
	}
	ordered := a.orderedProperties(d.Fields)
	summary := summaryPropertyFields(ordered, a.propertyPrefs)
	for i, field := range summary {
		if i >= 5 {
			break
		}
		model.Summary = append(model.Summary, a.panelField(field, false))
	}
	if len(summary) > 4 {
		model.MoreCount = len(summary) - 4
	}
	for i, field := range ordered {
		if propertyOmitted(field.Key) || isCoreProperty(field.Key) {
			continue
		}
		projected := a.panelField(field, false)
		projected.CanMoveUp = i > 0 && a.propertyPrefs.Fields[ordered[i-1].Key].Pinned == a.propertyPrefs.Fields[field.Key].Pinned
		projected.CanMoveDown = i+1 < len(ordered) && a.propertyPrefs.Fields[ordered[i+1].Key].Pinned == a.propertyPrefs.Fields[field.Key].Pinned
		if a.propertyPrefs.Fields[field.Key].Hidden || propertySecondary(field.Key) {
			model.HiddenFields = append(model.HiddenFields, projected)
		} else {
			model.Fields = append(model.Fields, projected)
		}
	}
	for _, system := range a.propertySystem {
		if system.key == systemPropertyCreatedAt || system.key == systemPropertyUpdatedAt {
			continue
		}
		model.SystemFields = append(model.SystemFields, propertypanel.Field{
			Key: "system-" + system.key, Label: systemPropertyLabel(system.key),
			Value: system.value, DisplayValue: system.value, Kind: propertypanel.Text, System: true,
		})
	}
	if !a.propertyOpenedAt.IsZero() && !propertyHasField(d, "last_opened") {
		value := locales.PropertyDateTime(a.propertyOpenedAt.Local())
		model.SystemFields = append(model.SystemFields, propertypanel.Field{
			Key: "system-last-opened", Label: locales.PropertyLastOpenedDate(), Value: value,
			DisplayValue: value, Kind: propertypanel.Date, System: true,
		})
	}
	for _, source := range a.documentBacklinkSources() {
		model.Backlinks = append(model.Backlinks, propertypanel.Backlink{
			Path: source.Source, Label: propertyBacklinkLabel(source.Source), Offset: source.Start, Count: source.Count,
		})
	}
	model.BacklinkCount = len(model.Backlinks)
	return model
}

func (a *App) panelField(field frontmatter.Field, core bool) propertypanel.Field {
	pref := a.propertyPrefs.Fields[field.Key]
	projected := propertypanel.Field{
		Key: field.Key, Label: propertyLabel(field.Key), Value: field.Value,
		DisplayValue: displayPropertyValue(field), Kind: panelFieldKind(field.Kind),
		Editable: field.Editable, Removable: field.Editable && !isCoreProperty(field.Key),
		Pinned: pref.Pinned, Hidden: pref.Hidden, CanPin: field.Editable, CanHide: field.Editable, CanReorder: field.Editable && !core,
	}
	if field.Kind == "list" {
		for _, tag := range strings.Split(field.Value, ",") {
			if tag = strings.TrimSpace(tag); tag != "" {
				projected.Tags = append(projected.Tags, tag)
			}
		}
	}
	return projected
}

func panelFieldKind(kind string) propertypanel.FieldKind {
	switch kind {
	case "list":
		return propertypanel.List
	case "bool":
		return propertypanel.Boolean
	case "number":
		return propertypanel.Number
	case "date":
		return propertypanel.Date
	case "structure":
		return propertypanel.Structure
	default:
		return propertypanel.Text
	}
}

func frontmatterKind(kind propertypanel.FieldKind) string {
	switch kind {
	case propertypanel.List:
		return "list"
	case propertypanel.Boolean:
		return "bool"
	case propertypanel.Number:
		return "number"
	case propertypanel.Date:
		return "date"
	case propertypanel.Structure:
		return "structure"
	default:
		return "text"
	}
}

func (a *App) propertyField(key string) (frontmatter.Field, bool) {
	d := a.propertyDocument()
	for _, field := range d.Fields {
		if field.Key == key {
			return field, true
		}
	}
	for _, field := range propertyCoreFields(d.Fields) {
		if field.Key == key {
			return field, true
		}
	}
	return frontmatter.Field{}, false
}

func (a *App) dispatchPropertyAction(action propertypanel.Action) {
	switch action.Kind {
	case propertypanel.BeginEdit:
		if field, ok := a.propertyField(action.FieldKey); ok && field.Editable {
			a.propertyEditKey, a.propertyEditLocation, a.propertyEditDraft = field.Key, "sidebar", field.Value
			a.invalidate()
		}
	case propertypanel.DraftChanged:
		a.propertyEditDraft = action.Value
	case propertypanel.SaveEdit:
		a.propertySaveField(action.FieldKey, action.Value)
	case propertypanel.CancelEdit:
		a.propertyEditKey, a.propertyEditLocation, a.propertyEditDraft = "", "", ""
		a.invalidate()
	case propertypanel.OpenAddForm:
		a.propertyAddOpen = true
		a.invalidate()
	case propertypanel.CloseAddForm:
		a.propertyAddOpen = false
		a.invalidate()
	case propertypanel.AddDraftChanged:
		a.propertyAddKey, a.propertyAddValue = action.AddDraft.Key, action.AddDraft.Value
		a.propertyAddKind = frontmatterKind(action.AddDraft.Kind)
	case propertypanel.AddField:
		a.propertyAddKey, a.propertyAddValue = action.AddDraft.Key, action.AddDraft.Value
		a.propertyAddKind = frontmatterKind(action.AddDraft.Kind)
		a.propertyAdd(action.AddDraft.Value)
	case propertypanel.DeleteField:
		if err := a.propertyDelete(action.FieldKey); err != nil {
			a.propertyError = err.Error()
			a.invalidate()
		}
	case propertypanel.TogglePinned:
		a.updatePropertyPreference(action.FieldKey, func(pref *fieldDisplayPreference) { pref.Pinned = !pref.Pinned })
	case propertypanel.ToggleHidden:
		a.updatePropertyPreference(action.FieldKey, func(pref *fieldDisplayPreference) { pref.Hidden = !pref.Hidden })
	case propertypanel.MoveFieldUp:
		a.moveProperty(action.FieldKey, -1)
	case propertypanel.MoveFieldDown:
		a.moveProperty(action.FieldKey, 1)
	case propertypanel.ToggleHiddenSection:
		a.propertyHiddenOpen = !a.propertyHiddenOpen
		a.invalidate()
	case propertypanel.OpenBacklink:
		a.openPropertyBacklink(action.Backlink.Path, action.Backlink.Offset)
	case propertypanel.OpenBacklinks:
		a.openLinks()
	case propertypanel.OpenPanel:
		a.openProperties()
	case propertypanel.OpenMigrationPreview:
		a.openCoreMigrationPreview()
	case propertypanel.OpenSource:
		a.openPropertySource()
	case propertypanel.ReturnToProperties:
		a.propertySourceMode = false
		a.invalidate()
	case propertypanel.SourceDraftChanged:
		a.propertySourceDraft = action.Value
	case propertypanel.ApplySource:
		if err := a.applyPropertyYAMLSource(action.Value); err != nil {
			a.propertyError = err.Error()
		} else {
			a.propertyError = ""
			a.refreshSystemProperties()
		}
		a.invalidate()
	case propertypanel.ClosePanel:
		a.propertiesOpen = false
		a.invalidate()
	}
}

// Property model projection and source mutation helpers retained at the App edge.
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

type fieldDisplayPreference = config.FieldDisplayPreference
type documentPropertyPrefs = config.DocumentPropertyPreferences

func (a *App) propertiesEnabled() bool { return !a.imageDocument.open && codeLanguage(a.path) == "" }

// propertyDocument caches parsing for one source/path pair. frontmatter owns
// YAML interpretation; this cache is only an App presentation optimization.
func (a *App) propertyDocument() frontmatter.Document {
	if a.propertyCache.path != a.path {
		preferences, err := config.LoadDocumentPropertyPreferences(a.path)
		if err != nil {
			a.propertyError = err.Error()
		} else {
			a.propertyPrefs = preferences
		}
	}
	if a.propertyCache.source != a.source || a.propertyCache.path != a.path {
		a.propertyCache = propertyDocumentCache{source: a.source, path: a.path, doc: frontmatter.Parse(a.source)}
	}
	return a.propertyCache.doc
}

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
	propertypanel.RenderSummary(c, a.propertiesViewModel(), a.dispatchPropertyAction)
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

func displayPropertyValue(f frontmatter.Field) string {
	return frontmatter.DisplayValue(f)
}

func isNaturalDateProperty(key string) bool {
	return frontmatter.IsDateKey(key)
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

func (a *App) yamlPropertySource() string {
	return frontmatter.YAMLSource(a.source)
}

func (a *App) applyPropertyYAMLSource(draft string) error {
	next, err := frontmatter.ReplaceYAMLSource(a.source, draft)
	if err != nil {
		return err
	}
	a.applyPropertySource(next)
	return nil
}

func propertyCoreFields(fields []frontmatter.Field) []frontmatter.Field {
	return frontmatter.CoreFields(fields)
}

func isCoreProperty(key string) bool {
	return frontmatter.IsCoreKey(key)
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
	next, err := frontmatter.SetProperty(a.source, key, value, kind)
	if err != nil {
		a.propertyError = err.Error()
		a.invalidate()
		return
	}
	a.applyPropertySource(next)
	a.propertyAddKey, a.propertyAddValue, a.propertyError = "", "", ""
	a.propertyAddOpen = false
	a.refreshSystemProperties()
}

func (a *App) propertySet(key, value string) error {
	next, err := frontmatter.SetProperty(a.source, key, value, "")
	if err != nil {
		return err
	}
	a.applyPropertySource(next)
	a.refreshSystemProperties()
	return nil
}

func (a *App) propertyDelete(key string) error {
	next, err := frontmatter.DeleteProperty(a.source, key)
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
	a.propertyEditKey, a.propertyEditLocation, a.propertyEditDraft, a.propertyError = "", "", "", ""
	a.invalidate()
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

func saveDocumentPropertyPrefs(path string, prefs documentPropertyPrefs) error {
	return config.SaveDocumentPropertyPreferences(path, prefs)
}

// Metadata labels and reverse-link projection for the App adapter.
// propertyQuietColor is shared with the image document's compact backlink row.
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

func propertyOmitted(key string) bool {
	switch strings.ToLower(key) {
	case "title", "created by", "created_by", "createdby", "last modified by", "modified_by", "links":
		return true
	default:
		return false
	}
}

func propertySecondary(key string) bool {
	switch strings.ToLower(key) {
	case "updated", "modified", "modified_at", "last_modified", "last modified date", "last_opened", "last opened date":
		return true
	default:
		return false
	}
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
	default:
		return key
	}
}

func propertyCanonicalKey(key string) string {
	return frontmatter.CanonicalKey(key)
}

func propertyHasField(d frontmatter.Document, key string) bool {
	return frontmatter.HasField(d, key)
}

func propertyTypeField(d frontmatter.Document) frontmatter.Field {
	return frontmatter.CoreFields(d.Fields)[0]
}

func propertyDocumentType(d frontmatter.Document) string {
	return frontmatter.DocumentType(d)
}

// documentBacklinkSources delegates reverse-link projection to the vault index.
func (a *App) documentBacklinkSources() []vaultlinks.BacklinkSource {
	v := a.vault
	path := a.path
	if a.imageDocument.open && a.imageDocument.path != "" {
		path = a.imageDocument.path
	}
	if v == nil || v.index == nil || path == "" || v.links == nil {
		return nil
	}
	return v.links.BacklinkSources(relativeVaultPath(v.root, path))
}

func (a *App) openPropertySource() {
	a.propertySourceMode = true
	a.propertySourceDraft = a.yamlPropertySource()
	a.invalidate()
}

func (a *App) openPropertyBacklink(source string, offset int) {
	a.navigateIndexed(source, offset)
}

func propertyBacklinkLabel(source string) string {
	return strings.TrimSuffix(filepath.Base(source), filepath.Ext(source))
}
