package render

import "testing"

func TestStyleHeading(t *testing.T) {
	for _, tc := range []struct {
		level int
		want  HeadingStyle
	}{
		{1, HeadingStyle{Size: 40, Weight: 700, LineHeight: 1.25, TopMargin: 51.2}},
		{3, HeadingStyle{Size: 21.6, Weight: 600, LineHeight: 1.35, TopMargin: 40}},
		{6, HeadingStyle{Size: 16, Weight: 600, LineHeight: 1.35, TopMargin: 32}},
		{0, HeadingStyle{Size: 40, Weight: 700, LineHeight: 1.25, TopMargin: 51.2}},
	} {
		if got := StyleHeading(tc.level, 16); got != tc.want {
			t.Errorf("StyleHeading(%d) = %+v, want %+v", tc.level, got, tc.want)
		}
	}
}
