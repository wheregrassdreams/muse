// Package images owns bounded remote image fetching and its optional disk
// cache. The application supplies the cache root; this package has no
// platform path policy and accepts image validation as a domain callback.
package images

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"time"
)

const maxRemoteBytes = 20 << 20

// Validate checks image headers without allocating decoded pixel buffers.
type Validate func([]byte) error

// CacheStore is the disk-cache port used by the remote image pipeline.
// Implementations own storage location, permissions, and atomic persistence.
type CacheStore interface {
	Read(key string) ([]byte, error)
	Write(key string, data []byte) error
	Delete(key string) error
}

// RemoteCache fetches remote image source bytes and optionally persists
// validated responses below cacheRoot/images. An empty root disables disk IO.
type RemoteCache struct {
	store    CacheStore
	validate Validate
}

// NewRemoteCache creates a fetcher with an application supplied cache store.
func NewRemoteCache(store CacheStore, validate Validate) *RemoteCache {
	return &RemoteCache{store: store, validate: validate}
}

// Key returns the stable opaque cache key for src.
func (c *RemoteCache) Key(src string) string {
	digest := sha256.Sum256([]byte(src))
	return hex.EncodeToString(digest[:])
}

// Fetch reads a valid cached response or downloads and atomically caches it.
func (c *RemoteCache) Fetch(src string) ([]byte, error) {
	key := c.Key(src)
	if c.store != nil {
		if data, err := c.store.Read(key); err == nil {
			if len(data) > 0 && len(data) <= maxRemoteBytes && c.valid(data) == nil {
				return data, nil
			}
			_ = c.store.Delete(key)
		}
	}
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Get(src)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("download image: HTTP %s", resp.Status)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxRemoteBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxRemoteBytes {
		return nil, fmt.Errorf("download image: response exceeds %d bytes", maxRemoteBytes)
	}
	if err := c.valid(data); err != nil {
		return nil, err
	}
	if c.store != nil {
		_ = c.store.Write(key, data)
	}
	return data, nil
}

func (c *RemoteCache) valid(data []byte) error {
	if c.validate == nil {
		return fmt.Errorf("remote image cache: validator is required")
	}
	return c.validate(data)
}
