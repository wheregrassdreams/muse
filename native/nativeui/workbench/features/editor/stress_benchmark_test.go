package editor

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"runtime"
	"runtime/pprof"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"
	"unsafe"

	"github.com/egoist/mygo/ui"
	"github.com/wheregrassdreams/muse-editor/text/navigation"
	"muse/markdown/images"
	syntax "github.com/wheregrassdreams/muse-editor/markdown/syntax"
)

func stressTester(e *Editor) *ui.Tester {
	t := ui.NewTester(func(c *ui.Context) {
		ui.Column(c).Fill().Children(func() { e.View(c).Grow(1) })
	}, 1200, 800)
	return t
}

func stressImages(dir string, count int) string {
	return stressImagesSized(dir, count, 160, 120)
}

func stressImagesSized(dir string, count, width, height int) string {
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x), G: uint8(y), B: 120, A: 255})
		}
	}
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, img); err != nil {
		panic(err)
	}
	template := filepath.Join(dir, "image-template.png")
	if err := os.WriteFile(template, encoded.Bytes(), 0o600); err != nil {
		panic(err)
	}
	var source strings.Builder
	for i := 0; i < count; i++ {
		name := fmt.Sprintf("image-%05d.png", i)
		path := filepath.Join(dir, name)
		if err := os.Link(template, path); err != nil {
			// Some filesystems disallow hard links; write the single encoded
			// payload in that case instead of re-encoding the pixels.
			if err := os.WriteFile(path, encoded.Bytes(), 0o600); err != nil {
				panic(err)
			}
		}
		source.WriteString("![[")
		source.WriteString(name)
		source.WriteString("]]\n\n")
	}
	return source.String()
}

// stressBase64MarkdownFixture writes one realistic, very large pasted-image
// Markdown sample to disk, then reads it once for reuse across benchmark runs.
// The repeated base64 alphabet decodes to invalid image data, avoiding a huge
// decoded bitmap while still exercising Markdown parsing, image URL handling,
// asynchronous data-URL decoding, and retained source memory.
func stressBase64MarkdownFixture(b *testing.B, payloadBytes int) (string, string) {
	b.Helper()
	if path := os.Getenv("MUSE_STRESS_BASE64_PATH"); path != "" {
		data, err := os.ReadFile(path)
		if err != nil {
			b.Fatal(err)
		}
		if len(data) == 0 {
			b.Fatalf("stress fixture %q is empty", path)
		}
		return path, unsafe.String(unsafe.SliceData(data), len(data))
	}
	path := filepath.Join(b.TempDir(), "base64-heavy.md")
	f, err := os.Create(path)
	if err != nil {
		b.Fatal(err)
	}
	if _, err := f.WriteString("![pasted image](data:image/png;base64,"); err != nil {
		f.Close()
		b.Fatal(err)
	}
	chunk := strings.Repeat("A", 1<<20)
	for written := 0; written < payloadBytes; written += len(chunk) {
		n := min(len(chunk), payloadBytes-written)
		if _, err := f.WriteString(chunk[:n]); err != nil {
			f.Close()
			b.Fatal(err)
		}
	}
	if _, err := f.WriteString(")\n"); err != nil {
		f.Close()
		b.Fatal(err)
	}
	if err := f.Close(); err != nil {
		b.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		b.Fatal(err)
	}
	// The loaded fixture is immutable for the rest of the benchmark. Reuse
	// its byte buffer as a string so a 100 MiB stress document does not briefly
	// require another 100 MiB copy just to enter the editor.
	if len(data) == 0 {
		return path, ""
	}
	return path, unsafe.String(unsafe.SliceData(data), len(data))
}

