package editor

import (
	"encoding/base64"
	"io"
	"os"
	"strconv"
	"strings"
	"testing"

	syntax "github.com/wheregrassdreams/muse-editor/markdown/syntax"
)

func TestLargeDataImageSourceStaysAtomicWhenCaretRevealsImage(t *testing.T) {
	const sourcePrefix = "![image](data:image/png;base64,"
	src := sourcePrefix + strings.Repeat("A", syntax.LargeDataImageThreshold+1) + ")"
	e := New(src, Options{})
	if len(e.leaves) == 0 {
		t.Fatal("image did not produce a document leaf")
	}
	foundImage := false
	for _, in := range e.viewInlines(e.leaves[0]) {
		foundImage = foundImage || in.Kind == syntax.IncrementalImage
	}
	if !foundImage {
		t.Fatal("large data image was not parsed as an image")
	}
	e.focused = true
	e.setSessionHead(len(sourcePrefix) + 1)
	v := e.view(e.leaves[0], 720, false, syntax.IncrementalAlignNone)
	if !v.disp.hasObjects {
		t.Fatal("large data image lost its object presentation")
	}
	if got := len(v.disp.text); got > 4 {
		t.Fatalf("revealed large data image shaped %d display bytes; want one atomic object", got)
	}
	for _, off := range []int{e.leaves[0].start, e.leaves[0].end} {
		if _, ok := v.disp.srcToDisp(off - e.leaves[0].lineStart); !ok {
			t.Fatalf("image source boundary %d no longer maps to its object", off)
		}
	}
}

func TestLargeDataImageFirstStopSkipsBuildingTheInitialView(t *testing.T) {
	payload := strings.Repeat("A", syntax.LargeDataImageThreshold+8)
	for _, tc := range []struct {
		name string
		src  string
	}{
		{"empty alt", "![](data:image/png;base64," + payload + ")"},
		{"plain alt with line ending", "![pasted image](data:image/png;base64," + payload + ")\r\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := New(tc.src, Options{})
			if got := e.Caret(); got != 0 {
				t.Fatalf("initial caret = %d, want source start", got)
			}
			if len(e.leaves) == 0 || e.leaves[0].kind != kPara || e.leaves[0].view != nil {
				t.Fatal("initial caret lookup built a view for the opaque image")
			}
			if got := e.firstStop(); got != 0 || e.leaves[0].view != nil {
				t.Fatalf("firstStop() = %d or built a view, want source start without layout", got)
			}
		})
	}
}

func TestLargeDataImageInlineFastPathPreservesSourceAndAlt(t *testing.T) {
	const prefix = "![pasted image](data:image/png;base64,"
	src := prefix + strings.Repeat("A", syntax.LargeDataImageThreshold+4) + ")"
	e := New(src, Options{})
	ins := e.viewInlines(e.leaves[0])
	wantEnd := strings.LastIndex(src, ")") + 1
	wantDestStart := strings.Index(src, "data:image/")
	if len(ins) != 2 {
		t.Fatalf("viewInlines returned %d tokens, want image and alt", len(ins))
	}
	if in := ins[0]; in.Kind != syntax.IncrementalImage || in.Start != 0 || in.End != wantEnd || in.Dest != src[wantDestStart:wantEnd-1] || in.Next != 2 {
		t.Fatalf("image inline: kind=%v range=%d..%d dest=%d next=%d", in.Kind, in.Start, in.End, len(in.Dest), in.Next)
	}
	if in := ins[1]; in.Kind != syntax.IncrementalText || in.Text != "pasted image" || in.Start != 2 || in.End != len("![pasted image") {
		t.Fatalf("alt inline: %+v", in)
	}
	if !e.pictureBlock(e.leaves[0]) {
		t.Fatal("cached large image projection was not recognized as a standalone picture")
	}
	if len(e.p.ImageInlines()) != len(ins) || !e.p.IsIdentityMask() {
		t.Fatalf("source initialization did not retain the precomputed image tokens and source mask: tokens=%d identity=%v", len(e.p.ImageInlines()), e.p.IsIdentityMask())
	}
}

