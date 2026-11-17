package editor

import (
	"math"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/egoist/mygo/transfer"
	"github.com/egoist/mygo/ui"

	"muse/editor/input"
	"muse/editor/layout"
	"muse/locales"
	"muse/markdown/images"
	syntax "muse/markdown/syntax"
	media "muse/nativeui/components/editor/blocks/media"
	"muse/nativeui/components/editor/feedback"
	render "muse/nativeui/components/editor/render"
	"muse/nativeui/interactions/dragpayload"
)

// dropVaultItem inserts an Obsidian-style wiki link for a vault item dragged
// from Muse's tree. Links use the visible name, not an absolute filesystem path.
func (e *Editor) dropVaultItem(item dragpayload.VaultItem) {
	path := item.Path
	if path == "" {
		path = item.Name
	}
	target := strings.TrimSpace(item.Name)
	if target == "" {
		target = filepath.Base(path)
	}
	if path != "" && strings.TrimSpace(e.opts.AssetRoot) != "" {
		if rel, err := filepath.Rel(e.opts.AssetRoot, path); err == nil && rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel) {
			target = filepath.ToSlash(rel)
		}
	}
	if target == "" {
		return
	}
	isImage := !item.Directory && isImagePath(target)
	if isImage {
		target = strings.ReplaceAll(target, "|", "\\|")
		target = strings.ReplaceAll(target, "]", "\\]")
		e.insertText("![[" + target + "]]")
		return
	}
	if !item.Directory && isMarkdownVaultItem(path) {
		target = strings.TrimSuffix(target, filepath.Ext(target))
		target = strings.ReplaceAll(target, "|", "\\|")
		target = strings.ReplaceAll(target, "]", "\\]")
		e.insertText("[[" + target + "]]")
		return
	}
	// Non-Markdown files cannot resolve through the note index. Use a normal
	// Markdown link relative to this note so Muse can open the actual file.
	linkTarget := path
	if path != "" && e.opts.BaseDir != "" {
		if rel, err := filepath.Rel(e.opts.BaseDir, path); err == nil {
			linkTarget = filepath.ToSlash(rel)
		}
	} else if path != "" && e.opts.AssetRoot != "" {
		if rel, err := filepath.Rel(e.opts.AssetRoot, path); err == nil {
			linkTarget = filepath.ToSlash(rel)
		}
	}
	label := filepath.Base(target)
	label = strings.ReplaceAll(label, "[", "\\[")
	label = strings.ReplaceAll(label, "]", "\\]")
	e.insertText("[" + label + "](" + linkDest(filepath.ToSlash(linkTarget)) + ")")
}

func isMarkdownVaultItem(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".md", ".markdown", ".mdx":
		return true
	default:
		return false
	}
}