func stressContent(kind string, n int) string {
	var b strings.Builder
	for i := 0; i < n; i++ {
		switch kind {
		case "mixed":
			b.WriteString(fmt.Sprintf("## Section %05d\n\nA paragraph with **emphasis**, `inline code`, a link, and enough text to exercise shaping and line wrapping.\n\n- item one\n- item two\n\n```go\nfunc work() { println(%d) }\n```\n\n| key | value |\n| --- | --- |\n| a | b |\n\n", i, i))
		case "prose":
			b.WriteString(fmt.Sprintf("Paragraph %05d: A large Markdown document should keep layout work bounded while scrolling through many independent blocks. The quick brown fox jumps over the lazy dog.\n\n", i))
		case "math-diagram":
			b.WriteString(fmt.Sprintf("$$\nx_%d = \\sum_{i=1}^{32} i^2\n$$\n\n```mermaid\ngraph TD\n A%d --> B%d --> C%d\n```\n\n", i, i, i, i))
		}
	}
	return b.String()
}

func reportFrameDistribution(b *testing.B, samples []time.Duration) {
	b.Helper()
	if len(samples) == 0 {
		return
	}
	for _, budget := range []struct {
		frame time.Duration
		name  string
	}{{8 * time.Millisecond, "over-8ms-pct"}, {13 * time.Millisecond, "over-13ms-pct"}, {8333333, "over-8_3ms-pct"}, {16666667, "over-16_7ms-pct"}, {33333333, "over-33_3ms-pct"}} {
		over := 0
		for _, sample := range samples {
			if sample > budget.frame {
				over++
			}
		}
		b.ReportMetric(float64(over)*100/float64(len(samples)), budget.name)
	}
	sort.Slice(samples, func(i, j int) bool { return samples[i] < samples[j] })
	b.ReportMetric(float64(samples[len(samples)/2].Nanoseconds()), "p50-ns/frame")
	b.ReportMetric(float64(samples[(len(samples)*95-1)/100].Nanoseconds()), "p95-ns/frame")
	b.ReportMetric(float64(samples[(len(samples)*99-1)/100].Nanoseconds()), "p99-ns/frame")
	b.ReportMetric(float64(samples[len(samples)-1].Nanoseconds()), "max-ns/frame")
}

// BenchmarkStressNormalScrollFrames is the standard-note counterpart to the
// large-document benchmark. It deliberately stays below virtualLayoutMin.
func BenchmarkStressNormalScrollFrames(b *testing.B) {
	src := stressContent("prose", 12)
	e := New(src, Options{FontSize: 16})
	t := stressTester(e)
	t.Frame()
	t.Frame()
	baseline := heapBaseline()
	var peak uint64
	samples := make([]time.Duration, 0, b.N)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		start := time.Now()
		e.scroll.Y = float32((i % 5) * 100)
		t.Frame()
		samples = append(samples, time.Since(start))
		if heap := sampleHeapPeak(); heap > peak {
			peak = heap
		}
	}
	b.StopTimer()
	reportFrameDistribution(b, samples)
	reportHeap(b, baseline, peak)
	runtime.KeepAlive(e)
}

// BenchmarkStressRepeatedDocumentSwitches models normal use opening multiple
// Markdown notes and moving through each before selecting the next one.
func BenchmarkStressRepeatedDocumentSwitches(b *testing.B) {
	e := New("", Options{FontSize: 16})
	t := stressTester(e)
	t.Frame()
	t.Frame()
	baseline := heapBaseline()
	var peak uint64
	samples := make([]time.Duration, 0, b.N*8)
	openSamples := make([]time.Duration, 0, b.N)
	b.ReportAllocs()
	b.ResetTimer()
	for note := 0; note < b.N; note++ {
		e.SetSource(stressContent("prose", 120))
		start := time.Now()
		t.Frame()
		openSamples = append(openSamples, time.Since(start))
		for step := 0; step < 8; step++ {
			e.scroll.Y = float32(step) * max(e.docH-e.viewH, 0) / 7
			start = time.Now()
			t.Frame()
			samples = append(samples, time.Since(start))
			if heap := sampleHeapPeak(); heap > peak {
				peak = heap
			}
		}
	}
	b.StopTimer()
	reportFrameDistribution(b, samples)
	sort.Slice(openSamples, func(i, j int) bool { return openSamples[i] < openSamples[j] })
	if len(openSamples) > 0 {
		b.ReportMetric(float64(openSamples[(len(openSamples)*99-1)/100].Nanoseconds()), "open-p99-ns")
		b.ReportMetric(float64(openSamples[len(openSamples)-1].Nanoseconds()), "open-max-ns")
	}
	reportHeap(b, baseline, peak)
	writeStressHeapProfile(b)
	runtime.KeepAlive(e)
}

