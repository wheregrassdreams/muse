package markdown

import (
	"muse/locales"
	"muse/markdown/diagrams"
)

// adaptStateDiagram attaches the backend-neutral state graph to the native
// flowchart drawing/layout adapter.
func adaptStateDiagram(source string) (*mermaidGraph, bool) {
	model, ok := diagrams.ParseState(source, diagrams.StateOptions{StartLabel: locales.DiagramStart(), EndLabel: locales.DiagramEnd()})
	if !ok {
		return nil, false
	}
	return adaptFlowchart(model.Flowchart()), true
}
