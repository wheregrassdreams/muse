package mermaidpreview

import (
	"testing"

	"github.com/wheregrassdreams/muse-editor/markdown/diagrams"
)

func TestNewContentSnapshotsDiagramLabels(t *testing.T) {
	model := diagrams.Result{Width: 20, Height: 10, Shapes: "<svg/>", Labels: []diagrams.Label{{Text: "before"}}}
	content := NewContent("flowchart TD", model, nil)
	model.Labels[0].Text = "mutated"
	if got := content.model.Labels[0].Text; got != "before" {
		t.Fatalf("surface retained caller label storage: %q", got)
	}
	content.Close()
	content.Close()
	if !content.closed || content.model.Width != 0 {
		t.Fatalf("closed surface retained model: closed=%v model=%+v", content.closed, content.model)
	}
}
