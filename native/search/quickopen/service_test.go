package quickopen

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func TestServiceRepeatedQueryCachesDetachedRankedResults(t *testing.T) {
	svc := NewService(4)
	candidates := []Candidate{
		{Path: "z/guide.md", PathText: "notes/guide.md", Title: "快速开始", Aliases: []string{"入门"}},
		{Path: "a/guide.md", PathText: "notes/guide.md", Title: "快速开始"},
	}
	svc.ReplaceSnapshot(7, candidates)
	candidates[0].Aliases[0] = "mutated"
	first := svc.Search("快速")
	if got := pathsOf(first); !reflect.DeepEqual(got, []string{"a/guide.md", "z/guide.md"}) {
		t.Fatalf("rank order = %v", got)
	}
	if !reflect.DeepEqual(first[0].TitleMatches, []int{0, 1}) {
		t.Fatalf("title highlights = %v", first[0].TitleMatches)
	}
	if len(svc.queryCache) != 1 {
		t.Fatalf("cache entries = %d, want 1", len(svc.queryCache))
	}
	if got := svc.Search("入门"); len(got) != 1 || got[0].Path != "z/guide.md" {
		t.Fatalf("caller alias mutation changed snapshot: %+v", got)
	}
	first[0].TitleMatches[0] = 99
	second := svc.Search(" 快速 ")
	if !reflect.DeepEqual(second[0].TitleMatches, []int{0, 1}) {
		t.Fatalf("cached result leaked caller mutation: %v", second[0].TitleMatches)
	}
	if len(svc.queryCache) != 2 {
		t.Fatalf("normalized repeat grew cache: %d", len(svc.queryCache))
	}
	if svc.Revision() != 7 {
		t.Fatalf("revision = %d, want 7", svc.Revision())
	}
}

func TestServiceSnapshotRevisionInvalidatesQueryResults(t *testing.T) {
	svc := NewService(4)
	svc.ReplaceSnapshot(1, []Candidate{{Path: "old.md", PathText: "guide.md", Title: "Guide"}})
	if got := svc.Search("guide"); len(got) != 1 || got[0].Path != "old.md" {
		t.Fatalf("revision 1 results = %+v", got)
	}
	svc.ReplaceSnapshot(2, []Candidate{{Path: "new.md", PathText: "guide.md", Title: "Guide"}})
	if len(svc.queryCache) != 0 || svc.Revision() != 2 {
		t.Fatalf("snapshot replace did not invalidate: revision=%d cache=%d", svc.Revision(), len(svc.queryCache))
	}
	if got := svc.Search("guide"); len(got) != 1 || got[0].Path != "new.md" {
		t.Fatalf("revision 2 results = %+v", got)
	}
}

func TestServiceCacheEntryAndByteBounds(t *testing.T) {
	svc := NewService(2)
	svc.ReplaceSnapshot(1, []Candidate{
		{Path: "a.md", PathText: "alpha", Title: "alpha"},
		{Path: "b.md", PathText: "bravo", Title: "bravo"},
	})
	for _, query := range []string{"alpha", "bravo", "a", "b", "alpha"} {
		svc.Search(query)
	}
	if len(svc.queryCache) > 2 || len(svc.cacheOrder) > 2 {
		t.Fatalf("query cache exceeded entry cap: %d/%d", len(svc.queryCache), len(svc.cacheOrder))
	}
	if svc.cacheBytes > maxQueryCacheBytes {
		t.Fatalf("cache retained %d bytes, cap %d", svc.cacheBytes, maxQueryCacheBytes)
	}

	byteSvc := NewService(100)
	var phrase strings.Builder
	for n := range 12 {
		fmt.Fprintf(&phrase, "a%d ", n)
	}
	shared := phrase.String()
	byteCandidates := make([]Candidate, maxCachedResults)
	for i := range byteCandidates {
		byteCandidates[i] = Candidate{Path: fmt.Sprintf("%05d.md", i), PathText: shared, Title: shared}
	}
	byteSvc.ReplaceSnapshot(1, byteCandidates)
	for n := range 12 {
		byteSvc.Search(fmt.Sprintf("a%d", n))
	}
	if byteSvc.cacheBytes > maxQueryCacheBytes {
		t.Fatalf("byte cache retained %d bytes, cap %d", byteSvc.cacheBytes, maxQueryCacheBytes)
	}
	if len(byteSvc.queryCache) >= 12 {
		t.Fatalf("byte budget did not evict entries: %d entries, %d bytes", len(byteSvc.queryCache), byteSvc.cacheBytes)
	}

	large := make([]Candidate, maxCachedResults+1)
	for i := range large {
		large[i] = Candidate{Path: fmt.Sprintf("%05d.md", i), PathText: "match", Title: "match"}
	}
	svc.ReplaceSnapshot(2, large)
	if got := svc.Search("match"); len(got) != len(large) {
		t.Fatalf("uncached large result count = %d, want %d", len(got), len(large))
	}
	if len(svc.queryCache) != 0 {
		t.Fatalf("oversized result was retained: cache entries=%d", len(svc.queryCache))
	}

	longQuery := strings.Repeat("x", maxCachedQueryBytes+1)
	svc.Search(longQuery)
	if len(svc.queryCache) != 0 {
		t.Fatalf("oversized query key was retained: cache entries=%d", len(svc.queryCache))
	}

	largeText := strings.Repeat("x", maxQueryCacheBytes) + "needle"
	for _, candidate := range []Candidate{
		{Path: "large-title.md", PathText: "large-title.md", Title: largeText},
		{Path: largeText, PathText: "needle", Title: "ordinary"},
	} {
		svc.ReplaceSnapshot(3, []Candidate{candidate})
		if got := svc.Search("needle"); len(got) != 1 {
			t.Fatalf("large field result count = %d, want 1", len(got))
		}
		if len(svc.queryCache) != 0 {
			t.Fatalf("result retaining an oversized title/path was cached: entries=%d bytes=%d", len(svc.queryCache), svc.cacheBytes)
		}
	}
}

func pathsOf(matches []Match) []string {
	paths := make([]string, len(matches))
	for i := range matches {
		paths[i] = matches[i].Path
	}
	return paths
}
