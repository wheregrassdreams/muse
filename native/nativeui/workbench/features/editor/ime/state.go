package ime

// Range is a half-open range measured in UTF-16 code units, as required by
// native text-input APIs.
type Range struct {
	Start int
	End   int
}

// Composition is the text currently marked by an input method and the
// selection inside that marked text.
type Composition struct {
	Text      string
	Selection Range
}

type Selection struct {
	Range    Range
	Reversed bool
}

// Snapshot is the detached source state needed to translate platform text
// input coordinates. Caret and Selection are UTF-8 byte offsets in Source.
type Snapshot struct {
	Source    string
	Revision  uint64
	Caret     int
	Selection Selection
}

// CompositionHost is the narrow text/selection boundary for composition
// state. It receives immutable values and never exposes the editor widget or
// leaf model.
type CompositionHost interface {
	TextInputSnapshot() Snapshot
	SetIMEComposition(Composition)
}

// Controller coordinates text-input queries with a host and owns marked-text
// state. Hosts remain responsible for committing source edits and layout.
type Controller struct {
	host  CompositionHost
	state State
}

func NewController(host CompositionHost, index *Index, composition Composition) Controller {
	return Controller{host: host, state: NewState(index, composition)}
}

func (c Controller) State() State { return c.state }

func (c Controller) TextForRange(r Range) (string, Range) {
	if c.host == nil {
		return "", Range{}
	}
	return c.state.TextForRange(c.host.TextInputSnapshot(), r)
}

func (c Controller) Selection() (Selection, bool) {
	if c.host == nil {
		return Selection{}, false
	}
	return c.state.Selection(c.host.TextInputSnapshot())
}

func (c Controller) MarkedRange() (Range, bool) {
	if c.host == nil {
		return Range{}, false
	}
	return c.state.MarkedRange(c.host.TextInputSnapshot())
}

func (c Controller) Composing() bool { return c.state.Composing() }

func (c Controller) VirtualByte(units int) (sourceByte, compositionUnits int) {
	if c.host == nil {
		return 0, 0
	}
	return c.state.VirtualByte(c.host.TextInputSnapshot(), units)
}

func (c Controller) VirtualUnits(sourceByte int) int {
	if c.host == nil {
		return 0
	}
	return c.state.VirtualUnits(c.host.TextInputSnapshot(), sourceByte)
}

func (c Controller) SetMarkedText(text string, selection Range) {
	c.state.SetComposition(text, selection)
	if c.host != nil {
		c.host.SetIMEComposition(c.state.Composition())
	}
}

func (c Controller) ClearMarkedText() string {
	text := c.state.ClearComposition()
	if c.host != nil {
		c.host.SetIMEComposition(Composition{})
	}
	return text
}

// State owns the UTF-16 source index and transient composition model for one
// editor session. UI hosts apply the edits returned by these conversions.
type State struct {
	index       *Index
	composition Composition
}

func NewState(index *Index, composition Composition) State {
	state := State{index: index}
	state.SetComposition(composition.Text, composition.Selection)
	return state
}

func (s *State) Composition() Composition {
	if s == nil {
		return Composition{}
	}
	return s.composition
}

func (s *State) SetComposition(text string, selection Range) {
	if s == nil {
		return
	}
	selection = normalizeRange(selection)
	n := UTF16Len(text)
	selection.Start = min(selection.Start, n)
	selection.End = min(selection.End, n)
	s.composition = Composition{Text: text, Selection: selection}
}

// ClearComposition clears marked text and returns it for the host to commit.
func (s *State) ClearComposition() string {
	if s == nil {
		return ""
	}
	text := s.composition.Text
	s.composition = Composition{}
	return text
}

func (s *State) Composing() bool { return s != nil && s.composition.Text != "" }

func (s *State) Index() *Index {
	if s == nil {
		return nil
	}
	if s.index == nil {
		s.index = &Index{}
	}
	return s.index
}

func (s *State) TextForRange(snapshot Snapshot, r Range) (string, Range) {
	if s == nil {
		return "", Range{}
	}
	return s.Index().TextForRange(snapshot, s.composition, r)
}

func (s *State) Selection(snapshot Snapshot) (Selection, bool) {
	if s == nil {
		return Selection{}, false
	}
	if s.composition.Text != "" {
		caret := s.Index().UnitsOf(snapshot.Source, snapshot.Revision, snapshot.Caret)
		return Selection{Range: Range{
			Start: caret + s.composition.Selection.Start,
			End:   caret + s.composition.Selection.End,
		}}, true
	}
	selection := normalizeRange(snapshot.Selection.Range)
	return Selection{
		Range: Range{
			Start: s.Index().UnitsOf(snapshot.Source, snapshot.Revision, selection.Start),
			End:   s.Index().UnitsOf(snapshot.Source, snapshot.Revision, selection.End),
		},
		Reversed: snapshot.Selection.Reversed,
	}, true
}

func (s *State) MarkedRange(snapshot Snapshot) (Range, bool) {
	if s == nil || s.composition.Text == "" {
		return Range{}, false
	}
	start := s.Index().UnitsOf(snapshot.Source, snapshot.Revision, snapshot.Caret)
	return Range{Start: start, End: start + UTF16Len(s.composition.Text)}, true
}

// VirtualByte maps an offset in source-plus-composition text back to a source
// byte offset and an offset inside the composition, if the point is marked.
func (s *State) VirtualByte(snapshot Snapshot, units int) (sourceByte, compositionUnits int) {
	if s == nil {
		return 0, 0
	}
	caretUnits := s.Index().UnitsOf(snapshot.Source, snapshot.Revision, snapshot.Caret)
	compositionUnits = UTF16Len(s.composition.Text)
	full := s.Index().UnitsOf(snapshot.Source, snapshot.Revision, len(snapshot.Source)) + compositionUnits
	units = max(0, min(units, full))
	if s.composition.Text == "" || units <= caretUnits {
		return s.Index().ByteOf(snapshot.Source, snapshot.Revision, units), 0
	}
	if units <= caretUnits+compositionUnits {
		return snapshot.Caret, units - caretUnits
	}
	return s.Index().ByteOf(snapshot.Source, snapshot.Revision, units-compositionUnits), 0
}

// VirtualUnits maps a source byte offset into source-plus-composition UTF-16
// coordinates.
func (s *State) VirtualUnits(snapshot Snapshot, sourceByte int) int {
	if s == nil {
		return 0
	}
	units := s.Index().UnitsOf(snapshot.Source, snapshot.Revision, sourceByte)
	if s.composition.Text != "" && sourceByte > snapshot.Caret {
		units += UTF16Len(s.composition.Text)
	}
	return units
}

func normalizeRange(r Range) Range {
	if r.Start > r.End {
		r.Start, r.End = r.End, r.Start
	}
	return r
}
