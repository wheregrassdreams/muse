package editor

import (
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/egoist/mygo/transfer"
	"github.com/egoist/mygo/ui"
	uieditor "github.com/egoist/mygo/ui/editor"

	editormodel "muse/editor"
	inlineblock "muse/editor/blocks/inline"
	paragraph "muse/editor/blocks/paragraph"
	tablemodel "muse/editor/blocks/table"
	"muse/editor/input"
	"muse/editor/input/vim"
	layoutindex "muse/editor/layout"
	textnav "muse/editor/navigation"
	"muse/editor/source"
	"muse/markdown/diagrams"
	"muse/markdown/images"
	texmath "muse/markdown/math"
	"muse/markdown/syntax"
	inlineui "muse/nativeui/editor/blocks/inline"
	media "muse/nativeui/editor/blocks/media"
	tableui "muse/nativeui/editor/blocks/table"
	"muse/nativeui/editor/feedback"
	"muse/nativeui/editor/ime"
	editornavigation "muse/nativeui/editor/navigation"
	"muse/nativeui/editor/navigationrail"
	editorprojection "muse/nativeui/editor/projection"
	"muse/nativeui/editor/render"
	"muse/nativeui/editor/textlayout"
	"muse/ui/scrollbar"
	vaultlinks "muse/vault/links"
)

// CodeLanguageFor adapts a document path to MyGo's syntax-highlighting
// language identifier. Markdown and extensionless note names use the rich
// Markdown surface and therefore return an empty language.
func CodeLanguageFor(path string) string {
	if path == "" {
		return ""
	}
	base := filepath.Base(path)
	ext := strings.ToLower(filepath.Ext(base))
	switch ext {
	case ".md", ".markdown", ".mdx":
		return ""
	}
	if language := uieditor.LanguageFor(base); language != nil {
		if strings.EqualFold(language.Name, "markdown") {
			return ""
		}
		return language.Name
	}
	if ext == "" {
		return ""
	}
	return "text"
}

const invalidParsedRevision = ^uint64(0)

var nextEditorScopeID atomic.Uint64

// Options configure an editor. The app sets them before building the
// editor and changes them as its settings change.
type Options struct {
	// FontSize is the body text's size in DIPs (16 when 0), LineHeight its
	// line height as a multiple of it (1.75 when 0).
	FontSize   float32
	LineHeight float32
	// DisableSmartTypography opts out of bounded Knuth-Plass paragraph layout.
	// The zero value enables it for prose; code, headings and tables stay native.
	DisableSmartTypography bool
	// SoftBreaksAsSpaces projects ordinary Markdown newlines as spaces.
	// Explicit hard breaks and blank-line paragraph boundaries remain visible.
	SoftBreaksAsSpaces bool
	// PreserveTypedLineBreaks retains user-created source line boundaries in
	// this editing session. Focus/caret changes never toggle paragraph wrapping.
	PreserveTypedLineBreaks bool
	// VimVirtualImageRows exposes an image's visual height as navigation rows
	// while keeping every row mapped to that image's single source line.
	VimVirtualImageRows bool
	// HeadingMarkerReveal reveals a heading's Markdown prefix when the caret
	// moves left from the first heading character into its source prefix.
	HeadingMarkerReveal bool
	// CursorFade softens the caret's blink transitions. SmoothCursor animates
	// visual caret movement in Vim and Helix normal mode.
	CursorFade   bool
	SmoothCursor bool
	// HideMouseCursor hides the native pointer while the editor is receiving
	// keyboard input; the app restores it after the idle interval.
	HideMouseCursor bool
	// SmoothPageJumps animates long, explicit in-document navigation jumps.
	// Ordinary caret movement and small scroll adjustments remain immediate.
	SmoothPageJumps bool
	// SearchMatchPulse briefly emphasizes a newly selected current match.
	SearchMatchPulse bool
	// VimYankFlash briefly marks ranges yanked through Vim commands.
	VimYankFlash bool
	// Font and MonoFont are the families of text and of code: the
	// theme's font and the system's monospaced one when empty.
	Font, MonoFont string
	// MaxWidth caps the column of text (820 when 0); PadX is the least room
	// beside it.
	MaxWidth float32
	PadX     float32
	// TopInset reserves space for an overlaid header, in DIPs.
	TopInset float32
	// HideFrontmatter omits the YAML frontmatter leaf from the layout while
	// keeping it in the source, parser offsets, and undo history.
	HideFrontmatter bool
	// Header is an optional document header that scrolls with the body.
	Header       func(*ui.Context)
	HeaderHeight float32
	// BaseDir resolves the relative paths of images.
	BaseDir   string
	AssetRoot string
	// ImageCacheStore is an application supplied store for remote image bytes.
	ImageCacheStore images.CacheStore
	// ImageLocalFiles is the filesystem capability used by local image sources.
	ImageLocalFiles images.LocalFiles
	// ResolveImage uses the application's shared vault path catalog when set.
	ResolveImage func(src, base, root string) string
	// Placeholder shows in an empty document.
	Placeholder string
	// Label names the editor to assistive technology.
	Label string
	// Invalidate asks the window for a frame, from any goroutine, as an
	// image loaded in the background does.
	Invalidate func()
	// OnChange is called after every change of the source, the user's and
	// the app's.
	OnChange func()
	// OnClose runs once when the editor surface releases its UI resources.
	OnClose func()
	// OnLink opens a link the user Cmd-clicked (Ctrl-clicked).
	OnLink func(url string)
	// WikiLinkSuggestions returns vault-relative targets for the partial
	// text after [[. The editor owns filtering presentation and selection.
	WikiLinkSuggestions func(query string) []string
	// WikiLinkSuggestionDetails optionally adds display titles and aliases while
	// keeping Target as the stable text inserted into the source.
	WikiLinkSuggestionDetails func(query string) []vaultlinks.Suggestion
	// OnWikiLinkHover reports a wiki target, pointer coordinates and the visible
	// link fragment's bounds in editor-local coordinates. An empty target ends
	// the hover. Embedded links are excluded.
	OnWikiLinkHover func(target string, x, y float32, linkBounds ui.Rect)
	// ResolveWikiEmbed loads a target note using the source document's path as
	// its resolution context. The returned path must identify that note.
	ResolveWikiEmbed func(currentPath, target string) (path, source string, ok bool)
	// CurrentPath is the absolute path of this editor's Markdown source.
	CurrentPath string
	// ResolveWikiLink reports the current index status of a wiki link target.
	// A nil callback or an unavailable index means inlineui.LinkTargetUnknown.
	ResolveWikiLink func(currentPath, target string) inlineui.LinkTargetState
	// LinkRevision changes when the target index changes; it refreshes cached link styles.
	LinkRevision uint64
	// LinkContext identifies the current document/workspace context for relative wiki resolution.
	LinkContext string
	// OnImagePreview opens an image preview for its resolved local path or URL.
	OnImagePreview func(pathOrURL string)
	// OnDiagramPreview opens the selected Mermaid block in an app-owned
	// surface. Result is a detached backend-neutral diagram snapshot.
	OnDiagramPreview func(source string, result diagrams.Result)
	// Code makes the document code rather than Markdown, as a .json or a
	// .go file is: its lines show as they are, highlighted as the language
	// of that name, or plain when the name is "text".
	Code string
	// SaveImage stores an image pasted into the document and returns the
	// path a Markdown image links it by; nil keeps pasted images out.
	SaveImage func(data []byte, ext string) (string, error)
	// OnDropFiles takes the files dropped on the editor that are not
	// images, as a document to open.
	OnDropFiles func(paths []string)
	// OnVaultItemDragOver reports that an in-process tree item is over the editor.
	OnVaultItemDragOver func()
	// ReadClipboard and WriteClipboard replace the system's clipboard, as
	// tests do.
	ReadClipboard  func() transfer.Data
	WriteClipboard func(transfer.Data)
}

