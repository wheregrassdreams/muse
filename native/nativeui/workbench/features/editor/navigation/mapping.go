// Package navigation adapts visible editor text positions to Markdown source
// positions. It contains no editor, layout, or toolkit state.
package navigation

import "unicode/utf8"

// Offset is a half-open UTF-8 byte range in source or UTF-16 code units in a
// visible-text projection, depending on the field that contains it.
type Offset struct {
	Start int
	End   int
}

// Kind describes how one visible run relates to its source range.
type Kind uint8

const (
	// Text maps rune boundaries one-to-one while allowing UTF-8 source bytes
	// and UTF-16 visible units to differ.
	Text Kind = iota
	// Atom maps only its visible endpoints to source endpoints. Interior points
	// choose the nearest endpoint.
	Atom
	// Decoration has visible text but no source extent; both edges map to the
	// source anchor.
	Decoration
)

// Piece describes one contiguous portion of a visible projection.
// SourceText and VisibleText are required for Text pieces and must contain
// the same rune sequence. Offsets are relative to the projected block.
type Piece struct {
	Kind        Kind
	Source      Offset
	Visible     Offset
	SourceText  string
	VisibleText string
}

// Bias resolves positions where multiple pieces meet or syntax is hidden
// between adjacent visible runs.
type Bias uint8

const (
	// TowardStart selects the earliest source candidate.
	TowardStart Bias = iota
	// TowardEnd selects the latest source candidate.
	TowardEnd
)

// PieceProvider exposes a display mapping without requiring a second copy of
// its pieces. Pieces are ordered by visible start and source start.
type PieceProvider interface {
	PieceCount() int
	PieceAt(index int) Piece
}

// Map is an immutable projection of one visible block onto its source.
// Callers may provide a slice or a provider backed by an existing display.
type Map struct {
	Pieces   []Piece
	Provider PieceProvider
}

func (m Map) pieceCount() int {
	if m.Provider != nil {
		return m.Provider.PieceCount()
	}
	return len(m.Pieces)
}

func (m Map) pieceAt(index int) Piece {
	if m.Provider != nil {
		return m.Provider.PieceAt(index)
	}
	return m.Pieces[index]
}

// SourceAt maps a visible UTF-16 position to a source UTF-8 byte position.
// The boolean is false when the position is outside the projection. Positions
// inside a UTF-16 surrogate pair snap to the following source rune boundary.
func (m Map) SourceAt(visible int, bias Bias) (int, bool) {
	var best int
	found := false
	take := func(source int) {
		if !found || bias == TowardStart && source < best || bias == TowardEnd && source > best {
			best, found = source, true
		}
	}
	for i := 0; i < m.pieceCount(); i++ {
		p := m.pieceAt(i)
		if visible < p.Visible.Start || visible > p.Visible.End {
			continue
		}
		switch p.Kind {
		case Text:
			if source, ok := textSourceAt(p, visible); ok {
				take(source)
			}
		case Atom:
			switch {
			case visible == p.Visible.Start:
				take(p.Source.Start)
			case visible == p.Visible.End:
				take(p.Source.End)
			case visible-p.Visible.Start <= p.Visible.End-visible:
				take(p.Source.Start)
			default:
				take(p.Source.End)
			}
		case Decoration:
			// Display decorations, including visible icons, have no source
			// extent and map every visible position back to their source anchor.
			take(p.Source.Start)
		}
	}
	return best, found
}

// VisibleAt maps a source byte position to a visible UTF-16 position. Text
// and atom endpoints are caret stops; decorations never expose source stops.
func (m Map) VisibleAt(source int, bias Bias) (int, bool) {
	var best int
	found := false
	take := func(visible int) {
		if !found || bias == TowardStart && visible < best || bias == TowardEnd && visible > best {
			best, found = visible, true
		}
	}
	for i := 0; i < m.pieceCount(); i++ {
		p := m.pieceAt(i)
		if source < p.Source.Start || source > p.Source.End {
			continue
		}
		switch p.Kind {
		case Text:
			if visible, ok := textVisibleAt(p, source); ok {
				take(visible)
			}
		case Atom:
			if source == p.Source.Start {
				take(p.Visible.Start)
			} else if source == p.Source.End {
				take(p.Visible.End)
			}
		}
	}
	return best, found
}

// CaretStops returns the source offsets where the display permits a caret.
// Text pieces expose rune boundaries, atoms expose their endpoints, and
// decorations expose their source anchor.
func (m Map) CaretStops() []int {
	var stops []int
	add := func(source int) {
		if len(stops) == 0 || stops[len(stops)-1] != source {
			stops = append(stops, source)
		}
	}
	for i := 0; i < m.pieceCount(); i++ {
		p := m.pieceAt(i)
		switch p.Kind {
		case Text:
			if !validTextPiece(p) {
				continue
			}
			for offset := 0; offset < len(p.SourceText); {
				add(p.Source.Start + offset)
				_, width := utf8.DecodeRuneInString(p.SourceText[offset:])
				offset += width
			}
			add(p.Source.End)
		case Atom:
			add(p.Source.Start)
			add(p.Source.End)
		case Decoration:
			add(p.Source.Start)
		}
	}
	if len(stops) == 0 {
		return []int{0}
	}
	return stops
}

func textSourceAt(p Piece, visible int) (int, bool) {
	if !validTextPiece(p) {
		return 0, false
	}
	units := 0
	for sourceByte, r := range p.VisibleText {
		if p.Visible.Start+units == visible {
			return p.Source.Start + sourceByte, true
		}
		if visible < p.Visible.Start+units+runeUTF16(r) {
			// The platform display API accepts a position inside a UTF-16
			// surrogate pair by resolving it to the following rune boundary.
			return p.Source.Start + sourceByte + utf8.RuneLen(r), true
		}
		units += runeUTF16(r)
	}
	if p.Visible.Start+units == visible {
		return p.Source.End, true
	}
	return 0, false
}

func textVisibleAt(p Piece, source int) (int, bool) {
	if !validTextPiece(p) {
		return 0, false
	}
	units := 0
	for sourceByte, r := range p.SourceText {
		if p.Source.Start+sourceByte == source {
			return p.Visible.Start + units, true
		}
		units += runeUTF16(r)
	}
	if source == p.Source.End {
		return p.Visible.Start + units, true
	}
	return 0, false
}

func validTextPiece(p Piece) bool {
	if p.Source.Start < 0 || p.Source.End < p.Source.Start ||
		p.Visible.Start < 0 || p.Visible.End < p.Visible.Start {
		return false
	}
	units := 0
	for _, r := range p.VisibleText {
		units += runeUTF16(r)
	}
	return p.Source.End-p.Source.Start == len(p.SourceText) &&
		p.Visible.End-p.Visible.Start == units && sameRunes(p.SourceText, p.VisibleText)
}

func sameRunes(a, b string) bool {
	if len(a) == len(b) {
		return a == b
	}
	for len(a) > 0 && len(b) > 0 {
		ra, sa := utf8.DecodeRuneInString(a)
		rb, sb := utf8.DecodeRuneInString(b)
		if ra != rb {
			return false
		}
		a, b = a[sa:], b[sb:]
	}
	return len(a) == 0 && len(b) == 0
}

func runeUTF16(r rune) int {
	if r > 0xffff {
		return 2
	}
	return 1
}
