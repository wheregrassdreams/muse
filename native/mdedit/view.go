package mdedit

import (
	"time"

	"github.com/egoist/mygo/transfer"
	"github.com/egoist/mygo/ui"

	"muse/mdoc"
)

// laid is what the last layout was made of: the layout is done again only
// when it changes.
type laid struct {
	rev        uint64
	folds      uint64
	inclusive  bool
	linewise   bool
	caret      int
	anchor     int
	reveal     int
	compose    string
	colX, colW float32
	look       lookKey
	assets     uint64
	blockRev   uint64
	phantom    *phantom
	focused    bool
	// scrollY and viewH are part of the layout of a virtualized document
	// only: the leaves it shapes are the ones its viewport shows, so both a
	// scroll and a window made taller or shorter ask for another layout.
	// A document laid out whole keeps exact positions, and neither moves a
	// leaf.
	scrollY float32
	viewH   float32
}

// layoutIfNeeded lays the document out again when what it shows changed:
// the source, the caret, the width, the look.
func (e *Editor) layoutIfNeeded() {
	e.sync()
	// The viewport is what the layout shapes around. Both the key below and
	// the layout pass read it from the scroll position of this moment, so
	// that a scroll made during a frame is laid out for where it landed,
	// not where it was when the frame began.
	if e.viewH > 0 {
		// A screen above and below what shows, so that a scroll does not
		// have to shape the leaves it brings in on the frame it shows them.
		e.viewTop, e.viewBottom = e.scroll.Y-e.viewH, e.scroll.Y+2*e.viewH
	}
	virtual := e.virtualLayout()
	blockRev := uint64(0)
	if e.block != nil {
		blockRev = e.block.revision
	}
	k := laid{
		rev:       e.rev,
		folds:     e.foldRevision,
		inclusive: e.selectionInclusive,
		linewise:  e.selectionLinewise,
		caret:     e.caret,
		anchor:    e.anchor,
		reveal:    e.revealCaret(),
		compose:   e.compose,
		colX:      e.colX,
		colW:      e.colW,
		look:      e.lk.key(),
		assets:    e.assetGen.Load(),
		blockRev:  blockRev,
		phantom:   e.phantom,
		focused:   e.focused,
	}
	if virtual {
		k.scrollY, k.viewH = e.scroll.Y, e.viewH
	}
	if k == e.laidKey && e.leavesLaid {
		return
	}
	e.laidKey = k
	e.leavesLaid = true
	if e.colW <= 0 {
		e.colW = 720
	}
	e.layoutDoc(e.colX, e.colW, virtual)
}

// virtualLayout reports whether the layout shapes only the leaves near
// the viewport. A small document is laid out whole: its layout is cheap,
// and exact positions make it simpler. A large one would pay for shaping
// every leaf of text it does not show, on the first frame as much as on
// any other.
func (e *Editor) virtualLayout() bool {
	return len(e.leaves) >= virtualLayoutMin
}

// virtualLayoutMin is the number of leaves from which a layout is
// virtualized.
const virtualLayoutMin = 1000

