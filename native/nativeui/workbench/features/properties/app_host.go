// Package nativeui is Muse's fully native shell: MyGo draws the window, the
// icon rail, the context panel and the Markdown editor, with no webview and
// no frontend build.
//
// The layout is a 56px icon rail, one shared context panel (files / outline
// / search / AI) and the editor card, which shows the document as Typora
// does: one surface the user writes on, rendered as it reads.
package nativeui

func (p appPropertyPreferences) Load(path string) (map[string]propertypanel.FieldPreference, error) {
	if p.app == nil {
		return nil, errors.New("properties: application is unavailable")
	}
	stored, err := applicationconfig.NewService(p.app.configRepository).LoadDocumentPropertyPreferences(path)
	if err != nil {
		return nil, err
	}
	return fieldPreferencesFromConfig(stored), nil
}

func (p appPropertyPreferences) Save(path string, preferences map[string]propertypanel.FieldPreference) error {
	if p.app == nil {
		return errors.New("properties: application is unavailable")
	}
	return applicationconfig.NewService(p.app.configRepository).SaveDocumentPropertyPreferences(path, fieldPreferencesToConfig(preferences))
}

func (m appPropertyMetadata) SetProperty(key, value string) error {
	if m.app == nil {
		return propertypanel.ErrDocumentUnavailable
	}
	return documentsave.NewMetadataEditor(m.app.noteEditor()).SetProperty(key, value)
}

func (m appPropertyMetadata) AddProperty(key, value string, kind propertypanel.FieldKind) error {
	if m.app == nil {
		return propertypanel.ErrDocumentUnavailable
	}
	return documentsave.NewMetadataEditor(m.app.noteEditor()).AddProperty(key, value, appPropertyValueKind(kind))
}

func (m appPropertyMetadata) DeleteProperty(key string) error {
	if m.app == nil {
		return propertypanel.ErrDocumentUnavailable
	}
	return documentsave.NewMetadataEditor(m.app.noteEditor()).DeleteProperty(key)
}

func (m appPropertyMetadata) ReplaceYAML(source string) error {
	if m.app == nil {
		return propertypanel.ErrDocumentUnavailable
	}
	return documentsave.NewMetadataEditor(m.app.noteEditor()).ReplaceYAML(source)
}

func appPropertyValueKind(kind propertypanel.FieldKind) documentsave.PropertyValueKind {
	switch kind {
	case propertypanel.List:
		return documentsave.PropertyList
	case propertypanel.Boolean:
		return documentsave.PropertyBoolean
	case propertypanel.Number:
		return documentsave.PropertyNumber
	case propertypanel.Date:
		return documentsave.PropertyDate
	case propertypanel.Structure:
		return documentsave.PropertyStructure
	default:
		return documentsave.PropertyText
	}
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

func (a *App) propertySystemProperties(doc propertypanel.DocumentSnapshot) []propertypanel.SystemProperty {
	body := doc.Source
	if parsed := frontmatter.Parse(doc.Source); parsed.Err == nil && parsed.End > 0 && parsed.End <= len(body) {
		body = body[parsed.End:]
	}
	snapshot := documentsave.Inspect(doc.Source, body, doc.Path, a.metadataReader)
	properties := []propertypanel.SystemProperty{
		{Key: propertypanel.SystemPropertyFileName, Value: filepath.Base(doc.Path)},
		{Key: propertypanel.SystemPropertyPath, Value: doc.Path},
		{Key: propertypanel.SystemPropertyWordCount, Value: locales.PropertyWordCount(snapshot.Text.Characters, snapshot.BodyWords)},
	}
	if snapshot.HasMetadata {
		metadata := snapshot.Metadata
		if !metadata.CreatedAt.IsZero() {
			properties = append(properties, propertypanel.SystemProperty{Key: propertypanel.SystemPropertyCreatedAt, Value: locales.PropertyDateTime(metadata.CreatedAt.Local())})
		}
		properties = append(properties,
			propertypanel.SystemProperty{Key: propertypanel.SystemPropertyFileSize, Value: formatPropertySize(metadata.Size)},
			propertypanel.SystemProperty{Key: propertypanel.SystemPropertyUpdatedAt, Value: locales.PropertyDateTime(metadata.ModifiedAt.Local())},
		)
	}
	return properties
}

func formatPropertySize(size int64) string {
	if size < 1024 {
		return fmt.Sprintf("%d B", size)
	}
	return fmt.Sprintf("%.1f KB", float64(size)/1024)
}

func (a *App) documentBacklinkSources() []vaultlinks.BacklinkSource {
	if a == nil {
		return nil
	}
	vault := a.vault
	path := a.documentPath()
	if a.imageDocumentOpen() && a.imageDocumentPath() != "" {
		path = a.imageDocumentPath()
	}
	if vault == nil || path == "" || !a.vaultSnapshot.Ready {
		return nil
	}
	return vault.BacklinkSources(propertyVaultPath(a.vaultSnapshot.Root, path))
}

func propertyVaultPath(root, path string) string {
	relative, err := filepath.Rel(root, path)
	if err != nil {
		return filepath.ToSlash(path)
	}
	return filepath.ToSlash(relative)
}

type appPropertyPreferences struct{ app *App }

type appPropertyMetadata struct{ app *App }