// laid is what the last layout was made of: the layout is done again only
// when it changes.
type laid struct {
	rev                 uint64
	searchRev           uint64
	folds               uint64
	inclusive           bool
	linewise            bool
	caret               int
	anchor              int
	reveal              int
	headingMarkerReveal bool
	compose             string
	colX, colW          float32
	look                lookKey
	assets              uint64
	blockRev            uint64
	phantom             *phantom
	focused             bool
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
	coverageTop, coverageBottom := e.viewTop, e.viewBottom
	// The viewport is what the layout shapes around. Both the key below and
	// the layout pass read it from the scroll position of this moment, so
	// that a scroll made during a frame is laid out for where it landed,
	// not where it was when the frame began.
	if e.viewH > 0 {
		band := layout.VirtualBand(e.scroll.Y, e.viewH, e.laidKey.scrollY, e.leavesLaid)
		e.viewTop, e.viewBottom = band.Top, band.Bottom
	}
	virtual := e.virtualLayout()
	searchRev := uint64(0)
	if !virtual {
		// Small whole-document layouts are cheap to refresh as a unit. Large
		// virtual views instead detect stale source views in the viewport and
		// leave offscreen search results for the next time they are shaped.
		searchRev = e.searchRev
	}
	blockRev := uint64(0)
	if e.block != nil {
		blockRev = e.block.revision
	}
	k := laid{
		rev:                 e.Session.Revision(),
		searchRev:           searchRev,
		folds:               e.foldRevision,
		inclusive:           e.Session.Selection().Inclusive,
		linewise:            e.Session.Selection().Linewise,
		caret:               e.Caret(),
		anchor:              e.Session.Selection().Anchor,
		reveal:              e.revealCaret(),
		headingMarkerReveal: e.opts.HeadingMarkerReveal,
		compose:             e.compositionText(),
		colX:                e.colX,
		colW:                e.colW,
		look:                e.lk.key(),
		assets:              e.assetGen.Load(),
		blockRev:            blockRev,
		phantom:             e.phantom,
		focused:             e.focused,
	}
	if virtual {
		k.scrollY, k.viewH = e.scroll.Y, e.viewH
	}
	// A prior height correction can expose estimated paragraphs while the
	// viewport remains inside the same cached scroll band. Check this invariant
	// before the same-key fast return so those paragraphs are shaped before the
	// draw callback consumes the current frame.
	if virtual && e.leavesLaid && k == e.laidKey && e.visibleLeafNeedsShape() {
		if e.layoutVirtualBand() {
			return
		}
		e.leavesLaid = false
	}
	// A view can be released while its leaf is offscreen, then the user can
	// scroll back into it without leaving the cached layout band. Check the
	// actual viewport before either same-key or buffered-band reuse can preserve
	// an unshaped visible leaf (including text views pruned while offscreen).
	if virtual && e.leavesLaid && e.visibleLeafNeedsShape() {
		if e.layoutVirtualBand() {
			e.laidKey = k
			return
		}
		e.leavesLaid = false
	}
	if k == e.laidKey && e.leavesLaid {
		return
	}
	if virtual && e.leavesLaid {
		// A virtual layout shapes leaves around the viewport, leaving a buffer
		// above and below the screen. While the new viewport stays inside that
		// already-shaped band, the scroll position does not affect leaf heights
		// or their y coordinates, so avoid another full-document layout pass.
		// Any non-scroll input changes a different part of laid and falls back
		// to layoutDoc below.
		previous := e.laidKey
		withoutScroll := k
		withoutScroll.scrollY = previous.scrollY
		if withoutScroll == previous && e.scroll.Y >= coverageTop && e.scroll.Y+e.viewH <= coverageBottom {
			e.laidKey.scrollY = e.scroll.Y
			e.viewTop, e.viewBottom = coverageTop, coverageBottom
			return
		}
		if withoutScroll == previous && e.layoutVirtualBand() {
			e.laidKey = k
			return
		}
	}
	e.laidKey = k
	e.leavesLaid = true
	if e.colW <= 0 {
		e.colW = 720
	}
	e.layoutDoc(e.colX, e.colW, virtual)
}

// visibleLeafNeedsShape reports when virtual-band reuse would leave a visible
// renderable leaf with no view to paint. Images outside the viewport and stale
// text views may both be released while their leaves remain in the buffered
// layout band, so that band's coverage alone is not enough to reuse it.
func (e *Editor) visibleLeafNeedsShape() bool {
	if e.viewH <= 0 || len(e.visibleLeaves) == 0 {
		return false
	}
	top, bottom := e.scroll.Y, e.scroll.Y+e.viewH
	for _, l := range e.paintVisibleSpan(top, bottom) {
		needsShape := l.view == nil || e.sourceRevealStale(l)
		if l.kind == kTable {
			needsShape = e.sourceRevealStale(l)
		}
		if needsShape && l.y+l.h > top && l.y < bottom {
			switch l.kind {
			case kPara, kHeading, kCode, kMath, kHTML, kRaw, kTable:
				return true
			}
		}
	}
	return false
}

// refreshVisibleSourceViews updates just the rendered leaves whose search
// match set changed. If their heights stay fixed, the document geometry stays
// valid; a changed height falls back to the normal layout pass below.
func (e *Editor) refreshVisibleSourceViews() (changed, stableGeometry bool) {
	stableGeometry = true
	for _, l := range e.paintVisibleSpan(e.scroll.Y, e.scroll.Y+e.viewH) {
		if !e.sourceRevealStale(l) {
			continue
		}
		changed = true
		if l.kind == kTable {
			return true, false
		}
		oldHeight := l.h
		v := e.view(l, l.w, false, syntax.IncrementalAlignNone)
		l.view, l.h = v, v.h
		l.horizontal.Offset = min(l.horizontal.Offset, max(blockContentWidth(l)-l.w, 0))
		if math.Abs(float64(v.h-oldHeight)) > 0.01 {
			return true, false
		}
	}
	return changed, stableGeometry
}

func (e *Editor) sourceRevealStale(l *leaf) bool {
	if l == nil {
		return false
	}
	if l.kind == kTable && l.tab != nil {
		for _, row := range l.tab.rows {
			for _, cell := range row {
				if cell.view == nil || e.sourceRevealStale(cell) {
					return true
				}
			}
		}
		return false
	}
	return l.view != nil && l.view.params.sourceReveal != e.sourceRevealFingerprint(l.lineStart, blockEnd(e.Source(), l))
}

