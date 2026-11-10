// searchbench measures the persistent vaultsearch backend in a standalone
// process. It intentionally reports Go heap separately from process RSS:
// Tantivy and its Rust allocations are not visible to runtime.MemStats.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
	"unicode/utf8"

	"muse/vaultsearch"
)

const sourceBytes = 2 * 1024

type sample struct {
	Elapsed           string `json:"elapsed"`
	ElapsedNS         int64  `json:"elapsed_ns"`
	GoHeapBeforeBytes uint64 `json:"go_heap_before_bytes"`
	GoHeapAfterBytes  uint64 `json:"go_heap_after_bytes"`
	GoAllocBytes      uint64 `json:"go_alloc_bytes"`
}

type queryResult struct {
	Query       string `json:"query"`
	HitCount    int    `json:"hit_count"`
	Measurement sample `json:"measurement"`
}

type report struct {
	Backend                   string                 `json:"backend"`
	Dataset                   string                 `json:"dataset"`
	Documents                 int                    `json:"documents"`
	SourceBytesPerDocument    int                    `json:"source_bytes_per_document"`
	TotalSourceBytes          int64                  `json:"total_source_bytes"`
	IndexDirectory            string                 `json:"index_directory"`
	InitialOpen               sample                 `json:"initial_open"`
	Build                     sample                 `json:"build_reconcile"`
	Reopen                    sample                 `json:"reopen"`
	NoChangeReconcile         sample                 `json:"no_change_reconcile"`
	SingleDocumentUpdate      sample                 `json:"single_document_update"`
	UpdateCheck               *updateCheck           `json:"update_check,omitempty"`
	Searches                  map[string]queryResult `json:"searches"`
	IndexDiskBytesAfterBuild  int64                  `json:"index_disk_bytes_after_build"`
	IndexDiskBytesAfterUpdate int64                  `json:"index_disk_bytes_after_update"`
	GoHeapScope               string                 `json:"go_heap_scope"`
	RSSMeasurement            string                 `json:"rss_measurement"`
	Hold                      string                 `json:"hold"`
}

type updateCheck struct {
	UpdatedTokenHits int `json:"updated_token_hits"`
	OldTokenHits     int `json:"old_token_hits"`
}

func main() {
	n := flag.Int("n", 1000, "number of synthetic documents (each source is exactly 2 KiB)")
	indexDir := flag.String("index", "", "index directory (default: a temporary directory)")
	output := flag.String("output", "", "write the JSON report to this file instead of stdout")
	hold := flag.Duration("hold", 0, "keep the open index alive for this long after printing JSON, for external RSS sampling")
	flag.Parse()
	if *n < 0 {
		fatalf("-n must be zero (empty RSS baseline) or greater")
	}
	if *hold < 0 {
		fatalf("-hold must not be negative")
	}
	if flag.NArg() != 0 {
		fatalf("unexpected positional arguments: %s", strings.Join(flag.Args(), " "))
	}

	root := ""
	if *indexDir == "" {
		var err error
		root, err = os.MkdirTemp("", "muse-searchbench-")
		must(err)
		defer os.RemoveAll(root)
		*indexDir = filepath.Join(root, "index")
	} else {
		abs, err := filepath.Abs(*indexDir)
		must(err)
		*indexDir = abs
	}

	docs := synthetic(*n)
	r := report{
		Backend:                "muse/vaultsearch (Tantivy)",
		Dataset:                "deterministic mixed English/Chinese UTF-8 notes",
		Documents:              len(docs),
		SourceBytesPerDocument: sourceBytes,
		TotalSourceBytes:       int64(len(docs) * sourceBytes),
		IndexDirectory:         *indexDir,
		Searches:               make(map[string]queryResult),
		GoHeapScope:            "runtime.MemStats only; native Tantivy/Rust allocations are excluded",
		RSSMeasurement:         "run the command under /usr/bin/time -l; max RSS is process-wide",
		Hold:                   hold.String(),
	}
	if *n == 0 {
		r.Dataset = "empty index baseline"
	}

	ctx := context.Background()
	var idx *vaultsearch.Index
	r.InitialOpen = measure(func() error {
		var err error
		idx, err = vaultsearch.Open(*indexDir)
		return err
	})
	defer func() {
		if idx != nil {
			_ = idx.Close()
		}
	}()

	r.Build = measure(func() error { return idx.Reconcile(ctx, docs) })
	var err error
	r.IndexDiskBytesAfterBuild, err = diskBytes(*indexDir)
	must(err)

	if err := idx.Close(); err != nil {
		must(err)
	}
	idx = nil
	r.Reopen = measure(func() error {
		var err error
		idx, err = vaultsearch.Open(*indexDir)
		return err
	})
	r.NoChangeReconcile = measure(func() error { return idx.Reconcile(ctx, docs) })

	if len(docs) > 0 {
		updated := docs[len(docs)/2]
		updated.Revision = "revision-updated"
		updated.Source = strings.Replace(updated.Source, "oldrevisiontoken", "updatedrevisiontoken", 1)
		updated.Source = fitUTF8Bytes(updated.Source, sourceBytes)
		r.SingleDocumentUpdate = measure(func() error { return idx.Apply(ctx, []vaultsearch.Document{updated}, nil) })
		updatedHits, searchErr := idx.Search(ctx, "updatedrevisiontoken", 10)
		must(searchErr)
		oldHits, searchErr := idx.Search(ctx, "oldrevisiontoken", 10)
		must(searchErr)
		r.UpdateCheck = &updateCheck{UpdatedTokenHits: len(updatedHits), OldTokenHits: len(oldHits)}
		if len(updatedHits) != 1 || len(oldHits) != 0 {
			fatalf("single-document update check failed: updated hits=%d, old hits=%d", len(updatedHits), len(oldHits))
		}
	}
	r.IndexDiskBytesAfterUpdate, err = diskBytes(*indexDir)
	must(err)

	queries := []struct{ name, query string }{
		{"common", "musecommon"},
		{"rare", "rareterm"},
		{"nohit", "query-that-does-not-exist-7fa91"},
		{"chinese_one", "知"},
		{"chinese_two", "索引"},
		{"literal", "musecommon"},
		{"literal_rare", "rareterm"},
		{"literal_nohit", "query-that-does-not-exist-7fa91"},
	}
	for _, item := range queries {
		name, query := item.name, item.query
		var hits []vaultsearch.Hit
		measurement := measure(func() error {
			var searchErr error
			if name == "literal" || strings.HasPrefix(name, "literal_") {
				hits, searchErr = idx.Literal(ctx, query, 500)
			} else {
				hits, searchErr = idx.Search(ctx, query, 500)
			}
			return searchErr
		})
		r.Searches[name] = queryResult{Query: query, HitCount: len(hits), Measurement: measurement}
	}

	if *output == "" {
		if err := json.NewEncoder(os.Stdout).Encode(r); err != nil {
			must(err)
		}
	} else {
		encoded, err := json.MarshalIndent(r, "", "  ")
		must(err)
		encoded = append(encoded, '\n')
		must(os.WriteFile(*output, encoded, 0o644))
		fmt.Fprintf(os.Stderr, "searchbench: JSON report written to %s\n", *output)
	}
	if *hold > 0 {
		time.Sleep(*hold)
	}
	if err := idx.Close(); err != nil {
		must(err)
	}
	idx = nil
}

