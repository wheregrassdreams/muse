package nativeui

import (
	"log"
	"os"
	"runtime"
	"runtime/pprof"
	"sync"
	"sync/atomic"
	"time"

	"muse/mdoc"
)

var (
	memoryDiagnosticsOnce sync.Once
	memoryDiagnosticsOn   bool
	memoryDiagnosticsLast atomic.Int64
	memoryProfileOnce     sync.Once
	memoryHighProfileOnce sync.Once
)

const memoryHighProfileThreshold = 50 << 20

func memoryDiagnosticsEnabled() bool {
	memoryDiagnosticsOnce.Do(func() {
		memoryDiagnosticsOn = os.Getenv("MUSE_MEMORY_DIAGNOSTICS") == "1"
	})
	return memoryDiagnosticsOn
}

func startMemoryDiagnostics(app *App) {
	if !memoryDiagnosticsEnabled() {
		return
	}
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for {
			logMemoryDiagnostics(app)
			<-ticker.C
		}
	}()
}

// logMemoryDiagnostics is opt-in so ordinary app frames do not pay for
// runtime memory snapshots. Set MUSE_MEMORY_DIAGNOSTICS=1 to log Go heap,
// decoder queue and decoded-image cache state every five seconds.
func logMemoryDiagnostics(app *App) {
	if !memoryDiagnosticsEnabled() {
		return
	}
	now := time.Now().UnixNano()
	for {
		last := memoryDiagnosticsLast.Load()
		if now-last < int64(5*time.Second) {
			return
		}
		if memoryDiagnosticsLast.CompareAndSwap(last, now) {
			break
		}
	}
	app.mainMu.Lock()
	image := app.ed.MemoryDiagnostics()
	vaultReady := app.vault != nil && app.vault.index != nil
	app.mainMu.Unlock()
	profilePath := os.Getenv("MUSE_MEMORY_PROFILE_PATH")
	// A vault can retain meaningful memory even while the editor is on its
	// blank home page. Wait for its index to become ready, then profile the
	// whole process instead of requiring an image-bearing note to be open.
	if profilePath != "" && vaultReady {
		memoryProfileOnce.Do(func() {
			// The profile is explicitly requested. Collect first so it
			// distinguishes live references from garbage awaiting the next normal
			// GC cycle.
			runtime.GC()
			writeMemoryHeapProfile(profilePath)
		})
	}
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)
	if profilePath != "" && vaultReady && mem.HeapAlloc >= memoryHighProfileThreshold {
		memoryHighProfileOnce.Do(func() {
			beforeGC := mem.HeapAlloc
			// Capture only once after the heap has grown, and collect first to
			// separate retained objects from short-lived allocation churn.
			runtime.GC()
			profilePath := profilePath + ".high"
			writeMemoryHeapProfile(profilePath)
			runtime.ReadMemStats(&mem)
			log.Printf("memory: high-heap profile path=%s threshold=%d heap_alloc_before_gc=%d heap_alloc_after_gc=%d",
				profilePath, memoryHighProfileThreshold, beforeGC, mem.HeapAlloc)
		})
	}
	pending, active := mdoc.ImageDecodeQueueStats()
	cacheEntries, cacheBytes := mdoc.ImageAssetCacheStats()
	log.Printf("memory: heap_alloc=%d heap_inuse=%d heap_idle=%d heap_released=%d heap_sys=%d heap_objects=%d stack_inuse=%d stack_sys=%d sys=%d total_alloc=%d gc_cycles=%d gc_pause_total_ms=%.2f goroutines=%d image_cache_entries=%d image_cache_estimated_bytes=%d image_decode_pending=%d image_decode_active=%d doc_source_bytes=%d doc_leaves=%d doc_virtual=%t doc_image_candidate_leaves=%d doc_image_views=%d doc_image_assets=%d doc_loaded_image_assets=%d doc_image_pixel_bytes_estimate=%d",
		mem.HeapAlloc, mem.HeapInuse, mem.HeapIdle, mem.HeapReleased, mem.HeapSys, mem.HeapObjects,
		mem.StackInuse, mem.StackSys, mem.Sys, mem.TotalAlloc, mem.NumGC,
		float64(mem.PauseTotalNs)/1e6, runtime.NumGoroutine(), cacheEntries, cacheBytes, pending, active,
		image.SourceBytes, image.Leaves, image.VirtualLayout, image.ImageCandidateLeaves, image.ImageAssetViews,
		image.ImageAssets, image.LoadedImageAssets, image.ImagePixelBytes)
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