func sampleHeapPeak() uint64 {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return m.HeapAlloc
}

func heapBaseline() uint64 {
	runtime.GC()
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return m.HeapAlloc
}

func reportHeap(b *testing.B, baseline, peak uint64) {
	b.Helper()
	if peak > baseline {
		b.ReportMetric(float64(peak-baseline), "peak-heap-delta-B")
	}
	runtime.GC()
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	if m.HeapAlloc > baseline {
		b.ReportMetric(float64(m.HeapAlloc-baseline), "retained-heap-delta-B")
	} else {
		b.ReportMetric(0, "retained-heap-delta-B")
	}
}

func writeStressHeapProfile(b *testing.B) {
	b.Helper()
	path := os.Getenv("MUSE_STRESS_HEAP_PROFILE")
	if path == "" {
		return
	}
	runtime.GC()
	f, err := os.Create(path)
	if err != nil {
		b.Fatal(err)
	}
	if err := pprof.WriteHeapProfile(f); err != nil {
		f.Close()
		b.Fatal(err)
	}
	if err := f.Close(); err != nil {
		b.Fatal(err)
	}
}

func writeStressGoroutineProfile(b *testing.B) {
	b.Helper()
	path := os.Getenv("MUSE_STRESS_GOROUTINE_PROFILE")
	if path == "" {
		return
	}
	f, err := os.Create(path)
	if err != nil {
		b.Fatal(err)
	}
	if err := pprof.Lookup("goroutine").WriteTo(f, 2); err != nil {
		f.Close()
		b.Fatal(err)
	}
	if err := f.Close(); err != nil {
		b.Fatal(err)
	}
}

