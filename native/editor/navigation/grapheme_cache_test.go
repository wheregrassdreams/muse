package navigation

import "testing"

func TestGraphemeBoundaryCacheBuildsOncePerRevision(t *testing.T) {
	var cache GraphemeBoundaryCache
	first := cache.Edges("a👨‍👩‍👧‍👦b", 1)
	if got := cache.Edges("a👨‍👩‍👧‍👦b", 1); len(got) != len(first) {
		t.Fatal("cache returned inconsistent boundaries for one revision")
	}
	if got := cache.BuildCount(); got != 1 {
		t.Fatalf("build count after same revision = %d; want 1", got)
	}
	cache.Edges("甲乙", 2)
	if got := cache.BuildCount(); got != 2 {
		t.Fatalf("build count after revision change = %d; want 2", got)
	}
}

func TestBoundaryIndexMovementClampsOffsets(t *testing.T) {
	edges := []int{0, 1, 5, 6}
	for _, tc := range []struct {
		name string
		got  int
		want int
	}{
		{"next from inside cluster", NextBoundary(edges, 6, 2), 5},
		{"next after end", NextBoundary(edges, 6, 10), 6},
		{"previous from inside cluster", PreviousBoundary(edges, 6, 4), 1},
		{"previous before start", PreviousBoundary(edges, 6, -4), 0},
		{"snap backward", SnapBoundary(edges, 6, 4, -1), 1},
		{"snap forward", SnapBoundary(edges, 6, 4, 1), 5},
	} {
		if tc.got != tc.want {
			t.Errorf("%s = %d; want %d", tc.name, tc.got, tc.want)
		}
	}
}
