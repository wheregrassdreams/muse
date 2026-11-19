package linkpreview

import (
	"time"

	"github.com/egoist/mygo/ui"
)

// Controller owns the transient state and view lifecycle for one link-preview
// feature instance. Application-specific vault, image, and placement behavior
// is supplied through typed callbacks rather than an App dependency.
type Controller struct {
	handle handle
	ports  Ports
}

// Host is the narrow rendering and lifecycle surface consumed by the shell.
// The shell can display and dismiss previews without depending on the
// controller implementation or owning its transient hover state.
type Host interface {
	Snapshot() Snapshot
	Hover(target string, x, y float32, bounds ui.Rect)
	Clear()
	IsOpen() bool
	Render(context *ui.Context)
}

var _ Host = (*Controller)(nil)

type ControllerOptions struct {
	Ports Ports
}

func NewController(options ControllerOptions) *Controller {
	return &Controller{handle: newHandle(), ports: options.Ports}
}

func (c *Controller) Snapshot() Snapshot {
	if c == nil {
		return Snapshot{}
	}
	return c.handle.Snapshot()
}

func (c *Controller) Hover(target string, x, y float32, bounds ui.Rect) {
	if c != nil {
		c.handle.Hover(target, x, y, bounds, time.Now())
	}
}

func (c *Controller) Clear() {
	if c != nil {
		c.handle.Clear()
	}
}

func (c *Controller) IsOpen() bool {
	return c != nil && c.handle.Snapshot().Target != ""
}

func (c *Controller) Render(context *ui.Context) {
	if c != nil {
		c.handle.RenderActive(context, c.ports)
	}
}