// BenchmarkStressFirstFrame compares parse/layout/render startup cost for
// large prose, structurally rich Markdown, and repeated expensive blocks.
func BenchmarkStressFirstFrame(b *testing.B) {
	for _, tc := range []struct {
		name string
		src  string
	}{
		{"1MiB-mixed", benchmarkSource(1 << 20)},
		{"10MiB-mixed", benchmarkSource(10 << 20)},
		{"10MiB-prose", stressContent("prose", 70000)},
		{"4MiB-mixed", stressContent("mixed", 18000)},
		{"math-diagram", stressContent("math-diagram", 300)},
	} {
		b.Run(tc.name, func(b *testing.B) {
			if os.Getenv("MUSE_STRESS_WARM_TEXTNAV") == "1" {
				if err := navigation.WarmUp(); err != nil {
					b.Fatal(err)
				}
			}
			b.ReportAllocs()
			b.SetBytes(int64(len(tc.src)))
			baseline := heapBaseline()
			var peak uint64
			var keepAlive *Editor
			var leaves, leafChunks, blocks int
			var newTime, testerTime, frameTime time.Duration
			coldSamples := make([]time.Duration, 0, b.N)
			frameSamples := make([]time.Duration, 0, b.N)
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				start := time.Now()
				newStart := time.Now()
				opts := Options{FontSize: 16}
				if os.Getenv("MUSE_STRESS_DISABLE_SMART_TYPOGRAPHY") == "1" {
					opts.DisableSmartTypography = true
				}
				e := New(tc.src, opts)
				newTime += time.Since(newStart)
				testerStart := time.Now()
				t := stressTester(e)
				testerTime += time.Since(testerStart)
				frameStart := time.Now()
				t.Frame()
				frameElapsed := time.Since(frameStart)
				frameTime += frameElapsed
				frameSamples = append(frameSamples, frameElapsed)
				coldSamples = append(coldSamples, time.Since(start))
				keepAlive = e
				leaves = len(e.leaves)
				leafChunks = len(e.arena[e.arenaCur].chunks)
				blocks = e.p.Parser().Len()
				if heap := sampleHeapPeak(); heap > peak {
					peak = heap
				}
			}
			b.StopTimer()
			reportFrameDistribution(b, frameSamples)
			if len(coldSamples) > 0 {
				sort.Slice(coldSamples, func(i, j int) bool { return coldSamples[i] < coldSamples[j] })
				b.ReportMetric(float64(coldSamples[len(coldSamples)/2].Nanoseconds()), "cold-first-frame-p50-ns/op")
				b.ReportMetric(float64(coldSamples[(len(coldSamples)*95-1)/100].Nanoseconds()), "cold-first-frame-p95-ns/op")
				b.ReportMetric(float64(coldSamples[(len(coldSamples)*99-1)/100].Nanoseconds()), "cold-first-frame-p99-ns/op")
			}
			b.ReportMetric(float64(newTime.Nanoseconds())/float64(b.N), "new-ns/op")
			b.ReportMetric(float64(testerTime.Nanoseconds())/float64(b.N), "tester-setup-ns/op")
			b.ReportMetric(float64(frameTime.Nanoseconds())/float64(b.N), "frame-ns/op")
			b.ReportMetric(float64(leaves), "editor-leaves")
			b.ReportMetric(float64(leafChunks), "editor-leaf-chunks")
			b.ReportMetric(float64(strings.Count(tc.src, "\n")+1), "source-lines")
			b.ReportMetric(float64(blocks), "parser-blocks")
			b.ReportMetric(float64(unsafe.Sizeof(syntax.IncrementalBlock{})), "parser-block-B")
			b.ReportMetric(float64(unsafe.Sizeof(leaf{})), "leaf-struct-B")
			b.ReportMetric(float64(unsafe.Sizeof(viewParams{})), "view-params-B")
			b.ReportMetric(float64(unsafe.Sizeof(blockHorizontalState{})), "horizontal-state-B")
			writeStressHeapProfile(b)
			reportHeap(b, baseline, peak)
			runtime.KeepAlive(keepAlive)
		})
	}
}

