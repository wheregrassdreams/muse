package frontmatter

import (
	"fmt"
	"strings"
	"time"

	notedate "muse/document/date"
	"muse/locales"
)

// CanonicalKey maps accepted user spellings to the semantic property key.
// The authored key itself is preserved when updating existing frontmatter.
func CanonicalKey(key string) string {
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

// IsCoreKey reports fields with schema-managed deletion and edit behavior.
func IsCoreKey(key string) bool {
	switch CanonicalKey(key) {
	case "type", "tags", "aliases", "created_at", "updated_at":
		return true
	default:
		return false
	}
}

func IsManagedKey(key string) bool {
	switch CanonicalKey(key) {
	case "created_at", "updated_at":
		return true
	default:
		return false
	}
}

// OmitFromPropertySummary lists metadata keys that remain available in YAML
// but are intentionally not rendered in the compact document header.
func OmitFromPropertySummary(key string) bool {
	switch strings.ToLower(strings.TrimSpace(key)) {
	case "title", "created by", "created_by", "createdby", "last modified by", "modified_by", "links":
		return true
	default:
		return false
	}
}

// IsSecondaryProperty groups infrequently used timestamps in the collapsed
// hidden section while retaining their authored frontmatter values.
func IsSecondaryProperty(key string) bool {
	switch CanonicalKey(key) {
	case "updated_at", "last_opened":
		return true
	default:
		return false
	}
}

// HasField tests semantic identity so legacy spellings behave consistently.
func HasField(d Document, key string) bool {
	canonical := CanonicalKey(key)
	for _, field := range d.Fields {
		if CanonicalKey(field.Key) == canonical {
			return true
		}
	}
	return false
}

// CoreFields projects the schema keys in stable order. Missing Type has the
// read-only presentation fallback "Note"; it is not written to source here.
// Creation and update timestamps are managed by document lifecycle operations.
func CoreFields(fields []Field) []Field {
	keys := []struct{ key, kind, fallback string }{
		{"type", "text", "note"}, {"tags", "list", ""}, {"aliases", "list", ""},
		{"created_at", "date", ""}, {"updated_at", "date", ""},
	}
	out := make([]Field, 0, len(keys))
	for _, core := range keys {
		field := Field{Key: core.key, Kind: core.kind, Editable: !IsManagedKey(core.key)}
		for _, existing := range fields {
			if CanonicalKey(existing.Key) == core.key {
				field = existing
				field.Key = existing.Key
				break
			}
		}
		if strings.TrimSpace(field.Value) == "" && core.fallback != "" {
			field.Value = core.fallback
		}
		if IsManagedKey(field.Key) {
			field.Editable = false
		}
		out = append(out, field)
	}
	return out
}

// DocumentType returns the display value for the typed document kind.
func DocumentType(d Document) string {
	if d.Err == nil {
		for _, field := range d.Fields {
			if CanonicalKey(field.Key) == "type" && strings.TrimSpace(field.Value) != "" {
				return DisplayValue(field)
			}
		}
	}
	return locales.PropertyDefaultNoteType()
}

// DisplayValue formats presentation values derived from a parsed field.
func DisplayValue(field Field) string {
	value := strings.TrimSpace(field.Value)
	if field.Kind == "structure" {
		return locales.PropertyStructureSourceOnly()
	}
	switch CanonicalKey(field.Key) {
	case "type":
		if strings.EqualFold(value, "note") {
			return locales.PropertyDefaultNoteType()
		}
	case "status":
		switch strings.ToLower(value) {
		case "draft":
			return locales.PropertyStatusDraft()
		case "done":
			return locales.PropertyStatusDone()
		case "in_progress":
			return locales.PropertyStatusInProgress()
		}
	}
	if field.Kind == "date" || IsDateKey(field.Key) {
		return FormatDateValue(value)
	}
	return value
}

func IsDateKey(key string) bool {
	switch CanonicalKey(key) {
	case "date", "created_at", "updated_at", "last_opened":
		return true
	default:
		return false
	}
}

func FormatDateValue(value string) string {
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

// SetProperty applies the editor's typed-property rules. Empty title removes
// the legacy override; tags and aliases default to list when added. Existing
// authored key spelling and type are retained. Managed timestamps cannot be
// edited through the properties inspector.
func SetProperty(source, key, value, kind string) (string, error) {
	key = strings.TrimSpace(key)
	if key == "" {
		return source, fmt.Errorf("%s", locales.FrontmatterKeyEmpty())
	}
	if strings.EqualFold(key, "title") && strings.TrimSpace(value) == "" {
		return DeleteProperty(source, key)
	}
	if IsManagedKey(key) {
		return source, fmt.Errorf("managed property %q is read-only", key)
	}
	d := Parse(source)
	if d.Err != nil {
		return source, d.Err
	}
	canonical := CanonicalKey(key)
	for _, field := range d.Fields {
		if CanonicalKey(field.Key) == canonical {
			return Set(source, field.Key, value)
		}
	}
	if kind == "" {
		kind = "text"
	}
	if canonical == "tags" || canonical == "aliases" {
		kind = "list"
	}
	return SetTyped(source, key, value, kind)
}

// DeleteProperty prevents removing the inspector's core keys. A legacy title
// override remains removable because it is not a core metadata field.
func DeleteProperty(source, key string) (string, error) {
	if IsCoreKey(key) {
		return source, fmt.Errorf("%s", locales.PropertyCoreCannotDelete(key))
	}
	return Delete(source, key)
}

// YAMLSource returns only the authored frontmatter block, or an empty block
// that can be edited when the document currently has no frontmatter.
func YAMLSource(source string) string {
	doc := Parse(source)
	if doc.End > 0 {
		return source[:doc.End]
	}
	return "---\n---\n"
}

// ReplaceYAMLSource validates a frontmatter-only draft and merges it back into
// the original body. Body text in the draft is always rejected.
func ReplaceYAMLSource(source, draft string) (string, error) {
	end := Parse(source).End
	if strings.HasPrefix(source, "---") && end == 0 {
		return source, fmt.Errorf("%s", locales.PropertyYAMLEndDelimiterMissing())
	}
	doc := Parse(draft)
	if doc.Err != nil || doc.End == 0 {
		return source, fmt.Errorf("%s", locales.PropertyYAMLCompleteRequired())
	}
	if strings.TrimSpace(draft[doc.End:]) != "" {
		return source, fmt.Errorf("%s", locales.PropertyYAMLOnly())
	}
	prefix := draft[:doc.End]
	if !strings.HasSuffix(prefix, "\n") {
		prefix += "\n"
	}
	if end == 0 && !strings.HasSuffix(prefix, "\n\n") {
		prefix = strings.TrimRight(prefix, "\r\n") + "\n\n"
	}
	return prefix + source[end:], nil
}
