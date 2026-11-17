package document

import (
	"errors"
	"strings"

	"muse/document/frontmatter"
)

var ErrMetadataSourceUnavailable = errors.New("application/document: metadata source is unavailable")

// MetadataSource is the application-owned port for applying a source mutation
// to an editable document. A UI surface or another editing host can implement
// this without exposing its editor implementation to the use case.
type MetadataSource interface {
	Source() string
	ReplaceUser(source string) error
}

// PropertyValueKind selects the authored YAML representation for a newly
// added property. Existing property types are preserved by SetProperty.
type PropertyValueKind uint8

const (
	PropertyText PropertyValueKind = iota
	PropertyList
	PropertyBoolean
	PropertyNumber
	PropertyDate
	PropertyStructure
)

// MetadataEditor applies frontmatter edits through a narrow source port.
// Frontmatter owns validation and YAML mutation rules; this use case owns the
// workflow that applies a successful mutation as one user edit.
type MetadataEditor struct {
	source MetadataSource
}

func NewMetadataEditor(source MetadataSource) *MetadataEditor {
	return &MetadataEditor{source: source}
}

func (e *MetadataEditor) AddProperty(key, value string, kind PropertyValueKind) error {
	key = strings.TrimSpace(key)
	kindName, err := propertyKindName(kind)
	if err != nil {
		return err
	}
	return e.apply(func(source string) (string, error) {
		return frontmatter.SetProperty(source, key, value, kindName)
	})
}

func (e *MetadataEditor) SetProperty(key, value string) error {
	return e.apply(func(source string) (string, error) {
		return frontmatter.SetProperty(source, key, value, "")
	})
}

func (e *MetadataEditor) DeleteProperty(key string) error {
	return e.apply(func(source string) (string, error) {
		return frontmatter.DeleteProperty(source, key)
	})
}

func (e *MetadataEditor) ReplaceYAML(draft string) error {
	return e.apply(func(source string) (string, error) {
		return frontmatter.ReplaceYAMLSource(source, draft)
	})
}

func (e *MetadataEditor) apply(mutate func(string) (string, error)) error {
	if e == nil || e.source == nil {
		return ErrMetadataSourceUnavailable
	}
	current := e.source.Source()
	next, err := mutate(current)
	if err != nil {
		return err
	}
	if next == current {
		return nil
	}
	return e.source.ReplaceUser(next)
}

func propertyKindName(kind PropertyValueKind) (string, error) {
	switch kind {
	case PropertyText:
		return "text", nil
	case PropertyList:
		return "list", nil
	case PropertyBoolean:
		return "bool", nil
	case PropertyNumber:
		return "number", nil
	case PropertyDate:
		return "date", nil
	case PropertyStructure:
		return "structure", nil
	default:
		return "", errors.New("application/document: unsupported property value kind")
	}
}
