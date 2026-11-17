package navigation

import "testing"

func TestBlockEndIncludesFencesAndSetextUnderline(t *testing.T) {
	cases := []struct {
		name   string
		source string
		block  SourceBlock
		want   int
	}{
		{
			name:   "fenced code with quote prefixes",
			source: "> ```go\n> body\n> ```go\nafter",
			block:  SourceBlock{LineStart: 0, ContentEnd: 13, Kind: SourceBlockCode, Fenced: true},
			want:   22,
		},
		{
			name:   "unclosed code fence",
			source: "```go\nbody",
			block:  SourceBlock{LineStart: 0, ContentEnd: 10, Kind: SourceBlockCode, Fenced: true},
			want:   10,
		},
		{
			name:   "unfenced code",
			source: "body\nafter",
			block:  SourceBlock{LineStart: 0, ContentEnd: 4, Kind: SourceBlockCode},
			want:   4,
		},
		{
			name:   "setext heading",
			source: "title\n---\nafter",
			block:  SourceBlock{LineStart: 0, ContentEnd: 5, Kind: SourceBlockHeading, Setext: true},
			want:   9,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := BlockEnd(tc.source, tc.block); got != tc.want {
				t.Fatalf("BlockEnd() = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestClosingFenceRecognizesBlockKindAndContainerPrefix(t *testing.T) {
	if !IsClosingFence("  > > ~~~ {.go}", SourceBlockCode) {
		t.Fatal("quoted code fence was not recognized")
	}
	if !IsClosingFence(" > \\] ", SourceBlockMath) {
		t.Fatal("quoted math fence was not recognized")
	}
	if IsClosingFence("```", SourceBlockMath) || IsClosingFence("$$", SourceBlockCode) {
		t.Fatal("a fence was accepted for the wrong block kind")
	}
}

func TestNearestStopAtOrBefore(t *testing.T) {
	cases := []struct {
		name   string
		stops  []int
		offset int
		want   int
		ok     bool
	}{
		{name: "empty", offset: 4},
		{name: "before first", stops: []int{2, 5, 9}, offset: 1, want: 2, ok: true},
		{name: "exact", stops: []int{2, 5, 9}, offset: 5, want: 5, ok: true},
		{name: "between", stops: []int{2, 5, 9}, offset: 8, want: 5, ok: true},
		{name: "after last", stops: []int{2, 5, 9}, offset: 12, want: 9, ok: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := NearestStopAtOrBefore(tc.stops, tc.offset)
			if got != tc.want || ok != tc.ok {
				t.Fatalf("NearestStopAtOrBefore(%v, %d) = (%d, %t), want (%d, %t)", tc.stops, tc.offset, got, ok, tc.want, tc.ok)
			}
		})
	}
}
