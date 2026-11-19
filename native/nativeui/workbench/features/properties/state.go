package properties

import (
	"github.com/egoist/mygo/ui"
	"muse/ui/scrollbar"
)

// state owns transient inspector presentation and editing state. The host
// supplies document projections and performs metadata operations.
type state struct {
	open            bool
	scroll          ui.ScrollState
	indicator       scrollbar.Indicator
	editingKey      string
	draft           string
	editLocation    string
	addOpen         bool
	addDraft        AddDraft
	hiddenOpen      bool
	sourceMode      bool
	sourceDraft     string
	errorMessage    string
	preferencesPath string
	preferences     map[string]FieldPreference
}

func (s *state) IsOpen() bool       { return s.open }
func (s *state) Open()              { s.open = true; s.sourceMode = false; s.errorMessage = "" }
func (s *state) Close()             { s.open = false }
func (s *state) IsSourceMode() bool { return s.sourceMode }
func (s *state) EditingKey() string { return s.editingKey }
func (s *state) AddDraft() AddDraft { return s.addDraft }
func (s *state) HasTransientUI() bool {
	return s.editingKey != "" || s.sourceMode || s.hiddenOpen || s.addOpen
}

func (s *state) BeginEdit(key, location, draft string) {
	s.editingKey, s.editLocation, s.draft = key, location, draft
}

func (s *state) CancelEdit()             { s.editingKey, s.editLocation, s.draft = "", "", "" }
func (s *state) SetError(message string) { s.errorMessage = message }
func (s *state) Error() string {
	if s == nil {
		return ""
	}
	return s.errorMessage
}
func (s *state) OpenSource(draft string) { s.sourceMode, s.sourceDraft = true, draft }
func (s *state) ReturnToProperties()     { s.sourceMode = false }

func (s *state) Apply(action Action) {
	switch action.Kind {
	case DraftChanged:
		s.draft = action.Value
	case CancelEdit:
		s.CancelEdit()
	case OpenAddForm:
		s.addOpen = true
	case CloseAddForm:
		s.addOpen = false
	case AddDraftChanged:
		s.addDraft = action.AddDraft
	case AddField:
		s.addDraft = action.AddDraft
	case TogglePinned:
		preference := s.Preference(action.FieldKey)
		preference.Pinned = !preference.Pinned
		s.SetPreference(action.FieldKey, preference)
	case ToggleHidden:
		preference := s.Preference(action.FieldKey)
		preference.Hidden = !preference.Hidden
		s.SetPreference(action.FieldKey, preference)
	case ToggleHiddenSection:
		s.hiddenOpen = !s.hiddenOpen
	case ReturnToProperties:
		s.sourceMode = false
	case SourceDraftChanged:
		s.sourceDraft = action.Value
	case ClosePanel:
		s.Close()
	}
}

func (s *state) ClearAddedField() { s.addDraft = AddDraft{}; s.addOpen = false; s.errorMessage = "" }
func (s *state) ClearEdit()       { s.CancelEdit(); s.errorMessage = "" }
func (s *state) ClearError()      { s.errorMessage = "" }

func (s *state) PreferencesPath() string {
	if s == nil {
		return ""
	}
	return s.preferencesPath
}

func (s *state) SetPreferences(path string, preferences map[string]FieldPreference) {
	if s == nil {
		return
	}
	s.preferencesPath = path
	s.preferences = clonePreferences(preferences)
}

func (s *state) Preferences() map[string]FieldPreference {
	if s == nil {
		return nil
	}
	return clonePreferences(s.preferences)
}

func (s *state) Preference(key string) FieldPreference {
	if s == nil {
		return FieldPreference{}
	}
	return s.preferences[key]
}

func (s *state) SetPreference(key string, preference FieldPreference) {
	if s == nil || key == "" {
		return
	}
	if s.preferences == nil {
		s.preferences = make(map[string]FieldPreference)
	}
	s.preferences[key] = preference
}

// MoveField reorders a display field within its current pinned/hidden group.
// Field editability and ordering semantics are supplied by the projection.
func (s *state) MoveField(key string, delta int, fields []Field) bool {
	if s == nil || key == "" || delta == 0 {
		return false
	}
	index := -1
	for i, field := range fields {
		if field.Key == key {
			index = i
			if !field.CanReorder {
				return false
			}
			break
		}
	}
	target := index + delta
	if index < 0 || target < 0 || target >= len(fields) {
		return false
	}
	if fields[index].Pinned != fields[target].Pinned || fields[index].Hidden != fields[target].Hidden || !fields[target].CanReorder {
		return false
	}
	ordered := append([]Field(nil), fields...)
	ordered[index], ordered[target] = ordered[target], ordered[index]
	if s.preferences == nil {
		s.preferences = make(map[string]FieldPreference)
	}
	movedPinned, movedHidden := fields[index].Pinned, fields[index].Hidden
	for i, field := range ordered {
		preference := s.preferences[field.Key]
		if field.Pinned == movedPinned && field.Hidden == movedHidden {
			preference.Order = i + 1
			s.preferences[field.Key] = preference
		}
	}
	return true
}

func clonePreferences(source map[string]FieldPreference) map[string]FieldPreference {
	if len(source) == 0 {
		return nil
	}
	clone := make(map[string]FieldPreference, len(source))
	for key, preference := range source {
		clone[key] = preference
	}
	return clone
}

func (s *state) project(model ViewModel) ViewModel {
	model.EditingKey = s.editingKey
	model.EditingLocation = s.editLocation
	model.Draft = s.draft
	model.AddOpen = s.addOpen
	model.AddDraft = s.addDraft
	model.HiddenOpen = s.hiddenOpen
	model.SourceMode = s.sourceMode
	model.SourceDraft = s.sourceDraft
	if s.errorMessage != "" {
		model.Error = s.errorMessage
	}
	model.Scroll = ScrollPort{state: &s.scroll, Indicator: &s.indicator}
	return model
}