// View builds the editor: a scroll view of the document as one surface
// the user writes on.
func (e *Editor) View(c *ui.Context) ui.Element {
	t := c.Theme()
	e.setLook(t)
	e.frame++
	e.services = c.Services()
	e.focused = e.handle.Focused(c)
	if !e.focused && e.dragging {
		e.dragging, e.frozen = false, false
	}

	b := e.scrollHandle.Bounds(c)
	if b.W > 0 {
		e.elemW, e.viewH = b.W, b.H
	}
	maxW := e.opts.MaxWidth
	if maxW <= 0 {
		maxW = 820
	}
	padX := e.opts.PadX
	if padX <= 0 {
		padX = 40
	}
	if e.elemW > 0 {
		e.colW = max(min(maxW, e.elemW-2*padX), 120)
		e.colX = max((e.elemW-e.colW)/2, 0)
	}
	padTop := e.lk.size*2.2 + max(e.opts.TopInset, 0) + max(e.opts.HeaderHeight, 0)
	if e.padTop != padTop {
		e.leavesLaid = false
	}
	e.padTop = padTop
	e.padBottom = max(e.viewH*0.35, e.lk.size*4)
	e.layoutIfNeeded()
	e.pruneViews()
	e.scrollIntoView()
	e.advanceVisibleImageAssets(c)

	h := max(e.docH, e.viewH)
	showBar := e.scrollIndicator.Visible(c.Now(), e.scroll.X, e.scroll.Y)
	if showBar {
		c.After(e.scrollIndicator.Remaining(c.Now()))
	}
	sc := ui.Scroll(c).Fill().Scrollbars(showBar).TrackScroll(&e.scroll).Bind(&e.scrollHandle)
	tablePointer := ui.Point{X: -1, Y: -1}
	sc.Children(func() {
		ui.Column(c).FillWidth().Height(h).Shrink(0).Children(func() {
			label := e.opts.Label
			if label == "" {
				label = "文档"
			}
			// Cursor comes after HandleTextInput, which sets the text cursor
			// itself: the shape set last is the one the window shows, and the
			// editor changes it as the pointer moves over links and boxes.
			el := ui.Box(c).Absolute().Top(0).Left(0).Right(0).Height(h).Focusable().FocusRing(false).Bind(&e.handle).
				Role(ui.RoleTextField).Label(label).
				HandleTextInput(e).Cursor(e.cursor).
				HandleInput(e.input).Draw(e.paint).ContextMenu(e.menu)
			if px, py, over := el.PointerPosition(); over {
				// Read the native surface every frame: scrolling under a
				// stationary mouse changes its document-space cell, too.
				tablePointer = ui.Point{X: px, Y: py}
			}
			if files := el.DroppedFiles(); len(files) > 0 {
				if x, y, ok := el.PointerPosition(); ok {
					e.setCaret(e.hitTest(x, y), false)
				}
				e.dropFiles(files)
			}
			if e.opts.Header != nil && e.opts.HeaderHeight > 0 {
				ui.Box(c).Height(max(e.opts.TopInset, 0)).Shrink(0)
				ui.Column(c).Key("document-header").ZIndex(1).Margin(0, e.colX).Width(e.colW).Height(e.opts.HeaderHeight).Shrink(0).Children(func() { e.opts.Header(c) })
			}

		})
	})
	if e.dragging {
		// Momentum can move the document without a pointer-move event. Read
		// the pointer in the stable viewport, then project it through the
		// current scroll. A cached document Y can otherwise appear outside
		// the viewport and start a self-sustaining selection autoscroll.
		px, py, _ := sc.PointerPosition()
		e.pointerX, e.pointerY = px, py+e.scroll.Y
	}
	if e.autoScroll() {
		c.After(16 * time.Millisecond)
	}
	// The rail floats over the document's right edge, and follows the pointer
	// there as an overlay does.
	e.railView(c, b)
	e.tableEdgesView(c, b, tablePointer)
	e.mathPreviewView(c, b)
	e.wikiSuggestionsView(c, b)
	if e.railAnim {
		// The lens and the preview are on their way to somewhere: frames
		// move them, as CSS transitions would.
		c.After(16 * time.Millisecond)
	} else if e.railHover >= 0 {
		// The pointer's leaving the rail makes no event here: ask for a
		// frame, so the hover and its preview clear once it has gone.
		c.After(80 * time.Millisecond)
	}
	if e.focused {
		phase := c.Now().Sub(e.blinkStart)
		if phase < 30*time.Second {
			c.After(blink - phase%blink)
		}
	}
	if e.elemW <= 0 {
		// The first frame has no size yet: lay out again at the next.
		c.After(time.Millisecond)
	}
	return sc
}

// advanceVisibleImageAssets advances animated assets on the UI thread and
// schedules the next repaint. Nested wiki embeds share the same frame clock.
func (e *Editor) advanceVisibleImageAssets(c *ui.Context) {
	now := c.Now()
	var nextFrame time.Duration
	seen := make(map[*mdoc.ImageAsset]struct{})
	var visit func(*Editor)
	visit = func(editor *Editor) {
		if editor == nil {
			return
		}
		visibleTop := editor.scroll.Y - 48
		visibleBottom := editor.scroll.Y + editor.viewH + 48
		advance := func(o *object) {
			if o == nil {
				return
			}
			if o.asset != nil {
				if _, ok := seen[o.asset]; !ok {
					seen[o.asset] = struct{}{}
					next, _ := o.asset.Advance(now)
					if next > 0 && (nextFrame == 0 || next < nextFrame) {
						nextFrame = next
					}
				}
				o.refreshAssetBitmap()
			}
			if o.embed != nil {
				visit(o.embed)
			}
		}
		for _, leaf := range editor.leaves {
			if leaf.y+leaf.h < visibleTop || leaf.y > visibleBottom {
				continue
			}
			if leaf.view == nil {
				continue
			}
			advance(leaf.view.block)
			for i := range leaf.view.disp.pieces {
				advance(leaf.view.disp.pieces[i].obj)
			}
		}
	}
	visit(e)
	if nextFrame > 0 {
		c.After(nextFrame)
	}
}

// scrollIntoView scrolls the caret into view after it moved, or a source
// offset to the top for ScrollToOffset.
func (e *Editor) scrollIntoView() {
	if e.viewH <= 0 {
		return
	}
	if e.hasScrollTo {
		e.revealHorizontalCaret(e.scrollTo)
		e.hasScrollTo = false
		e.wantReveal = false
		if r, ok := e.caretRect(e.scrollTo); ok {
			if e.scrollCenter {
				e.scroll.Y = min(max(r.Y-(e.viewH-r.H)/2, 0), max(e.docH-e.viewH, 0))
			} else {
				e.scroll.Y = max(r.Y-e.lk.size*2, 0)
			}
		}
		e.scrollCenter = false
		return
	}
	if !e.wantReveal {
		return
	}
	e.revealHorizontalCaret(e.selectionFocus())
	e.wantReveal = false
	r, ok := e.caretRect(e.selectionFocus())
	if !ok {
		return
	}
	margin := min(e.lk.size*3, e.viewH/4)
	topMargin := max(margin, e.opts.TopInset+8)
	switch {
	case r.Y < e.scroll.Y+topMargin:
		e.scroll.Y = max(r.Y-topMargin, 0)
	case r.Y+r.H > e.scroll.Y+e.viewH-margin:
		e.scroll.Y = r.Y + r.H - e.viewH + margin
	}
}

// textData returns text as clipboard data.
func textData(s string) transfer.Data { return transfer.TextData(s) }