// MathRenderer is an optional host painter for a math raster. It allows an
// embedding surface to share its own math visual cache with the editor.
type MathRenderer func(p *ui.Painter, rect ui.Rect, raster *texmath.Raster, dark bool, font string)

// Editor is the state of a Markdown editor, kept by the app across frames:
// the source and its parse, the selection, the history of undo and what
// the editor laid out. View builds it.
type Editor struct {
	// Session is the single owner of source, revision, selection and undo state.
	// Presentation caches below derive from its source revision.
	*editormodel.Session
	// uiScopeID gives keyed child controls a stable scalar namespace without
	// retaining the Editor through toolkit-owned keys.
	uiScopeID uint64
	// paintMu keeps a frame's leaves and parser snapshot together. MyGo calls
	// Draw after the app's View callback returns, so an app edit can otherwise
	// replace the parser while paint is still walking the previous leaves.
	paintMu        paintMutex
	changePending  atomic.Bool
	changeMu       sync.Mutex
	pendingChanges []editormodel.Change
	editBatch      *editormodel.EditBatch
	modal          modalState
	vimState       *vim.Controller
	// folded is presentation state keyed by a heading's source line start.
	// A source edit clears it so offsets can never fold an unrelated section.
	folded                 map[int]bool
	foldRevision           uint64
	foldCacheRevision      uint64
	foldSectionsCache      []foldSection
	opts                   Options
	mathRenderer           MathRenderer
	viewport               media.Viewport
	inlineMath             inlineblock.MathSession
	closed                 bool
	diagrams               []diagramPainterCacheEntry
	diagramClock           uint64
	rail                   *navigationrail.Handle
	typedBreaks            paragraph.Breaks
	preserveInsertedBreaks bool

	leaves []*leaf
	// visibleLeaves is the layout-ordered subset of leaves that participate in
	// document geometry. Hidden folded leaves stay in leaves for source access,
	// but are excluded here so painting can binary-search the viewport.
	visibleLeaves     []*leaf
	visibleIndexRev   uint64
	visibleIndexFolds uint64
	visibleIndexReady bool
	virtualIndex      layoutindex.VirtualIndex
	// p is what the parser reads, and what the editor derives from the
	// source for it: the document, the masked source it holds, the comments
	// that mask hides, where the front matter ends. Only p.set and p.edit
	// write it (see parsed.go); everything else reads it.
	p *source.Document
	// largeDataImageCache keeps a source-backed synthetic inline projection for
	// one parsed block. The source module owns its revision lifetime.
	largeDataImageCache source.ImageInlineCache
	// arena holds the leaves of the parse before and the one being built:
	// a leaf the edit did not change keeps its view, so the leaves of the
	// parse before have to stay whole while the next is built.
	arena              [2]leafArena
	arenaCur           int
	documentProjection editorprojection.Projector
	// leafSrc is the source the leaves were built from, which an edit has
	// usually moved on from by the time they are compared with the next.
	leafSrc string
	// leafComment is the comment the leaves were built for: a comment's own
	// leaves are dropped from the layout unless the caret is in it, and the
	// caret moving in or out rebuilds the leaves (never the parse).
	leafComment   source.Comment
	leafCommentOK bool
	// pending is the one source edit not yet given to the parser;
	// pendingMulti says a committed step changed more than one range, which
	// has to be replayed from the whole source.
	pending      *pendingEdit
	pendingMulti bool
	// parsed is the source revision the leaves reflect.
	parsed uint64
	// parserRevision is the source revision currently held by p. It is advanced
	// from committed text-engine deltas, independently of layout's parsed rev.
	parserRevision uint64

	smoothCursorNavigation bool
	feedbackState          feedback.Handle
	vimImageRows           media.VirtualImageRows
	block                  *blockSelection
	blockAnchorOffset      int
	blockInput             *blockInput
	graphemeCache          textnav.GraphemeBoundaryCache
	wordIndexCache         editornavigation.WordIndexCache
	// tableSelection owns table-cell presentation. anchor/caret retain the
	// document range, whose outside-table text and table structure are projected
	// independently.
	tableController tablemodel.Controller
	tableNavigating bool
	tableSelection  *tableSelection
	// tableDocumentRegion preserves the 2D cell focus when an extended
	// selection leaves its table; anchor/caret alone cannot encode a rectangle.
	tableDocumentRegion *tableSelection
	tableVirtualEdit    *tableVirtualEditState
	tableHandleDrag     *tableHandleDrag
	tableBridge         *tableContentBridge
	// phantom is the empty paragraph the caret is in, if it is in one.
	phantom *phantom
	// goalX is the x a run of Up and Down keeps the caret at.
	goalX   float32
	hasGoal bool
	// upstream draws the caret at the end of the line before when it sits
	// where a line wraps.
	upstream bool

	lk              look
	views           map[viewKey]*leafView
	dataImageAssets media.DataImageAssets
	typographyCache textlayout.Cache
	frame           uint64
	renderScratch   editorRenderScratch
	renderer        render.Renderer
	assetGen        atomic.Uint64
	imageHits       []imageHit
	imageDrag       *imageResizeDrag
	// imageAlignments stores per-image presentation overrides by source offset.
	// Standalone images default to centered; false is an explicit left alignment.
	imageAlignments media.ImageAlignmentState

	// The element's size and where the column of text is in it.
	elemW, viewH float32
	colX, colW   float32
	padTop       float32
	padBottom    float32
	docH         float32
	// viewTop and viewBottom bound the shaped band in content coordinates:
	// a large document shapes only leaves near them (see layoutDoc). They
	// remain fixed while the viewport scrolls inside this band.
	viewTop, viewBottom float32
	// estW, estSize and estLH are the width and text size the heights of
	// the leaves that are not shaped were last estimated at. A layout at
	// another width or size measures them again: their text wraps
	// differently, and what is below them would sit where the old look put
	// it.
	estW, estSize, estLH float32

	scroll          ui.ScrollState
	scrollIndicator scrollbar.Indicator
	scrollHandle    ui.Handle
	handle          ui.Handle
	focused         bool
	blinkStart      time.Time
	// wantReveal scrolls the caret into view at the next frame.
	wantReveal bool
	// scrollTo, when set, scrolls an offset to the top of the view at the
	// next frame; scrollCenter centers it there instead, as the rail does.
	scrollTo        int
	hasScrollTo     bool
	scrollCenter    bool
	scrollPlacement input.CaretLinePlacement
	pageJump        bool
	pageJumpAlign   layoutindex.PageJumpAlignment

	// The pointer: a drag selecting by characters, words or blocks, from
	// the range it started on; the syntax shown is frozen meanwhile, so
	// that the text does not move under it.
	dragging bool
	dragUnit int
	dragFrom [2]int
	// tableCellDrag is transient pointer gesture state. The active region itself
	// remains owned by tableSelection and this capture never enters history.
	tableCellDrag        *tableCellDragState
	frozen               bool
	frozenAt             int
	pointerX             float32
	pointerY             float32
	contextImagePath     string
	contextDiagramSource string
	hoverLink            Range
	// diagDrag is a diagram's scroll bar being dragged, if one is; diagPan
	// is the pointer dragging a rendered diagram itself sideways.
	diagDrag       *diagDrag
	diagPan        *diagPan
	horizontalDrag *blockHorizontalDrag
	services       ui.Services
	lastChange     time.Time

	// highlights are ranges of the source drawn as found, as a search's
	// matches; current is the one chosen.
	highlights []Range
	current    int
	// sourceRevealRanges is a sorted, merged index of the active search
	// highlights. Virtual layout asks whether many small syntax ranges overlap
	// a match, so keeping this index avoids rescanning every match per leaf.
	sourceRevealRanges []Range
	searchRev          uint64
	// Vim search highlights share the same painter but retain separate
	// ownership, so cancelling a Vim search cannot erase Cmd+F results.
	vimHighlights       []Range
	vimCurrent          int
	vimHighlightsActive bool
	// elemRect is the element's box in the window, as the last frame drew it.
	elemRect ui.Rect
	// cursor is the pointer's shape where it rests.
	cursor ui.Cursor

	// phantomStays keeps the phantom an edit made through its commit.
	phantomStays bool
	imeClient    *ime.Client
	laidKey      laid
	leavesLaid   bool
	// unitList is the leaves and table cells the caret moves through,
	// built again when the source changes (see units).
	unitList       []*leaf
	unitRev        uint64
	unitIndexCache struct {
		rev, folds uint64
		off        int
		first      *leaf
		length     int
		index      int
		valid      bool
	}
	// tableEdgeIndex is rebuilt with the parse and lets caret geometry query
	// table boundary stops without scanning every table on each frame.
	tableEdgeIndex    []tableEdgePosition
	tableEdgeIndexRev uint64
	tableEdgeIndexed  bool

	// The navigation rail's heading list, plus the next content block for
	// each preview. Pointer, animation and paint state live in nativeui.
	railLeaves   []*leaf
	railNext     []*leaf
	railRev      uint64
	railReady    bool
	railItems    []navigationrail.Item
	railItemsRev uint64
	// hoverBlock is the start of the block the pointer is on, or -1: a
	// block shows its source control there (see sourceChip).
	hoverBlock int
	// foldHover is the source offset of the heading under the pointer, or -1.
	// Its control is shown only while this heading owns the pointer.
	foldHover       int
	wikiPopup       inlineui.Popup
	wikiHover       string
	wikiHoverBounds ui.Rect
	wikiEmbeds      map[string]*wikiEmbedCache
	wikiEmbedStack  []string
	// sourceAt is the block the source control was last used on, while it
	// shows its Markdown source instead of the block it draws, and sourceOK
	// says it does. The caret leaving the block stops it.
	sourceAt    int
	sourceOK    bool
	railText    string
	railDesc    string
	railTextRev uint64
	railTextIdx int
}

