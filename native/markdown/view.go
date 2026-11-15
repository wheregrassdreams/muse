package markdown

import (
	"encoding/base64"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"muse/locales"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"muse/markdown/images"
	"muse/markdown/syntax"
)

const (
	defBodySize = 16
	defPadding  = 30
	defIndent   = 20
)

// Options controls how a document is drawn natively.
type Options struct {
	// SoftBreaksAsSpaces displays ordinary source newlines as Markdown spaces.
	// Explicit hard breaks and paragraph boundaries are unaffected.
	SoftBreaksAsSpaces bool
	// BodyFontSize is the size of body text in DIPs; 0 means 16.
	BodyFontSize float32
	// Font is the body font family; "" means the system's.
	Font string
	// MonoFont is the code font family; "" means "monospace".
	MonoFont string
	// Padding is the content padding in DIPs; 0 means 30.
	Padding float32
	// IndentStep is the nesting step for lists and quotes; 0 means 20.
	IndentStep float32
	// ContentWidth caps the column at this many DIPs and centers it; 0
	// fills the window, as a native preview did before the editor existed.
	ContentWidth float32
	// BaseDir resolves relative image paths; "" uses the working directory.
	BaseDir string
	// AssetRoot is an optional vault root used after BaseDir lookup fails.
	// Bare attachment names can then resolve to a file elsewhere in the vault.
	AssetRoot string
	// ImageCacheStore is supplied by application composition. Markdown owns
	// cache keys and validation; the store owns concrete persistence.
	ImageCacheStore images.CacheStore
	// ResolveImage overrides path resolution when the application maintains a
	// shared vault file catalog. Nil uses the standalone resolver.
	ResolveImage func(src, base, root string) string
	// OnTask is called when a task-list checkbox is clicked, with the item's
	// index in the list and its new checked state. Nil leaves the boxes
	// read-only.
	OnTask func(item int, checked bool)
	// OnCell is called when a table cell is clicked, with its row and column.
	OnCell func(row, col int)
	// OnCellCommit is called when the edited cell is submitted or loses focus.
	OnCellCommit func(row, col int, value string)
	// OnAsync is called on a worker goroutine when a remote image has loaded,
	// so the view can repaint.
	OnAsync func()
	// OnImagePreview opens an image's preview when a missing-image fallback
	// is clicked. The path is resolved relative to BaseDir when possible.
	OnImagePreview func(path string)
	// OnCopyCode is called from a code block's context menu.
	OnCopyCode func(text string)
	// CellEdit marks the one table cell drawn as a text input instead of text.
	CellEdit *CellEditor
}

// CellEditor is the table cell currently being edited in place.
type CellEditor struct {
	Row, Col int
	Value    *string
	WasFocus *bool
}

func (o Options) withDefaults() Options {
	if o.BodyFontSize == 0 {
		o.BodyFontSize = defBodySize
	}
	if o.MonoFont == "" {
		o.MonoFont = "monospace"
	}
	if o.Padding == 0 {
		o.Padding = defPadding
	}
	if o.IndentStep == 0 {
		o.IndentStep = defIndent
	}
	return o
}

// View draws the document natively: a scrollable column of blocks, built
// from the ui package with no webview and no frontend build.
func View(c *ui.Context, d *Doc, opts Options) ui.Element {
	if d == nil {
		return ui.Column(c)
	}
	s := rstyle{o: opts.withDefaults()}
	return ui.Scroll(c).Fill().Children(func() {
		col := ui.Column(c).FillWidth().Gap(s.o.BodyFontSize * 0.6).Padding(s.o.Padding)
		if s.o.ContentWidth > 0 {
			col.MaxWidth(s.o.ContentWidth).Margin(0, ui.Auto)
		}
		col.Children(func() {
			renderBlocks(c, d.Blocks, 0, s)
		})
	})
}

