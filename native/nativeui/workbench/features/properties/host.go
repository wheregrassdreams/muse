package properties

import (
	"errors"

	"github.com/egoist/mygo/ui"
)

var ErrUnavailable = errors.New("properties: host capability is unavailable")

// PreferenceStore persists only per-field presentation preferences. It does
// not read or write frontmatter.
type PreferenceStore interface {
	Load(path string) (map[string]FieldPreference, error)
	Save(path string, preferences map[string]FieldPreference) error
}

// Projection is a host-provided snapshot prepared from document/frontmatter
// and application/document APIs. The properties feature only presents it.
type Projection struct {
	Model            ViewModel
	YAMLSource       string
	Markdown         bool
	FrontmatterValid bool
}

type ProjectionPort func(preferences map[string]FieldPreference) Projection

type WorkflowKind uint8

const (
	SetValue WorkflowKind = iota + 1
	AddValue
	DeleteValue
	ReplaceSource
	NavigateBacklink
	NavigateBacklinks
	OpenInspector
	OpenMigration
)

// WorkflowRequest is the narrow set of document/application operations the
// inspector can request. Presentation-only actions never cross this port.
type WorkflowRequest struct {
	Kind     WorkflowKind
	FieldKey string
	Value    string
	Add      AddDraft
	Backlink Backlink
}

type ActionPort func(WorkflowRequest) error

// Ports provide the active document projection, UI preference persistence,
// and typed workflow routing. None expose application or editor objects.
type Ports struct {
	CurrentPath func() string
	Project     ProjectionPort
	Preferences PreferenceStore
	Route       ActionPort
	Invalidate  func()
}

// Host owns inspector drafts, source-mode and per-field display preferences.
// Frontmatter parsing, inspection and metadata workflows are supplied by its
// ports as snapshots and typed action outcomes.
type Host struct {
	state *state
	ports Ports
}

func NewHost(ports Ports) *Host { return &Host{state: &state{}, ports: ports} }

func (h *Host) IsOpen() bool { return h != nil && h.state != nil && h.state.IsOpen() }
func (h *Host) IsSourceMode() bool {
	return h != nil && h.state != nil && h.state.IsSourceMode()
}
func (h *Host) EditingKey() string {
	if h == nil || h.state == nil {
		return ""
	}
	return h.state.EditingKey()
}
func (h *Host) HasTransientUI() bool {
	return h != nil && h.state != nil && h.state.HasTransientUI()
}
func (h *Host) CancelEdit() {
	if h != nil && h.state != nil {
		h.state.CancelEdit()
		h.invalidate()
	}
}
func (h *Host) ReturnToProperties() {
	if h != nil && h.state != nil {
		h.state.ReturnToProperties()
		h.invalidate()
	}
}
func (h *Host) Preference(key string) FieldPreference {
	if h == nil || h.state == nil {
		return FieldPreference{}
	}
	return h.state.Preference(key)
}
func (h *Host) Preferences() map[string]FieldPreference {
	if h == nil || h.state == nil {
		return nil
	}
	return h.state.Preferences()
}

func (h *Host) Enabled() bool {
	return h != nil && h.projection().Markdown
}

func (h *Host) FrontmatterValid() bool {
	return h != nil && h.projection().FrontmatterValid
}

func (h *Host) Open() {
	if h == nil {
		return
	}
	h.ensureState().Open()
	h.ViewModel()
	h.invalidate()
}

func (h *Host) Close() {
	if h != nil {
		h.ensureState().Close()
		h.invalidate()
	}
}

func (h *Host) ViewModel() ViewModel {
	if h == nil {
		return ViewModel{}
	}
	projection := h.projection()
	model := h.state.project(projection.Model)
	if err := h.state.Error(); err != "" {
		model.Error = err
	}
	return model
}

func (h *Host) RenderSidebar(ctx *ui.Context) {
	if h == nil || !h.IsOpen() {
		return
	}
	t := ctx.Theme()
	ui.Column(ctx).Key("properties-sidebar").Width(340).Shrink(0).FillHeight().
		BorderWidth(0, 0, 0, 1).BorderColor(t.Border).Background(t.Background).Children(func() {
		renderPanel(ctx, h.state, h.ViewModel(), h.Dispatch)
	})
}

func (h *Host) RenderSummary(ctx *ui.Context) {
	if h == nil || !h.Enabled() {
		return
	}
	ui.Column(ctx).FillWidth().Children(func() {
		ui.Box(ctx).Height(100).Shrink(0)
		renderMetadataHeader(ctx, h.state, h.ViewModel(), h.Dispatch)
	})
}