// layoutVirtualBand shapes the visible band from cached source-order geometry.
// Height corrections update the Fenwick index, moving following leaves without
// a document-wide layout pass.
func (e *Editor) layoutVirtualBand() bool {
	if !e.visibleIndexReady || e.visibleIndexRev != e.Session.Revision() || e.visibleIndexFolds != e.foldRevision {
		return false
	}
	leaves := e.visibleLeaves
	if !e.virtualIndex.Ready(len(leaves)) {
		return false
	}
	s := e.lk.size
	// A correction can expose new estimated leaves at the bottom of the view.
	// Extend the band until all visible renderable leaves have a current view.
	for rounds := 0; ; rounds++ {
		if rounds > len(leaves) {
			return false
		}
		start, end := e.virtualIndex.VisibleRange(e.viewTop-s, e.viewBottom+s, func(i int) float32 {
			return max(leaves[i].h, s)
		})
		shaped := false
		for i := start; i < end; i++ {
			l := leaves[i]
			l.y = e.virtualY(i)
			stale := e.sourceRevealStale(l)
			if l.kind == kTable {
				if l.tab == nil || !stale {
					continue
				}
				oldHeight := l.h
				e.layoutTable(l)
				if delta := l.h - oldHeight; math.Abs(float64(delta)) > 0.01 {
					e.virtualAddHeight(i, delta)
					shaped = true
				}
				continue
			}
			if l.kind == kBlank || l.view != nil && !stale {
				continue
			}
			outsideViewport := l.y > e.scroll.Y+e.viewH || l.y+l.h < e.scroll.Y
			imageCandidate := l.view != nil && l.view.hasImageAsset() || leafMayContainImage(e.Source(), l)
			if imageCandidate && outsideViewport && !stale && !sourceRangeTargeted(e, l.lineStart, blockEnd(e.Source(), l), false) {
				continue
			}
			oldHeight := l.h
			v := e.view(l, l.w, false, syntax.IncrementalAlignNone)
			l.view, l.h = v, v.h
			shaped = true
			if delta := v.h - oldHeight; math.Abs(float64(delta)) > 0.01 {
				e.virtualAddHeight(i, delta)
			}
			l.horizontal.Offset = min(l.horizontal.Offset, max(blockContentWidth(l)-l.w, 0))
		}
		e.docH = e.virtualIndex.DocumentHeight()
		for i := start; i < end; i++ {
			leaves[i].y = e.virtualY(i)
		}
		if !e.visibleLeafNeedsShape() {
			break
		}
		if !shaped {
			return false
		}
	}
	// The caret may be outside the new viewport. Update its cached y so the
	// caret layer and a pending reveal use the corrected prefix height.
	if i := sort.Search(len(leaves), func(i int) bool { return leaves[i].end >= e.Caret() }); i < len(leaves) {
		leaves[i].y = e.virtualY(i)
	}
	return true
}

func (e *Editor) virtualY(i int) float32 {
	return e.virtualIndex.Y(i)
}

func (e *Editor) virtualYFor(l *leaf) float32 {
	if !e.virtualIndex.Ready(len(e.visibleLeaves)) || l == nil {
		if l == nil {
			return 0
		}
		return l.y
	}
	i := sort.Search(len(e.visibleLeaves), func(i int) bool {
		return e.visibleLeaves[i].lineStart >= l.lineStart
	})
	for ; i < len(e.visibleLeaves) && e.visibleLeaves[i].lineStart == l.lineStart; i++ {
		if e.visibleLeaves[i] == l {
			return e.virtualY(i)
		}
	}
	return l.y
}

func (e *Editor) virtualHeightPrefix(n int) float32 {
	return e.virtualIndex.Prefix(n)
}

func (e *Editor) virtualAddHeight(i int, delta float32) {
	e.virtualIndex.AddHeight(i, delta)
}

func (e *Editor) captureVirtualLayoutIndex() {
	n := len(e.visibleLeaves)
	e.virtualIndex.CaptureCount(n, e.docH)
	for i, l := range e.visibleLeaves {
		e.virtualIndex.SetBaseY(i, l.y)
	}
}

