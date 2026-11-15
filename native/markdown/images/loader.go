package images

import (
	"os"
	"path/filepath"
	"strings"
	"time"
	"unsafe"
)

const (
	// DefaultAssetCacheBytes is the single decoded-image retention budget.
	DefaultAssetCacheBytes = 64 << 20
	// MaxCachedDataSourceBytes bounds retained embedded data URL keys.
	MaxCachedDataSourceBytes = 1 << 20
)

// LoadOptions supplies renderer-neutral source resolution and remote storage.
type LoadOptions struct {
	Source SourceOptions
	Store  CacheStore
}

// Loader coalesces source reads/decodes and retains one bounded decoded LRU.
type Loader struct{ cache *AssetCache }

// NewLoader creates a loader with one decoded LRU of maxCachedBytes. The cache
// weight uses the same estimates as the built-in 64MiB global loader.
func NewLoader(maxCachedBytes int64) *Loader { return &Loader{cache: NewAssetCache(maxCachedBytes)} }

var defaultLoader = NewLoader(DefaultAssetCacheBytes)

// Load returns a stable feature Asset. Source reads and decoding begin
// asynchronously; callbacks run on worker goroutines and should publish only
// thread-safe state or request invalidation.
func Load(src string, options LoadOptions, onSize, onLoad func()) *Asset {
	return defaultLoader.Load(src, options, onSize, onLoad)
}

// CacheStats reports entries and estimated retained bytes for this loader.
func (l *Loader) CacheStats() (int, int64) {
	if l == nil {
		return 0, 0
	}
	return l.cache.Stats()
}

// Load returns a stable feature Asset from this loader's cache.
func (l *Loader) Load(src string, options LoadOptions, onSize, onLoad func()) *Asset {
	if l == nil {
		l = defaultLoader
	}
	src = strings.TrimSpace(src)
	if src == "" {
		return &Asset{loaded: true, failed: true}
	}
	var key AssetKey
	var identity SourceIdentity
	var path string
	var mod time.Time
	var size int64
	cacheable := true
	switch {
	case isRemote(src):
		key = AssetKey{Kind: AssetRemote, Value: src}
	case strings.HasPrefix(src, "data:"):
		identity = SourceIdentity{Data: unsafe.StringData(src), Length: len(src)}
		if old, ok := l.cache.LoadSource(identity); ok {
			old.Asset.addWaiters(onSize, onLoad)
			return old.Asset
		}
		key = AssetKey{Kind: AssetData, Value: src}
		cacheable = len(src) <= MaxCachedDataSourceBytes
		if cacheable {
			key.Value = strings.Clone(src)
		}
	default:
		path = ResolveSource(src, options.Source)
		if path == "" {
			return &Asset{loaded: true, failed: true}
		}
		absPath, err := filepath.Abs(path)
		if err != nil {
			return &Asset{loaded: true, failed: true}
		}
		path = absPath
		st, err := os.Stat(path)
		if err != nil || !st.Mode().IsRegular() {
			return &Asset{loaded: true, failed: true}
		}
		mod, size = st.ModTime(), st.Size()
		key = AssetKey{Kind: AssetFile, Value: path}
	}
	if old, ok := l.cache.Load(key); ok {
		if key.Kind != AssetFile || old.ModTime == mod.UnixNano() && old.Size == size {
			old.Asset.addWaiters(onSize, onLoad)
			return old.Asset
		}
	}
	a := &Asset{cache: l.cache, cacheKey: key, sizeWaiters: nonNilWaiters(onSize), loadWaiters: nonNilWaiters(onLoad)}
	e := &AssetCacheEntry{Key: key, Asset: a, ModTime: mod.UnixNano(), Size: size, Weight: int64(len(key.Value)), Pinned: true, Cacheable: cacheable, Source: identity}
	if key.Kind == AssetFile {
		if old, ok := l.cache.Load(key); ok && (old.ModTime != mod.UnixNano() || old.Size != size) {
			l.cache.Replace(e)
		} else if got, exists := l.cache.StoreIfAbsent(e); exists {
			got.Asset.addWaiters(onSize, onLoad)
			return got.Asset
		}
	} else if got, exists := l.cache.StoreIfAbsent(e); exists {
		got.Asset.addWaiters(onSize, onLoad)
		return got.Asset
	}
	loadPath := path
	if key.Kind == AssetRemote || key.Kind == AssetData {
		loadPath = src
	}
	LoadSourceBytes(loadPath, SourceOptions{}, options.Store, func(data []byte, err error) {
		if err != nil {
			a.fail(err)
			return
		}
		SubmitDecode(func() {
			decoded, decodeErr := Decode(data, a.publishIntrinsicSize)
			data = nil
			a.finish(decoded, decodeErr)
		})
	})
	return a
}

func nonNilWaiters(fn func()) []func() {
	if fn == nil {
		return nil
	}
	return []func(){fn}
}

// CacheStats reports entries and estimated retained bytes in the global loader.
func CacheStats() (int, int64) { return defaultLoader.cache.Stats() }