// BenchmarkStressBase64FirstFrame is intentionally run with -benchtime=1x:
// one sample is prepared once on disk and reused, with no extra copies left
// on disk. MUSE_STRESS_BASE64_BYTES can increase the payload for local stress
// runs; the default remains a 32 MiB pasted image.
func BenchmarkStressBase64FirstFrame(b *testing.B) {
	payloadBytes := 32 << 20
	if raw := os.Getenv("MUSE_STRESS_BASE64_BYTES"); raw != "" {
		var err error
		payloadBytes, err = strconv.Atoi(raw)
		if err != nil || payloadBytes <= 0 {
			b.Fatalf("MUSE_STRESS_BASE64_BYTES must be a positive byte count, got %q", raw)
		}
	}
	baseline := heapBaseline()
	path, src := stressBase64MarkdownFixture(b, payloadBytes)
	sourcePeak := sampleHeapPeak()
	runtime.GC()
	sourceRetained := sampleHeapPeak()
	b.ReportAllocs()
	b.SetBytes(int64(len(src)))
	b.ReportMetric(float64(len(src)), "fixture-bytes")
	var peak = sourcePeak
	var frameSamples []time.Duration
	var keepAlive *Editor
	var newTime, testerTime, frameTime time.Duration
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		newStart := time.Now()
		e := New(src, editorTestOptions(Options{FontSize: 16, BaseDir: filepath.Dir(path)}))
		newTime += time.Since(newStart)
		testerStart := time.Now()
		t := stressTester(e)
		testerTime += time.Since(testerStart)
		frameStart := time.Now()
		t.Frame()
		frameTime += time.Since(frameStart)
		frameSamples = append(frameSamples, time.Since(frameStart))
		if heap := sampleHeapPeak(); heap > peak {
			peak = heap
		}
		keepAlive = e
	}
	b.StopTimer()
	settleStart := time.Now()
	settleDeadline := settleStart.Add(30 * time.Second)
	for {
		pending, active := images.QueueStats()
		if pending == 0 && active == 0 {
			break
		}
		if heap := sampleHeapPeak(); heap > peak {
			peak = heap
		}
		if time.Now().After(settleDeadline) {
			b.Fatalf("image decode queue did not settle: pending=%d active=%d", pending, active)
		}
		time.Sleep(time.Millisecond)
	}
	settleDuration := time.Since(settleStart)
	settledFrameStart := time.Now()
	if keepAlive != nil {
		// Apply the asynchronous decode result as the live UI does after its
		// invalidation callback, then measure the first fully settled frame.
		lastTester := stressTester(keepAlive)
		lastTester.Frame()
	}
	settledFrame := time.Since(settledFrameStart)
	if heap := sampleHeapPeak(); heap > peak {
		peak = heap
	}
	reportFrameDistribution(b, frameSamples)
	b.ReportMetric(float64(newTime.Nanoseconds())/float64(b.N), "new-ns/op")
	b.ReportMetric(float64(testerTime.Nanoseconds())/float64(b.N), "tester-setup-ns/op")
	b.ReportMetric(float64(frameTime.Nanoseconds())/float64(b.N), "frame-ns/op")
	b.ReportMetric(float64(settleDuration.Nanoseconds()), "image-decode-settle-ns")
	b.ReportMetric(float64(settledFrame.Nanoseconds()), "settled-frame-ns")
	b.ReportMetric(float64((newTime+testerTime+frameTime).Nanoseconds())/float64(b.N), "cold-first-frame-ns/op")
	cacheEntries, cacheBytes := images.CacheStats()
	b.ReportMetric(float64(cacheEntries), "image-cache-entries")
	b.ReportMetric(float64(cacheBytes), "image-cache-estimated-bytes")
	postPending, postActive := images.QueueStats()
	b.ReportMetric(float64(postPending), "post-frame-pending-image-decodes")
	b.ReportMetric(float64(postActive), "post-frame-active-image-decodes")
	if postPending != 0 || postActive != 0 || len(src) > 1<<20 && cacheEntries != 0 {
		b.Fatalf("settled data image was reloaded: pending=%d active=%d cache_entries=%d", postPending, postActive, cacheEntries)
	}
	if sourceRetained > baseline {
		b.ReportMetric(float64(sourceRetained-baseline), "source-retained-heap-B")
	} else {
		b.ReportMetric(0, "source-retained-heap-B")
	}
	reportHeap(b, baseline, peak)
	writeStressHeapProfile(b)
	runtime.KeepAlive(keepAlive)
}

// BenchmarkStressScrollFrames reports a latency distribution and allocations
// per rendered scroll frame; the Go benchmark also reports aggregate ns/op.
func BenchmarkStressScrollFrames(b *testing.B) {
	src := stressContent("mixed", 18000)
	e := New(src, Options{FontSize: 16})
	t := stressTester(e)
	t.Frame()
	t.Frame()
	samples := make([]time.Duration, 0, 128)
	var offsetFrames, offsetOver13 [24]int
	var offsetMax [24]time.Duration
	baseline := heapBaseline()
	var peak uint64
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		start := time.Now()
		// Settle a viewport position and render exactly one frame. Tester.Scroll
		// drains its animation queue, which measures a whole scroll gesture
		// rather than the latency of an individual display frame.
		e.scroll.Y = float32((i % 24) * 600)
		t.Frame()
		elapsed := time.Since(start)
		samples = append(samples, elapsed)
		offset := i % len(offsetFrames)
		offsetFrames[offset]++
		if elapsed > 13*time.Millisecond {
			offsetOver13[offset]++
		}
		if elapsed > offsetMax[offset] {
			offsetMax[offset] = elapsed
		}
		if heap := sampleHeapPeak(); heap > peak {
			peak = heap
		}
	}
	b.StopTimer()
	reportFrameDistribution(b, samples)
	worstOffset, worstRatio := 0, -1.0
	for i, count := range offsetFrames {
		if count == 0 {
			continue
		}
		ratio := float64(offsetOver13[i]) / float64(count)
		if ratio > worstRatio {
			worstOffset, worstRatio = i, ratio
		}
	}
	b.ReportMetric(float64(worstOffset), "worst-offset-index")
	b.ReportMetric(worstRatio*100, "worst-offset-over-13ms-pct")
	b.ReportMetric(float64(offsetMax[worstOffset].Nanoseconds()), "worst-offset-max-ns")
	reportHeap(b, baseline, peak)
	runtime.KeepAlive(e)
}