// virtualLayout reports whether the layout shapes only the leaves near
// the viewport. A small document is laid out whole: its layout is cheap,
// and exact positions make it simpler. A large one would pay for shaping
// every leaf of text it does not show, on the first frame as much as on
// any other.
func (e *Editor) virtualLayout() bool {
	if len(e.leaves) >= virtualLayoutMin {
		return true
	}
	// A medium note with images can cost more to shape than a much longer
	// prose note: each image view starts an asynchronous read/decode and keeps
	// its bitmap alive. Use the viewport layout for these notes too, so their
	// offscreen image leaves stay as estimates until the user reaches them.
	if len(e.leaves) < virtualImageLayoutMin {
		return false
	}
	for _, l := range e.leaves {
		if leafMayContainImage(e.Source(), l) {
			return true
		}
	}
	return false
}

// virtualLayoutMin is the number of leaves from which a layout is
// virtualized. A note of a few dozen paragraphs can already contain enough
// rich text, tables, images or formulas to make shaping the whole document
// on its first frame visible; 64 leaves keeps that work near the viewport.
const virtualLayoutMin = 64

// virtualImageLayoutMin starts viewport layout earlier for notes with images.
// Smaller image notes remain exact, while medium image notes avoid decoding
// every offscreen bitmap during their first layout.
const virtualImageLayoutMin = 16

// View builds the editor: a scroll view of the document as one surface
// the user writes on.
func (e *Editor) View(c *ui.Context) ui.Element {
	t := c.Theme()
	e.setLook(t)
	e.frame++
	e.services = c.Services()
	e.focused = e.handle.Focused(c)
	vimState := e.VimViewState()
	if vimState.Pending == "z" {
		remaining := 900*time.Millisecond - c.Now().Sub(vimState.PrefixStartedAt)
		if remaining <= 0 {
			e.vimController().ClearPending()
		} else {
			c.After(remaining)
		}
	}
	if !e.focused && e.dragging {
		e.dragging, e.frozen = false, false
	}

	b := e.scrollHandle.Bounds(c)
	e.viewport = media.Viewport{X: b.X, Y: b.Y, W: b.W, H: b.H}
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
	root := ui.Column(c).Fill()
	root.Children(func() {
		sc := ui.Scroll(c).Grow(1).MinHeight(0).Scrollbars(showBar).TrackScroll(&e.scroll).Bind(&e.scrollHandle)
		tablePointer := ui.Point{X: -1, Y: -1}
		sc.Children(func() {
			ui.Column(c).FillWidth().Height(h).Shrink(0).Children(func() {
				label := e.opts.Label
				if label == "" {
					label = locales.DocumentLabel()
				}
				// Cursor comes after HandleTextInput, which sets the text cursor
				// itself: the shape set last is the one the window shows, and the
				// editor changes it as the pointer moves over links and boxes.
				dragInsertOffset := -1
				drawEditor := func(p *ui.Painter, bounds ui.Rect) {
					e.paint(p, bounds)
					if dragInsertOffset >= 0 {
						e.paintVaultItemInsertion(p, dragInsertOffset)
					}
				}
				cursor := e.cursor
				if e.opts.HideMouseCursor {
					cursor = ui.CursorNone
				}
				el := ui.Box(c).Absolute().Top(0).Left(0).Right(0).Height(h).Focusable().FocusRing(false).Bind(&e.handle).
					Role(ui.RoleTextField).Label(label).
					HandleTextInput(e).Cursor(cursor).
					HandleInput(e.input).Draw(drawEditor).ContextMenu(e.menu)
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
				if _, over := ui.DragOver[dragpayload.VaultItem](el); over {
					el.Cursor(ui.CursorCopy)
					if x, y, inside := el.PointerPosition(); inside {
						dragInsertOffset = vaultItemInsertionOffset(true, x, y, e.hitTest)
					}
					if e.opts.OnVaultItemDragOver != nil {
						e.opts.OnVaultItemDragOver()
					}
				}
				if drop, ok := ui.Drop[dragpayload.VaultItem](el); ok {
					if dragInsertOffset >= 0 {
						e.setCaret(dragInsertOffset, false)
					} else if x, y, over := el.PointerPosition(); over {
						e.setCaret(e.hitTest(x, y), false)
					}
					e.dropVaultItem(drop)
				}
				if e.smoothCursorOverlayEnabled() {
					// Counter the Scroll content transform so this pass stays a
					// viewport-sized overlay, rather than invalidating the entire
					// document-height element on each animation frame.
					ui.Box(c).Absolute().Top(e.scroll.Y).Left(0).Width(e.elemW).Height(e.viewH).PassThrough().Draw(func(p *ui.Painter, bounds ui.Rect) {
						e.paintMu.Lock()
						defer e.paintMu.Unlock()
						e.paintCaretLayer(p, bounds.X, bounds.Y-e.scroll.Y)
					})
				}
				if e.opts.Header != nil && e.opts.HeaderHeight > 0 {
					ui.Box(c).Height(max(e.opts.TopInset, 0)).Shrink(0)
					ui.Column(c).Key("document-header").ZIndex(1).Margin(0, e.colX).Width(e.colW).Height(e.opts.HeaderHeight).Shrink(0).Children(func() { e.opts.Header(c) })
				}

			})
		})
		e.modalCommandLineView(c)
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
		tableUI := e.tableUI()
		tableFrame := tableUI.Frame()
		tableUI.EdgesView(c, b, tablePointer, tableFrame, e.lk.dark)
		tableUI.SelectionHandles(c, b, tablePointer, tableFrame, e.lk.size, e.lk.accent, e.lk.dark)
		tableUI.CellMenus(c, b, tablePointer, tableFrame, e.lk.dark)
		e.wikiSuggestionsView(c, b)
		if _, active := e.searchMatchPulse(c.Now()); active {
			c.After(16 * time.Millisecond)
		}
		if _, active := e.vimYankFlashOpacity(c.Now()); active {
			c.After(16 * time.Millisecond)
		}
		if e.focused && !e.smoothCursorOverlayEnabled() {
			phase := c.Now().Sub(e.blinkStart)
			if phase < 30*time.Second {
				c.After(feedback.CaretBlinkPeriod - phase%feedback.CaretBlinkPeriod)
			}
		}
		if e.elemW <= 0 {
			// The first frame has no size yet: lay out again at the next.
			c.After(time.Millisecond)
		}
	})
	return root
}