// wordsForDisplay keeps a small feature-owned cache of immutable navigation
// indexes. The index semantics themselves live in the navigation subsystem.
func (e *Editor) wordsForDisplay(text string) *editornavigation.WordIndex {
	return e.wordIndexCache.Get(text)
}

// SetMathRenderer installs the presentation adapter used to paint math
// rasters. Pass nil when the host releases the editor.
func (e *Editor) SetMathRenderer(renderer MathRenderer) {
	if e == nil {
		return
	}
	e.paintMu.Lock()
	e.mathRenderer = renderer
	e.paintMu.Unlock()
}

// Range is a range of the source, in bytes.
type Range struct{ Start, End int }

// pendingEdit is a change of the source not yet given to the parser: the
// bytes from to were replaced by text.
type pendingEdit struct {
	from, to int
	text     string
}

// diagDrag is the dragging of a diagram's scroll bar: the leaf it belongs
// to, the pointer's x as the drag began, and the scroll offset then.
type diagDrag struct {
	leaf  *leaf
	start float32
	from  float32
}

// diagPan is the dragging of a rendered diagram itself: the leaf, the
// pointer's x as the drag began, and the scroll offset then.
type diagPan struct {
	leaf  *leaf
	start float32
	from  float32
}

// phantom is an empty paragraph the caret is in: Markdown has no empty
// paragraphs, so it is not in the source until text is typed in it, which
// goes in at at with pre before it and post after it.
type phantom struct {
	at        int
	pre, post string
	conts     []cont
	// before places it above the leaf starting at at, rather than below
	// the one ending there.
	before bool
}

// New returns an editor of src with a new session.
func New(src string, opts Options) *Editor {
	e := NewFromSession(editormodel.NewSession(src), opts)
	// Preserve the historical constructor behavior: a newly opened document
	// starts at its first editable stop (for example, after a list marker),
	// while NewFromSession keeps the caller's explicit restored selection.
	e.setSessionHead(e.firstStop())
	e.setSessionAnchor(e.Caret())
	return e
}

// NewFromSession attaches the production renderer/input adapter to the
// caller's authoritative session. It does not create a second source,
// selection, or history owner.
func NewFromSession(session *editormodel.Session, opts Options) *Editor {
	if session == nil {
		session = editormodel.NewSession("")
	}
	e := &Editor{Session: session, opts: opts, uiScopeID: nextEditorScopeID.Add(1), views: map[viewKey]*leafView{}, current: -1, cursor: ui.CursorText,
		hoverBlock: -1, foldHover: -1, rail: navigationrail.NewHandle()}
	e.paintMu.owner = e
	e.imeClient = ime.NewClient(e, e)
	e.p = source.New(exts, source.ParserText)
	e.bindSessionAdapters()
	e.Session.BindHistoryParticipant(e)
	// The window's theme comes with the first frame; until then, the light
	// one, so that a document opened before it lays out.
	e.setLook(ui.LightTheme())
	e.resetParserForSource(session.Source())
	e.sync()
	if session.Source() == "" {
		e.phantom = &phantom{at: 0}
		e.sync()
	}
	return e
}