// BenchmarkStressProseScrollFrames measures full-range scroll jumps through
// a 10MiB note with many short paragraphs and editable blank lines.
func BenchmarkStressProseScrollFrames(b *testing.B) {
	if os.Getenv("MUSE_STRESS_WARM_TEXTNAV") == "1" {
		if err := navigation.WarmUp(); err != nil {
			b.Fatal(err)
		}
	}
	e := New(stressContent("prose", 70000), Options{FontSize: 16})
	t := stressTester(e)
	t.Frame()
	t.Frame()
	baseline := heapBaseline()
	var peak uint64
	var gcBefore, gcAfter runtime.MemStats
	runtime.ReadMemStats(&gcBefore)
	samples := make([]time.Duration, 0, b.N)
	maxScroll := max(e.docH-e.viewH, 0)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		step := float32(i % 24)
		e.scroll.Y = step * maxScroll / 23
		start := time.Now()
		t.Frame()
		samples = append(samples, time.Since(start))
		if heap := sampleHeapPeak(); heap > peak {
			peak = heap
		}
	}
	b.StopTimer()
	runtime.ReadMemStats(&gcAfter)
	reportFrameDistribution(b, samples)
	reportHeap(b, baseline, peak)
	b.ReportMetric(float64(gcAfter.NumGC-gcBefore.NumGC), "gc-cycles")
	if gcAfter.PauseTotalNs >= gcBefore.PauseTotalNs {
		pause := gcAfter.PauseTotalNs - gcBefore.PauseTotalNs
		b.ReportMetric(float64(pause), "gc-pause-total-ns")
		if cycles := gcAfter.NumGC - gcBefore.NumGC; cycles > 0 {
			b.ReportMetric(float64(pause)/float64(cycles), "gc-pause-avg-ns")
		}
	}
	writeStressHeapProfile(b)
	runtime.KeepAlive(e)
}

// BenchmarkStressIncrementalScrollFrames models a smooth scroll gesture where
// each display frame advances only a small distance through the document.
func BenchmarkStressIncrementalScrollFrames(b *testing.B) {
	for _, tc := range []struct {
		name      string
		forceFull bool
	}{
		{name: "reuse-shaped-band"},
		{name: "force-full-layout", forceFull: true},
	} {
		b.Run(tc.name, func(b *testing.B) {
			src := stressContent("mixed", 18000)
			e := New(src, Options{FontSize: 16})
			t := stressTester(e)
			t.Frame()
			t.Frame()
			baseline := heapBaseline()
			var peak uint64
			samples := make([]time.Duration, 0, b.N)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				start := time.Now()
				e.scroll.Y = float32((i % 24) * 16)
				if tc.forceFull {
					e.leavesLaid = false
				}
				t.Frame()
				samples = append(samples, time.Since(start))
				if heap := sampleHeapPeak(); heap > peak {
					peak = heap
				}
			}
			b.StopTimer()
			reportFrameDistribution(b, samples)
			reportHeap(b, baseline, peak)
			runtime.KeepAlive(e)
		})
	}
}

