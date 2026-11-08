package mdoc

import (
	"encoding/base64"
	"html"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/egoist/mygo/ui"
)

const (
	defBodySize = 16
	defPadding  = 30
	defIndent   = 20
)

// Options controls how a document is drawn natively.
type Options struct {
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
		col := ui.Column(c).FillWidth().Gap(12).Padding(s.o.Padding)
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
	return ui.Column(c).FillWidth().Gap(12).Children(func() {
		renderBlocks(c, blocks, 0, s)
	})
}

// rstyle carries the resolved options plus the context a block renders in,
// so a quote's contents draw muted without a second options type.
type rstyle struct {
	o     Options
	muted bool
	width float32
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
	if s.muted {
		return c.Theme().TextMuted
	}
	return ui.Color{}
}

func renderBlocks(c *ui.Context, blocks []Block, depth int, s rstyle) {
	for _, b := range blocks {
		renderBlock(c, b, depth, s)
	}
}

func renderBlock(c *ui.Context, b Block, depth int, s rstyle) {
	t := c.Theme()
	switch b.Kind {
	case Heading:
		if spansHaveMath(b.Inlines) {
			pad := float32(0)
			switch b.Level {
			case 1:
				pad = 12
			case 2:
				pad = 8
			case 3:
				pad = 5
			}
			ui.Column(c).Padding(pad, 0, 0, 0).Children(func() {
				renderInlineMath(c, b.Inlines, s, HeadingSize(b.Level, s.o.BodyFontSize), headingWeight(b.Level), 1.3)
			})
			return
		}
		el := ui.RichText(c, uiSpans(c, b.Inlines, s)...).
			FontSize(HeadingSize(b.Level, s.o.BodyFontSize)).
			FontWeight(headingWeight(b.Level)).Wrap().LineHeight(1.3)
		switch b.Level {
		case 1:
			el.Padding(12, 0, 0, 0)
		case 2:
			el.Padding(8, 0, 0, 0)
		case 3:
			el.Padding(5, 0, 0, 0)
		}
	case Paragraph:
		ui.RichText(c, uiSpans(c, b.Inlines, s)...).
			FontSize(s.o.BodyFontSize).Wrap().LineHeight(1.75)
	case Code:
		renderCode(c, b, s)
	case Quote:
		ui.Column(c).Gap(6).Padding(0, 0, 0, 16).
			BorderWidth(0, 0, 0, 3).BorderColor(t.Border).
			Children(func() {
				renderBlocks(c, b.Children, depth+1, rstyle{o: s.o, muted: true, width: s.inlineWidth() - 19})
			})
	case List:
		renderList(c, b, depth, s)
	case Rule:
		ui.Box(c).FillWidth().Height(2).Background(t.Border)
	case HTML:
		if txt := htmlToText(b.Text); strings.TrimSpace(txt) != "" {
			ui.Text(c, txt).FontSize(s.o.BodyFontSize).Wrap().LineHeight(1.75)
		}
	case Table:
		renderTable(c, b, s)
	case Image:
		renderImage(c, b, s)
	case Math:
		if bmp := mathBitmap(b.Text, 18, t.Dark, true); bmp != nil {
			drawMathBlock(c, bmp, s)
			return
		}
		ui.Row(c).FillWidth().Justify(ui.Center).Padding(8, 0).Children(func() {
			ui.RichText(c, ui.Span{
				Text:   texToUnicode(b.Text),
				Font:   "Georgia",
				Italic: true,
				Size:   s.o.BodyFontSize + 1,
			}).TextAlign(ui.Center).Wrap().LineHeight(1.6)
		})
	}
}

// renderCode follows Typora's block: a soft filled panel, comfortable
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
	code := ui.Column(c).FillWidth().Gap(1).Padding(14, 16).Radius(4).
		Background(t.Surface)
	if s.o.OnCopyCode != nil {
		code.ContextMenu(func(m *ui.Menu) {
			if m.Item("复制代码").Chosen() {
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
				Font(s.o.MonoFont).FontSize(s.o.BodyFontSize - 2).
				LineHeight(1.55).Wrap()
		}
	})
}

