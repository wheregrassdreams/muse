// Package properties renders caller-projected document properties and turns
// edits into typed actions. Frontmatter parsing, metadata policy, persistence,
// and backlink lookup stay with the caller.
package properties

import (
	"strings"

	"github.com/egoist/mygo/ui"
	"muse/locales"
	"muse/ui/scrollbar"
)

type FieldKind uint8

const (
	Text FieldKind = iota + 1
	List
	Boolean
	Number
	Date
	Structure
)

// Field is display-ready metadata. The caller provides Labels, DisplayValue,
// and tag tokens; this package does not parse frontmatter or infer semantics.
type Field struct {
	Key, Label, Value, DisplayValue string
	Tags                            []string
	Kind                            FieldKind
	Editable                        bool
	Removable                       bool
	System                          bool
	Pinned                          bool
	Hidden                          bool
	CanPin                          bool
	CanHide                         bool
	CanReorder                      bool
	CanMoveUp                       bool
	CanMoveDown                     bool
}

type Backlink struct {
	Path, Label string
	Offset      int
	Count       int
}

type AddDraft struct {
	Key, Value string
	Kind       FieldKind
}

type ScrollPort struct {
	state     *ui.ScrollState
	Indicator *scrollbar.Indicator
}

// ViewModel is a complete presentation projection. HiddenFields and
// SystemFields are already selected by the caller; the panel only lays them
// out and reports user intent.
type ViewModel struct {
	Title, Error, EmptyMessage string
	DocumentType               string
	Fields, HiddenFields       []Field
	SystemFields               []Field
	Summary                    []Field
	Backlinks                  []Backlink
	EditingKey, Draft          string
	AddOpen                    bool
	AddDraft                   AddDraft
	HiddenOpen                 bool
	SourceMode                 bool
	SourceDraft                string
	BacklinkCount              int
	MoreCount                  int
	Scroll                     ScrollPort
}

type ActionKind uint8

const (
	BeginEdit ActionKind = iota + 1
	DraftChanged
	SaveEdit
	CancelEdit
	OpenAddForm
	CloseAddForm
	AddDraftChanged
	AddField
	DeleteField
	TogglePinned
	ToggleHidden
	MoveFieldUp
	MoveFieldDown
	ToggleHiddenSection
	OpenBacklink
	OpenBacklinks
	OpenPanel
	OpenMigrationPreview
	OpenSource
	ReturnToProperties
	SourceDraftChanged
	ApplySource
	ClosePanel
)

type Action struct {
	Kind     ActionKind
	FieldKey string
	Value    string
	AddDraft AddDraft
	Backlink Backlink
}

func Render(c *ui.Context, handle *Handle, model ViewModel, dispatch func(Action)) {
	if handle == nil {
		return
	}
	model = handle.state.project(model)
	t := c.Theme()
	title := model.Title
	if title == "" {
		title = locales.PropertyTitle()
	}
	ui.Column(c).Key("properties-panel").Fill().Background(t.Background).Children(func() {
		renderHeader(c, model, title, dispatch)
		if model.Error != "" {
			ui.Text(c, model.Error).FillWidth().Padding(8, 18).FontSize(12).TextColor(ui.Hex("#c34242")).Wrap()
		}
		if model.SourceMode {
			renderSourceEditor(c, model, dispatch)
			return
		}
		renderContent(c, model, dispatch)
	})
}

// RenderSummary draws the compact document metadata row above the editor.
// It emits navigation and editing intent without looking up document state.
func RenderSummary(c *ui.Context, handle *Handle, model ViewModel, dispatch func(Action)) {
	if handle != nil {
		model = handle.state.project(model)
	}
	t := c.Theme()
	quiet := t.TextMuted
	ui.Row(c).Height(36).Shrink(0).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
		if model.DocumentType != "" {
			ui.Text(c, model.DocumentType).FontSize(13).TextColor(quiet)
		}
		separator := func() { ui.Text(c, "·").FontSize(13).TextColor(quiet) }
		if model.BacklinkCount > 0 {
			if model.DocumentType != "" {
				separator()
			}
			b := ui.ButtonBase(c).Height(32).Padding(0).Label(locales.PropertyViewBacklinks())
			b.OnClick(func() { dispatchAction(dispatch, Action{Kind: OpenBacklinks}) })
			b.Children(func() { ui.Text(c, locales.PropertyBacklinkCount(model.BacklinkCount)).FontSize(13).TextColor(quiet) })
		}
		for i, field := range model.Summary {
			separator()
			if i >= 4 {
				b := ui.ButtonBase(c).Height(32).Padding(0, 5).Label(locales.PropertyMoreCount(model.MoreCount))
				b.OnClick(func() { dispatchAction(dispatch, Action{Kind: OpenPanel}) })
				b.Children(func() { ui.Text(c, locales.PropertyMoreCount(model.MoreCount)).FontSize(13).TextColor(quiet) })
				break
			}
			field := field
			if field.Editable {
				b := ui.ButtonBase(c).Height(32).MinWidth(0).MaxWidth(260).Padding(0, 1).Radius(5).Label(field.Label)
				if b.Hovered() {
					b.Background(t.SurfaceHover)
				}
				b.OnClick(func() { dispatchAction(dispatch, Action{Kind: BeginEdit, FieldKey: field.Key}) })
				b.Children(func() { renderFieldValue(c, field, true) })
			} else {
				renderFieldValue(c, field, true)
			}
		}
	})
}

