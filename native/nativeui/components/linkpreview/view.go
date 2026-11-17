// Package linkpreview renders reusable hover previews for linked notes and
// images. Target resolution, preview timing, and navigation remain with the
// caller.
package linkpreview

import (
	"path/filepath"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"muse/locales"
	"muse/markdown"
	markdownui "muse/nativeui/components/markdown"
)

type Kind uint8

const (
	Markdown Kind = iota + 1
	Image
)

// state owns transient hover and dismissal behavior for one preview.
type state struct {
	Target         string
	X, Y           float32
	LinkBounds     ui.Rect
	Entered        time.Time
	LeaveAt        time.Time
	OverlayHovered bool
	imagePath      string
	imageAsset     *markdownui.ImageAsset
	imageBitmap    *ui.Bitmap
}

// Hover updates the active link and preserves its dwell timer while the same
// link remains under the pointer.
func (s *state) Hover(target string, x, y float32, bounds ui.Rect, now time.Time, leaveDelay time.Duration) {
	target = strings.TrimSpace(target)
	if target == "" {
		if s.Target != "" && s.LeaveAt.IsZero() {
			s.LeaveAt = now.Add(leaveDelay)
		}
		return
	}
	if s.Target != target {
		*s = state{Target: target, Entered: now}
	}
	s.X, s.Y, s.LinkBounds = x, y, bounds
	s.LeaveAt = time.Time{}
}

// SetOverlayHovered starts or cancels the grace period as the pointer enters
// or leaves the rendered preview card.
func (s *state) SetOverlayHovered(hovered bool, now time.Time, leaveDelay time.Duration) {
	s.OverlayHovered = hovered
	if hovered {
		s.LeaveAt = time.Time{}
	} else if !s.LeaveAt.IsZero() || s.Target != "" {
		s.LeaveAt = now.Add(leaveDelay)
	}
}

// EnsureImage keeps the image resource associated with the active preview.
// The loader is invoked only when the path changes; replacing or clearing the
// path drops the previous asset and bitmap references.
func (s *state) EnsureImage(path string, load func(string) *markdownui.ImageAsset) {
	if s.imagePath == path {
		return
	}
	s.imagePath = path
	s.imageAsset = nil
	s.imageBitmap = nil
	if path != "" && load != nil {
		s.imageAsset = load(path)
	}
}

// AdvanceImage advances an animated preview and returns its next frame
// deadline together with the currently available renderer bitmap.
func (s *state) AdvanceImage(now time.Time) (time.Duration, *ui.Bitmap) {
	if s.imageAsset == nil {
		s.imageBitmap = nil
		return 0, nil
	}
	next, _ := s.imageAsset.Advance(now)
	s.imageBitmap = s.imageAsset.Bitmap()
	return next, s.imageBitmap
}

// ImagePath reports the source associated with the active preview.
func (s *state) ImagePath() string { return s.imagePath }

// ImageBitmap returns the currently available image preview bitmap.
func (s *state) ImageBitmap() *ui.Bitmap { return s.imageBitmap }

// HasImageAsset reports whether an image resource is attached to this state.
func (s *state) HasImageAsset() bool { return s.imageAsset != nil }

// Clear ends the transient hover session and releases its image resources.
func (s *state) Clear() { *s = state{} }

type ViewModel struct {
	Kind            Kind
	Title           string
	Path            string
	Anchor          ui.Rect
	Viewport        ui.Rect
	Width, Height   float32
	OverlayHovered  bool
	Bitmap          *ui.Bitmap
	Document        *markdown.Doc
	MarkdownOptions markdownui.Options
}

type ActionKind uint8

const HoverChanged ActionKind = 1

type Action struct {
	Kind    ActionKind
	Hovered bool
}

func Render(c *ui.Context, model ViewModel, dispatch func(Action)) {
	width, height := model.Width, model.Height
	if width <= 0 {
		width = 380
	}
	if height <= 0 {
		height = 300
	}
	boxRect := Place(model.Viewport, model.Anchor, width, height)
	t := c.Theme()
	key, label := "wiki-link-preview", locales.LinkPreviewTitle()
	if model.Kind == Image {
		key, label = "wiki-image-link-preview", locales.ImageLinkPreviewTitle()
	}
	box := ui.Box(c).Key(key).Label(label).Absolute().Left(boxRect.X).Top(boxRect.Y).
		Width(boxRect.W).Height(boxRect.H).Radius(10).Background(t.Background).Border(1, t.Border).
		Shadow(0, 10, 28, -5, ui.RGBA(0, 0, 0, 0.24)).Clip()
	hovered := box.Hovered()
	if hovered != model.OverlayHovered && dispatch != nil {
		dispatch(Action{Kind: HoverChanged, Hovered: hovered})
	}
	box.Children(func() {
		if model.Kind == Image {
			renderImage(c, model, t)
			return
		}
		renderMarkdown(c, model, t)
	})
}

func renderImage(c *ui.Context, model ViewModel, t *ui.Theme) {
	ui.Column(c).Fill().Padding(10).Gap(7).Children(func() {
		title := model.Title
		if title == "" {
			title = filepath.Base(model.Path)
		}
		ui.Text(c, title).FontSize(11).TextColor(t.TextMuted).SingleLine().Ellipsis("…")
		ui.Box(c).FillWidth().Height(1).Background(t.Border)
		if model.Bitmap == nil {
			ui.Text(c, locales.ImagePreviewUnavailable()).Fill().Center().TextColor(t.TextMuted)
			return
		}
		bw, bh := ui.ImageSize(model.Bitmap)
		if bw <= 0 || bh <= 0 {
			ui.Text(c, locales.ImagePreviewUnavailable()).Fill().Center().TextColor(t.TextMuted)
			return
		}
		fit := min(254/bw, 178/bh)
		if fit <= 0 {
			fit = 1
		}
		ui.Box(c).Fill().Center().Children(func() {
			ui.Image(c, model.Bitmap).Key("wiki-image-hover-thumbnail").Size(bw*fit, bh*fit)
		})
	})
}

func renderMarkdown(c *ui.Context, model ViewModel, t *ui.Theme) {
	ui.Column(c).Fill().Padding(14, 16).Gap(7).Children(func() {
		ui.Text(c, model.Title).FontSize(15).FontWeight(650).SingleLine().Ellipsis("…")
		ui.Text(c, model.Path).FontSize(10.5).TextColor(t.TextMuted).SingleLine().Ellipsis("…")
		ui.Box(c).FillWidth().Height(1).Background(t.Border)
		markdownui.View(c, model.Document, model.MarkdownOptions).Fill()
	})
}

// Place positions the card beside its trigger, preferring below then above,
// while keeping the trigger's vertical range unobscured when possible.
func Place(viewport, anchor ui.Rect, width, height float32) ui.Rect {
	const (
		edge = float32(8)
		gap  = float32(12)
	)
	width = min(width, max(viewport.W-2*edge, 1))
	height = min(height, max(viewport.H-2*edge, 1))
	x := max(viewport.X+edge, min(anchor.X, viewport.X+viewport.W-width-edge))
	below := anchor.Y + anchor.H + gap
	above := anchor.Y - height - gap
	bottom := viewport.Y + viewport.H - edge
	topEdge := viewport.Y + edge
	var y float32
	switch {
	case below+height <= bottom:
		y = below
	case above >= topEdge:
		y = above
	case bottom-below >= anchor.Y-topEdge:
		y = max(topEdge, min(below, bottom-height))
	default:
		y = max(topEdge, min(above, bottom-height))
	}
	return ui.Rect{X: x, Y: y, W: width, H: height}
}
