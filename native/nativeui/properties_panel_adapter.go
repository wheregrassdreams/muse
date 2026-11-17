package nativeui

import (
	"fmt"
	"path/filepath"

	"github.com/egoist/mygo/ui"
	applicationconfig "muse/application/config"
	applicationdocument "muse/application/document"
	"muse/config"
	"muse/document/frontmatter"
	"muse/locales"
	doctypes "muse/nativeui/components/doctypes"
	propertypanel "muse/nativeui/panels/properties"
	vaultlinks "muse/vault/links"
)

// propertiesSidebar preserves the app composition root's existing panel hook.
// All widget rendering lives in the properties package; this edge supplies a
// snapshot and translates returned actions to current app/use-case APIs.
func (a *App) propertiesSidebar(c *ui.Context) {
	if !a.propertiesPanel.IsOpen() {
		return
	}
	t := c.Theme()
	ui.Column(c).Key("properties-sidebar").Width(propertiesSidebarWidth).Shrink(0).FillHeight().
		BorderWidth(0, 0, 0, 1).BorderColor(t.Border).Background(t.Background).Children(func() {
		propertypanel.Render(c, &a.propertiesPanel, a.propertiesViewModel(), a.dispatchPropertyAction)
	})
}

func (a *App) propertiesViewModel() propertypanel.ViewModel {
	d := a.propertyDocument()
	input := propertypanel.ProjectionInput{Title: locales.PropertyTitle()}
	if d.Err != nil {
		input.Error = d.Err.Error()
		return propertypanel.ProjectViewModel(input)
	}
	input.DocumentType = propertypanel.DocumentType(d)
	input.Projection = propertypanel.ProjectFields(d.Fields, a.propertiesPanel.Preferences())
	input.Preferences = a.propertiesPanel.Preferences()
	input.System = a.propertiesPanel.SystemProperties()
	openedAt := a.propertiesPanel.OpenedAt()
	if !openedAt.IsZero() && !propertyHasField(d, "last_opened") {
		input.LastOpened = locales.PropertyDateTime(openedAt.Local())
	}
	for _, source := range a.documentBacklinkSources() {
		input.Backlinks = append(input.Backlinks, propertypanel.BacklinkSource{
			Path: source.Source, Offset: source.Start, Count: source.Count,
		})
	}
	return propertypanel.ProjectViewModel(input)
}

func propertyValueKind(kind propertypanel.FieldKind) applicationdocument.PropertyValueKind {
	switch kind {
	case propertypanel.List:
		return applicationdocument.PropertyList
	case propertypanel.Boolean:
		return applicationdocument.PropertyBoolean
	case propertypanel.Number:
		return applicationdocument.PropertyNumber
	case propertypanel.Date:
		return applicationdocument.PropertyDate
	case propertypanel.Structure:
		return applicationdocument.PropertyStructure
	default:
		return applicationdocument.PropertyText
	}
}

func (a *App) propertyField(key string) (frontmatter.Field, bool) {
	d := a.propertyDocument()
	for _, field := range d.Fields {
		if field.Key == key {
			return field, true
		}
	}
	for _, field := range propertypanel.ProjectFields(d.Fields, a.propertiesPanel.Preferences()).Core {
		if field.Key == key {
			return field, true
		}
	}
	return frontmatter.Field{}, false
}

func (a *App) dispatchPropertyAction(action propertypanel.Action) {
	a.propertiesPanel.Apply(action)
	switch action.Kind {
	case propertypanel.BeginEdit:
		if field, ok := a.propertyField(action.FieldKey); ok && field.Editable {
			a.propertiesPanel.BeginEdit(field.Key, "sidebar", field.Value)
			a.invalidate()
		}
	case propertypanel.SaveEdit:
		a.propertySaveField(action.FieldKey, action.Value)
	case propertypanel.AddField:
		a.propertyAdd(action.AddDraft)
	case propertypanel.DeleteField:
		if err := a.propertyDelete(action.FieldKey); err != nil {
			a.propertiesPanel.SetError(err.Error())
			a.invalidate()
		}
	case propertypanel.TogglePinned, propertypanel.ToggleHidden:
		if err := a.saveDocumentPropertyPrefs(a.documentPath()); err != nil {
			a.propertiesPanel.SetError(err.Error())
		}
		a.invalidate()
	case propertypanel.MoveFieldUp:
		a.moveProperty(action.FieldKey, -1)
	case propertypanel.MoveFieldDown:
		a.moveProperty(action.FieldKey, 1)
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
		a.invalidate()
	case propertypanel.ApplySource:
		if err := a.applyPropertyYAMLSource(action.Value); err != nil {
			a.propertiesPanel.SetError(err.Error())
		} else {
			a.propertiesPanel.ReturnToProperties()
			a.propertiesPanel.ClearError()
			a.refreshSystemProperties()
		}
		a.invalidate()
	case propertypanel.ClosePanel:
		a.invalidate()
	}
}

