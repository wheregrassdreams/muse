package navigation

import (
	"reflect"
	"strings"
	"sync"
	"testing"
	"unicode/utf8"
)

func TestDefaultASCIIPathDoesNotLoadDictionary(t *testing.T) {
	p := &lazyGSEProvider{}
	if spans := p.Segment([]byte("English 123")); spans != nil {
		t.Fatalf("default generic path should return nil spans, got %+v", spans)
	}
	if p.ready || p.err != nil {
		t.Fatalf("ASCII path loaded dictionary: ready=%v err=%v", p.ready, p.err)
	}
}

func TestContainsHanStringSkipsASCIIAndFindsHanAfterUTF8(t *testing.T) {
	for _, tc := range []struct {
		text string
		want bool
	}{
		{text: "ASCII 123 `QUFB`", want: false},
		{text: "emoji 👩‍💻 then 漢字", want: true},
		{text: "emoji 👩‍💻 without Han", want: false},
	} {
		if got := containsHanString(tc.text); got != tc.want {
			t.Errorf("containsHanString(%q) = %v, want %v", tc.text, got, tc.want)
		}
	}
}

func TestContainsHanContentSkipsLargeBase64ImagePayload(t *testing.T) {
	src := "![image](data:image/png;base64," + strings.Repeat("A", 1<<20) + ")"
	if containsHanContent(src) {
		t.Fatal("ASCII Base64 payload was treated as Han text")
	}
	if !containsHanContent(src + " 后文") {
		t.Fatal("Han text after an image payload was missed")
	}
	if !containsHanContent("标题汉字 " + src) {
		t.Fatal("Han text before an image payload was missed")
	}
}

func TestContainsHanContentStopsImageSkippingAtLineEnd(t *testing.T) {
	for _, src := range []string{
		"![image](data:image/png;base64,AAAA\n中文)",
		"![image](data:image/png;base64,AAAA\r中文)",
	} {
		if !containsHanContent(src) {
			t.Fatalf("Han text after a line break inside a data destination was skipped: %q", src)
		}
	}
}

func TestTokensUseUTF8ByteRangesAndRealChineseWords(t *testing.T) {
	text := "中文分词，go语言 123 👩‍💻 e\u0301"
	idx := New(text)
	got := make([]string, len(idx.Tokens()))
	for i, tok := range idx.Tokens() {
		if tok.Start < 0 || tok.End > len(text) || tok.Start >= tok.End {
			t.Fatalf("invalid token range: %+v for %q", tok, text)
		}
		got[i] = text[tok.Start:tok.End]
	}
	if !reflect.DeepEqual(got[:3], []string{"中文", "分词", "，"}) {
		t.Fatalf("Chinese should use dictionary segmentation; first tokens = %#v", got)
	}
	for i := 1; i < len(idx.Tokens()); i++ {
		if idx.Tokens()[i-1].End > idx.Tokens()[i].Start {
			t.Fatalf("overlapping tokens: %+v then %+v", idx.Tokens()[i-1], idx.Tokens()[i])
		}
	}
	if got[len(got)-3] != "👩‍💻" || got[len(got)-1] != "e\u0301" {
		t.Fatalf("emoji and combining sequence must remain whole: %#v", got)
	}
}

func TestNavigationAndTargetQuery(t *testing.T) {
	text := "中文 分词, hello 42"
	idx := New(text)
	if got, ok := idx.NextStart(0); !ok || got != len("中文 ") {
		t.Fatalf("NextStart(0) = %d, %v", got, ok)
	}
	if got, ok := idx.PrevStart(len(text)); !ok || text[got:] != "42" {
		t.Fatalf("PrevStart(end) = %d, %v (%q)", got, ok, text[got:])
	}
	targets := idx.Targets(Range{Start: 1, End: len(text)}, Forward, KindWord, KindNumber)
	if len(targets) != 3 || targets[0].Start != len("中文 ") || targets[1].Start != len("中文 分词, ") || targets[2].Start != len("中文 分词, hello ") {
		t.Fatalf("filtered forward targets = %+v", targets)
	}
	backward := idx.Targets(Range{Start: 0, End: len(text)}, Backward, KindWord)
	if len(backward) != 3 || backward[0].Start != len("中文 分词, ") || backward[2].Start != 0 {
		t.Fatalf("filtered backward targets = %+v", backward)
	}
	if got := idx.Targets(Range{Start: 0, End: len(text)}, 0); got != nil {
		t.Fatalf("zero direction should be rejected: %+v", got)
	}
}

func TestConcurrentDefaultProviderReads(t *testing.T) {
	_ = New("中文分词")
	const workers = 12
	var wg sync.WaitGroup
	for n := 0; n < workers; n++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 20 {
				idx := New("今天我们学习中文分词")
				if len(idx.Tokens()) == 0 {
					t.Error("concurrent segmenter returned no tokens")
				}
			}
		}()
	}
	wg.Wait()
}

func TestOriginalRangesKeepNormalizationSensitiveText(t *testing.T) {
	text := "éİＡ９龘龘👩‍💻e\u0301"
	idx := New(text)
	var reconstructed strings.Builder
	for _, tok := range idx.Tokens() {
		piece := text[tok.Start:tok.End]
		if !utf8.ValidString(piece) {
			t.Fatalf("token cuts UTF-8: %+v (%q)", tok, piece)
		}
		reconstructed.WriteString(piece)
	}
	if reconstructed.String() != text {
		t.Fatalf("tokens changed/dropped source: got %q want %q", reconstructed.String(), text)
	}
	unknown := "龘龘龘龘"
	oov := New(unknown).Tokens()
	if len(oov) != 4 {
		t.Fatalf("gse OOV fallback should emit four unknown Han graphemes, got %+v", oov)
	}
}

type stubProvider struct {
	spans []Span
	calls int
}

func (p *stubProvider) Segment(text []byte) []Span {
	p.calls++
	return p.spans
}

func TestProviderCanHandleOtherRunsAndCannotSplitGrapheme(t *testing.T) {
	p := &stubProvider{spans: []Span{{Start: 0, End: 3}, {Start: 6, End: 9}}}
	latin := NewWithProvider("hello 123", p)
	if p.calls != 2 || len(latin.Tokens()) == 0 {
		t.Fatalf("provider should receive generic linguistic runs, calls=%d", p.calls)
	}
	text := "汉\uFE0F字"
	idx := NewWithProvider(text, p)
	if p.calls != 3 {
		t.Fatalf("Han provider calls=%d", p.calls)
	}
	for _, tok := range idx.Tokens() {
		if tok.Start == 3 || tok.End == 3 {
			t.Fatalf("provider split a variation-selector grapheme: %+v", tok)
		}
	}
	joined := ""
	for _, tok := range idx.Tokens() {
		joined += text[tok.Start:tok.End]
	}
	if joined != text {
		t.Fatalf("invalid provider spans dropped source: %q", joined)
	}
}

func BenchmarkNewChineseIndex(b *testing.B) {
	text := "这是一段用于代表性分词基准的中文混合文本，包含Markdown编辑器和UTF8字节位置。" +
		"English words, identifiers_with_underscores, and numbers 12345. "
	b.ReportAllocs()
	b.SetBytes(int64(len(text)))
	_ = New(text) // load the one-time embedded dictionary outside the timed loop
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = New(text)
	}
}