func renderHeader(c *ui.Context, model ViewModel, title string, dispatch func(Action)) {
	t := c.Theme()
	ui.Row(c).Height(58).Shrink(0).Padding(0, 18).Gap(8).AlignItems(ui.Center).Children(func() {
		ui.Text(c, title).FontSize(17).FontWeight(600).TextColor(t.Text)
		ui.Spacer(c)
		if model.SourceMode {
			propertyButton(c, locales.PropertyBackToProperties(), func() {
				dispatchAction(dispatch, Action{Kind: ReturnToProperties})
			})
		} else {
			if model.DocumentType != "" {
				propertyButton(c, locales.PropertyEditType(), func() {
					dispatchAction(dispatch, Action{Kind: BeginEdit, FieldKey: "type"})
				})
			}
			propertyButton(c, locales.PropertyAdd(), func() {
				dispatchAction(dispatch, Action{Kind: OpenAddForm})
			})
			propertyButton(c, locales.PropertyYAMLSource(), func() {
				dispatchAction(dispatch, Action{Kind: OpenSource})
			})
			menu := ui.ButtonBase(c).Height(25).Padding(0, 7).Radius(5).Label(locales.PropertyMenu())
			menu.Menu(func(m *ui.Menu) {
				if m.Item(locales.PropertyMigrationPreviewMenu()).Chosen() {
					dispatchAction(dispatch, Action{Kind: OpenMigrationPreview})
				}
			})
			menu.Children(func() { ui.Text(c, "⋯").FontSize(14).TextColor(c.Theme().TextMuted) })
		}
		propertyButton(c, "×", func() { dispatchAction(dispatch, Action{Kind: ClosePanel}) })
	})
}

func renderContent(c *ui.Context, model ViewModel, dispatch func(Action)) {
	t := c.Theme()
	var fallback ui.ScrollState
	scroll := model.Scroll.state
	if scroll == nil {
		scroll = &fallback
	}
	showScrollbar := model.Scroll.state != nil && model.Scroll.Indicator != nil &&
		model.Scroll.Indicator.Visible(c.Now(), model.Scroll.state.X, model.Scroll.state.Y)
	if showScrollbar {
		c.After(model.Scroll.Indicator.Remaining(c.Now()))
	}
	ui.Scroll(c).Fill().TrackScroll(scroll).Scrollbars(showScrollbar).Children(func() {
		ui.Column(c).FillWidth().Padding(0, 18, 24, 18).Gap(10).Children(func() {
			if len(model.Summary) > 0 {
				renderSummary(c, model.Summary, model, dispatch)
			}
			if len(model.Fields) == 0 && len(model.HiddenFields) == 0 && len(model.SystemFields) == 0 && len(model.Backlinks) == 0 && !model.AddOpen {
				message := model.EmptyMessage
				if message == "" {
					message = locales.PropertyNoValue()
				}
				ui.Column(c).FillWidth().MinHeight(72).Center().Padding(18).Radius(18).Background(propertyCardColor(t)).Children(func() {
					ui.Text(c, message).FontSize(13).TextColor(t.TextMuted).Wrap()
				})
			}
			for _, field := range model.Fields {
				renderField(c, field, model, dispatch)
			}
			if len(model.Backlinks) > 0 {
				renderBacklinks(c, model.Backlinks, dispatch)
			}
			if len(model.HiddenFields) > 0 || len(model.SystemFields) > 0 {
				renderHiddenFields(c, model, dispatch)
			}
			if model.AddOpen {
				renderAddForm(c, model.AddDraft, dispatch)
			}
		})
	})
}

