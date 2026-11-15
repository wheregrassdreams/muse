package imagecache

import (
	"bytes"
	"strings"
	"testing"
)

func TestStoreRoundTripAndRejectsPathKeys(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	key := strings.Repeat("a", 64)
	data := []byte("encoded image payload")
	if err := store.Write(key, data); err != nil {
		t.Fatal(err)
	}
	got, err := store.Read(key)
	if err != nil || !bytes.Equal(got, data) {
		t.Fatalf("read = %q, %v", got, err)
	}
	if err := store.Delete(key); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Read(key); err == nil {
		t.Fatal("deleted entry is still readable")
	}
	if err := store.Write("../escape", data); err == nil {
		t.Fatal("accepted traversal cache key")
	}
}