// BlocksView draws blocks natively as a column, without the scroll view and
// padding View adds. The editor uses it to render one block at a time, so a
// click can turn just that block into an input.
func BlocksView(c *ui.Context, blocks []Block, opts Options) ui.Element {
	s := rstyle{o: opts.withDefaults()}
	return ui.Column(c).FillWidth().Gap(s.o.BodyFontSize * 0.6).Children(func() {
		renderBlocks(c, blocks, 0, s)
	})
}

// rstyle carries the resolved options plus the context a block renders in,
// so a quote's contents draw gently subdued without a second options type.
type rstyle struct {
	o                  Options
	muted              bool
	preserveSoftBreaks bool
	first              bool
	afterHeading       bool
	heading            bool
	width              float32
}

func (s rstyle) inlineWidth() float32 {
	if s.width > 0 {
		return s.width
	}
	if s.o.ContentWidth > 0 {
		return s.o.ContentWidth
	}
	return 760
}

func (s rstyle) body(c *ui.Context) ui.Color {
	return BodyTextColor(c.Theme().Dark)
}

func renderBlocks(c *ui.Context, blocks []Block, depth int, s rstyle) {
	for i, b := range blocks {
		bs := s
		bs.first = i == 0
		bs.afterHeading = i > 0 && blocks[i-1].Kind == Heading && blocks[i-1].Level != 1
		renderBlock(c, b, depth, bs)
	}
}

func renderBlock(c *ui.Context, b Block, depth int, s rstyle) {
	t := c.Theme()
	switch b.Kind {
	case Heading:
		s.heading = true
		// The column supplies the ordinary block gap; headings add only the
		// remaining room so short sections do not acquire oversized gaps.
		pad := max(HeadingTopMargin(b.Level, s.o.BodyFontSize)-s.o.BodyFontSize*0.6, 0)
		if s.first || s.afterHeading {
			pad = 0
		}
		if spansHaveMath(b.Inlines) {
			ui.Column(c).Padding(pad, 0, 0, 0).Children(func() {
				renderInlineMath(c, b.Inlines, s, HeadingSize(b.Level, s.o.BodyFontSize), HeadingWeight(b.Level), HeadingLineHeight(b.Level))
			})
			return
		}
		ui.RichText(c, uiSpans(c, b.Inlines, s)...).
			FontSize(HeadingSize(b.Level, s.o.BodyFontSize)).
			FontWeight(HeadingWeight(b.Level)).Wrap().LineHeight(HeadingLineHeight(b.Level)).
			Padding(pad, 0, 0, 0)
	case Paragraph:
		ui.RichText(c, uiSpans(c, b.Inlines, s)...).
			FontSize(s.o.BodyFontSize).Wrap().LineHeight(1.7)
	case Code:
		renderCode(c, b, s)
	case Quote:
		ui.Column(c).Gap(6).Padding(0, 0, 0, 16).
			BorderWidth(0, 0, 0, 2).BorderColor(MarkdownRuleColor(t.Dark)).
			Children(func() {
				renderBlocks(c, b.Children, depth+1, rstyle{o: s.o, muted: true, preserveSoftBreaks: true, width: s.inlineWidth() - 18})
			})
	case List:
		renderList(c, b, depth, s)
	case Rule:
		ui.Box(c).FillWidth().Height(2).Background(t.Border)
	case HTML:
		if txt := syntax.HTMLText(b.Text); strings.TrimSpace(txt) != "" {
			ui.Text(c, txt).FontSize(s.o.BodyFontSize).TextColor(BodyTextColor(t.Dark)).Wrap().LineHeight(1.7)
		}
	case Table:
		renderTable(c, b, s)
	case Image:
		renderImage(c, b, s)
	case Math:
		if visual := RenderMathVisual(b.Text, 18, t.Dark, true, s.o.Font); visual != nil && visual.Bitmap() != nil {
			drawMathBlock(c, visual, s)
			return
		}
		ui.Row(c).FillWidth().Justify(ui.Center).Padding(8, 0).Children(func() {
			ui.RichText(c, ui.Span{
				Text:   texToUnicode(b.Text),
				Font:   "Georgia",
				Italic: true,
				Size:   s.o.BodyFontSize + 1,
			}).TextColor(BodyTextColor(t.Dark)).TextAlign(ui.Center).Wrap().LineHeight(1.7)
		})
	}
}