func renderSummary(c *ui.Context, fields []Field, model ViewModel, dispatch func(Action)) {
	t := c.Theme()
	ui.Row(c).FillWidth().MinHeight(36).Gap(6).AlignItems(ui.Center).Wrap().Children(func() {
		for _, field := range fields {
			field := field
			if field.Editable {
				b := ui.ButtonBase(c).Label(locales.PropertyEditLabel(field.Key)).Height(30).Padding(0, 6).Radius(5)
				if b.Hovered() {
					b.Background(t.SurfaceHover)
				}
				b.OnClick(func() { dispatchAction(dispatch, Action{Kind: BeginEdit, FieldKey: field.Key}) })
				b.Children(func() { renderFieldValue(c, field, true) })
			} else {
				renderFieldValue(c, field, true)
			}
		}
	})
}

func renderField(c *ui.Context, field Field, model ViewModel, dispatch func(Action)) {
	t := c.Theme()
	editing := model.EditingKey == field.Key && field.Editable
	ui.Column(c).Key("property-card-"+field.Key).FillWidth().MinHeight(60).Gap(8).Padding(16, 18).Radius(17).Background(propertyCardColor(t)).Children(func() {
		if editing {
			ui.Text(c, field.Label).FontSize(13).TextColor(t.TextMuted)
			renderEditor(c, field, model.Draft, dispatch)
			return
		}
		stacked := field.Kind == Date || field.Kind == Structure || len([]rune(fieldDisplay(field))) > 26
		content := func() {
			ui.Text(c, field.Label).FontSize(13).TextColor(t.TextMuted).Shrink(0)
			if field.Editable {
				value := ui.ButtonBase(c).Key("property-value-" + field.Key).Label(field.Label + " " + fieldDisplay(field)).Grow(1).MinWidth(0).MinHeight(28).Padding(0).Justify(ui.Start)
				value.OnClick(func() { dispatchAction(dispatch, Action{Kind: BeginEdit, FieldKey: field.Key}) })
				value.Children(func() { renderFieldValue(c, field, false) })
			} else {
				renderFieldValue(c, field, false)
			}
		}
		if stacked {
			ui.Column(c).FillWidth().Gap(7).Children(content)
		} else {
			ui.Row(c).FillWidth().Gap(10).AlignItems(ui.Center).Children(content)
		}
		if !field.System {
			renderFieldActions(c, field, dispatch)
		}
	})
}

func renderEditor(c *ui.Context, field Field, value string, dispatch func(Action)) {
	if field.Kind == Boolean {
		ui.Row(c).Gap(5).AlignItems(ui.Center).Children(func() {
			propertyButton(c, "true", func() { dispatchAction(dispatch, Action{Kind: SaveEdit, FieldKey: field.Key, Value: "true"}) })
			propertyButton(c, "false", func() { dispatchAction(dispatch, Action{Kind: SaveEdit, FieldKey: field.Key, Value: "false"}) })
			propertyButton(c, locales.CommonCancel(), func() { dispatchAction(dispatch, Action{Kind: CancelEdit, FieldKey: field.Key}) })
		})
		return
	}
	draft := value
	ui.Row(c).FillWidth().Gap(5).AlignItems(ui.Center).Children(func() {
		input := ui.TextInput(c.Key("property-edit-"+field.Key), &draft).Grow(1).MinWidth(0).
			Placeholder(locales.PropertyListPlaceholder()).Label(field.Label).AutoFocus().
			OnSubmit(func() { dispatchAction(dispatch, Action{Kind: SaveEdit, FieldKey: field.Key, Value: draft}) })
		_ = input
		if draft != value {
			dispatchAction(dispatch, Action{Kind: DraftChanged, FieldKey: field.Key, Value: draft})
		}
		propertyButton(c, locales.CommonSave(), func() { dispatchAction(dispatch, Action{Kind: SaveEdit, FieldKey: field.Key, Value: draft}) })
		propertyButton(c, locales.CommonCancel(), func() { dispatchAction(dispatch, Action{Kind: CancelEdit, FieldKey: field.Key}) })
	})
}

