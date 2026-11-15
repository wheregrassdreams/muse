package diagrams

import "strings"

// SequenceParticipant is one participant or actor in a sequence diagram.
type SequenceParticipant struct {
	ID, Label string
}

// SequenceEventKind identifies a message or note row.
type SequenceEventKind uint8

const (
	SequenceMessage SequenceEventKind = iota
	SequenceNote
)

// SequenceEvent is one ordered message or note.
type SequenceEvent struct {
	Kind     SequenceEventKind
	From, To string
	Text     string
	Dashed   bool
}

// SequenceDiagram is the parsed, backend-neutral sequence model.
type SequenceDiagram struct {
	Participants []SequenceParticipant
	Events       []SequenceEvent
}

// SequencePosition is the laid out center and width for a participant.
type SequencePosition struct{ X, Width float32 }

// ParseSequence parses participants, messages, dashed replies and notes. It
// skips control-frame syntax while preserving the messages inside it.
func ParseSequence(source string) (*SequenceDiagram, bool) {
	lines := strings.Split(source, "\n")
	if len(lines) == 0 || !strings.EqualFold(strings.TrimSpace(lines[0]), "sequencediagram") {
		return nil, false
	}
	diagram := &SequenceDiagram{}
	byID := make(map[string]int)
	participant := func(id, label string) {
		id = strings.TrimSpace(id)
		if id == "" {
			return
		}
		if i, ok := byID[id]; ok {
			if label != "" && label != id {
				diagram.Participants[i].Label = label
			}
			return
		}
		byID[id] = len(diagram.Participants)
		diagram.Participants = append(diagram.Participants, SequenceParticipant{ID: id, Label: label})
	}
	for _, raw := range lines[1:] {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "%%") {
			continue
		}
		low := strings.ToLower(line)
		switch {
		case strings.HasPrefix(low, "participant "), strings.HasPrefix(low, "actor "):
			prefix := "participant "
			if strings.HasPrefix(low, "actor ") {
				prefix = "actor "
			}
			rest := strings.TrimSpace(line[len(prefix):])
			id, label := rest, rest
			if at := strings.Index(low, " as "); at >= 0 {
				id = strings.TrimSpace(line[len(prefix):at])
				label = strings.TrimSpace(line[at+4:])
			}
			participant(id, label)
			continue
		case strings.HasPrefix(low, "note "):
			rest := line[len("note "):]
			colon := strings.Index(rest, ":")
			if colon < 0 {
				continue
			}
			whereRaw := strings.TrimSpace(rest[:colon])
			where := strings.ToLower(whereRaw)
			text := strings.TrimSpace(rest[colon+1:])
			var ids []string
			if strings.HasPrefix(where, "over ") {
				for _, id := range strings.Split(whereRaw[len("over "):], ",") {
					ids = append(ids, strings.TrimSpace(id))
				}
			} else if strings.HasPrefix(where, "left of ") {
				ids = append(ids, strings.TrimSpace(whereRaw[len("left of "):]))
			} else if strings.HasPrefix(where, "right of ") {
				ids = append(ids, strings.TrimSpace(whereRaw[len("right of "):]))
			}
			if len(ids) == 0 {
				continue
			}
			for _, id := range ids {
				participant(id, id)
			}
			diagram.Events = append(diagram.Events, SequenceEvent{Kind: SequenceNote, From: ids[0], To: ids[len(ids)-1], Text: text})
			continue
		case strings.HasPrefix(low, "loop "), strings.HasPrefix(low, "alt "),
			strings.HasPrefix(low, "else"), strings.HasPrefix(low, "end"),
			strings.HasPrefix(low, "opt "), strings.HasPrefix(low, "par "),
			strings.HasPrefix(low, "activate "), strings.HasPrefix(low, "deactivate "),
			strings.HasPrefix(low, "autonumber"), strings.HasPrefix(low, "title "),
			strings.HasPrefix(low, "rect "), strings.HasPrefix(low, "critical "),
			strings.HasPrefix(low, "break "):
			continue
		}
		colon := strings.Index(line, ":")
		if colon < 0 {
			continue
		}
		left := strings.TrimSpace(line[:colon])
		text := strings.TrimSpace(line[colon+1:])
		from, to, dashed, ok := splitSequenceArrow(left)
		if !ok {
			continue
		}
		participant(from, from)
		participant(to, to)
		diagram.Events = append(diagram.Events, SequenceEvent{Kind: SequenceMessage, From: from, To: to, Text: text, Dashed: dashed})
	}
	if len(diagram.Events) == 0 {
		return nil, false
	}
	return diagram, true
}

// Height returns the fixed row height for the current event list.
func (g *SequenceDiagram) Height() float32 { return 108 + float32(len(g.Events))*42 }

// Width returns the minimum width for participant columns and note margins.
func (g *SequenceDiagram) Width() float32 { return float32(max(len(g.Participants), 1))*70 + 80 }

// Layout returns participant centers and box widths for the requested view width.
func (g *SequenceDiagram) Layout(width float32) []SequencePosition {
	if len(g.Participants) == 0 {
		return nil
	}
	column := width / float32(len(g.Participants))
	if column > 180 {
		column = 180
	}
	if column < 70 {
		column = 70
	}
	total := column * float32(len(g.Participants))
	x := (width - total) / 2
	positions := make([]SequencePosition, len(g.Participants))
	for i := range positions {
		positions[i] = SequencePosition{X: x + column/2, Width: column - 10}
		x += column
	}
	return positions
}

func splitSequenceArrow(left string) (from, to string, dashed, ok bool) {
	for _, arrow := range []string{"-->>", "->>", "-->", "->", "--x", "-x", "--)"} {
		if i := strings.Index(left, arrow); i >= 0 {
			from = strings.TrimRight(strings.TrimSpace(left[:i]), "-+")
			to = strings.TrimLeft(strings.TrimSpace(left[i+len(arrow):]), "+-")
			to = strings.TrimRight(to, "-+")
			if from == "" || to == "" {
				return "", "", false, false
			}
			return from, to, strings.HasPrefix(arrow, "--"), true
		}
	}
	return "", "", false, false
}
