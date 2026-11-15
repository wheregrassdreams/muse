package images

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
)

type memoryStore struct {
	mu   sync.Mutex
	data map[string][]byte
}

func (s *memoryStore) Read(key string) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]byte(nil), s.data[key]...), nil
}

func (s *memoryStore) Write(key string, data []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.data == nil {
		s.data = make(map[string][]byte)
	}
	s.data[key] = append([]byte(nil), data...)
	return nil
}

func (s *memoryStore) Delete(key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.data, key)
	return nil
}

func TestRemoteCacheUsesStorePortAndReusesValidatedBytes(t *testing.T) {
	store := &memoryStore{}
	var requests atomic.Int32
	var unavailable atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		if unavailable.Load() {
			http.Error(w, "offline", http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte("image bytes"))
	}))
	defer server.Close()
	validate := func(data []byte) error {
		if !bytes.Equal(data, []byte("image bytes")) {
			return fmt.Errorf("invalid image fixture")
		}
		return nil
	}
	cache := NewRemoteCache(store, validate)
	got, err := cache.Fetch(server.URL)
	if err != nil || !bytes.Equal(got, []byte("image bytes")) {
		t.Fatalf("Fetch() = %q, %v", got, err)
	}
	if cache.Key(server.URL) == "" {
		t.Fatal("cache key is empty")
	}
	unavailable.Store(true)
	got, err = cache.Fetch(server.URL)
	if err != nil || !bytes.Equal(got, []byte("image bytes")) {
		t.Fatalf("Fetch() did not use persisted cache: %q, %v", got, err)
	}
	if got := requests.Load(); got != 1 {
		t.Fatalf("server received %d requests, want 1", got)
	}
}

func TestRemoteCacheWithoutStoreDoesNotPersist(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("image bytes"))
	}))
	defer server.Close()
	cache := NewRemoteCache(nil, func([]byte) error { return nil })
	if got, err := cache.Fetch(server.URL); err != nil || string(got) != "image bytes" {
		t.Fatalf("Fetch() = %q, %v", got, err)
	}
}
