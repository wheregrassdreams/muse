// Package render paints detached snapshots of the editor's visible document.
// It knows presentation data and resource capabilities, but not the editor's
// source model, parser, selection owner, or mutable UI component.
package render

import (
	"github.com/egoist/mygo/ui"
)

// Frame is a short-lived, immutable projection of the visible viewport. The
// host supplies only visible blocks plus overscan; strings in the frame are
// display text, never slices of the document source.
type Frame struct {
	Revision uint64
	Viewport ui.Rect
	Style    Style
	Behind   []Overlay
	Blocks   []Block
	InFront  []Overlay
}

// Style contains resolved values needed to paint one frame. It deliberately
// has no theme or editor pointer so a frame cannot observe later mutations.
type Style struct {
	Dark                                                 bool
	Text, Muted, Accent, Panel, Border                   ui.Color
	Selection, Caret, QuoteBar                           ui.Color
	Heading, Marker, Link, LinkHover                     ui.Color
	InlineCodeText, InlineCodeBackground, MarkBackground ui.Color
	Font, Mono                                           string
	FontSize, LineHeight                                 float32
}

// BlockKind identifies the presentation shape of a visible block.
type BlockKind uint8

const (
	TextBlock BlockKind = iota
	CodeBlock
	CalloutBlock
	ImageBlock
	MathBlock
	DiagramBlock
	EmbedBlock
)

// Block contains only visible presentation values. Frame and its referenced
// layout data are borrowed read-only for the synchronous Paint call; render
// never retains them after returning.
type Block struct {
	Kind       BlockKind
	Bounds     ui.Rect
	Clip       ui.Rect
	ClipRadius float32
	// Layout is an immutable layout of already-projected visible display text.
	// It contains no leaf/source reference and is scoped to this paint call.
	Layout           TextLayout
	TextX, TextY     float32
	Runs             []Run
	Children         []Block
	Resource         ResourceID
	Title            string
	Alt              string
	Alert            AlertKind
	Panel            bool
	Label            string
	HorizontalOffset float32
}

// CalloutLine is the immutable geometry and presentation metadata for one
// visible line inside a callout. The editor host supplies source-derived IDs,
// indentation and title-edit state; render composes those lines into cards.
type CalloutLine struct {
	ID                   int
	X, Y, Height, Right  float32
	FontSize             float32
	Title                string
	Alert                AlertKind
	HeaderSourceRevealed bool
}

// OverlayKind identifies a backend-neutral visual primitive whose geometry is
// already resolved by the host. Overlays contain no source or editor state.
type OverlayKind uint8

const (
	FillOverlay OverlayKind = iota
	StrokeOverlay
	TextOverlay
	PolylineOverlay
	LayoutOverlay
)

// Overlay is a small immutable drawing instruction. Behind overlays are
// painted before Blocks; InFront overlays are painted after them.
type Overlay struct {
	Kind       OverlayKind
	Bounds     ui.Rect
	Clip       ui.Rect
	Color      ui.Color
	Radius     float32
	Width      float32
	ClipRadius float32
	Text       ui.Span
	MaxW       float32
	Layout     TextLayout
	X, Y       float32
	Points     [][2]float32
}

// TextLayout exposes only the read-only painter operation needed by this
// package. The host owns the layout and keeps it alive only through Paint.
type TextLayout interface {
	Draw(*ui.Painter, float32, float32, ui.Color)
}

// Run is one styled display-text run from the editor's already-built display
// projection, not source text. The host must not pass source-backed strings.
type Run struct {
	Text  string
	Style ui.Span
	X, Y  float32
}

// AlertKind is the alert presentation, independent of parser block types.
type AlertKind uint8

const (
	AlertNote AlertKind = iota
	AlertTip
	AlertImportant
	AlertWarning
	AlertCaution
)

// ResourceID is an opaque host-owned key. It carries no path, source bytes,
// or decoded pixels into the frame.
type ResourceID uint64

// ImageResources resolves only image assets requested by visible blocks.
type ImageResources interface {
	Image(ResourceID) (*ui.Bitmap, bool)
}

// MathResources resolves only math rasters requested by visible blocks.
type MathResources interface {
	Math(ResourceID, bool, string) (MathVisual, bool)
	// DrawMath is an optional host-owned painter for a cached math visual.
	// Return true once it painted the requested visual.
	DrawMath(*ui.Painter, ui.Rect, ResourceID, bool, string) bool
}

// MathVisual is a cached UI-ready math visual. The resource owner controls
// its cache lifetime; render only reads the bitmap and paints its fallback
// glyphs at the supplied origin and scale without copying them.
type MathVisual interface {
	Bitmap() *ui.Bitmap
	DrawFallback(p *ui.Painter, x, y, naturalScale float32)
}

// DiagramResources resolves a cached, read-only painter for one visible
// diagram. Parser models and editor scroll/drag state stay with the host.
type DiagramResources interface {
	Diagram(ResourceID) (DiagramVisual, bool)
}

// DiagramVisual is the narrowly scoped painter capability implemented by the
// native Markdown diagram component.
type DiagramVisual interface {
	Width() float32
	Draw(*ui.Painter, ui.Rect, *ui.Theme)
}

// Resources groups narrow resource capabilities; implementations may provide
// any subset. It does not expose editor state or arbitrary callbacks.
type Resources struct {
	Images   ImageResources
	Math     MathResources
	Diagrams DiagramResources
}