// BenchmarkStressImageHeavyScroll measures opening and traversing a note
// with many distinct local images. Decode is asynchronous as in the app.
func BenchmarkStressImageHeavyScroll(b *testing.B) {
	dir := b.TempDir()
	src := stressImages(dir, 1200)
	e := New(src, editorTestOptions(Options{FontSize: 16, BaseDir: dir}))
	t := stressTester(e)
	b.ReportAllocs()
	b.ResetTimer()
	samples := make([]time.Duration, 0, b.N)
	baseline := heapBaseline()
	var gcStart, gcEnd runtime.MemStats
	runtime.ReadMemStats(&gcStart)
	var peak uint64
	for i := 0; i < b.N; i++ {
		start := time.Now()
		if i == 0 {
			t.Frame()
		} else {
			e.scroll.Y += 700
			if e.scroll.Y > max(e.docH-e.viewH, 0) {
				e.scroll.Y = 0
			}
			t.Frame()
		}
		samples = append(samples, time.Since(start))
		if heap := sampleHeapPeak(); heap > peak {
			peak = heap
		}
	}
	b.StopTimer()
	runtime.ReadMemStats(&gcEnd)
	reportFrameDistribution(b, samples)
	b.ReportMetric(float64(gcEnd.NumGC-gcStart.NumGC), "gc-cycles")
	b.ReportMetric(float64(gcEnd.PauseTotalNs-gcStart.PauseTotalNs)/1e6, "gc-pause-ms")
	reportHeap(b, baseline, peak)
	writeStressHeapProfile(b)
	runtime.KeepAlive(e)
}

// BenchmarkStressHighResolutionImageScroll traverses one note with images
// whose decoded pixel planes are large enough to expose overlapping decodes.
func BenchmarkStressHighResolutionImageScroll(b *testing.B) {
	dir := b.TempDir()
	src := stressImagesSized(dir, 64, 2048, 1536)
	e := New(src, editorTestOptions(Options{FontSize: 16, BaseDir: dir}))
	t := stressTester(e)
	b.ReportAllocs()
	b.ResetTimer()
	samples := make([]time.Duration, 0, b.N)
	baseline := heapBaseline()
	var gcStart, gcEnd runtime.MemStats
	runtime.ReadMemStats(&gcStart)
	var peak uint64
	for i := 0; i < b.N; i++ {
		start := time.Now()
		if i == 0 {
			t.Frame()
		} else {
			e.scroll.Y += 700
			if e.scroll.Y > max(e.docH-e.viewH, 0) {
				e.scroll.Y = 0
			}
			t.Frame()
		}
		samples = append(samples, time.Since(start))
		if heap := sampleHeapPeak(); heap > peak {
			peak = heap
		}
		// Let asynchronous decoders progress while keeping pressure on the
		// loader; the scroll interval remains faster than a display refresh.
		time.Sleep(time.Millisecond)
	}
	b.StopTimer()
	// Allow a short idle interval, then commit whichever decoded assets are
	// ready. The remaining asynchronous jobs are reported separately so a
	// fast-scroll backlog is not mislabeled as retained cache memory.
	time.Sleep(2 * time.Second)
	t.Frame()
	decodeJobs, activeDecodeJobs := images.QueueStats()
	runtime.ReadMemStats(&gcEnd)
	imageViews := 0
	offscreenImageViews := 0
	for _, l := range e.leaves {
		if l.view.hasImageAsset() {
			imageViews++
			if l.y > e.scroll.Y+e.viewH || l.y+l.h < e.scroll.Y {
				offscreenImageViews++
			}
		}
	}
	reportFrameDistribution(b, samples)
	b.ReportMetric(float64(gcEnd.NumGC-gcStart.NumGC), "gc-cycles")
	b.ReportMetric(float64(gcEnd.PauseTotalNs-gcStart.PauseTotalNs)/1e6, "gc-pause-ms")
	b.ReportMetric(float64(runtime.NumGoroutine()), "goroutines-after-settle")
	b.ReportMetric(float64(decodeJobs), "pending-image-decodes-after-settle")
	b.ReportMetric(float64(activeDecodeJobs), "active-image-decodes-after-settle")
	b.ReportMetric(float64(imageViews), "retained-image-views")
	b.ReportMetric(float64(offscreenImageViews), "offscreen-image-views")
	writeStressGoroutineProfile(b)
	reportHeap(b, baseline, peak)
	writeStressHeapProfile(b)
	runtime.KeepAlive(e)
}
