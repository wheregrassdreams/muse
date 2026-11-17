package rank

import "testing"

func TestScoreOverlayUsesTitlePathMetadataAndBody(t *testing.T) {
	tests := []struct {
		name  string
		input OverlayInput
		want  float64
		match bool
	}{
		{
			name:  "exact title and body",
			input: OverlayInput{Query: "Guide", Title: "Guide", Source: "guide body"},
			want:  26,
			match: true,
		},
		{
			name:  "path prefix and alias",
			input: OverlayInput{Query: "guide", Path: "guide/start.md", Aliases: []string{"Quick Guide"}},
			want:  17,
			match: true,
		},
		{
			name:  "token fallback",
			input: OverlayInput{Query: "quick guide", Title: "Quick start"},
			want:  4,
			match: true,
		},
		{
			name:  "no match",
			input: OverlayInput{Query: "missing", Title: "Guide", Source: "body"},
			match: false,
		},
		{
			name:  "empty query",
			input: OverlayInput{Query: "  ", Title: "Guide"},
			match: false,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, matched := ScoreOverlay(test.input)
			if got != test.want || matched != test.match {
				t.Fatalf("ScoreOverlay() = (%v, %v), want (%v, %v)", got, matched, test.want, test.match)
			}
		})
	}
}
