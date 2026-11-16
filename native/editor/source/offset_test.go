package source

import "testing"

func TestOffsetInNormalizedSource(t *testing.T) {
	source := "a\r\nb\r\n"
	tests := []struct {
		name   string
		offset int
		want   int
	}{
		{name: "before first line ending", offset: 1, want: 1},
		{name: "after first line ending", offset: 3, want: 2},
		{name: "between endings", offset: 4, want: 3},
		{name: "after final ending", offset: len(source), want: 4},
		{name: "negative offset", offset: -3, want: 0},
		{name: "past end", offset: len(source) + 3, want: 4},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := OffsetInNormalizedSource(source, test.offset); got != test.want {
				t.Fatalf("OffsetInNormalizedSource(%q, %d) = %d, want %d", source, test.offset, got, test.want)
			}
		})
	}
}
