package render

import "github.com/egoist/mygo/ui"

// Snapshot is the host's synchronous, read-only visible view for one editor
// revision. Blocks and overlays are borrowed through Build/Paint; Renderer
// never stores them.
type Snapshot struct {
	Revision uint64
	Viewport ui.Rect
	Style    Style
	Behind   []Overlay
	Blocks   []Block
	InFront  []Overlay
}

// Renderer composes borrowed host facts into a detached frame. It has no
// mutable state and does not retain the snapshot or resource capabilities.
type Renderer struct{}

// Build forms the visible presentation frame from the editor host's typed
// block projection. Resources are borrowed by Paint and never retained.
func (Renderer) Build(snapshot Snapshot, _ Resources) Frame {
	return Frame{
		Revision: snapshot.Revision,
		Viewport: snapshot.Viewport,
		Style:    snapshot.Style,
		Behind:   snapshot.Behind,
		Blocks:   snapshot.Blocks,
		InFront:  snapshot.InFront,
	}
}