// SetOptions changes the editor's options.
func (e *Editor) SetOptions(opts Options) {
	e.paintMu.Lock()
	defer e.paintMu.Unlock()
	if !opts.SmoothCursor {
		e.feedbackState.CancelCursor()
	}
	if !opts.SearchMatchPulse {
		e.feedbackState.CancelSearchPulse()
	}
	if !opts.VimYankFlash {
		e.feedbackState.CancelYankFlash()
	}
	if opts.DisableSmartTypography && !e.opts.DisableSmartTypography {
		e.typographyCache.Clear()
	}
	codeChanged := opts.Code != e.opts.Code
	frontmatterChanged := opts.HideFrontmatter != e.opts.HideFrontmatter
	imageBaseChanged := opts.BaseDir != e.opts.BaseDir || opts.AssetRoot != e.opts.AssetRoot
	linkRevisionChanged := opts.LinkRevision != e.opts.LinkRevision
	linkContextChanged := opts.LinkContext != e.opts.LinkContext
	currentPathChanged := opts.CurrentPath != e.opts.CurrentPath
	embedResolverChanged := (opts.ResolveWikiEmbed == nil) != (e.opts.ResolveWikiEmbed == nil)
	headingMarkerRevealChanged := opts.HeadingMarkerReveal != e.opts.HeadingMarkerReveal
	if codeChanged || frontmatterChanged {
		e.parsed = invalidParsedRevision
		if codeChanged {
			e.parsed = invalidParsedRevision
			e.pending, e.pendingMulti = nil, false
		}
	}
	if !opts.VimVirtualImageRows {
		e.clearVimImageRow()
	}
	e.opts = opts
	if imageBaseChanged {
		e.assetGen.Add(1)
	}
	if codeChanged || frontmatterChanged {
		// Keep leaves and parser paired when the parse mode or visibility
		// changes, including callers that inspect the editor before the next View.
		e.sync()
	}
	hideFMEnd := 0
	if opts.HideFrontmatter {
		hideFMEnd = hideFrontMatterEnd(e.Source())
	}
	if frontmatterChanged && hideFMEnd > 0 && e.Caret() < hideFMEnd {
		e.setSessionHead(e.firstStop())
		if e.Session.Selection().Anchor < hideFMEnd {
			e.setSessionAnchor(e.Caret())
		}
		e.hasGoal = false
		e.wantReveal = true
	}
	if linkContextChanged || currentPathChanged {
		e.wikiEmbeds = nil
	}
	if linkRevisionChanged || linkContextChanged || currentPathChanged || embedResolverChanged || headingMarkerRevealChanged {
		e.leavesLaid = false
		e.invalidate()
	}
}

// Options returns the editor's options.
func (e *Editor) Options() Options { return e.opts }

// actCaretStill runs an action that is not the text — a control on a block,
// a box of a task — without asking the view to follow the caret: the person
// is looking at what they clicked, not at where the caret was left.
func (e *Editor) actCaretStill(f func()) {
	want, goal, hasGoal := e.wantReveal, e.goalX, e.hasGoal
	f()
	e.wantReveal, e.goalX, e.hasGoal = want, goal, hasGoal
}

// Source returns the Markdown being edited.
func (e *Editor) Source() string { return e.Session.Source() }

// SetSource replaces the document, as opening another file does: the
// caret goes to its start and the history of undo is cleared.
func (e *Editor) SetSource(src string) {
	e.ResetForDocumentOpen()
	e.Session.ReplaceSource(src)
	e.setSessionSelection(0, 0)
	e.sync()
	if src == "" {
		// The empty document is the one line the source does not hold: a
		// line break is content, and gives the document its lines.
		e.phantom = &phantom{at: 0}
		e.sync()
	}
	e.setSessionHead(e.firstStop())
	e.setSessionAnchor(e.Caret())
	for _, l := range e.leaves {
		l.horizontal = blockHorizontalState{}
	}
}

// ResetForDocumentOpen releases editor-local state from the current document
// without changing the shared source session. The note surface calls it before
// replacing that session, so transient placeholders and document caches cannot
// leak across opens.
func (e *Editor) ResetForDocumentOpen() {
	e.finishBlockInput()
	if e.editBatch != nil {
		e.commit(e.Caret(), e.Caret())
	}
	e.paintMu.Lock()
	defer e.paintMu.Unlock()
	// Opening a document is a lifetime boundary. Incremental edits reuse these
	// arenas and caches, but carrying them across notes can retain the old
	// document's views, images, and peak-sized backing arrays indefinitely.
	e.leafSrc = ""
	e.leaves, e.visibleLeaves = nil, nil
	e.views = make(map[viewKey]*leafView)
	e.imageHits = nil
	e.wikiEmbeds = nil
	e.wikiPopup.Reset()
	e.wikiHover = ""
	e.wikiHoverBounds = ui.Rect{}
	e.railLeaves, e.railNext = nil, nil
	e.railText, e.railDesc = "", ""
	e.railTextRev, e.railTextIdx = 0, 0
	e.wordIndexCache.Reset()
	e.tableEdgeIndex = nil
	e.typographyCache.Clear()
	e.virtualIndex.Release()
	e.visibleIndexReady = false
	e.unitList = nil
	e.unitIndexCache.first, e.unitIndexCache.valid = nil, false
	e.arena[0].release()
	e.arena[1].release()
	e.documentProjection.Release()
	e.clearTableVirtualEdit()
	e.inlineMath.SetPreviewHidden(false)
	e.clearVimImageRow()
	e.clearTableSelectionSession()
	e.inlineMath.Clear()
	e.hoverLink = Range{}
	e.foldHover = -1
	e.typedBreaks.Clear()
	e.resetFolds()
	e.setSessionInclusive(false)
	e.setSessionLinewise(false)
	e.block, e.blockInput = nil, nil
	e.tableHandleDrag = nil
	e.horizontalDrag = nil
	e.phantom = nil
	e.imageAlignments.Clear()
	e.largeDataImageCache.Reset()
	e.p.Clear()
	e.parsed = invalidParsedRevision
	e.imeClient.Reset()
	e.pending, e.pendingMulti = nil, false
	e.setSessionSelection(0, 0)
	e.phantom = nil
	e.highlights, e.current = nil, -1
	e.vimHighlights, e.vimCurrent, e.vimHighlightsActive = nil, -1, false
	e.feedbackState.CancelYankFlash()
	e.feedbackState.CancelSearchPulse()
	e.scroll.Y = 0
	e.hasGoal = false
}

