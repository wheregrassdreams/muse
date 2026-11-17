package render

import "github.com/egoist/mygo/ui"

// SourceRange is a half-open range in the borrowed source snapshot. Ranges
// carry positions only; the renderer does not retain source text or ranges
// after Build returns.
type SourceRange struct {
	Start int
	End   int
}

// Snapshot is the host's synchronous, read-only view of one editor revision.
// Source and the slices are borrowed until Paint returns. The host must not
// mutate them during Build/Paint, and Renderer never stores them. Blocks are
// the currently visible view projection; syntax-derived projection is being
// moved behind Renderer in incremental steps.
type Snapshot struct {
	Revision     uint64
	Source       string
	Caret        int
	Selection    SourceRange
	HasSelection bool
	Reveal       []SourceRange
	Highlights   []SourceRange
	Viewport     ui.Rect
	Style        Style
	Options      ProjectionOptions
	Behind       []Overlay
	Blocks       []Block
	InFront      []Overlay
}

// ProjectionOptions controls source-to-view choices that affect visible
// content. Values are copied into the synchronous snapshot.
type ProjectionOptions struct {
	SmartTypography    bool
	SoftBreaksAsSpaces bool
}

// Renderer composes borrowed host facts into a detached frame. It has no
// mutable state and does not retain the snapshot or resource capabilities.
type Renderer struct{}

// Build forms the visible presentation frame for one source revision. The
// block projection is currently supplied by the editor host; subsequent
// renderer stages consume Source/Selection/Reveal and syntax snapshots here.
// Resources are accepted at this boundary for resource-aware projection, but
// are only borrowed by Paint today.
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