// Property model projection and source mutation helpers retained at the App edge.
const propertiesSidebarWidth float32 = 340

const (
	systemPropertyFileName  = "file-name"
	systemPropertyPath      = "path"
	systemPropertyWordCount = "word-count"
	systemPropertyCreatedAt = "created-at"
	systemPropertyFileSize  = "file-size"
	systemPropertyUpdatedAt = "updated-at"
)

func (a *App) propertiesEnabled() bool {
	return !a.imageDocumentOpen() && doctypes.IsMarkdownPath(a.documentPath())
}

// propertyDocument loads the active note's presentation preferences and asks
// the properties panel to own its parsed frontmatter projection cache.
func (a *App) propertyDocument() frontmatter.Document {
	path := a.documentPath()
	if a.propertiesPanel.PreferencesPath() != path {
		stored, err := applicationconfig.NewService(a.configRepository).LoadDocumentPropertyPreferences(path)
		if err != nil {
			a.propertiesPanel.SetError(err.Error())
		}
		a.propertiesPanel.SetPreferences(path, fieldPreferencesFromConfig(stored))
	}
	return a.propertiesPanel.Document(path, a.documentSource())
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
	propertypanel.RenderSummary(c, &a.propertiesPanel, a.propertiesViewModel(), a.dispatchPropertyAction)
}

func (a *App) openProperties() {
	a.propertiesPanel.Open()
	a.refreshSystemProperties()
	a.invalidate()
}

func (a *App) refreshSystemProperties() {
	source := a.documentSource()
	body := source
	if d := a.propertyDocument(); d.Err == nil && d.End > 0 && d.End <= len(body) {
		body = body[d.End:]
	}
	snapshot := applicationdocument.Inspect(source, body, a.documentPath(), a.metadataReader)
	props := []propertypanel.SystemProperty{
		{Key: systemPropertyFileName, Value: filepath.Base(a.documentPath())},
		{Key: systemPropertyPath, Value: a.documentPath()},
		{Key: systemPropertyWordCount, Value: locales.PropertyWordCount(snapshot.Text.Characters, snapshot.BodyWords)},
	}
	if snapshot.HasMetadata {
		metadata := snapshot.Metadata
		if created := metadata.CreatedAt; !created.IsZero() {
			props = append(props, propertypanel.SystemProperty{Key: systemPropertyCreatedAt, Value: locales.PropertyDateTime(created.Local())})
		}
		props = append(props,
			propertypanel.SystemProperty{Key: systemPropertyFileSize, Value: formatPropertySize(metadata.Size)},
			propertypanel.SystemProperty{Key: systemPropertyUpdatedAt, Value: locales.PropertyDateTime(metadata.ModifiedAt.Local())},
		)
	}
	a.propertiesPanel.SetSystemProperties(props)
}

func formatPropertySize(size int64) string {
	if size < 1024 {
		return fmt.Sprintf("%d B", size)
	}
	return fmt.Sprintf("%.1f KB", float64(size)/1024)
}

func (a *App) yamlPropertySource() string {
	return frontmatter.YAMLSource(a.documentSource())
}

func (a *App) applyPropertyYAMLSource(draft string) error {
	if err := applicationdocument.NewMetadataEditor(a.noteEditor()).ReplaceYAML(draft); err != nil {
		return err
	}
	a.propertySourceChanged()
	return nil
}