func TestLargeDataImageOnlySourceRequiresAnExactASCIIImageDocument(t *testing.T) {
	payload := strings.Repeat("A", syntax.LargeDataImageThreshold+4)
	for _, tc := range []struct {
		name string
		src  string
		want bool
	}{
		{"plain image", "![image](data:image/png;base64," + payload + ")\n", true},
		{"trailing prose", "![image](data:image/png;base64," + payload + ")\n中文", false},
		{"unicode alt", "![图片](data:image/png;base64," + payload + ")\n", false},
		{"payload whitespace", "![image](data:image/png;base64," + payload[:len(payload)/2] + " " + payload[len(payload)/2:] + ")\n", false},
		{"payload newline", "![image](data:image/png;base64," + payload[:len(payload)/2] + "\n" + payload[len(payload)/2:] + ")\n", false},
		{"alt newline", "![image\nalt](data:image/png;base64," + payload + ")\n", false},
		{"header newline", "![image](data:image/png;\nbase64," + payload + ")\n", false},
		{"alt comment", "![image%%hidden](data:image/png;base64," + payload + ")\n", false},
		{"header comment", "![image](data:image/png;%%hidden;base64," + payload + ")\n", false},
		{"payload comment", "![image](data:image/png;base64," + payload[:len(payload)/2] + "%%" + payload[len(payload)/2:] + ")\n", false},
		{"invalid base64", "![image](data:image/png;base64," + payload[:len(payload)-4] + "!!!!)\n", false},
		{"multiple lines", "![image](data:image/png;base64," + payload + ")\n\nnext", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, got := syntax.LargeDataImageOnlySource(tc.src)
			if got != tc.want {
				t.Fatalf("largeDataImageOnlySource() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestLargeDataImageInlineFastPathFallsBackForOtherSyntax(t *testing.T) {
	for _, src := range []string{
		"![**bold**](data:image/png;base64," + strings.Repeat("A", syntax.LargeDataImageThreshold+1) + ")",
		"![image](data:image/png;base64," + strings.Repeat("A", syntax.LargeDataImageThreshold+1) + ") trailing",
		"![image](data:image/png," + strings.Repeat("A", syntax.LargeDataImageThreshold+1) + ")",
		"![image](data:image/png;base64," + strings.Repeat("A", syntax.LargeDataImageThreshold+1) + "(nested))",
		"![image](data:image/png;base64," + strings.Repeat("A", syntax.LargeDataImageThreshold/2) + " " + strings.Repeat("A", syntax.LargeDataImageThreshold/2+1) + ")",
		"![image](data:image/png;base64," + strings.Repeat("A", syntax.LargeDataImageThreshold/2) + "\\" + strings.Repeat("A", syntax.LargeDataImageThreshold/2+1) + ")",
	} {
		e := New(src, Options{})
		if _, ok := syntax.LargeDataImageInlines(src, e.leaves[0].start, e.leaves[0].end); ok {
			t.Fatalf("fast path accepted unsupported syntax with %d source bytes", len(src))
		}
	}
}

func TestLargeDataImageDestinationThreshold(t *testing.T) {
	payload := strings.Repeat("A", syntax.LargeDataImageThreshold+1)
	for _, tc := range []struct {
		name string
		dest string
		want bool
	}{
		{"large-base64", "data:image/png;base64," + payload, true},
		{"case-insensitive-header", "DATA:IMAGE/PNG;BASE64," + payload, true},
		{"small-base64", "data:image/png;base64,short", false},
		{"non-image-data", "data:text/plain;base64," + payload, false},
		{"non-base64-image", "data:image/svg+xml," + payload, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := syntax.IsLargeDataImageDestination(tc.dest); got != tc.want {
				t.Fatalf("IsLargeDataImageDestination() = %t, want %t", got, tc.want)
			}
		})
	}
}

func BenchmarkStressLargeDataImageInlineScan(b *testing.B) {
	_, src := stressBase64MarkdownFixture(b, 32<<20)
	e := New(src, Options{})
	leaf := e.leaves[0]
	b.ReportAllocs()
	b.SetBytes(int64(len(src)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		inlines, ok := syntax.LargeDataImageInlines(src, leaf.start, leaf.end)
		if !ok || len(inlines) == 0 {
			b.Fatal("large data image fast path rejected the stress fixture")
		}
	}
}

func BenchmarkStressLargeDataImageBase64Validation(b *testing.B) {
	_, src := stressBase64MarkdownFixture(b, 32<<20)
	comma := strings.IndexByte(src, ',')
	close := strings.IndexByte(src[comma+1:], ')') + comma + 1
	payload := src[comma+1 : close]
	b.ReportAllocs()
	b.SetBytes(int64(len(payload)))
	b.ReportMetric(float64(base64.StdEncoding.DecodedLen(len(payload))), "validation-scratch-B")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		decoded, err := base64.StdEncoding.Strict().DecodeString(payload)
		if err != nil || len(decoded) == 0 {
			b.Fatalf("Base64 validation failed: decoded=%d err=%v", len(decoded), err)
		}
	}
}

func BenchmarkStressLargeDataImageBase64StreamValidation(b *testing.B) {
	_, src := stressBase64MarkdownFixture(b, 32<<20)
	comma := strings.IndexByte(src, ',')
	close := strings.IndexByte(src[comma+1:], ')') + comma + 1
	payload := src[comma+1 : close]
	b.ReportAllocs()
	b.SetBytes(int64(len(payload)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		decoder := base64.NewDecoder(base64.StdEncoding.Strict(), strings.NewReader(payload))
		if n, err := io.Copy(io.Discard, decoder); err != nil || n == 0 {
			b.Fatalf("stream Base64 validation failed: decoded=%d err=%v", n, err)
		}
	}
}

func BenchmarkStressLargeDataImageOnlySource(b *testing.B) {
	payloadBytes := 32 << 20
	if raw := os.Getenv("MUSE_STRESS_BASE64_BYTES"); raw != "" {
		var err error
		payloadBytes, err = strconv.Atoi(raw)
		if err != nil || payloadBytes <= 0 {
			b.Fatalf("MUSE_STRESS_BASE64_BYTES must be a positive byte count, got %q", raw)
		}
	}
	_, src := stressBase64MarkdownFixture(b, payloadBytes)
	b.SetBytes(int64(len(src)))
	for _, tc := range []struct {
		name string
		fn   func(string) ([]syntax.IncrementalInline, bool)
	}{
		{name: "current", fn: syntax.LargeDataImageOnlySource},
		{name: "whole-source-newline-prefilter", fn: largeDataImageOnlySourceWithNewlinePrefilter},
	} {
		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if _, ok := tc.fn(src); !ok {
					b.Fatal("image source was not recognized")
				}
			}
		})
	}
}

// largeDataImageOnlySourceWithNewlinePrefilter is the former full-source
// line check, kept as an A/B reference for the single-line fast path.
func largeDataImageOnlySourceWithNewlinePrefilter(src string) ([]syntax.IncrementalInline, bool) {
	if len(src) <= syntax.LargeDataImageThreshold || !strings.HasPrefix(src, "![") {
		return nil, false
	}
	end := len(src)
	if end > 0 && src[end-1] == '\n' {
		end--
		if end > 0 && src[end-1] == '\r' {
			end--
		}
	}
	if end <= 0 || strings.IndexByte(src[:end], '\r') >= 0 || strings.IndexByte(src[:end], '\n') >= 0 {
		return nil, false
	}
	inlines, ok := syntax.LargeDataImageOnlySource(src)
	if !ok || len(inlines) == 0 || inlines[0].Start != 0 || inlines[0].End != end {
		return nil, false
	}
	for _, inline := range inlines[1:] {
		if inline.Kind != syntax.IncrementalText {
			continue
		}
		for i := 0; i < len(inline.Text); i++ {
			if inline.Text[i] >= 0x80 {
				return nil, false
			}
		}
	}
	return inlines, true
}