func renderList(c *ui.Context, b Block, depth int, s rstyle) {
	t := c.Theme()
	indent := float32(depth) * s.o.IndentStep
	ui.Column(c).Gap(3).Padding(0, 0, 0, indent).Children(func() {
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
				ui.Column(c).Grow(1).Gap(3).Children(func() {
					if len(item.Inlines) > 0 {
						if spansHaveMath(item.Inlines) {
							renderInlineMath(c, item.Inlines, rstyle{o: s.o, muted: s.muted, width: s.inlineWidth() - indent - 30}, s.o.BodyFontSize, 0, 1.8)
						} else {
							ui.RichText(c, uiSpans(c, item.Inlines, s)...).
								FontSize(s.o.BodyFontSize).Wrap().LineHeight(1.75)
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
	ui.Column(c).FillWidth().Border(1, t.Border).Radius(4).Clip().Children(func() {
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
						BorderColor(t.Border)
					if head {
						box.Background(t.Surface)
					}
					if e := s.o.CellEdit; e != nil && e.Row == ri && e.Col == ci {
						box.Children(func() {
							inp := ui.TextInputBase(c, e.Value).AutoFocus().FillWidth().
								FontSize(s.o.BodyFontSize - 1).TextColor(t.Text).Label("单元格")
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
								renderInlineMath(c, cell, rstyle{o: s.o, muted: s.muted, width: w}, s.o.BodyFontSize-1, weight, 1.6)
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

// renderImage draws a standalone image from the document's folder. Remote
// images are not fetched: Typora can, a native renderer without a network
// stack shows the alt text instead.
func renderImage(c *ui.Context, b Block, s rstyle) {
	t := c.Theme()
	if isRemoteImage(b.Src) {
		if bmp := remoteBitmap(b.Src, s.o.OnAsync); bmp != nil {
			drawImage(c, bmp, s, b.ImgWidth, b.ImgHeight)
		} else {
			imagePlaceholder(c, b.Alt, t)
		}
		return
	}
	path := resolveImagePath(b.Src, s.o.BaseDir)
	if path == "" {
		imagePlaceholder(c, b.Alt, t)
		return
	}
	bmp := loadBitmap(path)
	if bmp == nil {
		imagePlaceholder(c, b.Alt, t)
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

func imagePlaceholder(c *ui.Context, alt string, t *ui.Theme) {
	if strings.TrimSpace(alt) == "" {
		alt = "图片"
	}
	ui.Row(c).FillWidth().Padding(12).Radius(4).Background(t.Surface).
		Justify(ui.Center).Children(func() {
		ui.Text(c, "🖼 "+alt).FontSize(13).TextColor(t.TextMuted)
	})
}

func isRemoteImage(src string) bool {
	return strings.HasPrefix(src, "http://") || strings.HasPrefix(src, "https://")
}

type remoteEntry struct {
	mu      sync.Mutex
	bmp     *ui.Bitmap
	loading bool
	failed  bool
}

var remoteImages sync.Map

// remoteBitmap returns a fetched remote image, or nil while it loads. The
// first caller starts the fetch; OnAsync repaints the view when it lands.
func remoteBitmap(src string, done func()) *ui.Bitmap {
	e := &remoteEntry{loading: true}
	actual, loaded := remoteImages.LoadOrStore(src, e)
	if loaded {
		e = actual.(*remoteEntry)
		e.mu.Lock()
		bmp := e.bmp
		e.mu.Unlock()
		return bmp
	}
	go func() {
		fail := func() {
			e.mu.Lock()
			e.loading, e.failed = false, true
			e.mu.Unlock()
			if done != nil {
				done()
			}
		}
		client := &http.Client{Timeout: 15 * time.Second}
		resp, err := client.Get(src)
		if err != nil {
			fail()
			return
		}
		defer resp.Body.Close()
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			fail()
			return
		}
		data, err := io.ReadAll(io.LimitReader(resp.Body, 20<<20))
		if err != nil {
			fail()
			return
		}
		bmp, err := ui.DecodeBitmap(data)
		e.mu.Lock()
		e.loading = false
		if err != nil {
			e.failed = true
		} else {
			e.bmp = bmp
		}
		e.mu.Unlock()
		if done != nil {
			done()
		}
	}()
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

type imageEntry struct {
	mod  time.Time
	size int64
	bmp  *ui.Bitmap
}

var imageCache sync.Map

func loadBitmap(path string) *ui.Bitmap {
	st, err := os.Stat(path)
	if err != nil {
		return nil
	}
	if v, ok := imageCache.Load(path); ok {
		e := v.(imageEntry)
		if e.mod.Equal(st.ModTime()) && e.size == st.Size() {
			return e.bmp
		}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	bmp, err := ui.DecodeBitmap(data)
	if err != nil {
		return nil
	}
	imageCache.Store(path, imageEntry{mod: st.ModTime(), size: st.Size(), bmp: bmp})
	return bmp
}

// uiSpans converts model spans to ui spans, resolving links and code to the
// theme's accent and mono face.
func uiSpans(c *ui.Context, spans []Span, s rstyle) []ui.Span {
	t := c.Theme()
	out := make([]ui.Span, 0, len(spans))
	for _, sp := range spans {
		u := ui.Span{Text: sp.Text}
		if s.muted {
			u.Color = t.TextMuted
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
			u.Size = s.o.BodyFontSize - 2
			u.Background = t.Surface
		}
		if sp.Link != "" {
			u.Color = t.Accent
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

// HeadingSize is the native heading scale, shared with the block editor.
func HeadingSize(level int, body float32) float32 {
	switch level {
	case 1:
		return body * 2.1
	case 2:
		return body * 1.65
	case 3:
		return body * 1.4
	case 4:
		return body * 1.2
	case 5:
		return body
	default:
		return body * 0.875
	}
}

func headingWeight(level int) int {
	if level <= 2 {
		return 700
	}
	return 650
}

// htmlToText is the small HTML subset Typora documents commonly embed:
// paragraphs, headings, breaks, links and images. It keeps the readable
// text and drops the tags, so README-style HTML does not arrive as source
// code in the native renderer.
func htmlToText(s string) string {
	var b strings.Builder
	skip := 0
	for i := 0; i < len(s); {
		if s[i] != '<' {
			if skip == 0 {
				b.WriteByte(s[i])
			}
			i++
			continue
		}
		end := strings.IndexByte(s[i:], '>')
		if end < 0 {
			break
		}
		tag := strings.TrimSpace(s[i+1 : i+end])
		low := strings.ToLower(tag)
		switch {
		case strings.HasPrefix(low, "script"), strings.HasPrefix(low, "style"):
			skip++
		case strings.HasPrefix(low, "/script"), strings.HasPrefix(low, "/style"):
			if skip > 0 {
				skip--
			}
		case skip == 0 && strings.HasPrefix(low, "br"):
			b.WriteByte('\n')
		case skip == 0 && strings.HasPrefix(low, "img"):
			if alt := htmlAttr(tag, "alt"); alt != "" {
				b.WriteString("🖼 ")
				b.WriteString(alt)
				b.WriteByte('\n')
			}
		case skip == 0 && (strings.HasPrefix(low, "/p") || strings.HasPrefix(low, "/div") ||
			strings.HasPrefix(low, "/h") || strings.HasPrefix(low, "/li") ||
			strings.HasPrefix(low, "/tr") || strings.HasPrefix(low, "hr")):
			b.WriteByte('\n')
		}
		i += end + 1
	}
	txt := html.UnescapeString(b.String())
	lines := strings.Split(txt, "\n")
	out := make([]string, 0, len(lines))
	blank := 0
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			blank++
			if blank > 1 {
				continue
			}
		} else {
			blank = 0
		}
		out = append(out, line)
	}
	return strings.TrimSpace(strings.Join(out, "\n"))
}

func htmlAttr(tag, name string) string {
	low := strings.ToLower(tag)
	i := strings.Index(low, name+"=")
	if i < 0 {
		return ""
	}
	rest := tag[i+len(name)+1:]
	if rest == "" {
		return ""
	}
	q := rest[0]
	if q != '"' && q != '\'' {
		end := strings.IndexAny(rest, " \t>")
		if end < 0 {
			return rest
		}
		return rest[:end]
	}
	end := strings.IndexByte(rest[1:], q)
	if end < 0 {
		return ""
	}
	return rest[1 : end+1]
}

// LoadImage returns the bitmap of an image a document links to: a file
// relative to baseDir, a data: URL, or a remote image, fetched once in the
// background (nil until then; onLoad runs on another goroutine when it
// lands). It returns nil for what cannot be read.
func LoadImage(src, baseDir string, onLoad func()) *ui.Bitmap {
	src = strings.TrimSpace(src)
	switch {
	case src == "":
		return nil
	case isRemoteImage(src):
		return remoteBitmap(src, onLoad)
	case strings.HasPrefix(src, "data:"):
		return dataBitmap(src)
	}
	path := resolveImagePath(src, baseDir)
	if path == "" {
		return nil
	}
	return loadBitmap(path)
}

var dataImages sync.Map

// dataBitmap decodes a base64 data: URL, as pasted images are kept.
func dataBitmap(src string) *ui.Bitmap {
	if v, ok := dataImages.Load(src); ok {
		bmp, _ := v.(*ui.Bitmap)
		return bmp
	}
	var bmp *ui.Bitmap
	if comma := strings.IndexByte(src, ','); comma > 0 && strings.Contains(src[:comma], ";base64") {
		if data, err := base64.StdEncoding.DecodeString(strings.TrimSpace(src[comma+1:])); err == nil {
			bmp, _ = ui.DecodeBitmap(data)
		}
	}
	dataImages.Store(src, bmp)
	return bmp
}

// HTMLOnlyImage reports the image of an HTML block that holds nothing but
// one <img>, as READMEs center their logos, with its width and height.
func HTMLOnlyImage(html string) (src, alt, title string, w, h float32, ok bool) {
	return htmlOnlyImage(html)
}
