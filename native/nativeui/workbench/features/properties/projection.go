package properties

import (
	"path/filepath"
	"strings"

	"muse/document/frontmatter"
	"muse/locales"
)

// FieldPreference is the presentation-relevant subset of persisted property
// preferences. The host loads and saves the complete preference record.
type FieldPreference struct {
	Pinned bool
	Hidden bool
	Order  int
}

// SystemProperty is a host-collected system value. The panel supplies its
// localized label and turns it into a display field.
type SystemProperty struct {
	Key, Value string
}

const (
	SystemPropertyFileName  = "file-name"
	SystemPropertyPath      = "path"
	SystemPropertyWordCount = "word-count"
	SystemPropertyCreatedAt = "created-at"
	SystemPropertyFileSize  = "file-size"
	SystemPropertyUpdatedAt = "updated-at"
)

// BacklinkSource is a host-collected reverse-link result. Label formatting is
// presentation policy and is applied by ProjectViewModel.
type BacklinkSource struct {
	Path   string
	Offset int
	Count  int
}

// ProjectionInput contains the document projection and system snapshots
// needed to build a complete properties-panel view model. It performs no I/O
// and does not retain caller-owned mutable state.
type ProjectionInput struct {
	Title        string
	Error        string
	DocumentType string
	Projection   frontmatter.InspectorProjection
	Preferences  map[string]FieldPreference
	System       []SystemProperty
	LastOpened   string
	Backlinks    []BacklinkSource
}

// ProjectFields delegates field ordering and grouping to frontmatter while
// adapting the panel's presentation preferences to the document policy's
// backend-neutral preference value.
func ProjectFields(fields []frontmatter.Field, preferences map[string]FieldPreference) frontmatter.InspectorProjection {
	documentPreferences := make(map[string]frontmatter.FieldPreference, len(preferences))
	for key, preference := range preferences {
		documentPreferences[key] = frontmatter.FieldPreference{
			Pinned: preference.Pinned, Hidden: preference.Hidden, Order: preference.Order,
		}
	}
	return frontmatter.ProjectInspector(fields, documentPreferences)
}

// DocumentType returns the localized metadata type label for a parsed
// document. The root adapter supplies the parsed snapshot; the panel owns its
// display projection.
func DocumentType(document frontmatter.Document) string {
	return frontmatter.DocumentType(document)
}

// ProjectViewModel converts a domain projection into display-ready panel
// fields. Frontmatter parsing and grouping policy remain owned by
// document/frontmatter; this package owns labels, value formatting, field
// capabilities, summary sizing, and the final presentation shape.
func ProjectViewModel(input ProjectionInput) ViewModel {
	model := ViewModel{Title: input.Title, Error: input.Error, DocumentType: input.DocumentType}
	if input.Error != "" {
		return model
	}

	moveUp, moveDown := moveCapabilities(input.Projection.Ordered, input.Preferences)
	for _, field := range input.Projection.Visible {
		model.Fields = append(model.Fields, projectField(field, input.Preferences[field.Key]))
	}
	for _, field := range input.Projection.Hidden {
		model.HiddenFields = append(model.HiddenFields, projectField(field, input.Preferences[field.Key]))
	}
	for i := range model.Fields {
		field := &model.Fields[i]
		if field.CanReorder {
			field.CanMoveUp, field.CanMoveDown = moveUp[field.Key], moveDown[field.Key]
		}
	}
	for i := range model.HiddenFields {
		field := &model.HiddenFields[i]
		if field.CanReorder {
			field.CanMoveUp, field.CanMoveDown = moveUp[field.Key], moveDown[field.Key]
		}
	}
	for i, field := range input.Projection.Summary {
		if i >= 5 {
			break
		}
		model.Summary = append(model.Summary, projectField(field, input.Preferences[field.Key]))
	}
	if len(input.Projection.Summary) > 4 {
		model.MoreCount = len(input.Projection.Summary) - 4
	}
	for _, system := range input.System {
		if system.Key == SystemPropertyCreatedAt || system.Key == SystemPropertyUpdatedAt {
			continue
		}
		model.SystemFields = append(model.SystemFields, Field{
			Key: "system-" + system.Key, Label: systemPropertyLabel(system.Key),
			Value: system.Value, DisplayValue: system.Value, Kind: Text, System: true,
		})
	}
	if input.LastOpened != "" {
		model.HiddenFields = append(model.HiddenFields, Field{
			Key: "system-last-opened", Label: locales.PropertyLastOpenedDate(), Value: input.LastOpened,
			DisplayValue: input.LastOpened, Kind: Date, System: true,
		})
	}
	for _, source := range input.Backlinks {
		model.Backlinks = append(model.Backlinks, Backlink{
			Path: source.Path, Label: backlinkLabel(source.Path), Offset: source.Offset, Count: source.Count,
		})
	}
	model.BacklinkCount = len(model.Backlinks)
	return model
}

func projectField(field frontmatter.Field, preference FieldPreference) Field {
	core := frontmatter.IsCoreKey(field.Key)
	projected := Field{
		Key: field.Key, Label: propertyLabel(field.Key), Value: field.Value,
		DisplayValue: frontmatter.DisplayValue(field), Kind: fieldKind(field.Kind),
		Editable: field.Editable, Removable: field.Editable && !core,
		Pinned: preference.Pinned, Hidden: preference.Hidden,
		CanPin: field.Editable, CanHide: field.Editable,
		CanReorder: field.Editable && !core,
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

func fieldKind(kind string) FieldKind {
	switch kind {
	case "list":
		return List
	case "bool":
		return Boolean
	case "number":
		return Number
	case "date":
		return Date
	case "structure":
		return Structure
	default:
		return Text
	}
}

func moveCapabilities(fields []frontmatter.Field, preferences map[string]FieldPreference) (map[string]bool, map[string]bool) {
	moveUp := make(map[string]bool, len(fields))
	moveDown := make(map[string]bool, len(fields))
	for i, field := range fields {
		if i > 0 && preferences[fields[i-1].Key].Pinned == preferences[field.Key].Pinned {
			moveUp[field.Key] = true
		}
		if i+1 < len(fields) && preferences[fields[i+1].Key].Pinned == preferences[field.Key].Pinned {
			moveDown[field.Key] = true
		}
	}
	return moveUp, moveDown
}

func propertyLabel(key string) string {
	switch frontmatter.CanonicalKey(key) {
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

func systemPropertyLabel(key string) string {
	switch key {
	case SystemPropertyFileName:
		return locales.PropertyFileName()
	case SystemPropertyPath:
		return locales.PropertyPath()
	case SystemPropertyWordCount:
		return locales.PropertyWordCountLabel()
	case SystemPropertyCreatedAt:
		return locales.PropertyCreationDate()
	case SystemPropertyFileSize:
		return locales.PropertyFileSize()
	case SystemPropertyUpdatedAt:
		return locales.PropertyModifiedDate()
	default:
		return key
	}
}

func backlinkLabel(source string) string {
	return strings.TrimSuffix(filepath.Base(source), filepath.Ext(source))
}
