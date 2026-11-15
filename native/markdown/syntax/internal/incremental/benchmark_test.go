package parser

import (
	"strings"
	"testing"
)

var syntaxBenchmarkDocument *Document

func benchmarkSource1MiB() string {
	const paragraph = "A large Markdown document keeps independent paragraphs available for bounded incremental edits.\n\n"
	return strings.Repeat(paragraph, (1<<20)/len(paragraph)+1)[:1<<20]
}

func BenchmarkParse1MiB(b *testing.B) {
	source := benchmarkSource1MiB()
	b.SetBytes(int64(len(source)))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		syntaxBenchmarkDocument = Parse(source)
	}
}

func BenchmarkEdit1ByteIn1MiB(b *testing.B) {
	source := benchmarkSource1MiB()
	doc := New(Default)
	doc.Set(source)
	const offset = (1 << 20) / 2
	if source[offset] != 'a' {
		b.Fatalf("benchmark edit offset has %q, want 'a'", source[offset])
	}
	current := byte('a')
	b.SetBytes(1 << 20)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		next := byte('b')
		if current == 'b' {
			next = 'a'
		}
		doc.Edit(offset, offset+1, string(next))
		current = next
	}
	syntaxBenchmarkDocument = doc
}
