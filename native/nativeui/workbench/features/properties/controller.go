package properties

import (
	"errors"
	"time"

	"github.com/egoist/mygo/ui"
	"muse/document/frontmatter"
	"muse/locales"
)

var ErrDocumentUnavailable = errors.New("properties: document source is unavailable")

// DocumentSnapshot is the document context required by the inspector. The
// application supplies the current path, source and document-kind decision.
type DocumentSnapshot struct {
	Path     string
	Source   string
	Markdown bool
}

// PreferenceStore persists presentation preferences separately from Markdown.
type PreferenceStore interface {
	Load(path string) (map[string]FieldPreference, error)
	Save(path string, preferences map[string]FieldPreference) error
}

// MetadataCommands applies a validated property edit through the active editor
// session. The application adapter owns editor access and use-case wiring.
type MetadataCommands interface {
	SetProperty(key, value string) error
	AddProperty(key, value string, kind FieldKind) error
	DeleteProperty(key string) error
	ReplaceYAML(source string) error
}

// ControllerPorts are the application capabilities consumed by Properties.
// They are deliberately values and narrow actions; the feature never holds an
// application or editor object.
type ControllerPorts struct {
	Document         func() DocumentSnapshot
	Preferences      PreferenceStore
	Metadata         MetadataCommands
	SystemProperties func(DocumentSnapshot) []SystemProperty
	Backlinks        func() []BacklinkSource
	OpenBacklink     func(path string, offset int)
	OpenBacklinks    func()
	OpenMigration    func()
	Invalidate       func()
}

// Controller owns the inspector lifecycle, document projection cache, field
// preferences, and action policy. The state handle is private; callers receive
// only the lifecycle and snapshot operations needed by the composition root.
type Controller struct {
	state        *state
	ports        ControllerPorts
	systemPath   string
	systemSource string
}

func NewController(ports ControllerPorts) *Controller {
	return &Controller{state: &state{}, ports: ports}
}

func (c *Controller) IsOpen() bool {
	return c != nil && c.state != nil && c.state.IsOpen()
}

func (c *Controller) IsSourceMode() bool {
	return c != nil && c.state != nil && c.state.IsSourceMode()
}

func (c *Controller) EditingKey() string {
	if c == nil || c.state == nil {
		return ""
	}
	return c.state.EditingKey()
}

func (c *Controller) HasTransientUI() bool {
	return c != nil && c.state != nil && c.state.HasTransientUI()
}

func (c *Controller) CancelEdit() {
	if c != nil && c.state != nil {
		c.state.CancelEdit()
	}
}

func (c *Controller) ReturnToProperties() {
	if c != nil && c.state != nil {
		c.state.ReturnToProperties()
		c.invalidate()
	}
}

func (c *Controller) Preference(key string) FieldPreference {
	if c == nil || c.state == nil {
		return FieldPreference{}
	}
	return c.state.Preference(key)
}

func (c *Controller) Preferences() map[string]FieldPreference {
	if c == nil || c.state == nil {
		return nil
	}
	return c.state.Preferences()
}

func (c *Controller) Enabled() bool {
	return c != nil && c.document().Markdown
}

func (c *Controller) Open() {
	if c == nil || c.state == nil {
		return
	}
	c.state.Open()
	c.refreshSystemProperties()
	c.invalidate()
}

func (c *Controller) Close() {
	if c == nil || c.state == nil {
		return
	}
	c.state.Close()
	c.invalidate()
}

func (c *Controller) SetOpenedAt(at time.Time) {
	if c != nil && c.state != nil {
		c.state.SetOpenedAt(at)
	}
}

// Document returns the cached frontmatter projection for the current source.
func (c *Controller) Document() frontmatter.Document {
	if c == nil || c.state == nil {
		return frontmatter.Document{}
	}
	doc := c.document()
	return c.state.Document(doc.Path, doc.Source)
}

// RenderSidebar presents the full inspector and dispatches typed user actions
// back through this controller.
func (c *Controller) RenderSidebar(ctx *ui.Context) {
	if c == nil || !c.IsOpen() {
		return
	}
	t := ctx.Theme()
	ui.Column(ctx).Key("properties-sidebar").Width(340).Shrink(0).FillHeight().
		BorderWidth(0, 0, 0, 1).BorderColor(t.Border).Background(t.Background).Children(func() {
		renderPanel(ctx, c.state, c.ViewModel(), c.Dispatch)
	})
}

