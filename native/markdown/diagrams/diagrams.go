// Package diagrams owns backend-neutral parsing and SVG-to-label projection
// for Mermaid diagrams rendered by the sketch engine.
package diagrams

import (
	"image/color"
	"time"

	"tuios.dev/sketch"
)

// Theme carries palette tokens accepted by sketch. Zero colors preserve the
// renderer's defaults.
type Theme struct {
	Bg, Fg, Muted, Line, Surface, SurfaceAlt, Border, Accent color.RGBA
	Radius                                                   float64
}

// Options supplies optional page theme and text measurement policy.
type Options struct {
	Theme          *Theme
	Measure        func(text string, size float64, bold bool) float64
	DefaultSection string
	StartLabel     string
	EndLabel       string
	Today          time.Time
}

// Label is one text run in a diagram's SVG coordinate space.
type Label struct {
	X, Y, Size float32
	Bold       bool
	Color      color.RGBA
	Anchor     int
	Text       string
}

// Result separates vector shapes from labels so a host can rasterize shapes
// and draw labels with its own text renderer.
type Result struct {
	Width, Height float32
	Shapes        string
	Labels        []Label
}

// ModelKind identifies a parsed diagram representation without choosing a
// drawing backend.
type ModelKind uint8

const (
	ModelSketch ModelKind = iota + 1
	ModelGantt
	ModelClass
	ModelPie
	ModelState
	ModelSequence
	ModelFlowchart
)

// Model is the typed parser result for one supported Mermaid subset. Its
// payload belongs to the caller and should be treated as read-only after parse.
// Exactly one payload is set according to Kind.
type Model struct {
	Kind      ModelKind
	Sketch    Result
	Gantt     *GanttChart
	Class     *ClassDiagram
	Pie       *PieChart
	State     *StateDiagram
	Sequence  *SequenceDiagram
	Flowchart *Flowchart
}

// ParseModel applies the application's diagram parser order and returns a
// backend-neutral model. Sketch handles the broad Mermaid subset first; the
// typed parsers preserve supported legacy fallbacks where sketch declines.
func ParseModel(source string, opts Options) (Model, bool) {
	if result, ok := Parse(source, opts); ok {
		result.Labels = append([]Label(nil), result.Labels...)
		return Model{Kind: ModelSketch, Sketch: result}, true
	}
	return ParseFallbackModel(source, opts)
}

// ParseFallbackModel applies the typed legacy parser order when the broad
// sketch parser is unavailable or declined a diagram.
func ParseFallbackModel(source string, opts Options) (Model, bool) {
	if chart, ok := ParseGantt(source, GanttOptions{DefaultSection: opts.DefaultSection, Today: opts.Today}); ok {
		return Model{Kind: ModelGantt, Gantt: chart}, true
	}
	if chart, ok := ParseClass(source); ok {
		return Model{Kind: ModelClass, Class: chart}, true
	}
	if chart, ok := ParsePie(source); ok {
		return Model{Kind: ModelPie, Pie: chart}, true
	}
	if chart, ok := ParseState(source, StateOptions{StartLabel: opts.StartLabel, EndLabel: opts.EndLabel}); ok {
		return Model{Kind: ModelState, State: chart}, true
	}
	if chart, ok := ParseSequence(source); ok {
		return Model{Kind: ModelSequence, Sequence: chart}, true
	}
	if chart, ok := ParseFlowchart(source, FlowchartOptions{StartLabel: opts.StartLabel}); ok {
		return Model{Kind: ModelFlowchart, Flowchart: chart}, true
	}
	return Model{}, false
}

// Width returns the natural width used by editor virtualization and the UI
// adapter for this model.
func (m Model) Width() float32 {
	switch m.Kind {
	case ModelSketch:
		return m.Sketch.Width
	case ModelGantt:
		if m.Gantt == nil {
			return 0
		}
		return m.Gantt.Width()
	case ModelClass:
		if m.Class == nil {
			return 0
		}
		return m.Class.Width()
	case ModelPie:
		return PieWidth
	case ModelState:
		if m.State == nil {
			return 0
		}
		return m.State.Flowchart().Width()
	case ModelSequence:
		if m.Sequence == nil {
			return 0
		}
		return m.Sequence.Width()
	case ModelFlowchart:
		if m.Flowchart == nil {
			return 0
		}
		return m.Flowchart.Width()
	default:
		return 0
	}
}

// Height returns the model's height at the requested drawing width.
func (m Model) Height(width float32) float32 {
	switch m.Kind {
	case ModelSketch:
		return m.Sketch.Height
	case ModelGantt:
		if m.Gantt == nil {
			return 0
		}
		return m.Gantt.Height()
	case ModelClass:
		if m.Class == nil {
			return 0
		}
		return m.Class.Height()
	case ModelPie:
		return 286
	case ModelState:
		if m.State == nil {
			return 0
		}
		return m.State.Flowchart().Height()
	case ModelSequence:
		if m.Sequence == nil {
			return 0
		}
		return m.Sequence.Height()
	case ModelFlowchart:
		if m.Flowchart == nil {
			return 0
		}
		return m.Flowchart.Height()
	default:
		return 0
	}
}

// Parse renders Mermaid source to a backend-neutral diagram result.
func Parse(source string, opts Options) (Result, bool) {
	out, err := sketch.Render(source, sketch.RenderOptions{
		Theme:    sketchTheme(opts.Theme),
		Measurer: sketchMeasurer(opts.Measure),
	})
	if err != nil {
		return Result{}, false
	}
	w, h := float32(out.Width), float32(out.Height)
	resolved := resolveSVGVars(out.Data)
	if w <= 0 || h <= 0 {
		w, h = svgSize(resolved)
	}
	if w <= 0 || h <= 0 {
		return Result{}, false
	}
	texts := svgTexts(resolved)
	labels := make([]Label, len(texts))
	for i, tx := range texts {
		labels[i] = Label{X: tx.x, Y: tx.y, Size: tx.size, Bold: tx.bold, Color: tx.color, Anchor: tx.anchor, Text: tx.text}
	}
	return Result{Width: w, Height: h, Shapes: svgShapes(resolved), Labels: labels}, true
}

func sketchTheme(t *Theme) *sketch.Theme {
	if t == nil {
		return nil
	}
	th := &sketch.Theme{
		Bg: t.Bg, Fg: t.Fg, Muted: t.Muted, Line: t.Line,
		Surface: t.Surface, SurfaceAlt: t.SurfaceAlt, Border: t.Border, Accent: t.Accent,
	}
	if t.Radius > 0 {
		th.Radius = t.Radius
	}
	return th
}

type measureFunc func(text string, size float64, bold bool) float64

func (f measureFunc) Width(text string, size float64, style int) float64 {
	return f(text, size, style == 1)
}

func sketchMeasurer(m func(text string, size float64, bold bool) float64) sketch.TextMeasurer {
	if m == nil {
		return nil
	}
	return measureFunc(m)
}
