package imagedocument

import (
	"errors"

	"muse/markdown/images"
	workspaceui "muse/nativeui/workspace"
)

var ErrHostUnavailable = errors.New("image document: window host is unavailable")

// Host is the generic window-manager capability needed to own a transient
// image document. The feature does not know about App or other document types.
type Host interface {
	Open(workspaceui.Content, workspaceui.Placement) (workspaceui.Handle, error)
	Close(workspaceui.Handle) error
	Contains(workspaceui.Handle) bool
}

// Ports are supplied by the workbench composition root. Functions are used
// for dependencies that can change when a vault is switched.
type Ports struct {
	Host      Host
	Placement func() workspaceui.Placement
	Cache     func() images.CacheStore
	Files     func() images.LocalFiles

	Surface SurfaceOptions

	OnSelectedPath  func(string)
	OnRecordHistory func(string)
	OnNotice        func(error)
	OnDecodeError   func(path string)
	OnChanged       func()
}

// Controller owns the image document's generic host handle and active path.
// Call it on the hosting manager's UI-owner thread.
type Controller struct {
	ports   Ports
	handle  workspaceui.Handle
	content *Content
	path    string
}

func NewController(ports Ports) *Controller { return &Controller{ports: ports} }

// Open replaces the current image document in the selected pane. An empty
// path is ignored, matching the previous workbench command behavior.
func (c *Controller) Open(path string, record bool) error {
	if c == nil || path == "" {
		return nil
	}
	if c.ports.Host == nil || c.ports.Placement == nil {
		return c.notice(ErrHostUnavailable)
	}
	c.Close()
	var cache images.CacheStore
	if c.ports.Cache != nil {
		cache = c.ports.Cache()
	}
	var files images.LocalFiles
	if c.ports.Files != nil {
		files = c.ports.Files()
	}
	surface := c.ports.Surface
	surface.LocalFiles = files
	var content *Content
	surface.OnClose = func() { c.closed(content) }
	content, hasImage := NewContent(path, cache, surface)
	handle, err := c.ports.Host.Open(content, c.ports.Placement())
	if err != nil {
		content.Close()
		return c.notice(err)
	}
	c.handle, c.content, c.path = handle, content, path
	if c.ports.OnSelectedPath != nil {
		c.ports.OnSelectedPath(path)
	}
	if record && c.ports.OnRecordHistory != nil {
		c.ports.OnRecordHistory(path)
	}
	if c.ports.OnChanged != nil {
		c.ports.OnChanged()
	}
	if !hasImage && c.ports.OnDecodeError != nil {
		c.ports.OnDecodeError(path)
	}
	return nil
}

func (c *Controller) Close() {
	if c == nil {
		return
	}
	handle := c.handle
	if c.ports.Host != nil && !handle.IsZero() {
		_ = c.ports.Host.Close(handle)
	}
	if c.handle == handle {
		c.handle, c.content, c.path = workspaceui.Handle{}, nil, ""
	}
}

func (c *Controller) IsOpen() bool {
	return c != nil && c.ports.Host != nil && !c.handle.IsZero() && c.ports.Host.Contains(c.handle)
}

func (c *Controller) Path() string {
	if !c.IsOpen() {
		return ""
	}
	return c.path
}

func (c *Controller) Handle() workspaceui.Handle {
	if !c.IsOpen() {
		return workspaceui.Handle{}
	}
	return c.handle
}

func (c *Controller) closed(content *Content) {
	if c == nil || c.content != content {
		return
	}
	c.handle, c.content, c.path = workspaceui.Handle{}, nil, ""
	if c.ports.OnChanged != nil {
		c.ports.OnChanged()
	}
}

func (c *Controller) notice(err error) error {
	if c != nil && c.ports.OnNotice != nil {
		c.ports.OnNotice(err)
	}
	return err
}
