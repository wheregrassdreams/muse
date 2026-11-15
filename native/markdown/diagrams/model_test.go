package diagrams

import (
	"testing"
	"time"
)

func TestParseFallbackModelSelectsNeutralDiagramVariants(t *testing.T) {
	tests := []struct {
		name string
		src  string
		kind ModelKind
	}{
		{"gantt", "gantt\ntitle Plan\nsection Work\nTask :a1, 2024-01-01, 3d\n", ModelGantt},
		{"class", "classDiagram\nclass A\nclass B\nA --> B\n", ModelClass},
		{"pie", "pie title Pets\n\"Dogs\": 3\n", ModelPie},
		{"state", "stateDiagram\n[*] --> A\nA --> B\n", ModelState},
		{"sequence", "sequenceDiagram\nAlice->>Bob: Hi\n", ModelSequence},
		{"flowchart", "graph LR\nA[一] --> B[二]\n", ModelFlowchart},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			model, ok := ParseFallbackModel(test.src, Options{
				DefaultSection: "Tasks", StartLabel: "Start", EndLabel: "End",
				Today: time.Date(2026, time.January, 2, 0, 0, 0, 0, time.UTC),
			})
			if !ok {
				t.Fatal("fallback parser rejected supported diagram")
			}
			if model.Kind != test.kind {
				t.Fatalf("model kind = %v, want %v", model.Kind, test.kind)
			}
			if model.Width() <= 0 || model.Height(model.Width()) <= 0 {
				t.Fatalf("model dimensions = %gx%g", model.Width(), model.Height(model.Width()))
			}
		})
	}
}

func TestParseModelReturnsSketchBeforeFallback(t *testing.T) {
	model, ok := ParseModel("graph TD\nA --> B\n", Options{})
	if !ok {
		t.Fatal("sketch parser rejected simple graph")
	}
	if model.Kind != ModelSketch || model.Width() <= 0 || model.Height(0) <= 0 {
		t.Fatalf("sketch model = %+v", model)
	}
}
