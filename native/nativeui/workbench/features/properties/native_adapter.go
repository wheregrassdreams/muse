package properties

import (
	"fmt"
	"path/filepath"
	"time"

	applicationconfig "muse/application/config"
	documentapp "muse/application/document"
	"muse/config"
	"muse/document/frontmatter"
	"muse/document/types"
	"muse/locales"
)

// NativePorts supplies the desktop-specific facts and typed workflow route
// needed to compose the properties feature. The feature owns projection and
// preference adaptation; document mutations remain application use cases.
type NativePorts struct {
	Configuration applicationconfig.Repository
	CurrentPath   func() string
	Document      func() (path, source string)
	ImageDocument func() bool
	Metadata      documentapp.MetadataReader
	Backlinks     func(path string) []BacklinkSource
	Route         ActionPort
	Invalidate    func()
}

type NativeAdapter struct{ ports NativePorts }

func NewNativeHost(ports NativePorts) *Host {
	adapter := &NativeAdapter{ports: ports}
	return NewHost(Ports{
		CurrentPath: ports.CurrentPath,
		Project:     adapter.Project,
		Preferences: adapter,
		Route:       ports.Route,
		Invalidate:  ports.Invalidate,
	})
}

func (a *NativeAdapter) Project(preferences map[string]FieldPreference) Projection {
	if a == nil || a.ports.Document == nil {
		return Projection{}
	}
	path, source := a.ports.Document()
	markdown := path != "" && types.IsMarkdownPath(path) && (a.ports.ImageDocument == nil || !a.ports.ImageDocument())
	projection := Projection{Markdown: markdown, FrontmatterValid: true}
	if !markdown {
		projection.Model = ProjectViewModel(ProjectionInput{Title: locales.PropertyTitle()})
		return projection
	}

	snapshot := documentapp.InspectDocumentProperties(source, path, a.ports.Metadata)
	parsed := snapshot.Frontmatter
	projection.YAMLSource = snapshot.YAMLSource
	projection.FrontmatterValid = parsed.Err == nil
	input := ProjectionInput{Title: locales.PropertyTitle(), Preferences: preferences}
	if parsed.Err != nil {
		input.Error = parsed.Err.Error()
		projection.Model = ProjectViewModel(input)
		return projection
	}
	frontmatterPreferences := make(map[string]frontmatter.FieldPreference, len(preferences))
	for key, preference := range preferences {
		frontmatterPreferences[key] = frontmatter.FieldPreference{Pinned: preference.Pinned, Hidden: preference.Hidden, Order: preference.Order}
	}
	input.DocumentType = frontmatter.DocumentType(parsed)
	input.Projection = frontmatter.ProjectInspector(parsed.Fields, frontmatterPreferences)
	input.System = systemProperties(path, snapshot.Facts)
	if !frontmatter.HasField(parsed, "last_opened") {
		input.LastOpened = locales.PropertyDateTime(time.Now())
	}
	if a.ports.Backlinks != nil {
		input.Backlinks = a.ports.Backlinks(path)
	}
	projection.Model = ProjectViewModel(input)
	return projection
}

func (a *NativeAdapter) Load(path string) (map[string]FieldPreference, error) {
	if a == nil || a.ports.Configuration == nil {
		return nil, ErrUnavailable
	}
	stored, err := applicationconfig.NewService(a.ports.Configuration).LoadDocumentPropertyPreferences(path)
	if err != nil {
		return nil, err
	}
	fields := make(map[string]FieldPreference, len(stored.Fields))
	for key, preference := range stored.Fields {
		fields[key] = FieldPreference{Pinned: preference.Pinned, Hidden: preference.Hidden, Order: preference.Order}
	}
	return fields, nil
}

func (a *NativeAdapter) Save(path string, fields map[string]FieldPreference) error {
	if a == nil || a.ports.Configuration == nil {
		return ErrUnavailable
	}
	stored := config.DocumentPropertyPreferences{Fields: make(map[string]config.FieldDisplayPreference, len(fields))}
	for key, preference := range fields {
		stored.Fields[key] = config.FieldDisplayPreference{Pinned: preference.Pinned, Hidden: preference.Hidden, Order: preference.Order}
	}
	return applicationconfig.NewService(a.ports.Configuration).SaveDocumentPropertyPreferences(path, stored)
}

func systemProperties(path string, snapshot documentapp.InspectorSnapshot) []SystemProperty {
	properties := []SystemProperty{
		{Key: SystemPropertyFileName, Value: filepath.Base(path)},
		{Key: SystemPropertyPath, Value: path},
		{Key: SystemPropertyWordCount, Value: locales.PropertyWordCount(snapshot.Text.Characters, snapshot.BodyWords)},
	}
	if snapshot.HasMetadata {
		metadata := snapshot.Metadata
		if !metadata.CreatedAt.IsZero() {
			properties = append(properties, SystemProperty{Key: SystemPropertyCreatedAt, Value: locales.PropertyDateTime(metadata.CreatedAt.Local())})
		}
		properties = append(properties,
			SystemProperty{Key: SystemPropertyFileSize, Value: formatSize(metadata.Size)},
			SystemProperty{Key: SystemPropertyUpdatedAt, Value: locales.PropertyDateTime(metadata.ModifiedAt.Local())},
		)
	}
	return properties
}

func formatSize(size int64) string {
	if size < 1024 {
		return fmt.Sprintf("%d B", size)
	}
	return fmt.Sprintf("%.1f KB", float64(size)/1024)
}