// Dispatch handles local drafts/navigation and sends document workflows to
// the root as typed actions. Preference changes remain feature-owned UI state
// and persist through PreferenceStore.
func (h *Host) Dispatch(action Action) {
	if h == nil {
		return
	}
	s := h.ensureState()
	switch action.Kind {
	case BeginEdit:
		field, ok := h.field(action.FieldKey)
		if !ok || !field.Editable {
			return
		}
		location := action.Location
		if location == "" {
			location = "sidebar"
		}
		s.BeginEdit(field.Key, location, field.Value)
	case DraftChanged, CancelEdit, OpenAddForm, CloseAddForm, AddDraftChanged:
		s.Apply(action)
	case AddField:
		s.Apply(action)
		if err := h.route(WorkflowRequest{Kind: AddValue, Add: action.AddDraft}); err != nil {
			s.SetError(err.Error())
		} else {
			s.ClearAddedField()
		}
	case TogglePinned, ToggleHidden:
		s.Apply(action)
		h.savePreferences()
	case MoveFieldUp:
		h.moveField(action.FieldKey, -1)
	case MoveFieldDown:
		h.moveField(action.FieldKey, 1)
	case ToggleHiddenSection:
		s.Apply(action)
	case OpenSource:
		s.OpenSource(h.projection().YAMLSource)
	case ReturnToProperties:
		s.Apply(action)
	case ClosePanel:
		s.Apply(action)
	case SaveEdit, DeleteField, ApplySource, OpenBacklink, OpenBacklinks, OpenPanel, OpenMigrationPreview:
		if err := h.route(workflowRequest(action)); err != nil {
			s.SetError(err.Error())
		} else {
			switch action.Kind {
			case SaveEdit:
				s.ClearEdit()
			case ApplySource:
				s.ReturnToProperties()
				s.ClearError()
			case OpenPanel:
				s.Open()
			}
		}
	default:
		s.Apply(action)
	}
	h.invalidate()
}

func (h *Host) SetProperty(key, value string) error {
	return h.route(WorkflowRequest{Kind: SetValue, FieldKey: key, Value: value})
}

func (h *Host) DeleteProperty(key string) error {
	return h.route(WorkflowRequest{Kind: DeleteValue, FieldKey: key})
}

func (h *Host) MoveProperty(key string, delta int) {
	switch {
	case delta < 0:
		h.moveField(key, -1)
	case delta > 0:
		h.moveField(key, 1)
	}
	h.invalidate()
}

func (h *Host) ApplyYAMLSource(source string) error {
	return h.route(WorkflowRequest{Kind: ReplaceSource, Value: source})
}

func (h *Host) route(request WorkflowRequest) error {
	if h == nil || h.ports.Route == nil {
		return ErrUnavailable
	}
	return h.ports.Route(request)
}

func workflowRequest(action Action) WorkflowRequest {
	switch action.Kind {
	case SaveEdit:
		return WorkflowRequest{Kind: SetValue, FieldKey: action.FieldKey, Value: action.Value}
	case AddField:
		return WorkflowRequest{Kind: AddValue, Add: action.AddDraft}
	case DeleteField:
		return WorkflowRequest{Kind: DeleteValue, FieldKey: action.FieldKey}
	case ApplySource:
		return WorkflowRequest{Kind: ReplaceSource, Value: action.Value}
	case OpenBacklink:
		return WorkflowRequest{Kind: NavigateBacklink, Backlink: action.Backlink}
	case OpenBacklinks:
		return WorkflowRequest{Kind: NavigateBacklinks}
	case OpenPanel:
		return WorkflowRequest{Kind: OpenInspector}
	case OpenMigrationPreview:
		return WorkflowRequest{Kind: OpenMigration}
	default:
		return WorkflowRequest{}
	}
}

func (h *Host) projection() Projection {
	if h == nil {
		return Projection{}
	}
	path := ""
	if h.ports.CurrentPath != nil {
		path = h.ports.CurrentPath()
	}
	h.loadPreferences(path)
	if h.ports.Project == nil {
		return Projection{}
	}
	return h.ports.Project(h.state.Preferences())
}

func (h *Host) loadPreferences(path string) {
	if h == nil || h.state == nil || h.state.PreferencesPath() == path {
		return
	}
	var preferences map[string]FieldPreference
	if h.ports.Preferences != nil && path != "" {
		loaded, err := h.ports.Preferences.Load(path)
		if err != nil {
			h.state.SetError(err.Error())
		} else {
			preferences = loaded
		}
	}
	h.state.SetPreferences(path, preferences)
}

func (h *Host) field(key string) (Field, bool) {
	model := h.ViewModel()
	for _, group := range [][]Field{model.Fields, model.HiddenFields, model.Summary} {
		for _, field := range group {
			if field.Key == key {
				return field, true
			}
		}
	}
	return Field{}, false
}

func (h *Host) moveField(key string, delta int) {
	model := h.ViewModel()
	fields := append(append([]Field(nil), model.Fields...), model.HiddenFields...)
	if h.state.MoveField(key, delta, fields) {
		h.savePreferences()
	}
}

func (h *Host) savePreferences() {
	if h.ports.Preferences == nil {
		return
	}
	path := h.state.PreferencesPath()
	if err := h.ports.Preferences.Save(path, h.state.Preferences()); err != nil {
		h.state.SetError(err.Error())
	}
}

func (h *Host) ensureState() *state {
	if h.state == nil {
		h.state = &state{}
	}
	return h.state
}

func (h *Host) invalidate() {
	if h != nil && h.ports.Invalidate != nil {
		h.ports.Invalidate()
	}
}
