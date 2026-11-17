package linkpreview

import (
	"path/filepath"
	"time"

	"github.com/egoist/mygo/ui"
	"muse/markdown"
	markdownui "muse/nativeui/components/markdown"
)

const (
	DwellDelay = 450 * time.Millisecond
	LeaveDelay = 220 * time.Millisecond
)

// Handle is an opaque reference to one link-preview component instance. The
// hover lifecycle and decoded thumbnail stay inside the component package.
type Handle struct{ state *state }

func NewHandle() Handle { return Handle{state: &state{}} }

// Snapshot is a detached read-only view for UI integration and diagnostics.
type Snapshot struct {
	Target         string
	X, Y           float32
	LinkBounds     ui.Rect
	Entered        time.Time
	LeaveAt        time.Time
	OverlayHovered bool
	ImagePath      string
	ImageBitmap    *ui.Bitmap
	HasImageAsset  bool
}

func (h Handle) Snapshot() Snapshot {
	if h.state == nil {
		return Snapshot{}
	}
	s := h.state
	return Snapshot{
		Target: s.Target, X: s.X, Y: s.Y, LinkBounds: s.LinkBounds,
		Entered: s.Entered, LeaveAt: s.LeaveAt, OverlayHovered: s.OverlayHovered,
		ImagePath: s.ImagePath(), ImageBitmap: s.ImageBitmap(), HasImageAsset: s.HasImageAsset(),
	}
}

func (h Handle) Hover(target string, x, y float32, bounds ui.Rect, now time.Time) {
	if h.state != nil {
		h.state.Hover(target, x, y, bounds, now, LeaveDelay)
	}
}

func (h Handle) Clear() {
	if h.state != nil {
		h.state.Clear()
	}
}

func (h Handle) SetOverlayHovered(hovered bool, now time.Time) {
	h.setOverlayHoveredAt(hovered, now)
}

func (h Handle) AdvanceImage(now time.Time) (time.Duration, *ui.Bitmap) {
	if h.state == nil {
		return 0, nil
	}
	return h.state.AdvanceImage(now)
}

// Tick advances the dwell and leave-grace lifecycle. It returns whether the
// preview remains active and the delay until the next lifecycle check.
func (h Handle) Tick(now time.Time) (bool, time.Duration) {
	if h.state == nil || h.state.Target == "" {
		return false, 0
	}
	s := h.state
	if !s.LeaveAt.IsZero() && !s.OverlayHovered {
		if remaining := s.LeaveAt.Sub(now); remaining > 0 {
			return true, min(remaining, 80*time.Millisecond)
		}
		s.Clear()
		return false, 0
	}
	if remaining := s.Entered.Add(DwellDelay).Sub(now); remaining > 0 {
		return true, remaining
	}
	return true, 0
}

type NoteTarget struct {
	Title  string
	Path   string // vault-relative path
	Source string
	Offset int
	Root   string // absolute vault root
}

// Ports are native capabilities required to resolve and present a preview.
// The component owns state, dwell/dismiss timing, section projection, and UI.
type Ports struct {
	Suppressed      func() bool
	ResolveImage    func(target string) (string, bool)
	ResolveNote     func(target string) (NoteTarget, bool)
	LoadImage       func(path string) *markdownui.ImageAsset
	Anchor          func(*ui.Context, Snapshot) ui.Rect
	MarkdownOptions func(NoteTarget) markdownui.Options
}

// RenderActive renders one hover preview using application-provided vault,
// image, viewport, and navigation capabilities.
func (h Handle) RenderActive(c *ui.Context, ports Ports) {
	if c == nil || h.state == nil {
		return
	}
	s := h.state
	if s.Target == "" || ports.Suppressed != nil && ports.Suppressed() {
		return
	}
	active, wait := h.Tick(time.Now())
	if !active {
		return
	}
	if wait > 0 {
		c.After(wait)
		return
	}

	anchor, viewport := ui.Rect{}, ui.Rect{}
	if ports.Anchor != nil {
		anchor = ports.Anchor(c, h.Snapshot())
	}
	if width, height := c.Size(); width > 0 && height > 0 {
		viewport = ui.Rect{W: width, H: height}
	}

	if ports.ResolveImage != nil {
		if imagePath, ok := ports.ResolveImage(s.Target); ok && imagePath != "" {
			s.EnsureImage(imagePath, ports.LoadImage)
			next, bitmap := s.AdvanceImage(c.Now())
			if next > 0 {
				c.After(next)
			}
			linkpreviewRenderImage(c, imagePath, bitmap, anchor, viewport, s.OverlayHovered, func(hovered bool) {
				h.setOverlayHovered(hovered)
			})
			return
		}
	}
	if ports.ResolveNote == nil {
		return
	}
	note, ok := ports.ResolveNote(s.Target)
	if !ok || note.Path == "" {
		return
	}
	title := note.Title
	if title == "" {
		title = filepath.Base(note.Path)
		title = title[:len(title)-len(filepath.Ext(title))]
	}
	options := markdownui.Options{}
	if ports.MarkdownOptions != nil {
		options = ports.MarkdownOptions(note)
	}
	previewDoc := markdown.Parse(PreviewSource(note.Source, note.Offset))
	options.BaseDir = filepath.Join(note.Root, filepath.Dir(note.Path))
	options.AssetRoot = note.Root
	linkpreviewRenderMarkdown(c, note, title, previewDoc, options, anchor, viewport, s.OverlayHovered, func(hovered bool) {
		h.setOverlayHovered(hovered)
	})
}

func (h Handle) setOverlayHovered(hovered bool) {
	h.setOverlayHoveredAt(hovered, time.Now())
}

func (h Handle) setOverlayHoveredAt(hovered bool, now time.Time) {
	if h.state == nil {
		return
	}
	h.state.SetOverlayHovered(hovered, now, LeaveDelay)
}

func linkpreviewRenderImage(c *ui.Context, imagePath string, bitmap *ui.Bitmap, anchor, viewport ui.Rect, overlayHovered bool, hover func(bool)) {
	Render(c, ViewModel{
		Kind: Image, Title: filepath.Base(imagePath), Path: imagePath,
		Anchor: anchor, Viewport: viewport, Width: 280, Height: 220, OverlayHovered: overlayHovered,
		Bitmap: bitmap,
	}, func(action Action) {
		if action.Kind == HoverChanged && hover != nil {
			hover(action.Hovered)
		}
	})
}

func linkpreviewRenderMarkdown(c *ui.Context, note NoteTarget, title string, doc *markdown.Doc, options markdownui.Options, anchor, viewport ui.Rect, overlayHovered bool, hover func(bool)) {
	Render(c, ViewModel{
		Kind: Markdown, Title: title, Path: note.Path,
		Anchor: anchor, Viewport: viewport, Width: 380, Height: 300, OverlayHovered: overlayHovered,
		Document: doc, MarkdownOptions: options,
	}, func(action Action) {
		if action.Kind == HoverChanged && hover != nil {
			hover(action.Hovered)
		}
	})
}