// sync parses the source again when it changed, and makes the leaves.
func (e *Editor) sync() {
	e.drainSessionChanges()
	e.dataImageAssets.ResetForRevision(e.Session.Revision())
	e.largeDataImageCache.ResetForRevision(e.Session.Revision())
	if e.parsed == e.Session.Revision() && e.leaves != nil {
		// The caret may have moved into or out of a comment: the leaves are
		// built for the comment it is in (its own show, the others do not),
		// so that a caret moving there rebuilds leaves, not the parse.
		if f, ok := source.FindComment(e.p.Comments(), e.Caret()); ok != e.leafCommentOK || (ok && f != e.leafComment) {
			e.parsed = invalidParsedRevision
		} else {
			return
		}
	}
	if e.opts.Code != "" {
		old := e.leaves
		e.arenaCur ^= 1
		a := &e.arena[e.arenaCur]
		a.reset()
		e.leaves = []*leaf{codeDocLeaf(a, e.Source(), e.opts.Code)}
		carryViews(e.leafSrc, e.Source(), old, e.leaves)
		e.leafSrc = e.Source()
		e.parsed = e.Session.Revision()
		e.p.Clear()
		e.pending, e.pendingMulti = nil, false
		return
	}
	// The parser's input is made here and nowhere else: one edit it can
	// follow, or the whole source again. The choice is the same for every
	// field of it, so a fast path cannot leave one of them behind.
	if e.p.Source() != e.Source() {
		fm := frontMatterEnd(e.Source())
		if e.pending == nil || e.pendingMulti || fm != e.p.FrontmatterEnd() || !e.p.Edit(e.Source(), e.pending.from, e.pending.to, e.pending.text) {
			var imageInlines []syntax.IncrementalInline
			if fm == 0 {
				imageInlines, _ = syntax.LargeDataImageOnlySource(e.Source())
			}
			e.p.Set(e.Source(), fm, imageInlines)
		}
	}
	e.pending, e.pendingMulti = nil, false
	old := e.leaves
	e.arenaCur ^= 1
	e.leaves = buildLeaves(e.Source(), e.p.Parser(), e.p.FrontmatterEnd(), &e.arena[e.arenaCur], &e.documentProjection)
	if e.opts.HideFrontmatter {
		hideFMEnd := hideFrontMatterEnd(e.Source())
		if hideFMEnd > 0 {
			kept := e.leaves[:0]
			for _, l := range e.leaves {
				if l.lineStart >= hideFMEnd {
					kept = append(kept, l)
				}
			}
			e.leaves = kept
			// A frontmatter-only document still has an editable body position.
			if len(e.leaves) == 0 {
				e.leaves = insertPhantom(e.leaves, &phantom{at: hideFMEnd})
			}
		}
	}
	// The block shown as its source is a block of lines, not the block it
	// draws: a table stops being cells and becomes the Markdown it is
	// written as. Its control opened it and closes it, like a fold: the
	// caret going elsewhere is not the control.
	if e.sourceOK {
		i := e.blockAt(e.sourceAt)
		switch {
		case i < 0:
			e.sourceOK = false
		case e.leaves[i].kind == kTable:
			// A table draws as a grid, not as the lines it is written in: it
			// becomes a block of lines to show its source at all.
			l := e.leaves[i]
			e.leaves[i] = rawLeaf(&e.arena[e.arenaCur], e.Source(), l.lineStart, blockEnd(e.Source(), l), false)
		case e.leaves[i].kind == kCode:
			// A block of code shows its source in place, fence and all: its
			// view asks sourceOpen (see codeView).
		case !e.pictureBlock(e.leaves[i]):
			e.sourceOK = false
		}
		// A picture opens its source in place: the display shows the tag
		// where it drew the picture (see builder.objectOpen).
	}
	// A comment covers its lines whole: the parser reads blanks there, which
	// used to become raw leaves showing the comment's own text. With the
	// comment blanked, its lines are a gap the caret reaches through the
	// phantom — and the comment shows when the caret is in it.
	if len(e.p.Comments()) > 0 {
		kept := e.leaves[:0]
		e.leafComment, e.leafCommentOK = source.FindComment(e.p.Comments(), e.Caret())
		for _, l := range e.leaves {
			if e.insideComment(l.start, l.end) {
				continue
			}
			kept = append(kept, l)
		}
		e.leaves = kept
	}
	if p := e.phantom; p != nil {
		// A line of the source at the phantom's offset is that line: the
		// phantom was the line while the source held none.
		if lineAt(e.leaves, p.at) != nil {
			e.phantom = nil
		} else {
			e.leaves = insertPhantom(e.leaves, p)
		}
	}
	// A parser/presentation rebuild can happen without a source revision
	// change (for example, revealing a code block's Markdown source). The
	// visible-leaf index stores pointers into the previous leaf slice, so it
	// must be rebuilt alongside the new leaves.
	e.visibleIndexReady = false
	// Caret units also retain leaf pointers. They can change when a comment is
	// revealed or a table switches to source view without a source revision.
	e.unitList = nil
	e.unitIndexCache.valid = false
	carryViews(e.leafSrc, e.Source(), old, e.leaves)
	e.leafSrc = e.Source()
	e.parsed = e.Session.Revision()
}

// carryViews gives the leaves of the new parse the views of the leaves of
// the parse before where the two show the same thing. Without it a
// document laid out after every keystroke would look up the view of every
// leaf, hashing all of its text to find it.
func carryViews(oldSrc, src string, old, next []*leaf) {
	for i := 0; i < min(len(old), len(next)); i++ {
		o, l := old[i], next[i]
		if o.kind == l.kind && o.lineStart == l.lineStart && (l.kind == kCode || l.kind == kMath || l.kind == kTable) {
			// The horizontal origin is presentation state. Keep it while an
			// existing block is edited in place, then let layout clamp it to
			// that block's new content width.
			l.horizontal = o.horizontal
		}
	}
	if len(old) != len(next) {
		return
	}
	for i, l := range next {
		o := old[i]
		if !sameLeaf(o, l) || !sameText(oldSrc, src, o, l) {
			continue
		}
		l.horizontal = o.horizontal
		l.view, l.geom = o.view, o.geom
		// The height is an estimate until the leaf is shaped again: it is
		// what the leaf took where it was last laid out.
		l.h = o.h
		if l.tab == nil {
			continue
		}
		for r, row := range l.tab.rows {
			for c, cell := range row {
				oc := o.tab.rows[r][c]
				cell.view = oc.view
			}
		}
	}
}

