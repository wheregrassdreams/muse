package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	probe "muse-searchprobe"
	"os"
	"path/filepath"
	"runtime"
	"time"
)

func main() {
	dir := flag.String("index", "", "persistent index directory (default temporary)")
	n := flag.Int("n", 1000, "synthetic documents; 0 to reuse")
	q := flag.String("query", "rareword", "plain query")
	flag.Parse()
	if *dir == "" {
		p, e := os.MkdirTemp("", "searchprobe-")
		check(e)
		defer os.RemoveAll(p)
		*dir = filepath.Join(p, "index")
	}
	ctx := context.Background()
	runtime.GC()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	start := time.Now()
	i, e := probe.Open(*dir)
	check(e)
	if *n > 0 {
		check(i.Batch(probe.Synthetic(*n), nil))
	}
	build := time.Since(start)
	runtime.ReadMemStats(&after)
	runtime.GC()
	var retained runtime.MemStats
	runtime.ReadMemStats(&retained)
	start = time.Now()
	h, e := i.Search(ctx, *q, 500)
	check(e)
	search := time.Since(start)
	start = time.Now()
	m, e := i.Literal(ctx, *q, 500)
	check(e)
	literal := time.Since(start)
	check(i.Close())
	start = time.Now()
	i, e = probe.Open(*dir)
	check(e)
	reopen := time.Since(start)
	check(i.Close())
	var bytes int64
	check(filepath.WalkDir(*dir, func(p string, d os.DirEntry, e error) error {
		if e != nil {
			return e
		}
		if !d.IsDir() {
			s, e := d.Info()
			if e != nil {
				return e
			}
			bytes += s.Size()
		}
		return nil
	}))
	check(json.NewEncoder(os.Stdout).Encode(map[string]any{"documents_requested": *n, "build": build.String(), "search": search.String(), "literal": literal.String(), "reopen": reopen.String(), "document_hits": len(h), "occurrences": len(m), "build_total_alloc_bytes": after.TotalAlloc - before.TotalAlloc, "heap_alloc_bytes": after.HeapAlloc, "retained_heap_after_gc_bytes": retained.HeapAlloc, "index_disk_bytes": bytes}))
}
func check(e error) {
	if e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
}