func renderFieldActions(c *ui.Context, field Field, dispatch func(Action)) {
	if !field.CanPin && !field.CanHide && !(field.CanReorder && (field.CanMoveUp || field.CanMoveDown)) && !field.Removable {
		return
	}
	ui.Row(c).FillWidth().Gap(4).AlignItems(ui.Center).Children(func() {
		if field.CanPin {
			label := locales.PropertyPin()
			if field.Pinned {
				label = locales.PropertyUnpin()
			}
			propertyButton(c, label, func() { dispatchAction(dispatch, Action{Kind: TogglePinned, FieldKey: field.Key}) })
		}
		if field.CanHide {
			label := locales.PropertyHideFromSummary()
			if field.Hidden {
				label = locales.PropertyShow()
			}
			propertyButton(c, label, func() { dispatchAction(dispatch, Action{Kind: ToggleHidden, FieldKey: field.Key}) })
		}
		if field.CanReorder && field.CanMoveUp {
			propertyButton(c, "↑", func() { dispatchAction(dispatch, Action{Kind: MoveFieldUp, FieldKey: field.Key}) })
		}
		if field.CanReorder && field.CanMoveDown {
			propertyButton(c, "↓", func() { dispatchAction(dispatch, Action{Kind: MoveFieldDown, FieldKey: field.Key}) })
		}
		if field.Removable {
			propertyButton(c, locales.PropertyDelete(), func() { dispatchAction(dispatch, Action{Kind: DeleteField, FieldKey: field.Key}) })
		}
	})
}

func renderFieldValue(c *ui.Context, field Field, compact bool) {
	t := c.Theme()
	value := fieldDisplay(field)
	if field.Kind == List && len(field.Tags) > 0 {
		ui.Row(c).FillWidth().MinWidth(0).Gap(5).Wrap().Children(func() {
			for _, tag := range field.Tags {
				ui.Text(c, tag).Key("property-tag-"+field.Key+"-"+tag).MaxWidth(150).
					FontSize(12).TextColor(t.Text).Padding(5, 9).Radius(16).Background(t.SurfaceHover).SingleLine().Ellipsis("…")
			}
		})
		return
	}
	if value == "" {
		value = locales.PropertyNoValue()
	}
	fontSize := float32(14)
	if compact {
		fontSize = 12
	}
	ui.Text(c, value).FillWidth().FontSize(fontSize).TextColor(t.Text).Wrap()
}

func renderBacklinks(c *ui.Context, backlinks []Backlink, dispatch func(Action)) {
	t := c.Theme()
	ui.Column(c).Key("property-card-backlinks").FillWidth().MinHeight(60).Gap(7).Padding(16, 18).Radius(17).Background(propertyCardColor(t)).Children(func() {
		ui.Text(c, locales.VaultBacklinksTitle()).FontSize(13).TextColor(t.TextMuted)
		for _, backlink := range backlinks {
			backlink := backlink
			label := backlink.Label
			if label == "" {
				label = backlink.Path
			}
			if backlink.Count > 1 {
				label += locales.VaultBacklinkCountSuffix(backlink.Count)
			}
			b := ui.ButtonBase(c).FillWidth().MinHeight(28).Padding(0).Justify(ui.Start).Label(label)
			b.OnClick(func() { dispatchAction(dispatch, Action{Kind: OpenBacklink, Backlink: backlink}) })
			b.Children(func() { ui.Text(c, label).FontSize(14).TextColor(t.Text).SingleLine().Ellipsis("…") })
		}
	})
}

func renderHiddenFields(c *ui.Context, model ViewModel, dispatch func(Action)) {
	label := locales.PropertyHidden()
	if model.HiddenOpen {
		label = locales.PropertyShow()
	}
	propertyButton(c, label, func() { dispatchAction(dispatch, Action{Kind: ToggleHiddenSection}) })
	if model.HiddenOpen {
		for _, field := range model.HiddenFields {
			renderField(c, field, model, dispatch)
		}
		for _, field := range model.SystemFields {
			renderField(c, field, model, dispatch)
		}
	}
}

