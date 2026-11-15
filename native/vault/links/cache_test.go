package links

import (
	"fmt"
	"testing"
)

func TestBacklinkQueryCacheHasBoundedCapacity(t *testing.T) {
	idx := New()
	if err := idx.Rebuild(t.TempDir(), nil); err != nil {
		t.Fatal(err)
	}
	for n := range backlinksCacheLimit + 10 {
		idx.Backlinks(fmt.Sprintf("missing-%d.md", n))
	}
	if len(idx.backlinksCache) != backlinksCacheLimit || len(idx.backlinksOrder) != backlinksCacheLimit {
		t.Fatalf("backlink cache sizes = %d/%d, limit %d", len(idx.backlinksCache), len(idx.backlinksOrder), backlinksCacheLimit)
	}
}
