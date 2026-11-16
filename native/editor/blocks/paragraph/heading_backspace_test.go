package editor

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestBackspaceHeadingSourceOffsets(t *testing.T) {
	for _, tc := range []struct {
		name  string
		src   string
		off   int
		want  string
		caret int
	}{
		{"before markers at document start", "## Title", 0, "## Title", 0},
		{"before markers after paragraph", "intro\n## Title", 6, "intro## Title", 5},
		{"before markers after blank line", "intro\n\n## Title", 7, "intro\n## Title", 6},
		{"before indented markers", "  ## Title", 2, " ## Title", 1},
		{"between hashes", "## Title", 1, "# Title", 0},
		{"after hashes before space", "## Title", 2, "# Title", 1},
		{"after markers", "## Title", 3, "Title", 0},
		{"after markers after paragraph", "intro\n\n## Title", 10, "intro\n\nTitle", 7},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(t, tc.src)
			h.caretAt(tc.off)
			if h.e.caret != tc.off {
				t.Fatalf("initial caret = %d, want source offset %d", h.e.caret, tc.off)
			}
			h.key(0, ui.KeyBackspace)
			h.want(tc.want)
			if h.e.caret != tc.caret || h.e.anchor != tc.caret {
				t.Fatalf("selection = %d..%d, want %d", h.e.anchor, h.e.caret, tc.caret)
			}
		})
	}
}

func TestBackspaceBeforeHeadingMarkersAfterHome(t *testing.T) {
	h := newHarness(t, "intro\n\n## Title")
	h.caretAt(10)
	h.key(0, ui.KeyLeft)
	h.key(0, ui.KeyHome)
	if h.e.caret != 7 {
		t.Fatalf("Home caret = %d, want 7 before ##", h.e.caret)
	}
	h.key(0, ui.KeyBackspace)
	h.want("intro\n## Title")
}