func renderAddForm(c *ui.Context, draft AddDraft, dispatch func(Action)) {
	t := c.Theme()
	ui.Column(c).FillWidth().Gap(7).Padding(14).Radius(15).Background(propertyCardColor(t)).Children(func() {
		ui.Row(c).FillWidth().AlignItems(ui.Center).Children(func() {
			ui.Text(c, locales.PropertyAdd()).FontSize(13).FontWeight(600).TextColor(t.Text)
			ui.Spacer(c)
			propertyButton(c, locales.CommonCancel(), func() { dispatchAction(dispatch, Action{Kind: CloseAddForm}) })
		})
		key, value := draft.Key, draft.Value
		ui.TextInput(c.Key("property-add-key"), &key).FillWidth().Placeholder(locales.PropertyName()).Label(locales.PropertyName())
		kindLabel := fieldKindLabel(draft.Kind)
		kind := ui.ButtonBase(c).Label(kindLabel).Height(28).Padding(0, 8)
		kind.Menu(func(m *ui.Menu) {
			for _, option := range []FieldKind{Text, List, Boolean, Number, Date} {
				if m.Item(fieldKindLabel(option)).Chosen() {
					dispatchAction(dispatch, Action{Kind: AddDraftChanged, AddDraft: AddDraft{Key: key, Value: value, Kind: option}})
				}
			}
		})
		kind.Children(func() { ui.Text(c, kindLabel).FontSize(11).TextColor(t.Text) })
		if draft.Kind == Boolean {
			ui.Row(c).Gap(5).Children(func() {
				propertyButton(c, locales.PropertyAddFalse(), func() {
					dispatchAction(dispatch, Action{Kind: AddField, AddDraft: AddDraft{Key: key, Value: "false", Kind: draft.Kind}})
				})
				propertyButton(c, locales.PropertyAddTrue(), func() {
					dispatchAction(dispatch, Action{Kind: AddField, AddDraft: AddDraft{Key: key, Value: "true", Kind: draft.Kind}})
				})
			})
		} else {
			placeholder := locales.PropertyValuePlaceholder()
			if draft.Kind == List {
				placeholder = locales.PropertyListValuePlaceholder()
			} else if draft.Kind == Date {
				placeholder = locales.PropertyDateValuePlaceholder()
			}
			ui.TextInput(c.Key("property-add-value"), &value).FillWidth().Placeholder(placeholder).Label(locales.PropertyValue()).OnSubmit(func() {
				dispatchAction(dispatch, Action{Kind: AddField, AddDraft: AddDraft{Key: key, Value: value, Kind: draft.Kind}})
			})
			propertyButton(c, locales.CommonAdd(), func() {
				dispatchAction(dispatch, Action{Kind: AddField, AddDraft: AddDraft{Key: key, Value: value, Kind: draft.Kind}})
			})
		}
		if key != draft.Key || value != draft.Value {
			dispatchAction(dispatch, Action{Kind: AddDraftChanged, AddDraft: AddDraft{Key: key, Value: value, Kind: draft.Kind}})
		}
	})
}

func renderSourceEditor(c *ui.Context, model ViewModel, dispatch func(Action)) {
	t := c.Theme()
	draft := model.SourceDraft
	ui.Column(c).Fill().MinHeight(0).Padding(12).Gap(8).Children(func() {
		ui.Text(c, locales.PropertyYAMLDetail()).FillWidth().FontSize(11).TextColor(t.TextMuted)
		ui.TextArea(c.Key("properties-yaml-source"), &draft).Grow(1).MinHeight(120).FillWidth().Font("monospace").Label(locales.PropertyYAMLSource())
		if draft != model.SourceDraft {
			dispatchAction(dispatch, Action{Kind: SourceDraftChanged, Value: draft})
		}
		ui.Row(c).FillWidth().Justify(ui.End).Children(func() {
			propertyButton(c, locales.PropertyApplySource(), func() { dispatchAction(dispatch, Action{Kind: ApplySource, Value: draft}) })
		})
	})
}

func fieldDisplay(field Field) string {
	if field.DisplayValue != "" {
		return field.DisplayValue
	}
	return strings.TrimSpace(field.Value)
}

func fieldKindLabel(kind FieldKind) string {
	switch kind {
	case List:
		return locales.PropertyTypeList()
	case Boolean:
		return locales.PropertyTypeBoolean()
	case Number:
		return locales.PropertyTypeNumber()
	case Date:
		return locales.PropertyTypeDate()
	case Structure:
		return locales.PropertyTypeStructure()
	default:
		return locales.PropertyTypeText()
	}
}

func propertyCardColor(t *ui.Theme) ui.Color {
	if t.Dark {
		return ui.Hex("#2d2d30")
	}
	return ui.Hex("#f7f7f7")
}

func propertyButton(c *ui.Context, label string, fn func()) {
	b := ui.ButtonBase(c).Height(25).Padding(0, 7).Radius(5).Label(label).OnClick(fn)
	if b.Hovered() {
		b.Background(c.Theme().SurfaceHover)
	}
	b.Children(func() { ui.Text(c, label).FontSize(10).TextColor(c.Theme().TextMuted) })
}

func dispatchAction(dispatch func(Action), action Action) {
	if dispatch != nil {
		dispatch(action)
	}
}
