package paragraph

import "testing"

func TestSourcePrefix(t *testing.T) {
	for _, tc := range []struct {
		name                    string
		source                  string
		lineStart, contentStart int
		wantStart, wantEnd      int
		wantOK                  bool
	}{
		{name: "one space", source: "alpha\n beta", lineStart: 6, contentStart: 7, wantStart: 6, wantEnd: 7, wantOK: true},
		{name: "two spaces", source: "alpha\n  beta", lineStart: 6, contentStart: 8, wantStart: 6, wantEnd: 8, wantOK: true},
		{name: "container marker", source: "alpha\n> beta", lineStart: 6, contentStart: 8},
		{name: "no prefix", source: "alpha\nbeta", lineStart: 6, contentStart: 6},
	} {
		t.Run(tc.name, func(t *testing.T) {
			start, end, ok := SourcePrefix(tc.source, tc.lineStart, tc.contentStart)
			if start != tc.wantStart || end != tc.wantEnd || ok != tc.wantOK {
				t.Fatalf("SourcePrefix() = (%d, %d, %v), want (%d, %d, %v)", start, end, ok, tc.wantStart, tc.wantEnd, tc.wantOK)
			}
		})
	}
}
