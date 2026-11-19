package quickopen

import (
	"errors"

	"github.com/egoist/mygo/ui"
	hosting "muse/nativeui/workbench/framework/hosting"
)

var ErrWindowHostUnavailable = errors.New("quick open window host is unavailable")

// WindowHost is the narrow hosted-window capability used by Quick Open. The
// application supplies its workbench manager; this feature owns the content
// lifecycle and placement request but not the manager implementation.
type WindowHost interface {
	Open(hosting.Content, hosting.Placement) (hosting.Handle, error)
	Close(hosting.Handle) error
	Contains(hosting.Handle) bool
}

// ControllerOptions contains feature inputs and application callbacks. Rows
// are already ranked by the candidate source; this package never implements
// a second matcher.
type ControllerOptions struct {
	Candidates CandidateSource
	FileIcon   *ui.SVG
	IconTint   func(*ui.Theme) ui.Color
	Windows    WindowHost
	Placement  hosting.Placement
	OnOpen     func(path string)
	OnClose    func()
}

// Controller owns Quick Open's hosted surface lifecycle. It does not retain
// App or any application/domain object.
type Controller struct {
	options ControllerOptions
	handle  hosting.Handle
	content *Content
}

// SetVault replaces the live vault behind this controller's candidate source.
// Workspace changes keep the same overlay host while switching candidate data.
func (c *Controller) SetVault(vault VaultQuickOpen) {
	if c == nil {
		return
	}
	source, ok := c.options.Candidates.(*VaultCandidates)
	if ok && source != nil {
		source.SetVault(vault)
	}
}

func NewController(options ControllerOptions) *Controller {
	return &Controller{options: options}
}

func (c *Controller) Open() error {
	if c == nil || c.options.Windows == nil {
		return ErrWindowHostUnavailable
	}
	if c.content != nil {
		if err := c.Close(); err != nil {
			return err
		}
	}
	opened := false
	var content *Content
	content = NewContent(SurfaceOptions{
		Candidates: c.options.Candidates,
		FileIcon:   c.options.FileIcon,
		IconTint:   c.options.IconTint,
		OnOpen:     c.options.OnOpen,
		OnClose: func() {
			if c.content == content {
				c.content = nil
				c.handle = hosting.Handle{}
			}
			if opened && c.options.OnClose != nil {
				c.options.OnClose()
			}
		},
	})
	handle, err := c.options.Windows.Open(content, c.options.Placement)
	if err != nil {
		content.Close()
		return err
	}
	opened = true
	c.content = content
	c.handle = handle
	return nil
}

func (c *Controller) Close() error {
	if c == nil || c.content == nil {
		return nil
	}
	if c.options.Windows != nil && !c.handle.IsZero() && c.options.Windows.Contains(c.handle) {
		return c.options.Windows.Close(c.handle)
	}
	c.content.Close()
	return nil
}

func (c *Controller) IsOpen() bool {
	return c != nil && c.options.Windows != nil && !c.handle.IsZero() && c.options.Windows.Contains(c.handle)
}
