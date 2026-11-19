package naviconpicker

import (
	"strings"

	"github.com/egoist/mygo/ui"
	componenticons "muse/nativeui/ui/icons"
	"muse/ui/iconlibrary"
)

// Preferences is the narrow persistence port for vault-relative item paths.
// The composition root adapts the application's vault icon repository to it.
type Preferences interface {
	Get(itemPath string) string
	Set(itemPath, iconID string) error
	Delete(itemPath string) error
}

// NoticeKind identifies user-visible outcomes without coupling this feature to
// application localization or notification presentation.
type NoticeKind uint8

const (
	PreferencesUnavailable NoticeKind = iota + 1
	PreferenceSaveFailed
	PreferenceUpdated
)

type Notice struct {
	Kind NoticeKind
	Err  error
}

// Ports are supplied by the composition root. Search is optional and defaults
// to the shared icon catalog; it can be injected in controller tests.
type Ports struct {
	Preferences  Preferences
	RelativePath func(absolutePath string) (itemPath string, ok bool)
	Search       func(query string) []Option
	Invalidate   func()
	Notify       func(Notice)
}

// Host owns picker lifecycle, asynchronous catalog searches, path projection
// and preference actions. It contains no application object or
// concrete repository dependency.
type Host struct {
	state *state
	ports Ports
}

func NewHost(ports Ports) *Host {
	return &Host{state: &state{}, ports: ports}
}

func (c *Host) IsOpen() bool {
	return c != nil && c.state != nil && c.state.IsOpen()
}

func (c *Host) Close() {
	if c == nil || c.state == nil {
		return
	}
	c.state.CancelSearch()
	c.state.Close()
	c.invalidate()
}

// Open resolves the host path to a vault-relative item path and starts the
// initial catalog search. It returns false if the host cannot provide the
// required preference/path capabilities.
func (c *Host) Open(absolutePath string) bool {
	if c == nil || c.state == nil || c.ports.Preferences == nil || c.ports.RelativePath == nil {
		c.notify(Notice{Kind: PreferencesUnavailable})
		return false
	}
	itemPath, ok := c.ports.RelativePath(absolutePath)
	if !ok || itemPath == "" {
		return false
	}
	request := c.state.Open(itemPath)
	c.search("", request)
	c.invalidate()
	return true
}

func (c *Host) Snapshot() ViewModel {
	if c == nil || c.state == nil {
		return ViewModel{}
	}
	return c.state.Snapshot()
}

// Dispatch applies view actions and owns their asynchronous/persistence
// effects. A superseded search cannot publish its results because state
// rejects completions whose request token is stale.
func (c *Host) Dispatch(action Action) {
	if c == nil || c.state == nil {
		return
	}
	switch action.Kind {
	case QueryChanged:
		query := c.state.Apply(action)
		request := c.state.BeginSearch()
		c.search(query, request)
		c.invalidate()
	case ScrollChanged:
		c.state.Apply(action)
	case Choose:
		c.choose(action.ID)
	case RestoreDefault:
		c.choose("")
	case Close:
		c.Close()
	}
}

// RenderPopover delegates the native popover lifecycle to the feature view.
func (c *Host) RenderPopover(ctx *ui.Context, anchor ui.Element) {
	if c == nil || c.state == nil || !c.state.IsOpen() {
		return
	}
	renderPopover(ctx, anchor, c.state, c.Dispatch)
}

// IconID returns the stored icon for a host path. Invalid/out-of-vault paths
// and unavailable stores naturally use the host's default icon.
func (c *Host) IconID(absolutePath string) string {
	if c == nil || c.ports.Preferences == nil || c.ports.RelativePath == nil {
		return ""
	}
	itemPath, ok := c.ports.RelativePath(absolutePath)
	if !ok || itemPath == "" {
		return ""
	}
	return c.ports.Preferences.Get(itemPath)
}

// IconForPath returns the chosen icon or the shared default file/folder icon.
func (c *Host) IconForPath(absolutePath string, directory bool) *ui.SVG {
	if id := c.IconID(absolutePath); id != "" {
		if icon, ok := componenticons.Lookup(id); ok {
			return icon
		}
	}
	if directory {
		return componenticons.Folder()
	}
	return componenticons.File()
}

func (c *Host) search(query string, request uint64) {
	search := c.ports.Search
	if search == nil {
		search = SearchOptions
	}
	go func() {
		results := search(query)
		if c.state.CompleteSearch(request, results) {
			c.invalidate()
		}
	}()
}

func (c *Host) choose(iconID string) {
	if c.ports.Preferences == nil {
		c.notify(Notice{Kind: PreferencesUnavailable})
	} else {
		itemPath := c.state.Snapshot().Path
		var err error
		if iconID == "" {
			err = c.ports.Preferences.Delete(itemPath)
		} else {
			err = c.ports.Preferences.Set(itemPath, iconID)
		}
		if err != nil {
			c.notify(Notice{Kind: PreferenceSaveFailed, Err: err})
		} else {
			c.notify(Notice{Kind: PreferenceUpdated})
		}
	}
	c.state.CancelSearch()
	c.state.Close()
	c.invalidate()
}

func (c *Host) invalidate() {
	if c != nil && c.ports.Invalidate != nil {
		c.ports.Invalidate()
	}
}

func (c *Host) notify(notice Notice) {
	if c != nil && c.ports.Notify != nil {
		c.ports.Notify(notice)
	}
}

// SearchOptions adapts the shared catalog to view-ready options. SVG lookup is
// delegated to workbench/framework/components/icons, which owns the single parsed-SVG cache.
func SearchOptions(query string) []Option {
	icons := iconlibrary.Search(strings.TrimSpace(query))
	options := make([]Option, 0, len(icons))
	for _, entry := range icons {
		icon, ok := componenticons.Lookup(entry.ID)
		if !ok {
			continue
		}
		options = append(options, Option{ID: entry.ID, Label: entry.Name, Icon: icon})
	}
	return options
}
