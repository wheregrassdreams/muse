package markdown

import (
	"muse/locales"
	"github.com/wheregrassdreams/muse-editor/markdown/diagrams"
)

// adaptStateDiagram attaches the backend-neutral state graph to the native
// flowchart drawing/layout adapter.
func adaptStateDiagram(source string) (*mermaidGraph, bool) {
	model, ok := diagrams.ParseState(source, diagrams.StateOptions{StartLabel: locales.DiagramStart(), EndLabel: locales.DiagramEnd()})
	if !ok {
		return nil, false
	}
	return adaptStateModel(model), true
}

func adaptStateModel(model *diagrams.StateDiagram) *mermaidGraph {
	return adaptFlowchart(model.Flowchart())
}
