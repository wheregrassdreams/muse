// Package diagnostics reports process and application memory measurements.
// Callers collect backend and feature details into Snapshot values; this
// package owns sampling cadence and heap profile output only.
package diagnostics

import (
	"log"
	"os"
	"runtime"
	"runtime/pprof"
	"sync"
	"sync/atomic"
	"time"
)

// Snapshot contains scalar measurements collected by the application
// composition root. Keeping values here avoids dependencies on App and UI
// feature packages.
type Snapshot struct {
	VaultReady               bool
	Editor                   EditorSnapshot
	ImageCacheEntries        int
	ImageCacheEstimatedBytes int64
	ImageDecodePending       int
	ImageDecodeActive        int
}

// EditorSnapshot reports active document and decoded image estimates.
type EditorSnapshot struct {
	SourceBytes          int
	Leaves               int
	VirtualLayout        bool
	ImageCandidateLeaves int
	ImageAssetViews      int
	ImageAssets          int
	LoadedImageAssets    int
	ImagePixelBytes      int64
}

var (
	diagnosticsOnce       sync.Once
	diagnosticsOn         bool
	diagnosticsLast       atomic.Int64
	memoryProfileOnce     sync.Once
	memoryHighProfileOnce sync.Once
)

const memoryHighProfileThreshold = 50 << 20

func enabled() bool {
	diagnosticsOnce.Do(func() {
		diagnosticsOn = os.Getenv("MUSE_MEMORY_DIAGNOSTICS") == "1"
	})
	return diagnosticsOn
}

// Start periodically schedules memory samples when diagnostics are explicitly
// enabled. schedule should deliver its callback to the owner thread so callers
// can safely collect UI-backed measurements.
func Start(schedule func(func()), snapshot func() Snapshot) {
	if !enabled() || schedule == nil || snapshot == nil {
		return
	}
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for {
			schedule(func() { Log(snapshot) })
			<-ticker.C
		}
	}()
}

// Log records a throttled sample. Passing a provider keeps normal UI frames
// free of snapshot work while diagnostics are disabled or rate-limited.
func Log(snapshot func() Snapshot) {
	if !enabled() || snapshot == nil {
		return
	}
	now := time.Now().UnixNano()
	for {
		last := diagnosticsLast.Load()
		if now-last < int64(5*time.Second) {
			return
		}
		if diagnosticsLast.CompareAndSwap(last, now) {
			break
		}
	}
	state := snapshot()
	profilePath := os.Getenv("MUSE_MEMORY_PROFILE_PATH")
	// A vault can retain meaningful memory even while the editor is on its
	// blank home page. Wait for its index to become ready, then profile the
	// whole process instead of requiring an image-bearing note to be open.
	if profilePath != "" && state.VaultReady {
		memoryProfileOnce.Do(func() {
			// Collect first to distinguish live references from garbage awaiting
			// the next normal GC cycle.
			runtime.GC()
			writeMemoryHeapProfile(profilePath)
		})
	}
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)
	if profilePath != "" && state.VaultReady && mem.HeapAlloc >= memoryHighProfileThreshold {
		memoryHighProfileOnce.Do(func() {
			beforeGC := mem.HeapAlloc
			// Capture only once after the heap has grown, and collect first to
			// separate retained objects from short-lived allocation churn.
			runtime.GC()
			highProfilePath := profilePath + ".high"
			writeMemoryHeapProfile(highProfilePath)
			runtime.ReadMemStats(&mem)
			log.Printf("memory: high-heap profile path=%s threshold=%d heap_alloc_before_gc=%d heap_alloc_after_gc=%d",
				highProfilePath, memoryHighProfileThreshold, beforeGC, mem.HeapAlloc)
		})
	}
	e := state.Editor
	log.Printf("memory: heap_alloc=%d heap_inuse=%d heap_idle=%d heap_released=%d heap_sys=%d heap_objects=%d stack_inuse=%d stack_sys=%d sys=%d total_alloc=%d gc_cycles=%d gc_pause_total_ms=%.2f goroutines=%d image_cache_entries=%d image_cache_estimated_bytes=%d image_decode_pending=%d image_decode_active=%d doc_source_bytes=%d doc_leaves=%d doc_virtual=%t doc_image_candidate_leaves=%d doc_image_views=%d doc_image_assets=%d doc_loaded_image_assets=%d doc_image_pixel_bytes_estimate=%d",
		mem.HeapAlloc, mem.HeapInuse, mem.HeapIdle, mem.HeapReleased, mem.HeapSys, mem.HeapObjects,
		mem.StackInuse, mem.StackSys, mem.Sys, mem.TotalAlloc, mem.NumGC,
		float64(mem.PauseTotalNs)/1e6, runtime.NumGoroutine(), state.ImageCacheEntries, state.ImageCacheEstimatedBytes,
		state.ImageDecodePending, state.ImageDecodeActive, e.SourceBytes, e.Leaves, e.VirtualLayout,
		e.ImageCandidateLeaves, e.ImageAssetViews, e.ImageAssets, e.LoadedImageAssets, e.ImagePixelBytes)
}

func writeMemoryHeapProfile(path string) {
	file, err := os.Create(path)
	if err != nil {
		log.Printf("memory: heap profile create failed path=%s err=%v", path, err)
		return
	}
	if err := pprof.WriteHeapProfile(file); err != nil {
		log.Printf("memory: heap profile write failed path=%s err=%v", path, err)
	}
	if err := file.Close(); err != nil {
		log.Printf("memory: heap profile close failed path=%s err=%v", path, err)
	}
}
