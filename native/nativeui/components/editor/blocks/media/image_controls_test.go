package media

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

type resizeHost struct {
	source      string
	from, to    int
	replacement string
}

func (h *resizeHost) Source() string { return h.source }

func (h *resizeHost) CommitImageResize(from, to int, replacement string) {
	h.from, h.to, h.replacement = from, to, replacement
}

func TestResizeImageSourcePreservesMarkdownReferenceLookup(t *testing.T) {
	for _, tc := range []struct {
		name, source, image string
		widthFrom, widthTo  int
		want                string
	}{
		{name: "shortcut", source: "![photo]\n\n[photo]: photo.png", image: "![photo]", want: "![photo|220][photo]"},
		{name: "collapsed", source: "![photo][]\n\n[photo]: photo.png", image: "![photo][]", want: "![photo|220][photo]"},
		{name: "nested alt", source: "![nested [alt]][pic]\n\n[pic]: photo.png", image: "![nested [alt]][pic]", want: "![nested [alt]|220][pic]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			from := 0
			to := len(tc.image)
			wf, wt := AltWidthRange(tc.source, from, to)
			host := &resizeHost{source: tc.source}
			if !ResizeImageSource(host, ImageSource{From: from, To: to, WidthFrom: wf, WidthTo: wt, Syntax: ImageSyntaxMarkdown}, 220, 0, 0) {
				t.Fatal("resize was rejected")
			}
			if got := host.source[:host.from] + host.replacement + host.source[host.to:]; got != tc.want+tc.source[to:] {
				t.Fatalf("resized source %q, want prefix %q", got, tc.want+tc.source[to:])
			}
		})
	}
}

func TestResizeImageSourcePreservesHTMLAttributesAndHeightRatio(t *testing.T) {
	source := `<img data-width="x" src="x.png" width='40' height="20" />`
	host := &resizeHost{source: source}
	if !ResizeImageSource(host, ImageSource{From: 0, To: len(source), Syntax: ImageSyntaxHTML}, 200, 120, 60) {
		t.Fatal("resize was rejected")
	}
	if got, want := host.replacement, `<img data-width="x" src="x.png" width='200' height="100" />`; got != want {
		t.Fatalf("HTML resize %q, want %q", got, want)
	}
}

func TestImageAlignmentStateShiftsAndDropsEditedOffsets(t *testing.T) {
	var state ImageAlignmentState
	if !state.Centered(10) {
		t.Fatal("unset alignment should default to centered")
	}
	if state.Toggle(10) {
		t.Fatal("first toggle should left-align")
	}
	state.Toggle(30)
	state.Shift(8, 12, 4)
	if !state.Centered(10) {
		t.Fatal("edited image alignment should reset to default")
	}
	if state.Centered(34) {
		t.Fatal("later image offset was not shifted")
	}
}

func TestImageResizeGestureClampsPreviewWidth(t *testing.T) {
	gesture := NewImageResizeGesture(10, 20, 80)
	if got := gesture.Drag(-1000, -1000); got != 40 {
		t.Fatalf("width %v, want clamp 40", got)
	}
	if got := gesture.Drag(30, 40); got != 100 {
		t.Fatalf("width %v, want 100", got)
	}
}

func TestImageToolPresentationAndHitTargetsAgree(t *testing.T) {
	box := ui.Rect{X: 20, Y: 30, W: 300, H: 180}
	buttons := ImageToolButtons(box, 16, true, false)
	if len(buttons) != 3 || buttons[1].Label != "↤" {
		t.Fatalf("standalone image tools = %#v", buttons)
	}
	if got := ImageToolAt(box, 16, buttons[0].Bounds.X+1, buttons[0].Bounds.Y+1, true); got != ImageToolPreview {
		t.Fatalf("preview hit = %v", got)
	}
	_, _, _, grip := ToolRects(box, 16)
	if got := ImageToolAt(box, 16, grip.X+1, grip.Y+1, false); got != ImageToolResize {
		t.Fatalf("resize hit = %v", got)
	}
}

func TestSetHTMLAttrsPreservesClosingForm(t *testing.T) {
	for _, tc := range []struct{ tag, want string }{
		{`<img src="x.png" alt="x">`, `<img src="x.png" alt="x" width="200">`},
		{`<img data-width="x" src="x.png" width='40' height="20" />`, `<img data-width="x" src="x.png" width='200' height="20" />`},
	} {
		got := setHTMLAttrs(tc.tag, htmlAttribute{name: "width", value: "200"})
		if got != tc.want {
			t.Errorf("setHTMLAttrs(%q)=%q, want %q", tc.tag, got, tc.want)
		}
	}
}