// commentsStay reports whether an edit leaves the comments of the source as
// they were, moved by it at most. A comment is written with `%`, escaped
// with a `\`, and hidden inside a fence of ` or ~: an edit that keeps away
// from all of them can neither make a comment nor unmake one, nor change
// which fences hide the ones after it.
func commentsStay(oldSrc, newSrc string, from, to, newLen int) bool {
	marker := func(b byte) bool {
		return b == '%' || b == '\\' || b == '`' || b == '~'
	}
	for i := from; i < to && i < len(oldSrc); i++ {
		if marker(oldSrc[i]) {
			return false
		}
	}
	for i := from; i < from+newLen && i < len(newSrc); i++ {
		if marker(newSrc[i]) {
			return false
		}
	}
	if from > 0 && marker(oldSrc[from-1]) {
		return false
	}
	if to < len(oldSrc) && marker(oldSrc[to]) {
		return false
	}
	return true
}

// insideComment reports whether a source range lies wholly in a comment the
// caret is not in: that comment's leaves are dropped, so it takes no room
// and its source shows only while the caret is in it.
func (e *Editor) insideComment(start, end int) bool {
	for _, c := range e.p.Comments() {
		if e.leafCommentOK && c == e.leafComment {
			continue
		}
		if c.Start <= start && end <= c.End {
			return true
		}
	}
	return false
}

// touchesComment reports whether the edit range reached a comment.
func touchesComment(cmts []source.Comment, from, to int) bool {
	for _, c := range cmts {
		if from <= c.End && to >= c.Start {
			return true
		}
	}
	return false
}

// parserText returns whole lines of the source as the parser reads them.
func parserText(lines string) string { return source.ParserText(lines) }

// blockAt returns the index of the leaf whose first line starts at lineStart,
// or -1: the block a source control was used on, found again by its line.
func (e *Editor) blockAt(lineStart int) int {
	for i, l := range e.leaves {
		if l.lineStart == lineStart {
			return i
		}
	}
	return -1
}

// lineAt returns the line leaf that starts at off, if the source holds one
// there: a blank line, or the phantom paragraph another pass put there.
func lineAt(leaves []*leaf, off int) *leaf {
	for _, l := range leaves {
		if l.isLine() && l.start == off {
			return l
		}
	}
	return nil
}

// insertPhantom puts the phantom's leaf among the leaves, by its offset.
func insertPhantom(leaves []*leaf, p *phantom) []*leaf {
	pl := &leaf{kind: kPhantom, id: -1, start: p.at, end: p.at, lineStart: p.at, conts: p.conts}
	i := sort.Search(len(leaves), func(i int) bool {
		l := leaves[i]
		if p.before {
			return l.lineStart >= p.at
		}
		return l.lineStart > p.at || (l.lineStart >= p.at && l.end > p.at)
	})
	if !p.before {
		// After every leaf ending at or before it.
		for i < len(leaves) && leaves[i].end <= p.at && leaves[i].lineStart <= p.at && !leaves[i].isLine() {
			i++
		}
	}
	out := make([]*leaf, 0, len(leaves)+1)
	out = append(out, leaves[:i]...)
	out = append(out, pl)
	return append(out, leaves[i:]...)
}

// leafLo returns the first offset the caret can take in a leaf: a fenced
// code block's info string comes before its lines.
func leafLo(e *Editor, l *leaf) int {
	if l.kind == kPara {
		if start, _, ok := paragraph.SourcePrefix(e.Source(), l.lineStart, l.start); ok {
			return start
		}
	}
	if s, _, ok := containerPrefixRange(l); ok {
		return s
	}
	for _, r := range quoteSourcePrefixes(e, l) {
		if r.Start < l.start {
			return r.Start
		}
	}
	if l.kind == kHeading {
		return l.lineStart
	}
	if l.kind == kCode && l.fenced {
		return l.infoStartOffset()
	}
	return l.start
}

// leafIndex returns the index of the leaf the caret at off is in: the
// phantom when it is in it, else the leaf whose content holds off, else
// -1.
func (e *Editor) leafIndex(off int) int {
	// A line of its own owns its offset: a blank line, or the phantom
	// paragraph the caret opens below a block. It may come after the leaf
	// that ends where it starts, so it is found before that leaf is taken.
	line, cover := -1, -1
	for i, l := range e.leaves {
		if e.foldHidden(l) {
			continue
		}
		if l.isLine() {
			// A whitespace-only source line owns its complete editable span,
			// including the stop after its final space. Phantom lines remain a
			// single-position stop because their source span is empty.
			ownsOffset := off == l.start
			if l.kind == kBlank {
				ownsOffset = off >= l.start && off <= l.end
			}
			if line < 0 && ownsOffset {
				line = i
			}
			continue
		}
		if cover >= 0 {
			continue
		}
		if off >= leafLo(e, l) && off <= l.end {
			if l.kind == kCode && l.fenced && off > l.infoEndOffset() && off < l.start {
				continue
			}
			cover = i
		}
	}
	if line >= 0 {
		return line
	}
	return cover
}

// leafAt returns the leaf the caret at off is in, or nil.
func (e *Editor) leafAt(off int) *leaf {
	if i := e.leafIndex(off); i >= 0 {
		return e.leaves[i]
	}
	return nil
}

// cellAt returns the cell of a table holding off, with its row and column.
func cellAt(l *leaf, off int) (*leaf, int, int) {
	if l.tab == nil {
		return nil, -1, -1
	}
	for r, row := range l.tab.rows {
		for c, cell := range row {
			if off >= cell.start && off <= cell.end {
				return cell, r, c
			}
		}
	}
	return nil, -1, -1
}

// selection returns the selection's start and end.
func (e *Editor) selection() (int, int) {
	if e.tableSelection != nil {
		if a, z, ok := e.tableSelectionSourceRange(e.tableSelectionLeaf(), e.tableSelection); ok {
			return a, z
		}
	}
	if e.block != nil {
		rs := e.blockRanges()
		if len(rs) == 0 {
			return e.Caret(), e.Caret()
		}
		return rs[0].Start, rs[len(rs)-1].End
	}
	if e.Session.Selection().Linewise {
		aa, _ := e.sourceLineRange(e.Session.Selection().Anchor)
		ca, cz := e.sourceLineRange(e.Caret())
		if cz < len(e.Source()) {
			cz++
		}
		if aa <= ca {
			return aa, cz
		}
		_, az := e.sourceLineRange(e.Session.Selection().Anchor)
		if az < len(e.Source()) {
			az++
		}
		return ca, az
	}
	if e.Session.Selection().Anchor < e.Caret() {
		if e.Session.Selection().Inclusive {
			return e.Session.Selection().Anchor, e.nextGraphemeBoundary(e.Caret())
		}
		return e.Session.Selection().Anchor, e.Caret()
	}
	if e.Session.Selection().Inclusive {
		return e.Caret(), e.nextGraphemeBoundary(e.Session.Selection().Anchor)
	}
	return e.Caret(), e.Session.Selection().Anchor
}

