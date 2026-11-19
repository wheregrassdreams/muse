package render

import "muse/nativeui/editor/navigation"

// MappingKind describes how one rendered piece relates to source positions.
type MappingKind uint8

const (
	MappingText MappingKind = iota
	MappingAtom
	MappingDecoration
)

// MappedPiece is a read-only value view of one display piece. Text is the
// displayed text for MappingText; other kinds map their range atomically.
type MappedPiece struct {
	Kind                     MappingKind
	SourceStart, SourceEnd   int
	DisplayStart, DisplayEnd int
	Text                     string
}

// PieceMapper exposes only position mapping metadata, not the editor's
// source model or mutable display builder.
type PieceMapper interface {
	MappedPieceCount() int
	MappedPieceAt(index int) MappedPiece
}

// displayProjection adapts the renderer's compact piece representation to the
// shared feature mapping policy without copying the piece list.
type displayProjection struct{ mapper PieceMapper }

func (p displayProjection) PieceCount() int {
	if p.mapper == nil {
		return 0
	}
	return p.mapper.MappedPieceCount()
}

func (p displayProjection) PieceAt(index int) navigation.Piece {
	piece := p.mapper.MappedPieceAt(index)
	kind := navigation.Text
	switch piece.Kind {
	case MappingAtom:
		kind = navigation.Atom
	case MappingDecoration:
		kind = navigation.Decoration
	}
	text := ""
	if piece.Kind == MappingText {
		text = piece.Text
	}
	return navigation.Piece{
		Kind:       kind,
		Source:     navigation.Offset{Start: piece.SourceStart, End: piece.SourceEnd},
		Visible:    navigation.Offset{Start: piece.DisplayStart, End: piece.DisplayEnd},
		SourceText: text, VisibleText: text,
	}
}

func projectionOf(mapper PieceMapper) navigation.Map {
	return navigation.Map{Provider: displayProjection{mapper: mapper}}
}

// SourceToDisplay maps a source byte offset to UTF-16 display units. Text
// pieces preserve character mapping; object atoms expose their two edges;
// decorations are intentionally not source caret stops.
func SourceToDisplay(mapper PieceMapper, source int) (int, bool) {
	if mapper == nil {
		return 0, false
	}
	if mapper.MappedPieceCount() == 0 && source == 0 {
		return 0, true
	}
	return projectionOf(mapper).VisibleAt(source, navigation.TowardStart)
}

// DisplayToSource maps UTF-16 display units to a source byte offset. When
// hidden syntax leaves multiple source candidates at one display edge, up
// chooses the smallest source offset and false chooses the largest. Offsets
// outside a non-empty display clamp to its nearest source edge.
func DisplayToSource(mapper PieceMapper, display int, up bool) int {
	if mapper == nil {
		return 0
	}
	count := mapper.MappedPieceCount()
	if source, ok := projectionOf(mapper).SourceAt(display, projectionBias(up)); ok {
		return source
	}
	if count > 0 {
		first, last := mapper.MappedPieceAt(0), mapper.MappedPieceAt(count-1)
		if display > last.DisplayEnd {
			return last.SourceEnd
		}
		return first.SourceStart
	}
	return 0
}

// CaretStops returns source offsets a caret may occupy, in source order.
func CaretStops(mapper PieceMapper) []int {
	return projectionOf(mapper).CaretStops()
}

func projectionBias(up bool) navigation.Bias {
	if up {
		return navigation.TowardStart
	}
	return navigation.TowardEnd
}