// renderCode uses a quiet neutral panel, comfortable
// padding, and no always-visible language badge.
func renderCode(c *ui.Context, b Block, s rstyle) {
	if strings.EqualFold(strings.TrimSpace(b.Lang), "mermaid") {
		if g, ok := parseGantt(b.Text); ok {
			renderGantt(c, g)
			return
		}
		if g, ok := parseClass(b.Text); ok {
			renderClass(c, g, s)
			return
		}
		if g, ok := parsePie(b.Text); ok {
			renderPie(c, g)
			return
		}
		if g, ok := parseState(b.Text); ok {
			renderMermaid(c, g, s)
			return
		}
		if g, ok := parseSequence(b.Text); ok {
			renderSequence(c, g)
			return
		}
		if g, ok := parseMermaid(b.Text); ok {
			renderMermaid(c, g, s)
			return
		}
	}
	t := c.Theme()
	code := ui.Column(c).FillWidth().Gap(0).Padding(s.o.BodyFontSize*0.875, s.o.BodyFontSize).Radius(6).
		Background(CodePanelColor(t.Dark))
	if s.o.OnCopyCode != nil {
		code.ContextMenu(func(m *ui.Menu) {
			if m.Item(locales.DocumentCopyCode()).Chosen() {
				s.o.OnCopyCode(b.Text)
			}
		})
	}
	code.Children(func() {
		text := strings.TrimRight(b.Text, "\n")
		if text == "" {
			text = " "
		}
		for _, line := range strings.Split(text, "\n") {
			ui.RichText(c, HighlightLine(line, b.Lang, t.Dark)...).
				Font(s.o.MonoFont).FontSize(s.o.BodyFontSize * 0.875).
				LineHeight(1.6).Wrap()
		}
	})
}

func renderList(c *ui.Context, b Block, depth int, s rstyle) {
	t := c.Theme()
	indent := float32(depth) * s.o.IndentStep
	ui.Column(c).Gap(s.o.BodyFontSize*0.2).Padding(0, 0, 0, indent).Children(func() {
		for idx, it := range b.Items {
			item := it
			ui.Row(c).Gap(7).AlignItems(ui.Start).Children(func() {
				if item.Task {
					checked := item.Checked
					box := taskBox(c, checked, t)
					if s.o.OnTask != nil {
						box.OnClick(func() { s.o.OnTask(idx, !checked) }).Cursor(ui.CursorPointer)
					}
				} else {
					ui.Text(c, item.Marker).FontSize(s.o.BodyFontSize).
						TextColor(t.TextMuted).MinWidth(20).TextAlign(ui.End)
				}
				ui.Column(c).Grow(1).Gap(s.o.BodyFontSize * 0.2).Children(func() {
					if len(item.Inlines) > 0 {
						if spansHaveMath(item.Inlines) {
							renderInlineMath(c, item.Inlines, rstyle{o: s.o, muted: s.muted, preserveSoftBreaks: s.preserveSoftBreaks, width: s.inlineWidth() - indent - 30}, s.o.BodyFontSize, 0, 1.7)
						} else {
							ui.RichText(c, uiSpans(c, item.Inlines, s)...).
								FontSize(s.o.BodyFontSize).TextColor(BodyTextColor(t.Dark)).Wrap().LineHeight(1.7)
						}
					}
					if len(item.Children) > 0 {
						renderBlocks(c, item.Children, depth+1, s)
					}
				})
			})
		}
	})
}

