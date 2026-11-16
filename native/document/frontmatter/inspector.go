package frontmatter

import (
	"sort"
	"strings"
)

// FieldPreference contains backend-neutral per-field presentation choices
// consumed by the document inspector policy. Persistence and UI schemas live
// outside frontmatter.
type FieldPreference struct {
	Pinned bool
	Hidden bool
	Order  int
}

// InspectorProjection applies document-property policy without producing a UI
// view model. The caller remains responsible for labels, localized display
// fields, controls, and converting these groups into presentation types.
type InspectorProjection struct {
	Core    []Field
	Ordered []Field
	Visible []Field
	Hidden  []Field
	Summary []Field
}

// ProjectInspector selects core metadata, applies the default/user ordering,
// groups secondary or hidden fields, and derives compact-summary fields.
// Preferences are keyed by authored field name to preserve existing settings.
func ProjectInspector(fields []Field, preferences map[string]FieldPreference) InspectorProjection {
	projection := InspectorProjection{
		Core:    CoreFields(fields),
		Ordered: append([]Field(nil), fields...),
	}
	sort.SliceStable(projection.Ordered, func(i, j int) bool {
		left, right := projection.Ordered[i], projection.Ordered[j]
		leftPreference, rightPreference := preferences[left.Key], preferences[right.Key]
		if leftPreference.Pinned != rightPreference.Pinned {
			return leftPreference.Pinned
		}
		leftHasOrder, rightHasOrder := leftPreference.Order > 0, rightPreference.Order > 0
		if leftHasOrder != rightHasOrder {
			return leftHasOrder
		}
		if leftHasOrder && leftPreference.Order != rightPreference.Order {
			return leftPreference.Order < rightPreference.Order
		}
		if !leftHasOrder {
			leftRank, rightRank := propertyDefaultRank(left.Key), propertyDefaultRank(right.Key)
			if leftRank != rightRank {
				return leftRank < rightRank
			}
		}
		return false
	})

	for _, field := range projection.Core {
		if propertyIsHidden(field.Key, preferences) {
			projection.Hidden = append(projection.Hidden, field)
		} else {
			projection.Visible = append(projection.Visible, field)
		}
	}
	for _, field := range projection.Ordered {
		if OmitFromPropertySummary(field.Key) || IsCoreKey(field.Key) {
			continue
		}
		if propertyIsHidden(field.Key, preferences) {
			projection.Hidden = append(projection.Hidden, field)
		} else {
			projection.Visible = append(projection.Visible, field)
		}
	}
	for _, field := range projection.Ordered {
		if OmitFromPropertySummary(field.Key) || strings.EqualFold(field.Key, "type") || preferences[field.Key].Hidden {
			continue
		}
		// updated_at is collapsed in the full inspector, but remains useful in
		// the compact document summary. last_opened stays secondary everywhere.
		if IsSecondaryProperty(field.Key) && CanonicalKey(field.Key) != "updated_at" {
			continue
		}
		if field.Kind != "structure" && strings.TrimSpace(field.Value) == "" {
			continue
		}
		projection.Summary = append(projection.Summary, field)
	}
	return projection
}

func propertyIsHidden(key string, preferences map[string]FieldPreference) bool {
	return preferences[key].Hidden || IsSecondaryProperty(key)
}

func propertyDefaultRank(key string) int {
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