// RenderSummary draws the metadata row above the editor when the active
// document is supported by this feature.
func (c *Controller) RenderSummary(ctx *ui.Context) {
	if c == nil || !c.Enabled() {
		return
	}
	ui.Column(ctx).FillWidth().Children(func() {
		ui.Box(ctx).Height(100).Shrink(0)
		renderMetadataHeader(ctx, c.state, c.ViewModel(), c.Dispatch)
	})
}

func (c *Controller) ViewModel() ViewModel {
	if c == nil || c.state == nil {
		return ViewModel{}
	}
	c.refreshSystemProperties()
	doc := c.document()
	if c.state.PreferencesPath() != doc.Path {
		var preferences map[string]FieldPreference
		if c.ports.Preferences != nil && doc.Path != "" {
			loaded, err := c.ports.Preferences.Load(doc.Path)
			if err != nil {
				c.state.SetError(err.Error())
			} else {
				preferences = loaded
			}
		}
		c.state.SetPreferences(doc.Path, preferences)
	}
	parsed := c.state.Document(doc.Path, doc.Source)
	input := ProjectionInput{Title: locales.PropertyTitle()}
	if !doc.Markdown {
		return ProjectViewModel(input)
	}
	if parsed.Err != nil {
		input.Error = parsed.Err.Error()
		return c.state.project(ProjectViewModel(input))
	}
	input.DocumentType = DocumentType(parsed)
	input.Preferences = c.state.Preferences()
	input.Projection = ProjectFields(parsed.Fields, input.Preferences)
	input.System = c.state.SystemProperties()
	openedAt := c.state.OpenedAt()
	if !openedAt.IsZero() && !frontmatter.HasField(parsed, "last_opened") {
		input.LastOpened = locales.PropertyDateTime(openedAt.Local())
	}
	if c.ports.Backlinks != nil {
		input.Backlinks = append([]BacklinkSource(nil), c.ports.Backlinks()...)
	}
	return c.state.project(ProjectViewModel(input))
}

func (c *Controller) Dispatch(action Action) {
	if c == nil || c.state == nil {
		return
	}
	c.state.Apply(action)
	switch action.Kind {
	case BeginEdit:
		if field, ok := c.field(action.FieldKey); ok && field.Editable {
			c.state.BeginEdit(field.Key, "sidebar", field.Value)
		}
	case SaveEdit:
		if err := c.setProperty(action.FieldKey, action.Value); err != nil {
			c.state.SetError(err.Error())
		} else {
			c.state.ClearEdit()
			c.refreshSystemProperties()
		}
	case AddField:
		if c.ports.Metadata == nil {
			c.setError(ErrDocumentUnavailable)
			break
		}
		if err := c.ports.Metadata.AddProperty(action.AddDraft.Key, action.AddDraft.Value, action.AddDraft.Kind); err != nil {
			c.state.SetError(err.Error())
			break
		}
		c.documentChanged()
		c.state.ClearAddedField()
		c.refreshSystemProperties()
	case DeleteField:
		if c.ports.Metadata == nil {
			c.setError(ErrDocumentUnavailable)
			break
		}
		if err := c.ports.Metadata.DeleteProperty(action.FieldKey); err != nil {
			c.state.SetError(err.Error())
			break
		}
		c.documentChanged()
		c.refreshSystemProperties()
	case TogglePinned, ToggleHidden:
		c.savePreferences()
	case MoveFieldUp:
		c.moveField(action.FieldKey, -1)
	case MoveFieldDown:
		c.moveField(action.FieldKey, 1)
	case OpenBacklink:
		if c.ports.OpenBacklink != nil {
			c.ports.OpenBacklink(action.Backlink.Path, action.Backlink.Offset)
		}
	case OpenBacklinks:
		if c.ports.OpenBacklinks != nil {
			c.ports.OpenBacklinks()
		}
	case OpenPanel:
		c.Open()
	case OpenMigrationPreview:
		if c.ports.OpenMigration != nil {
			c.ports.OpenMigration()
		}
	case OpenSource:
		c.state.OpenSource(frontmatter.YAMLSource(c.document().Source))
	case ReturnToProperties:
		c.state.ReturnToProperties()
	case ApplySource:
		if c.ports.Metadata == nil {
			c.setError(ErrDocumentUnavailable)
			break
		}
		if err := c.ports.Metadata.ReplaceYAML(action.Value); err != nil {
			c.state.SetError(err.Error())
			break
		}
		c.documentChanged()
		c.state.ReturnToProperties()
		c.state.ClearError()
		c.refreshSystemProperties()
	case ClosePanel:
		c.invalidate()
	default:
		c.invalidate()
	}
	c.invalidate()
}

