package nativeui

import (
	"image"
	"image/png"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

// fadeDoc is long enough to scroll, and dense: a line of text sits under
// the header whenever it scrolls.
var fadeDoc = "## 顶部模糊\n\n" + strings.Repeat("这是一行很长的正文，写满整个宽度，用来观察滚动时头部下面的模糊与淡出。", 120)

// fadeShot writes the window's image when MUSE_SHOTS says where.
func fadeShot(t *testing.T, img *image.RGBA, name string) {
	t.Helper()
	dir := os.Getenv("MUSE_SHOTS")
	if dir == "" {
		return
	}
	f, err := os.Create(filepath.Join(dir, name+".png"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
}

// fadeApp opens fadeDoc in the flat style: the editor's rect, and the
// tester at a window where the panel is open.
func fadeApp(t *testing.T, scroll int) (*App, *ui.Tester, ui.Rect) {
	t.Helper()
	a := New()
	a.ed.SetSource(fadeDoc)
	tt := ui.NewTester(a.View, 1200, 800)
	tt.Frame()
	tt.Frame()
	if scroll > 0 {
		a.ed.ScrollToOffset(scroll)
		tt.Frame()
		tt.Frame()
	}
	ed, ok := tt.Find("文档")
	if !ok {
		t.Fatal("no editor")
	}
	return a, tt, ed
}

// band is what a row of the window looks like: its mean, how much it
// changes between its pixels (a sharp line of text is rough, a blurred one
// smooth), and its darkest pixel.
type band struct {
	mean, rough float32
	min         uint8
}

func bandOf(img *image.RGBA, ed ui.Rect, y int) band {
	var b band
	b.min = 255
	x0, x1 := int(ed.X)+20, int(ed.X+ed.W)-20
	if x1 <= x0 || y < 0 || y >= img.Bounds().Dy() {
		return b
	}
	var sum, rough, prev float32
	for x := x0; x < x1; x++ {
		c := img.RGBAAt(x, y)
		l := 0.299*float32(c.R) + 0.587*float32(c.G) + 0.114*float32(c.B)
		sum += l
		if x > x0 {
			rough += absf(l - prev)
		}
		prev = l
		if c.R < b.min {
			b.min = c.R
		}
	}
	b.mean, b.rough = sum/float32(x1-x0), rough
	return b
}

// firstTextRow returns the first row of img in [from, to) that is a line
// of text: rough enough across the editor's width, or -1.
func firstTextRow(img *image.RGBA, ed ui.Rect, from, to int) int {
	for y := from; y < to; y++ {
		if bandOf(img, ed, y).rough > 3000 {
			return y
		}
	}
	return -1
}

// bandDiff is how much two frames differ, per pixel and channel, across the
// editor's width and the rows from..to.
func bandDiff(a, b *image.RGBA, ed ui.Rect, from, to int) float32 {
	var sum, n float32
	x0, x1 := int(ed.X)+20, int(ed.X+ed.W)-20
	for y := from; y < to; y++ {
		for x := x0; x < x1; x++ {
			p, q := a.RGBAAt(x, y), b.RGBAAt(x, y)
			sum += absf(float32(p.R)-float32(q.R)) + absf(float32(p.G)-float32(q.G)) + absf(float32(p.B)-float32(q.B))
			n += 3
		}
	}
	if n == 0 {
		return 0
	}
	return sum / n
}

func absf(v float32) float32 {
	if v < 0 {
		return -v
	}
	return v
}

// TestHeaderFadeScrollsUnderTheHeader is the fade's point: a line the
// document scrolls under the header comes out out of focus, and the
// document is untouched where the fade ends.
func TestHeaderFadeScrollsUnderTheHeader(t *testing.T) {
	a, tt, ed := fadeApp(t, 1300)
	on := tt.Image()
	fadeShot(t, on, "header-fade-on")

	a.headerFade = false
	tt.Frame()
	tt.Frame()
	off := tt.Image()
	fadeShot(t, off, "header-fade-off")

	// The strip paints over what it covers: the frames differ across the
	// header and the fade, and not at all below them.
	d := bandDiff(on, off, ed, 0, int(editorHeaderH+headerFadeReach))
	if d < 8 {
		t.Fatalf("the strip paints nothing over the header: mean difference %v", d)
	}
	// Past the fade the two frames are the same frame: the strip ends, and
	// nothing of it shows where the document is sharp again.
	below := int(editorHeaderH+headerFadeReach) + 10
	if a, b := bandOf(on, ed, below), bandOf(off, ed, below); a != b {
		t.Fatalf("the strip shows below the fade at y=%d: %+v against %+v", below, a, b)
	}
}

// TestHeaderFadeStartsAtTheTopOfTheDocument keeps the document's first line
// readable at rest: with nothing scrolled under the header, the strip draws
// nothing at all.
func TestHeaderFadeStartsAtTheTopOfTheDocument(t *testing.T) {
	a, tt, ed := fadeApp(t, 0)
	on := tt.Image()
	fadeShot(t, on, "header-fade-top")

	a.headerFade = false
	tt.Frame()
	tt.Frame()
	off := tt.Image()

	if a.ed.ScrollY() != 0 {
		t.Fatalf("scrolled at rest: %v", a.ed.ScrollY())
	}
	for y := int(editorHeaderH) + 4; y < 220; y += 4 {
		if a, b := bandOf(on, ed, y), bandOf(off, ed, y); a != b {
			t.Fatalf("the fade shows at rest at y=%d: %+v against %+v", y, a, b)
		}
	}
}

// TestHeaderFadeOffCoversTheHeader: with the fade off the header is a solid
// band, and the document starts sharp at its edge.
func TestHeaderFadeOffCoversTheHeader(t *testing.T) {
	a, tt, ed := fadeApp(t, 1300)
	a.headerFade = false
	tt.Frame()
	tt.Frame()
	img := tt.Image()
	fadeShot(t, img, "header-fade-off-solid")
	h := int(editorHeaderH)
	if b := bandOf(img, ed, h-2); b.rough > 0 || b.min < 240 {
		t.Fatalf("the off header is not solid: %+v", b)
	}
	if b := bandOf(img, ed, h+2); b.rough < 1000 {
		t.Fatalf("the document does not start at the header's edge: %+v", b)
	}
}

// TestHeaderFadeKeepsTheDocument: the fade is a view. Turning it on and off
// leaves the source, the caret and the scroll where they were.
func TestHeaderFadeKeepsTheDocument(t *testing.T) {
	a, tt, _ := fadeApp(t, 900)
	caret, src := a.ed.Caret(), a.ed.Source()
	y := a.ed.ScrollY()
	a.headerFade = false
	tt.Frame()
	tt.Frame()
	a.headerFade = true
	tt.Frame()
	tt.Frame()
	if a.ed.Caret() != caret || a.ed.Source() != src || a.ed.ScrollY() != y {
		t.Fatalf("toggling moved the document: caret %d→%d, scroll %v→%v, source %d→%d",
			caret, a.ed.Caret(), y, a.ed.ScrollY(), len(src), len(a.ed.Source()))
	}
}

// TestHeaderFadeDark is the same strip in the dark theme: a line under the
// header is out of focus, and the document is untouched where the fade ends.
func TestHeaderFadeDark(t *testing.T) {
	a := New()
	a.themeMode = 1
	a.ed.SetSource(fadeDoc)
	tt := ui.NewTester(a.View, 1000, 700)
	tt.Frame()
	tt.Frame()
	a.ed.ScrollToOffset(1300)
	tt.Frame()
	tt.Frame()
	ed, ok := tt.Find("文档")
	if !ok {
		t.Fatal("no editor")
	}
	on := tt.Image()
	fadeShot(t, on, "header-fade-dark")

	a.headerFade = false
	tt.Frame()
	tt.Frame()
	off := tt.Image()

	d := bandDiff(on, off, ed, 0, int(editorHeaderH+headerFadeReach))
	if d < 8 {
		t.Fatalf("the strip paints nothing over the header: mean difference %v", d)
	}
	below := int(editorHeaderH+headerFadeReach) + 10
	if p, q := bandOf(on, ed, below), bandOf(off, ed, below); p != q {
		t.Fatalf("the strip shows below the fade at y=%d: %+v against %+v", below, p, q)
	}
}

// TestHeaderFadeIsTransparent is why the strip's veil is light: over a
// callout's blue a veil of the page's color is a white band with an edge,
// which is the opposite of what a fade is for. A little of it takes the
// edge off the copy; much of it paints the page over the document.
func TestHeaderFadeIsTransparent(t *testing.T) {
	body := strings.Repeat("> callout 的一行内容，用来把整页铺满蓝色。\n>\n", 100)
	a := New()
	a.ed.SetSource("> [!NOTE] 提示\n>\n" + body)
	tt := ui.NewTester(a.View, 1000, 700)
	tt.Frame()
	tt.Frame()
	a.ed.ScrollToOffset(len(body) / 2)
	tt.Frame()
	tt.Frame()
	ed, ok := tt.Find("文档")
	if !ok {
		t.Fatal("no editor")
	}
	if a.ed.ScrollY() <= 0 {
		t.Fatal("not scrolled")
	}
	img := tt.Image()
	fadeShot(t, img, "header-fade-callout")
	// The middle of the band, a row across the callout: its blue is the
	// median, as the text's own pixels are few. The veil of 0.3 leaves
	// about 12 of its 17; one near 1 leaves 4.
	var blue []int
	y := int(editorHeaderH) + 6
	for x := int(ed.X) + 8; x < int(ed.X+ed.W)-8; x++ {
		c := img.RGBAAt(x, y)
		blue = append(blue, int(c.B)-int(c.R))
	}
	sort.Ints(blue)
	if med := blue[len(blue)/2]; med < 10 {
		t.Fatalf("the callout's color is washed out under the header: median blue %d", med)
	}
}

// TestHeaderFadeReduceTransparency: asked for opaque surfaces, the header is
// a gradient of its own color with no backdrop to blur, and the document
// under it is covered, not shown.
func TestHeaderFadeReduceTransparency(t *testing.T) {
	_, tt, ed := fadeApp(t, 1300)
	tt.SetPreferences(ui.Preferences{ReduceTransparency: true})
	tt.Frame()
	tt.Frame()
	img := tt.Image()
	fadeShot(t, img, "header-fade-reduced")
	if b := bandOf(img, ed, int(editorHeaderH)-4); b.min < 200 {
		t.Fatalf("the document shows through the reduced header: %+v", b)
	}
}

// TestHeaderFadeSetting: the switch in 外观 turns the fade, and the row's
// label does too.
func TestHeaderFadeSetting(t *testing.T) {
	a := New()
	a.openSettings()
	a.settingsTab = 0
	tt := ui.NewTester(a.View, 1200, 800)
	tt.Frame()
	fadeShot(t, tt.Image(), "header-fade-setting")
	if !a.headerFade {
		t.Fatal("the fade is off by default")
	}
	if err := tt.Click("顶部模糊"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.headerFade {
		t.Fatal("the row's label did not turn the fade off")
	}
	if err := tt.Click("顶部模糊"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !a.headerFade {
		t.Fatal("the row's label did not turn the fade back on")
	}
}