func taskBox(c *ui.Context, checked bool, t *ui.Theme) ui.Element {
	box := ui.Column(c).Width(16).Height(16).Shrink(0).Radius(3).
		Border(1, t.Border).Justify(ui.Center).AlignItems(ui.Center)
	if checked {
		box.Background(t.Accent).BorderColor(t.Accent)
		box.Children(func() {
			ui.Text(c, "✓").FontSize(10).FontWeight(700).TextColor(t.AccentText)
		})
	}
	return box
}

// renderTable is a real grid, as Typora's tables are: hairline borders,
// a tinted header, equal columns and per-column alignment.
func renderTable(c *ui.Context, b Block, s rstyle) {
	t := c.Theme()
	cols := 0
	for _, row := range b.Rows {
		if len(row) > cols {
			cols = len(row)
		}
	}
	if cols == 0 {
		return
	}
	ui.Column(c).FillWidth().Border(1, TableRuleColor(t.Dark)).Radius(5).Clip().Children(func() {
		for ri, row := range b.Rows {
			head := ri == 0
			ui.Row(c).AlignItems(ui.Stretch).Children(func() {
				for ci := 0; ci < cols; ci++ {
					var cell Cell
					if ci < len(row) {
						cell = row[ci]
					}
					right := float32(0)
					if ci < cols-1 {
						right = 1
					}
					bottom := float32(0)
					if ri < len(b.Rows)-1 {
						bottom = 1
					}
					box := ui.Column(c).Grow(1).MinHeight(34).
						Padding(7, 13).BorderWidth(0, right, bottom, 0).
						BorderColor(TableRuleColor(t.Dark))
					if e := s.o.CellEdit; e != nil && e.Row == ri && e.Col == ci {
						box.Children(func() {
							inp := ui.TextInputBase(c, e.Value).AutoFocus().FillWidth().
								FontSize(s.o.BodyFontSize - 1).TextColor(t.Text).Label(locales.DiagramCellLabel())
							if inp.Focused() && e.WasFocus != nil {
								*e.WasFocus = true
							}
							if inp.Submitted() || (e.WasFocus != nil && *e.WasFocus && !inp.Focused()) {
								if s.o.OnCellCommit != nil {
									s.o.OnCellCommit(ri, ci, *e.Value)
								}
							}
						})
					} else {
						box.Children(func() {
							if spansHaveMath(cell) {
								w := (s.inlineWidth()-2)/float32(cols) - 26
								weight := 0
								if head {
									weight = 600
								}
								renderInlineMath(c, cell, rstyle{o: s.o, muted: s.muted, preserveSoftBreaks: s.preserveSoftBreaks, width: w}, s.o.BodyFontSize-1, weight, 1.7)
								if s.o.OnCell != nil {
									box.OnClick(func() { s.o.OnCell(ri, ci) }).Cursor(ui.CursorText)
								}
								return
							}
							el := ui.RichText(c, uiSpans(c, cell, s)...).
								FontSize(s.o.BodyFontSize - 1).Wrap().LineHeight(1.6).
								TextAlign(cellAlign(b.Align, ci))
							if head {
								el.FontWeight(600)
							}
							if s.o.OnCell != nil {
								el.OnClick(func() { s.o.OnCell(ri, ci) }).Cursor(ui.CursorText)
							}
						})
						if s.o.OnCell != nil {
							box.OnClick(func() { s.o.OnCell(ri, ci) }).Cursor(ui.CursorText)
						}
					}
				}
			})
		}
	})
}

func cellAlign(a []Align, i int) ui.Align {
	if i >= len(a) {
		return ui.Start
	}
	switch a[i] {
	case AlignCenter:
		return ui.Center
	case AlignRight:
		return ui.End
	}
	return ui.Start
}