// textSelectionRange is the text envelope. textSelectionForUnit excludes the
// structures embedded in it; a dedicated structure has no active text envelope.
func (e *Editor) textSelectionRange() (int, int) {
	if e.tableSelection != nil || e.block != nil {
		return 0, 0
	}
	return e.selection()
}

// SelectionRange returns the selected range of the source.
func (e *Editor) SelectionRange() Range {
	a, z := e.selection()
	return Range{a, z}
}

// SelectedText returns the source the selection holds.
func (e *Editor) SelectedText() string {
	if e.block != nil {
		var selected strings.Builder
		for i, r := range e.SelectionRanges() {
			if i > 0 {
				selected.WriteByte('\n')
			}
			selected.WriteString(e.Source()[r.Start:r.End])
		}
		return selected.String()
	}
	if e.tableBridge == nil {
		if s := e.tableSelection; s != nil {
			if t := e.tableSelectionLeaf(); t != nil {
				if r := e.tableRegisterForSelection(t, s); r != nil {
					return r.Markdown()
				}
			}
		}
	}
	var selected strings.Builder
	for _, part := range e.selectionParts() {
		if part.structure != nil {
			t := e.tableByLineStart(part.structure.lineStart)
			if reg := e.tableRegisterForSelection(t, part.structure); reg != nil && reg.Region() && part.structure.axis != tableWhole {
				selected.WriteString(reg.Markdown())
				continue
			}
		}
		selected.WriteString(e.Source()[part.source.Start:part.source.End])
	}
	return selected.String()
}

// revealCaret returns the offset whose syntax shows: the caret's, or where
// it was when a drag began. -1 while the editor does not have the focus.
func (e *Editor) revealCaret() int {
	if !e.focused {
		return -1
	}
	if e.frozen {
		return e.frozenAt
	}
	return e.selectionFocus()
}

// firstStop returns the first offset the caret can take.
func (e *Editor) firstStop() int {
	for _, l := range e.leaves {
		if e.foldHidden(l) {
			continue
		}
		// A recognized whole-document data image starts at a legal caret
		// boundary. Building its unit view just to discover that boundary
		// scans and shapes the enormous destination during editor startup.
		if l.kind == kPara && l.start == 0 && len(e.p.ImageInlines()) > 0 &&
			e.p.ImageInlines()[0].Start == l.start {
			return l.start
		}
		if s, ok := e.edgeStop(l, false); ok {
			return s
		}
	}
	return 0
}

// lastStop returns the last offset the caret can take.
func (e *Editor) lastStop() int {
	for i := len(e.leaves) - 1; i >= 0; i-- {
		if e.foldHidden(e.leaves[i]) {
			continue
		}
		if s, ok := e.edgeStop(e.leaves[i], true); ok {
			return s
		}
	}
	return len(e.Source())
}

// setCaret moves the caret to off, keeping the anchor when extend is set,
// and leaves the phantom unless the caret stays in it.
func (e *Editor) setCaret(off int, extend bool) {
	if !extend {
		e.tableDocumentRegion = nil
	}
	if e.pageJump || e.scroll.Scrolling() {
		e.cancelPageJump()
	}
	e.pageJump = false
	if !e.tableNavigating {
		_, _ = e.tableUI().Dispatch(tableui.Action{Kind: tableui.ResetNavigation})
	}
	if b := e.tableBridge; b != nil {
		inside := false
		for _, l := range e.leaves {
			if l.kind == kTable && l.lineStart == b.lineStart && b.row < len(l.tab.rows) && b.col < len(l.tab.rows[b.row]) {
				cell := l.tab.rows[b.row][b.col]
				lo, hi := cell.start, cell.end
				if b.axis == tableRows {
					row := l.tab.rows[b.row]
					lo, hi = row[0].start, row[len(row)-1].end
				}
				inside = off >= lo && off <= hi
				break
			}
		}
		if !inside {
			e.tableBridge = nil
		}
	}
	if !extend && e.tableBridge == nil {
		e.tableSelection = nil
	}
	if e.inlineMath.Active() && !e.inlineMath.ContainsCaret(off) {
		e.inlineMath.Clear()
	}
	off = max(0, min(off, len(e.Source())))
	if off != e.Caret() {
		// A virtual destination belongs to the current structural focus. Once
		// navigation moves the source caret, later puts resolve from that caret.
		e.clearTableVirtualEdit()
	}
	if state, ok := e.vimImageRowState(); ok && off != state.LineStart {
		e.clearVimImageRow()
	}
	if e.Keymap() == "vim" && e.vimController().Mode() == vim.ModeNormal && !extend {
		off = e.vimNormalOffset(off)
	}
	if e.smoothCursorNavigation {
		e.beginSmoothCursorMove(e.Caret(), off, extend)
	} else {
		e.feedbackState.CancelCursor()
	}
	if e.phantom != nil && !(off == e.phantom.at && e.leafIsPhantom(off)) {
		e.dropPhantom()
	}
	e.setSessionHead(off)
	if !extend {
		e.setSessionAnchor(off)
	}
	e.wantReveal = true
	e.clearComposition()
	e.blinkStart = time.Now()
	if extend && e.tableBridge == nil {
		e.structuralSelectionForRange()
	}
}

// leafIsPhantom reports whether the caret at off stays in the phantom.
func (e *Editor) leafIsPhantom(off int) bool {
	i := e.leafIndex(off)
	return i >= 0 && e.leaves[i].kind == kPhantom
}

// dropPhantom takes the phantom away, as the caret leaves it.
func (e *Editor) dropPhantom() {
	if e.phantom == nil {
		return
	}
	e.phantom = nil
	e.parsed = invalidParsedRevision
	e.sync()
}

// setPhantom puts the caret in a phantom paragraph.
func (e *Editor) setPhantom(p *phantom) {
	e.clearTableSelection()
	e.phantom = p
	e.parsed = invalidParsedRevision
	e.sync()
	e.setSessionSelection(p.at, p.at)
	e.wantReveal = true
	e.clearComposition()
	e.blinkStart = time.Now()
}

// Select selects a range of the source, as a search's match, and scrolls
// to it.
func (e *Editor) Select(start, end int) {
	e.finishBlockInput()
	e.paintMu.Lock()
	defer e.paintMu.Unlock()
	e.cancelPageJump()
	e.clearTableSelectionSession()
	e.dropPhantom()
	start = max(0, min(start, len(e.Source())))
	end = max(start, min(end, len(e.Source())))
	e.expandFoldsForRange(start, end)
	e.setSessionSelection(start, end)
	e.block, e.blockInput = nil, nil
	e.setSessionInclusive(false)
	e.setSessionLinewise(false)
	e.wantReveal = true
	e.pageJump = false
	e.hasScrollTo = false
	e.hasGoal = false
	e.invalidate()
}