func (a *App) propertyAdd(draft propertypanel.AddDraft) {
	if err := applicationdocument.NewMetadataEditor(a.noteEditor()).AddProperty(draft.Key, draft.Value, propertyValueKind(draft.Kind)); err != nil {
		a.propertiesPanel.SetError(err.Error())
		a.invalidate()
		return
	}
	a.propertySourceChanged()
	a.propertiesPanel.ClearAddedField()
	a.refreshSystemProperties()
}

func (a *App) propertySet(key, value string) error {
	if err := applicationdocument.NewMetadataEditor(a.noteEditor()).SetProperty(key, value); err != nil {
		return err
	}
	a.propertySourceChanged()
	a.refreshSystemProperties()
	return nil
}

func (a *App) propertyDelete(key string) error {
	if err := applicationdocument.NewMetadataEditor(a.noteEditor()).DeleteProperty(key); err != nil {
		return err
	}
	a.propertySourceChanged()
	a.refreshSystemProperties()
	return nil
}

func (a *App) propertySourceChanged() {
	a.propertiesPanel.InvalidateDocument()
	a.invalidate()
}

func (a *App) propertySaveField(key, value string) {
	if err := a.propertySet(key, value); err != nil {
		a.propertiesPanel.SetError(err.Error())
		return
	}
	a.propertiesPanel.ClearEdit()
	a.invalidate()
}

func (a *App) moveProperty(key string, delta int) {
	d := a.propertyDocument()
	if d.Err != nil || !a.propertiesPanel.MoveField(key, delta, d.Fields) {
		return
	}
	if err := a.saveDocumentPropertyPrefs(a.documentPath()); err != nil {
		a.propertiesPanel.SetError(err.Error())
	}
	a.invalidate()
}

func (a *App) saveDocumentPropertyPrefs(path string) error {
	return applicationconfig.NewService(a.configRepository).SaveDocumentPropertyPreferences(path, fieldPreferencesToConfig(a.propertiesPanel.Preferences()))
}

func fieldPreferencesFromConfig(prefs config.DocumentPropertyPreferences) map[string]propertypanel.FieldPreference {
	if len(prefs.Fields) == 0 {
		return nil
	}
	fields := make(map[string]propertypanel.FieldPreference, len(prefs.Fields))
	for key, pref := range prefs.Fields {
		fields[key] = propertypanel.FieldPreference{Pinned: pref.Pinned, Hidden: pref.Hidden, Order: pref.Order}
	}
	return fields
}

func fieldPreferencesToConfig(fields map[string]propertypanel.FieldPreference) config.DocumentPropertyPreferences {
	if len(fields) == 0 {
		return config.DocumentPropertyPreferences{}
	}
	prefs := config.DocumentPropertyPreferences{Fields: make(map[string]config.FieldDisplayPreference, len(fields))}
	for key, pref := range fields {
		prefs.Fields[key] = config.FieldDisplayPreference{Pinned: pref.Pinned, Hidden: pref.Hidden, Order: pref.Order}
	}
	return prefs
}

func propertyHasField(d frontmatter.Document, key string) bool {
	return frontmatter.HasField(d, key)
}

// documentBacklinkSources delegates reverse-link projection to the vault index.
func (a *App) documentBacklinkSources() []vaultlinks.BacklinkSource {
	v := a.vault
	path := a.documentPath()
	if a.imageDocumentOpen() && a.imageDocumentPath() != "" {
		path = a.imageDocumentPath()
	}
	if v == nil || path == "" || !a.vaultSnapshot.Ready {
		return nil
	}
	return v.BacklinkSources(propertyVaultPath(a.vaultSnapshot.Root, path))
}

func propertyVaultPath(root, path string) string {
	relative, err := filepath.Rel(root, path)
	if err != nil {
		return filepath.ToSlash(path)
	}
	return filepath.ToSlash(relative)
}

func (a *App) openPropertySource() {
	a.propertiesPanel.OpenSource(a.yamlPropertySource())
	a.invalidate()
}

func (a *App) openPropertyBacklink(source string, offset int) {
	a.navigateIndexed(source, offset)
}