func (c *Controller) SetProperty(key, value string) error {
	if c == nil {
		return ErrDocumentUnavailable
	}
	if err := c.setProperty(key, value); err != nil {
		return err
	}
	c.refreshSystemProperties()
	c.invalidate()
	return nil
}

func (c *Controller) DeleteProperty(key string) error {
	if c == nil {
		return ErrDocumentUnavailable
	}
	if c.ports.Metadata == nil {
		return ErrDocumentUnavailable
	}
	if err := c.ports.Metadata.DeleteProperty(key); err != nil {
		return err
	}
	c.documentChanged()
	c.refreshSystemProperties()
	c.invalidate()
	return nil
}

// MoveProperty reorders one editable field inside its pinned/unpinned group
// and persists the presentation preference through the configured port.
func (c *Controller) MoveProperty(key string, delta int) {
	if c != nil {
		c.moveField(key, delta)
		c.invalidate()
	}
}

func (c *Controller) ApplyYAMLSource(draft string) error {
	if c == nil || c.ports.Metadata == nil {
		return ErrDocumentUnavailable
	}
	if err := c.ports.Metadata.ReplaceYAML(draft); err != nil {
		return err
	}
	c.documentChanged()
	c.refreshSystemProperties()
	c.invalidate()
	return nil
}

func (c *Controller) document() DocumentSnapshot {
	if c == nil || c.ports.Document == nil {
		return DocumentSnapshot{}
	}
	return c.ports.Document()
}

func (c *Controller) field(key string) (frontmatter.Field, bool) {
	doc := c.document()
	parsed := c.state.Document(doc.Path, doc.Source)
	for _, field := range parsed.Fields {
		if field.Key == key {
			return field, true
		}
	}
	for _, field := range ProjectFields(parsed.Fields, c.state.Preferences()).Core {
		if field.Key == key {
			return field, true
		}
	}
	return frontmatter.Field{}, false
}

func (c *Controller) setProperty(key, value string) error {
	if c.ports.Metadata == nil {
		return ErrDocumentUnavailable
	}
	if err := c.ports.Metadata.SetProperty(key, value); err != nil {
		return err
	}
	c.documentChanged()
	return nil
}

func (c *Controller) moveField(key string, delta int) {
	doc := c.document()
	parsed := c.state.Document(doc.Path, doc.Source)
	if parsed.Err != nil || !c.state.MoveField(key, delta, parsed.Fields) {
		return
	}
	c.savePreferences()
}

func (c *Controller) savePreferences() {
	if c.ports.Preferences == nil {
		return
	}
	doc := c.document()
	if err := c.ports.Preferences.Save(doc.Path, c.state.Preferences()); err != nil {
		c.state.SetError(err.Error())
	}
}

func (c *Controller) documentChanged() {
	c.state.InvalidateDocument()
	c.systemPath, c.systemSource = "", ""
}

func (c *Controller) refreshSystemProperties() {
	if c == nil || c.state == nil {
		return
	}
	doc := c.document()
	if c.systemPath == doc.Path && c.systemSource == doc.Source {
		return
	}
	c.systemPath, c.systemSource = doc.Path, doc.Source
	if c.ports.SystemProperties == nil {
		c.state.SetSystemProperties(nil)
		return
	}
	c.state.SetSystemProperties(c.ports.SystemProperties(doc))
}

func (c *Controller) setError(err error) {
	if err != nil {
		c.state.SetError(err.Error())
	}
}

func (c *Controller) invalidate() {
	if c != nil && c.ports.Invalidate != nil {
		c.ports.Invalidate()
	}
}