// ScrollToOffset scrolls the source offset off to the top of the view and
// puts the caret there, as an outline's heading is gone to.
func (e *Editor) ScrollToOffset(off int) {
	// Keep the long-standing outline navigation margin (two font sizes below
	// the viewport top); explicit zt uses the exact top placement operation.
	e.NavigateToOffset(off, layoutindex.PageJumpTopMargin)
}

// ScrollY returns how far the document is scrolled, in DIPs: 0 when its
// first line is at the top of the view, as the header's fade ramps in from.
func (e *Editor) ScrollY() float32 { return e.scroll.Y }

func (e *Editor) cancelPageJump() {
	if e.scroll.Scrolling() {
		// ScrollState's one-nanosecond linear animation is its public way to
		// stop a glide while preserving the ScrollState/Handle binding.
		e.scroll.ScrollTo(e.scroll.X, e.scroll.Y, ui.Animation{Duration: time.Nanosecond, Ease: ui.Linear})
	}
}

// SetHighlights draws ranges of the source as found, current the one
// chosen (or -1).
func (e *Editor) SetHighlights(rs []Range, current int) {
	e.paintMu.Lock()
	defer e.paintMu.Unlock()
	old, oldOK := e.visibleCurrentHighlight()
	e.highlights, e.current = rs, current
	e.sourceRevealRanges = normalizeSourceRevealRanges(rs)
	e.vimHighlightsActive = false
	e.searchRev++
	e.noteSearchHighlightChange(old, oldOK)
	e.invalidate()
}

func (e *Editor) setVimSearchHighlights(rs []Range, current int) {
	e.paintMu.Lock()
	defer e.paintMu.Unlock()
	old, oldOK := e.visibleCurrentHighlight()
	e.vimHighlights, e.vimCurrent, e.vimHighlightsActive = rs, current, true
	e.sourceRevealRanges = normalizeSourceRevealRanges(rs)
	e.searchRev++
	e.noteSearchHighlightChange(old, oldOK)
	e.invalidate()
}

func (e *Editor) clearVimSearchHighlights() {
	e.paintMu.Lock()
	defer e.paintMu.Unlock()
	e.vimHighlights, e.vimCurrent, e.vimHighlightsActive = nil, -1, false
	e.sourceRevealRanges = normalizeSourceRevealRanges(e.highlights)
	e.searchRev++
	e.feedbackState.CancelSearchPulse()
	e.invalidate()
}

func (e *Editor) visibleCurrentHighlight() (Range, bool) {
	ranges, current := e.highlights, e.current
	if e.vimHighlightsActive {
		ranges, current = e.vimHighlights, e.vimCurrent
	}
	if current < 0 || current >= len(ranges) {
		return Range{}, false
	}
	return ranges[current], true
}

func (e *Editor) noteSearchHighlightChange(old Range, oldOK bool) {
	next, nextOK := e.visibleCurrentHighlight()
	e.feedbackState.UpdateSearchPulse(feedback.SearchPulseChange{
		Previous:      feedback.TextRange{Start: old.Start, End: old.End},
		Next:          feedback.TextRange{Start: next.Start, End: next.End},
		PreviousValid: oldOK, NextValid: nextOK, Enabled: e.opts.SearchMatchPulse,
		Now: time.Now(),
	})
}

func (e *Editor) searchMatchPulse(now time.Time) (float32, bool) {
	if !e.opts.SearchMatchPulse {
		return 0, false
	}
	return e.feedbackState.SearchPulseOpacity(now)
}

func (e *Editor) startVimYankFlash(ranges []Range) {
	converted := make([]feedback.TextRange, 0, len(ranges))
	for _, r := range ranges {
		converted = append(converted, feedback.TextRange{Start: r.Start, End: r.End})
	}
	if e.feedbackState.StartVimYankFlash(feedback.YankFlashInput{
		Ranges: converted, SourceLength: len(e.Source()), Enabled: e.opts.VimYankFlash,
		VimKeymap: e.Keymap() == "vim", Now: time.Now(),
	}) {
		e.invalidate()
	}
}

func (e *Editor) vimYankFlashOpacity(now time.Time) (float32, bool) {
	return e.feedbackState.VimYankFlashOpacity(e.opts.VimYankFlash, e.Keymap() == "vim", now)
}

// beginSmoothCursorMove records an optional visual transition from a host
// snapshot. Document selection and movement remain synchronous.
func (e *Editor) beginSmoothCursorMove(from, to int, extend bool) {
	e.beginSmoothCursorMoveAt(from, to, extend, time.Now())
}

func (e *Editor) beginSmoothCursorMoveAt(from, to int, extend bool, now time.Time) {
	mode := e.vimController().Mode()
	e.feedbackState.BeginCursorMove(feedback.CursorMove{
		From: from, To: to, SelectionAnchor: e.Session.Selection().Anchor,
		Enabled: e.opts.SmoothCursor, Navigation: e.smoothCursorNavigation,
		Extending: extend, ModalNormal: (e.Keymap() == "vim" || e.Keymap() == "helix") && mode == vim.ModeNormal,
		Now: now,
	})
}

// Focus gives the editor the keyboard focus.
func (e *Editor) Focus() { e.handle.Focus() }

// Focused reports whether the editor has the keyboard focus.
func (e *Editor) Focused() bool { return e.focused }

// ViewportBounds returns the last editor viewport as logical coordinates.
// It is updated while building a frame and is intended for the UI adapter.
func (e *Editor) ViewportBounds() media.Viewport { return e.viewport }

func (e *Editor) invalidate() {
	if e.opts.Invalidate != nil {
		e.opts.Invalidate()
	}
}

// Headings returns the document's headings, in order: their level, text
// and offset.
func (e *Editor) Headings() []Heading {
	e.sync()
	var out []Heading
	for _, l := range e.leaves {
		if l.kind != kHeading || l.pending {
			continue
		}
		text := strings.TrimSpace(e.plainText(e.p.Parser().Inlines(l.id)))
		out = append(out, Heading{Level: l.level, Text: text, Offset: l.start})
	}
	return out
}

// Heading is a heading of the document.
type Heading struct {
	Level  int
	Text   string
	Offset int
}

// clearTableSelection hands ownership back to an explicit text/navigation action.
func (e *Editor) clearTableSelection() {
	e.tableSelection = nil
	e.tableBridge = nil
	if !e.tableNavigating {
		_, _ = e.tableUI().Dispatch(tableui.Action{Kind: tableui.ResetNavigation})
	}
	e.tableHandleDrag = nil
}

// clearTableSelectionSession ends both the active table selection and any
// region suspended while its caret was outside the table.
func (e *Editor) clearTableSelectionSession() {
	e.clearTableSelection()
	e.tableDocumentRegion = nil
}