// renderImage draws a standalone image from the document's folder or URL.
// File and network reads happen in ImageAsset loaders, outside this draw.
func renderImage(c *ui.Context, b Block, s rstyle) {
	t := c.Theme()
	path := ""
	if s.o.ResolveImage != nil {
		path = s.o.ResolveImage(b.Src, s.o.BaseDir, s.o.AssetRoot)
	} else {
		path = ResolveImageSource(b.Src, s.o.BaseDir, s.o.AssetRoot)
	}
	var asset *ImageAsset
	if path != "" {
		asset = LoadImageAssetWithCacheStore(path, "", s.o.ImageCacheStore, s.o.OnAsync)
	}
	if asset == nil {
		imagePlaceholder(c, b.Src, b.Src, t, s.o.OnImagePreview)
		return
	}
	next, _ := asset.Advance(c.Now())
	if next > 0 {
		c.After(next)
	}
	bmp := asset.Bitmap()
	if bmp == nil {
		if path == "" {
			path = b.Src
		}
		imagePlaceholder(c, b.Src, path, t, s.o.OnImagePreview)
		return
	}
	drawImage(c, bmp, s, b.ImgWidth, b.ImgHeight)
}

func drawImage(c *ui.Context, bmp *ui.Bitmap, s rstyle, wantW, wantH float32) {
	w, h := bmp.Size()
	maxW := float32(720)
	if s.o.ContentWidth > 0 {
		maxW = s.o.ContentWidth
	}
	fw, fh := float32(w), float32(h)
	switch {
	case wantW > 0 && wantH > 0:
		fw, fh = wantW, wantH
	case wantW > 0:
		fh = fh * wantW / fw
		fw = wantW
	case wantH > 0:
		fw = fw * wantH / fh
		fh = wantH
	}
	if fw > maxW {
		fh = fh * maxW / fw
		fw = maxW
	}
	ui.Row(c).FillWidth().Justify(ui.Center).Children(func() {
		ui.Image(c, bmp).Size(fw, fh)
	})
}

func imagePlaceholder(c *ui.Context, source, resolved string, t *ui.Theme, onPreview func(string)) {
	row := ui.Row(c).FillWidth().Padding(12).Radius(4).Background(t.Surface)
	if onPreview != nil {
		if resolved == "" {
			resolved = source
		}
		row.OnClick(func() { onPreview(resolved) }).Cursor(ui.CursorPointer)
	}
	row.Children(func() {
		ui.Text(c, MissingImageMessage(source)).FontSize(13).TextColor(t.TextMuted)
	})
}

func isRemoteImage(src string) bool {
	return strings.HasPrefix(src, "http://") || strings.HasPrefix(src, "https://")
}

// remoteBitmap returns a fetched remote image, or nil while it loads. The
// first caller starts the fetch; OnAsync repaints the view when it lands.
func remoteBitmap(src string, done func()) *ui.Bitmap {
	return remoteBitmapWithCacheStore(src, nil, done)
}

func remoteBitmapWithCacheStore(src string, store images.CacheStore, done func()) *ui.Bitmap {
	// Use the same in-flight request and bounded decoded-image cache as the
	// Markdown renderer. A buffered handoff covers a fetch completing before
	// LoadImageAsset returns to this caller.
	ready := make(chan *ImageAsset, 1)
	asset := LoadImageAssetWithCacheStore(src, "", store, func() {
		loaded := <-ready
		loaded.Advance(time.Now())
		if done != nil {
			done()
		}
	})
	ready <- asset
	if asset.Loaded() {
		asset.Advance(time.Now())
		bmp := asset.Bitmap()
		if bmp == nil && done != nil {
			// A previously cached failure still completes each new viewer so it
			// can replace its loading state with the error placeholder.
			go done()
		}
		return bmp
	}
	return nil
}

