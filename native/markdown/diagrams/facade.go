// Package diagrams preserves Muse's historical import path for the
// backend-neutral Mermaid models and SVG shape projection.
package diagrams

import model "github.com/wheregrassdreams/muse-editor/markdown/diagrams"

const PieWidth = model.PieWidth

type Theme = model.Theme
type Options = model.Options
type Label = model.Label
type Result = model.Result
type ModelKind = model.ModelKind
type Model = model.Model
type GanttChart = model.GanttChart
type GanttTask = model.GanttTask
type GanttRow = model.GanttRow
type GanttOptions = model.GanttOptions
type Class = model.Class
type ClassRelation = model.ClassRelation
type ClassDiagram = model.ClassDiagram
type ClassRelationLayout = model.ClassRelationLayout
type DiagramRect = model.DiagramRect
type FlowNode = model.FlowNode
type FlowEdge = model.FlowEdge
type Flowchart = model.Flowchart
type FlowchartOptions = model.FlowchartOptions
type PieSlice = model.PieSlice
type PieChart = model.PieChart
type Point = model.Point
type Segment = model.Segment
type SequenceDiagram = model.SequenceDiagram
type SequenceEvent = model.SequenceEvent
type SequenceEventKind = model.SequenceEventKind
type SequenceParticipant = model.SequenceParticipant
type SequencePosition = model.SequencePosition
type StateDiagram = model.StateDiagram
type StateEdge = model.StateEdge
type StateNode = model.StateNode
type StateOptions = model.StateOptions

const (
	ModelSketch    = model.ModelSketch
	ModelGantt     = model.ModelGantt
	ModelClass     = model.ModelClass
	ModelPie       = model.ModelPie
	ModelState     = model.ModelState
	ModelSequence  = model.ModelSequence
	ModelFlowchart = model.ModelFlowchart

	SequenceMessage = model.SequenceMessage
	SequenceNote    = model.SequenceNote
)

var (
	Parse              = model.Parse
	ParseModel         = model.ParseModel
	ParseFallbackModel = model.ParseFallbackModel
	ParseGantt         = model.ParseGantt
	ParseClass         = model.ParseClass
	ParsePie           = model.ParsePie
	ParseState         = model.ParseState
	ParseSequence      = model.ParseSequence
	ParseFlowchart     = model.ParseFlowchart
)