// vaultItemInsertionOffset intentionally returns no value outside the active
// target; the view recomputes it each frame so leave/cancel cannot retain a
// stale preview position.
func vaultItemInsertionOffset(over bool, x, y float32, hitTest func(float32, float32) int) int {
	if !over || hitTest == nil {
		return -1
	}
	return hitTest(x, y)
}

// paintVaultItemInsertion marks the exact source boundary used by the
// in-process tree item drop. The offset is frame-local, so leaving or ending
// the drag removes the marker without changing document or editor state.
func (e *Editor) paintVaultItemInsertion(p *ui.Painter, off int) {
	e.paintMu.Lock()
	defer e.paintMu.Unlock()
	r, ok := e.vaultItemInsertionRect(off)
	if !ok {
		return
	}
	render.PaintOverlay(p, render.Overlay{Kind: render.FillOverlay, Bounds: r, Color: e.lk.caret.Alpha(.86), Radius: 1})
}

func (e *Editor) vaultItemInsertionRect(off int) (ui.Rect, bool) {
	r, ok := e.caretRect(off)
	if !ok {
		return ui.Rect{}, false
	}
	// caretRect is editor-local; paint callbacks receive the editor's bounds
	// in the outer canvas. Scroll is already reflected by caretRect's layout.
	return ui.Rect{X: e.elemRect.X + r.X - 1, Y: e.elemRect.Y + r.Y, W: 2, H: max(r.H, 12)}, true
}

