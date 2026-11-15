package diagrams

import (
	"strings"
	"testing"
)

func TestParseReturnsBackendNeutralShapesAndLabels(t *testing.T) {
	measured := 0
	result, ok := Parse("flowchart TD\nA[Start] --> B[结束]", Options{
		Measure: func(text string, _ float64, _ bool) float64 {
			measured++
			return float64(len([]rune(text))) * 12
		},
	})
	if !ok {
		t.Fatal("flowchart was not parsed")
	}
	if result.Width <= 0 || result.Height <= 0 || result.Shapes == "" {
		t.Fatalf("incomplete diagram result: %+v", result)
	}
	if len(result.Labels) < 2 {
		t.Fatalf("labels = %+v, want at least two", result.Labels)
	}
	var text strings.Builder
	for _, label := range result.Labels {
		text.WriteString(label.Text)
	}
	if !strings.Contains(text.String(), "Start") || !strings.Contains(text.String(), "结束") {
		t.Fatalf("label text = %q", text.String())
	}
	if measured == 0 {
		t.Fatal("page text measurement was not passed to sketch")
	}
}

func TestParseRejectsUnsupportedDiagram(t *testing.T) {
	if _, ok := Parse("not a diagram", Options{}); ok {
		t.Fatal("unsupported source was accepted")
	}
}
