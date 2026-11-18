package paragraph

import "testing"

func TestMarkerWidthUsesMarkdownContinuationColumns(t *testing.T) {
	for _, tc := range []struct {
		source string
		at     int
		want   int
	}{
		{"- item\n", 0, 2},
		{"123. item", 0, 5},
		{"*    item", 0, 2},
		{"*", 0, 2},
		{"x", -1, 0},
	} {
		if got := MarkerWidth(tc.source, tc.at); got != tc.want {
			t.Errorf("MarkerWidth(%q, %d) = %d, want %d", tc.source, tc.at, got, tc.want)
		}
	}
}

func TestContainerContinuationAndBlankPrefixes(t *testing.T) {
	if got, want := ContinuationPrefix([]ContainerKind{QuoteContainer, ListContainer, QuoteContainer}), ">   > "; got != want {
		t.Fatalf("ContinuationPrefix = %q, want %q", got, want)
	}
	if got, want := BlankListMarkers("> - [x] "), ">       "; got != want {
		t.Fatalf("BlankListMarkers = %q, want %q", got, want)
	}
	if got, want := BlankParagraphPrefix(">   >   "), ">   >"; got != want {
		t.Fatalf("BlankParagraphPrefix = %q, want %q", got, want)
	}
}
