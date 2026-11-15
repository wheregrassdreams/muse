package markdown

import (
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

// View draws a document with the ui package alone; the headless Tester
// proves the native renderer shows every block without a window.
func TestViewRendersDocument(t *testing.T) {
	d := Parse("# Title\n\nHello **world**.\n\n- alpha\n- beta\n\n> quoted line\n\n```go\nfmt.Println(1)\n```\n")
	tt := ui.NewTester(func(c *ui.Context) { View(c, d, Options{}) }, 640, 480)

	for _, want := range []string{"Title", "world", "alpha", "beta", "quoted line", "fmt.Println(1)"} {
		if !tt.HasText(want) {
			t.Errorf("view does not show %q; texts = %v", want, tt.Texts())
		}
	}
}

func TestViewHandlesNilDocument(t *testing.T) {
	tt := ui.NewTester(func(c *ui.Context) { View(c, nil, Options{}) }, 320, 200)
	if len(tt.Texts()) != 0 {
		t.Errorf("nil document drew %v, want nothing", tt.Texts())
	}
}

func TestViewRendersTableTasksAndHTML(t *testing.T) {
	d := Parse("| name | state |\n| :--- | ---: |\n| mdoc | native |\n\n- [x] shipped\n\n<div><p>HTML <strong>text</strong></p><img alt=\"logo\"></div>\n")
	tt := ui.NewTester(func(c *ui.Context) {
		View(c, d, Options{ContentWidth: 600})
	}, 700, 500)

	for _, want := range []string{"name", "state", "mdoc", "native", "shipped", "HTML", "text", "logo"} {
		if !tt.HasText(want) {
			t.Errorf("view does not show %q; texts = %v", want, tt.Texts())
		}
	}
	if tt.HasText("<div>") || tt.HasText("<strong>") {
		t.Errorf("HTML tags leaked into the view: %v", tt.Texts())
	}
}

func TestRenderImageFallsBackWhenMissing(t *testing.T) {
	base := t.TempDir()
	d := Parse("![missing](附件/nope.png)\n")
	var opened string
	tt := ui.NewTester(func(c *ui.Context) {
		View(c, d, Options{BaseDir: base, OnImagePreview: func(src string) { opened = src }})
	}, 500, 300)
	want := MissingImageMessage("附件/nope.png")
	if !tt.HasText(want) {
		t.Fatalf("missing image alt not shown; texts = %v", tt.Texts())
	}
	if err := tt.Click(want); err != nil {
		t.Fatal(err)
	}
	if wantPath := filepath.Join(base, "附件", "nope.png"); opened != wantPath {
		t.Fatalf("missing-image click opened %q, want %q", opened, wantPath)
	}
	if dir := os.Getenv("MISSING_IMAGE_SHOTS"); dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		f, err := os.Create(filepath.Join(dir, "document-preview.png"))
		if err != nil {
			t.Fatal(err)
		}
		if err := png.Encode(f, tt.Image()); err != nil {
			_ = f.Close()
			t.Fatal(err)
		}
		if err := f.Close(); err != nil {
			t.Fatal(err)
		}
	}
}

func TestRenderBareAttachmentFallsBackToVaultAssetRoot(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", "testdata", "vault-fixture"))
	if err != nil {
		t.Fatal(err)
	}
	base := filepath.Join(root, "Projects", "Atlas")
	src := "![Bare attachment](landscape.webp)\n"
	doc := Parse(src)
	done := make(chan struct{}, 1)
	tt := ui.NewTester(func(c *ui.Context) {
		View(c, doc, Options{
			BaseDir:   base,
			AssetRoot: root,
			OnAsync: func() {
				select {
				case done <- struct{}{}:
				default:
				}
			},
		})
	}, 520, 320)

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("vault-root attachment did not finish loading")
	}
	tt.Frame()
	if want := MissingImageMessage("landscape.webp"); tt.HasText(want) {
		t.Fatalf("bare vault attachment rendered as missing: %q", tt.Texts())
	}
	if got, want := ResolveImageSource("landscape.webp", base, root), filepath.Join(root, "Attachments", "landscape.webp"); got != want {
		t.Fatalf("bare vault attachment resolved to %q, want %q", got, want)
	}
}

func TestRemoteImageLoadsAndCaches(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_ = png.Encode(w, image.NewRGBA(image.Rect(0, 0, 2, 2)))
	}))
	defer srv.Close()

	done := make(chan struct{}, 1)
	if bmp := remoteBitmap(srv.URL, func() { done <- struct{}{} }); bmp != nil {
		t.Fatal("remote image was returned before it loaded")
	}
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("remote image did not finish loading")
	}
	bmp := remoteBitmap(srv.URL, nil)
	if bmp == nil {
		t.Fatal("remote image was not cached")
	}
	if w, h := bmp.Size(); w != 2 || h != 2 {
		t.Fatalf("remote image is %dx%d", w, h)
	}
}

func TestHighlightLinePreservesText(t *testing.T) {
	lines := []struct {
		line, lang string
	}{
		{`func main() { fmt.Println("hi") }`, "go"},
		{`const x = 42; // comment`, "js"},
		{`def f(x): return x  # note`, "python"},
		{`<a href="x">link</a>`, "html"},
		{`.card { color: #fff; }`, "css"},
		{`{"a": true, "b": 2}`, "json"},
		{`plain text`, "unknown"},
	}
	for _, tt := range lines {
		var b strings.Builder
		for _, s := range HighlightLine(tt.line, tt.lang, false) {
			b.WriteString(s.Text)
		}
		if b.String() != tt.line {
			t.Errorf("%s: %q -> %q", tt.lang, tt.line, b.String())
		}
	}
}

func TestViewRendersMath(t *testing.T) {
	d := Parse(`energy $E = mc^2$ and

$$\frac{a}{b} + \alpha$$
`)
	tt := ui.NewTester(func(c *ui.Context) { View(c, d, Options{}) }, 700, 400)
	for _, want := range []string{"mc²"} {
		if !tt.HasText(want) {
			t.Errorf("view does not show %q; texts = %v", want, tt.Texts())
		}
	}
}

func TestMathBitmapTypesetsAndCaches(t *testing.T) {
	bmp := mathBitmap(`\frac{a}{b}`, 18, false, false)
	if bmp == nil {
		t.Fatal("math bitmap is nil")
	}
	if w, h := bmp.Size(); w <= 0 || h <= 0 {
		t.Fatalf("math bitmap size = %dx%d", w, h)
	}
	if again := mathBitmap(`\frac{a}{b}`, 18, false, false); again != bmp {
		t.Fatal("math bitmap was not cached")
	}
	if dark := mathBitmap(`\frac{a}{b}`, 18, true, false); dark == nil {
		t.Fatal("dark math bitmap is nil")
	}
}

func TestMatrixBitmap(t *testing.T) {
	expr := `\begin{pmatrix} a & b \\ c & d \end{pmatrix}`
	bmp := mathBitmap(expr, 18, false, false)
	if bmp == nil {
		t.Fatal("matrix bitmap is nil")
	}
	if w, h := bmp.Size(); w < 30 || h < 30 {
		t.Fatalf("matrix bitmap is %dx%d", w, h)
	}
	if mathBitmap(expr, 18, true, false) == nil {
		t.Fatal("dark matrix bitmap is nil")
	}
}