func resolveImagePath(src, base string) string {
	src = strings.TrimSpace(src)
	if src == "" || strings.HasPrefix(src, "http://") || strings.HasPrefix(src, "https://") ||
		strings.HasPrefix(src, "data:") {
		return ""
	}
	src = strings.TrimPrefix(src, "file://")
	if unescaped, err := url.PathUnescape(src); err == nil {
		src = unescaped
	}
	if filepath.IsAbs(src) {
		return src
	}
	if base == "" {
		return src
	}
	return filepath.Join(base, src)
}

func loadBitmap(path string) *ui.Bitmap {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	bmp, err := decodeDocumentImage(data)
	if err != nil {
		return nil
	}
	return bmp
}

// uiSpans converts model spans to ui spans, resolving document link colors
// and the mono face.
func uiSpans(c *ui.Context, spans []Span, s rstyle) []ui.Span {
	t := c.Theme()
	out := make([]ui.Span, 0, len(spans))
	for _, sp := range spans {
		u := ui.Span{Text: sp.Text}
		if sp.SoftBreak && s.o.SoftBreaksAsSpaces && !s.preserveSoftBreaks {
			u.Text = " "
		}
		u.Color = BodyTextColor(t.Dark)
		if s.heading {
			u.Color = HeadingTextColor(t.Dark)
		}
		if s.muted {
			u.Color = s.body(c).Mix(t.TextMuted, 0.25)
		}
		if sp.Bold {
			u.Weight = 600
		}
		if sp.Italic {
			u.Italic = true
		}
		if sp.Strike {
			u.Strikethrough = true
		}
		if sp.Code {
			u.Font = s.o.MonoFont
			u.Size = s.o.BodyFontSize * 0.875
			u.Background, u.Color = InlineCodeColorsFor(t.Accent, t.Dark)
			u.Radius = max(3, s.o.BodyFontSize/5)
			u.PaddingX = s.o.BodyFontSize * 0.2
			u.PaddingY = s.o.BodyFontSize * 0.1
		}
		if sp.Link != "" {
			u.Color = InlineLinkColorFor(t.Accent, t.Dark)
			u.Underline = true
		}
		if sp.Math {
			u.Text = texToUnicode(sp.Text)
			u.Font = "Georgia"
			u.Italic = true
		}
		out = append(out, u)
	}
	return out
}

// LoadImage returns the bitmap of an image a document links to: a file
// relative to baseDir, a data: URL, or a remote image, fetched once in the
// background (nil until then; onLoad runs on another goroutine when it
// lands). It returns nil for what cannot be read.
func LoadImage(src, baseDir string, onLoad func()) *ui.Bitmap {
	return LoadImageWithCacheStore(src, baseDir, nil, onLoad)
}

// LoadImageWithCacheStore is LoadImage with an application supplied store
// for remote image source bytes.
func LoadImageWithCacheStore(src, baseDir string, store images.CacheStore, onLoad func()) *ui.Bitmap {
	src = strings.TrimSpace(src)
	switch {
	case src == "":
		return nil
	case isRemoteImage(src):
		return remoteBitmapWithCacheStore(src, store, onLoad)
	case strings.HasPrefix(src, "data:"):
		return dataBitmap(src)
	}
	path := resolveImagePath(src, baseDir)
	if path == "" {
		return nil
	}
	return loadBitmap(path)
}

// dataBitmap decodes a base64 data: URL, as pasted images are kept.
func dataBitmap(src string) *ui.Bitmap {
	if comma := strings.IndexByte(src, ','); comma > 0 && strings.Contains(src[:comma], ";base64") {
		if data, err := base64.StdEncoding.DecodeString(strings.TrimSpace(src[comma+1:])); err == nil {
			bmp, _ := decodeDocumentImage(data)
			return bmp
		}
	}
	return nil
}

// HTMLOnlyImage reports the image of an HTML block that holds nothing but
// one <img>, as READMEs center their logos, with its width and height.
func HTMLOnlyImage(html string) (src, alt, title string, w, h float32, ok bool) {
	return syntax.HTMLOnlyImage(html)
}
