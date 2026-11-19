package imagepreview

import (
	"muse/markdown/images"
	workspaceui "muse/nativeui/workspace"
)

// ControllerOptions are the application capabilities used by the preview.
// Path policy, clipboard ownership and event-loop scheduling stay in the
// composition root and enter only through these ports.
type ControllerOptions struct {
	Windows   *workspaceui.Manager
	Placement workspaceui.Placement

	ImageCache images.CacheStore
	LocalFiles images.LocalFiles
	Label      func(source string) string
	CopyImage  func(source string)
	OnMain     func(func())
	Invalidate func()
	OnError    func(error)
}

// Controller owns the lifecycle of the image preview hosted in a generic
// workbench workspaceui. All methods must be called on the UI-owner thread.
type Controller struct {
	options ControllerOptions
	handle  workspaceui.Handle
}

// Host is the shell-facing lifecycle contract for the image preview.
type Host interface {
	Open(source string) error
	Close()
	IsOpen() bool
}

var _ Host = (*Controller)(nil)

func NewController(options ControllerOptions) *Controller {
	if options.Placement.Host == "" {
		options.Placement = workspaceui.Placement{Host: "main", Layer: workspaceui.ModalOverlay}
	}
	return &Controller{options: options}
}

// Open replaces the current preview and opens source in the configured host.
func (c *Controller) Open(source string) error {
	if c == nil {
		return workspaceui.ErrInvalidContent
	}
	if source == "" {
		return nil
	}
	c.Close()

	label := source
	if c.options.Label != nil {
		label = c.options.Label(source)
	}
	content := NewContent(
		source, label, c.options.ImageCache, c.options.OnMain,
		c.options.Invalidate, c.options.CopyImage, nil,
	)
	content.SetLocalFiles(c.options.LocalFiles)

	var handle workspaceui.Handle
	content.onClose = func() {
		if c.handle == handle {
			c.handle = workspaceui.Handle{}
		}
	}
	opened, err := c.options.Windows.Open(content, c.options.Placement)
	if err != nil {
		content.Close()
		if c.options.OnError != nil {
			c.options.OnError(err)
		}
		return err
	}
	handle = opened
	c.handle = opened
	if c.options.Invalidate != nil {
		c.options.Invalidate()
	}
	return nil
}

// Close dismisses the hosted preview if it is still present.
func (c *Controller) Close() {
	if c == nil {
		return
	}
	handle := c.handle
	c.handle = workspaceui.Handle{}
	if c.options.Windows != nil && !handle.IsZero() {
		_ = c.options.Windows.Close(handle)
	}
	if c.options.Invalidate != nil {
		c.options.Invalidate()
	}
}

// IsOpen reports whether the controller's hosted preview is still live.
func (c *Controller) IsOpen() bool {
	return c != nil && c.options.Windows != nil && c.options.Windows.Contains(c.handle)
}

// Handle returns the generic host handle while the preview is open.
func (c *Controller) Handle() workspaceui.Handle {
	if c == nil {
		return workspaceui.Handle{}
	}
	return c.handle
}