// advanceVisibleImageAssets advances animated assets on the UI thread and
// schedules the next repaint. Nested wiki embeds share the same frame clock.
func (e *Editor) advanceVisibleImageAssets(c *ui.Context) {
	now := c.Now()
	var nextFrame time.Duration
	seen := make(map[*images.Asset]struct{})
	var visit func(*Editor)
	visit = func(editor *Editor) {
		if editor == nil {
			return
		}
		layoutChanged := false
		visibleTop := editor.scroll.Y - 48
		visibleBottom := editor.scroll.Y + editor.viewH + 48
		advance := func(o *object) {
			if o == nil {
				return
			}
			if o.asset != nil {
				hadObjectBitmap := o.bmp != nil
				if _, ok := seen[o.asset]; !ok {
					seen[o.asset] = struct{}{}
					next, _ := o.asset.Advance(now)
					if next > 0 && (nextFrame == 0 || next < nextFrame) {
						nextFrame = next
					}
				}
				o.refreshAssetBitmap()
				if !hadObjectBitmap && o.bmp != nil {
					layoutChanged = true
				}
			}
			if o.embed != nil {
				visit(o.embed)
			}
		}
		for _, leaf := range editor.paintVisibleSpan(visibleTop, visibleBottom) {
			if leaf.view == nil {
				continue
			}
			advance(leaf.view.block)
			for i := range leaf.view.disp.pieces {
				advance(leaf.view.disp.pieces[i].obj)
			}
		}
		if layoutChanged {
			editor.assetGen.Add(1)
			// Embedded editors share the host's frame loop but keep their own
			// layout key. Route the invalidation through the editor that changed
			// so its host can include the new measured height in its layout key.
			editor.invalidate()
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
			target := e.pendingScrollTarget(r)
			if e.pageJump {
				e.scrollToTarget(target)
			} else {
				e.scroll.Y = target
			}
		}
		e.pageJump = false
		e.scrollCenter = false
		e.scrollPlacement = 0
		e.pageJumpAlign = layout.PageJumpNearest
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

func (e *Editor) pendingScrollTarget(r ui.Rect) float32 {
	maxY := max(e.docH-e.viewH, 0)
	if e.pageJump {
		var target float32
		switch e.pageJumpAlign {
		case layout.PageJumpTopMargin:
			target = r.Y - e.lk.size*2
		case layout.PageJumpCenter:
			target = r.Y - (e.viewH-r.H)/2
		case layout.PageJumpTop:
			target = r.Y
		case layout.PageJumpBottom:
			target = r.Y + r.H - e.viewH
		case layout.PageJumpNearest:
			topMargin := max(min(e.lk.size*3, e.viewH/4), e.opts.TopInset+8)
			bottomMargin := min(e.lk.size*3, e.viewH/4)
			target = e.scroll.Y
			if r.Y < e.scroll.Y+topMargin {
				target = r.Y - topMargin
			} else if r.Y+r.H > e.scroll.Y+e.viewH-bottomMargin {
				target = r.Y + r.H - e.viewH + bottomMargin
			}
		default:
			target = r.Y - e.lk.size*2
		}
		return min(max(target, 0), maxY)
	}
	switch e.scrollPlacement {
	case input.CaretLineTop:
		return min(max(r.Y, 0), maxY)
	case input.CaretLineCenter:
		return min(max(r.Y-(e.viewH-r.H)/2, 0), maxY)
	case input.CaretLineBottom:
		return min(max(r.Y+r.H-e.viewH, 0), maxY)
	default:
		if e.scrollCenter {
			return min(max(r.Y-(e.viewH-r.H)/2, 0), maxY)
		}
		return min(max(r.Y-e.lk.size*2, 0), maxY)
	}
}

// Small moves stay immediate. Longer distances add duration logarithmically,
// so an extreme jump is still quick instead of scaling with document length.
const (
	pageJumpMinDistanceInViews = float32(0.25)
	pageJumpMinDistance        = float32(120)
	pageJumpMaxDuration        = 400 * time.Millisecond
	// A virtual layout still walks its leaves to refresh absolute geometry at
	// each scroll position. Bound that repeated work for very large notes.
	pageJumpMaxAnimatedLeaves = 4096
)

func pageJumpDuration(distance, viewH float32) time.Duration {
	threshold := max(viewH*pageJumpMinDistanceInViews, pageJumpMinDistance)
	if distance <= threshold {
		return 0
	}
	extra := math.Log2(float64(distance / threshold))
	d := 170*time.Millisecond + time.Duration(extra*55*float64(time.Millisecond))
	return min(d, pageJumpMaxDuration)
}

func pageJumpAnimationDuration(distance, viewH float32, leaves int, enabled bool) time.Duration {
	if !enabled || leaves > pageJumpMaxAnimatedLeaves {
		return 0
	}
	return pageJumpDuration(distance, viewH)
}

func (e *Editor) scrollToTarget(target float32) {
	distance := float32(math.Abs(float64(target - e.scroll.Y)))
	duration := pageJumpAnimationDuration(distance, e.viewH, len(e.leaves), e.opts.SmoothPageJumps)
	if duration > 0 {
		e.scroll.ScrollTo(e.scroll.X, target, ui.Animation{Duration: duration, Ease: ui.EaseInOut})
		return
	}
	if e.scroll.Scrolling() {
		// Retarget an in-flight page glide with a one-tick linear transition;
		// ScrollState has no separate cancel operation.
		e.scroll.ScrollTo(e.scroll.X, target, ui.Animation{Duration: time.Nanosecond, Ease: ui.Linear})
	} else {
		e.scroll.Y = target
	}
}

// textData returns text as clipboard data.
func textData(s string) transfer.Data { return transfer.TextData(s) }