func synthetic(count int) []vaultsearch.Document {
	docs := make([]vaultsearch.Document, count)
	for i := range docs {
		id := fmt.Sprintf("note-%06d", i)
		path := fmt.Sprintf("notes/%06d.md", i)
		body := "musecommon 搜索索引 知识管理 English notes 中文笔记。"
		source := strings.Repeat(body, sourceBytes/32)
		if i == count-1 {
			source = "rareterm " + source
		}
		if i == count/2 {
			source = "oldrevisiontoken " + source
		}
		source = fitUTF8Bytes(source, sourceBytes)
		if len(source) != sourceBytes || !utf8.ValidString(source) {
			fatalf("generated source for %s is not valid UTF-8 with exactly %d bytes", path, sourceBytes)
		}
		docs[i] = vaultsearch.Document{
			ID:       id,
			Path:     path,
			Title:    fmt.Sprintf("知识笔记 Knowledge %06d", i),
			Source:   source,
			Revision: "revision-1",
			Aliases:  []string{fmt.Sprintf("alias-%06d", i)},
			Tags:     []string{"benchmark", "中文"},
		}
	}
	return docs
}

// fitUTF8Bytes returns valid UTF-8 with exactly n bytes, padding the tail with
// ASCII when the last rune does not fit.
func fitUTF8Bytes(s string, n int) string {
	var b strings.Builder
	b.Grow(n)
	for b.Len() < n {
		remaining := n - b.Len()
		for _, r := range s {
			runeLen := 1
			if r > 0x7f {
				runeLen = 2
			}
			if r > 0x7ff {
				runeLen = 3
			}
			if r > 0xffff {
				runeLen = 4
			}
			if runeLen > remaining {
				break
			}
			b.WriteRune(r)
			remaining -= runeLen
			if remaining == 0 {
				break
			}
		}
		if remaining := n - b.Len(); remaining > 0 {
			b.WriteString(strings.Repeat("x", remaining))
		}
	}
	return b.String()
}

func measure(fn func() error) sample {
	runtime.GC()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	start := time.Now()
	err := fn()
	elapsed := time.Since(start)
	if err != nil {
		must(err)
	}
	runtime.GC()
	runtime.ReadMemStats(&after)
	return sample{
		Elapsed:           elapsed.String(),
		ElapsedNS:         elapsed.Nanoseconds(),
		GoHeapBeforeBytes: before.HeapAlloc,
		GoHeapAfterBytes:  after.HeapAlloc,
		GoAllocBytes:      after.TotalAlloc - before.TotalAlloc,
	}
}

func diskBytes(root string) (int64, error) {
	var total int64
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		total += info.Size()
		return nil
	})
	return total, err
}

func must(err error) {
	if err != nil {
		fatalf("%v", err)
	}
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "searchbench: "+format+"\n", args...)
	os.Exit(1)
}
