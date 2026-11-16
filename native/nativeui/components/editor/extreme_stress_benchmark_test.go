package editor

import (
	"os"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
	"unsafe"

	render "muse/nativeui/components/editor/render"
)

// BenchmarkStressBase64Heavy measures one cold frame plus one scroll frame on
// the single persistent Base64-heavy Markdown fixture selected by
// MUSE_STRESS_BASE64_PATH. It never creates another on-disk sample.
func BenchmarkStressBase64Heavy(b *testing.B) {
	path := os.Getenv("MUSE_STRESS_BASE64_PATH")
	if path == "" {
		b.Skip("set MUSE_STRESS_BASE64_PATH to the persistent Base64 stress note")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		b.Fatal(err)
	}
	if len(data) == 0 {
		b.Fatal("stress fixture is empty")
	}
	// New accepts an immutable string and never mutates its source. Reuse the
	// file buffer as that string so loading the fixture does not add another
	// 100 MiB copy to the memory measurement.
	source := unsafe.String(unsafe.SliceData(data), len(data))

	b.ReportAllocs()
	baseline := heapBaseline()
	var peak uint64
	first := make([]time.Duration, 0, b.N)
	scroll := make([]time.Duration, 0, b.N)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		e := New(source, Options{FontSize: 16})
		t := stressTester(e)
		start := time.Now()
		t.Frame()
		first = append(first, time.Since(start))
		e.scroll.Y = 600
		start = time.Now()
		t.Frame()
		scroll = append(scroll, time.Since(start))
		if heap := sampleHeapPeak(); heap > peak {
			peak = heap
		}
	}
	b.StopTimer()
	b.ReportMetric(float64(len(data)), "fixture-B")
	if len(first) > 0 {
		b.ReportMetric(float64(first[0].Nanoseconds()), "cold-first-frame-ns")
	}
	if len(scroll) > 0 {
		b.ReportMetric(float64(scroll[0].Nanoseconds()), "scroll-frame-ns")
	}
	reportHeap(b, baseline, peak)
	writeStressHeapProfile(b)
	runtime.KeepAlive(data)
}

// BenchmarkStressSingleLongCodeLine scales the layout path that a Base64
// payload can trigger when it appears as opaque text inside a fenced block.
// It is intentionally small enough to profile without another disk fixture.
func BenchmarkStressSingleLongCodeLine(b *testing.B) {
	payloadSize := 1 << 20
	if raw := os.Getenv("MUSE_STRESS_LONG_LINE_BYTES"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 || parsed > 100<<20 {
			b.Fatalf("invalid MUSE_STRESS_LONG_LINE_BYTES %q", raw)
		}
		payloadSize = parsed
	}
	source := "```text\n" + strings.Repeat("A", payloadSize) + "\n```\n"
	b.ReportAllocs()
	baseline := heapBaseline()
	var peak uint64
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		e := New(source, Options{FontSize: 16})
		t := stressTester(e)
		start := time.Now()
		t.Frame()
		b.ReportMetric(float64(time.Since(start).Nanoseconds()), "cold-first-frame-ns")
		if heap := sampleHeapPeak(); heap > peak {
			peak = heap
		}
	}
	b.StopTimer()
	b.ReportMetric(float64(len(source)), "fixture-B")
	reportHeap(b, baseline, peak)
	writeStressHeapProfile(b)
	runtime.KeepAlive(source)
}

func BenchmarkStressMultilineCodeBlock(b *testing.B) {
	payloadSize := 1 << 20
	if raw := os.Getenv("MUSE_STRESS_MULTILINE_BYTES"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 || parsed > 100<<20 {
			b.Fatalf("invalid MUSE_STRESS_MULTILINE_BYTES %q", raw)
		}
		payloadSize = parsed
	}
	const lineBytes = 1024
	chunk := strings.Repeat("A", lineBytes)
	var sourceBuilder strings.Builder
	sourceBuilder.Grow(payloadSize + payloadSize/lineBytes + 16)
	sourceBuilder.WriteString("```text\n")
	for written := 0; written < payloadSize; written += lineBytes {
		sourceBuilder.WriteString(chunk[:min(lineBytes, payloadSize-written)])
		sourceBuilder.WriteByte('\n')
	}
	sourceBuilder.WriteString("```\n")
	source := sourceBuilder.String()
	b.ReportAllocs()
	baseline := heapBaseline()
	var peak uint64
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		e := New(source, Options{FontSize: 16})
		t := stressTester(e)
		start := time.Now()
		t.Frame()
		b.ReportMetric(float64(time.Since(start).Nanoseconds()), "cold-first-frame-ns")
		if heap := sampleHeapPeak(); heap > peak {
			peak = heap
		}
	}
	b.StopTimer()
	b.ReportMetric(float64(len(source)), "fixture-B")
	reportHeap(b, baseline, peak)
	writeStressHeapProfile(b)
	runtime.KeepAlive(source)
}

func TestVeryLongCodeLineShapesBoundedCaretWindow(t *testing.T) {
	const payloadSize = 1 << 20
	source := "```text\n" + strings.Repeat("A", payloadSize) + "\n```\n"
	h := newHarness(t, source)
	line, block := h.e.leaves[0].codeLines()[0], h.e.leaves[0]
	if got := len(block.view.disp.text); got > render.CodeLineBudgetBytes+64 {
		t.Fatalf("initial display has %d bytes; want <= %d", got, render.CodeLineBudgetBytes+64)
	}
	if got := len(block.view.disp.pieces); got > 5 {
		t.Fatalf("initial display has %d pieces; want a bounded preview", got)
	}
	target := line.off + payloadSize/2
	h.caretAt(target)
	block = h.e.leaves[0]
	d, ok := block.view.disp.srcToDisp(target - block.lineStart)
	if !ok || block.view.disp.dispToSrc(d, false)+block.lineStart != target {
		t.Fatalf("caret source offset %d did not round-trip through the focused preview", target)
	}
	if got := len(block.view.disp.text); got > render.CodeLineBudgetBytes+64 {
		t.Fatalf("focused display has %d bytes; want <= %d", got, render.CodeLineBudgetBytes+64)
	}
	if h.e.Source() != source {
		t.Fatal("compacting the rendered view changed the Markdown source")
	}
}

func TestVeryLongMultilineCodeBlockShapesBoundedSourceWindow(t *testing.T) {
	const lineCount = 1024
	payload := strings.Repeat(strings.Repeat("A", 1023)+"\n", lineCount)
	source := "```text\n" + payload + "```\n"
	h := newHarness(t, source)
	block := h.e.leaves[0]
	if got := len(block.view.disp.text); got > render.CodeBlockBudgetBytes+256 {
		t.Fatalf("initial display has %d bytes; want <= %d", got, render.CodeBlockBudgetBytes+256)
	}
	target := strings.Index(source, payload) + len(payload)/2
	h.caretAt(target)
	block = h.e.leaves[0]
	if got := len(block.view.disp.text); got > render.CodeBlockBudgetBytes+256 {
		t.Fatalf("focused display has %d bytes; want <= %d", got, render.CodeBlockBudgetBytes+256)
	}
	d, ok := block.view.disp.srcToDisp(target - block.lineStart)
	if !ok || block.view.disp.dispToSrc(d, false)+block.lineStart != target {
		t.Fatalf("caret source offset %d did not round-trip through the focused block window", target)
	}
	if h.e.Source() != source {
		t.Fatal("compacting the rendered block changed the Markdown source")
	}
}
