package paragraph

// HeadingShape describes the source form of the paragraph at the caret.
type HeadingShape uint8

const (
	ParagraphText HeadingShape = iota
	ATXHeading
	SetextHeading
)

// HeadingInput contains parser/layout facts needed to turn a paragraph into
// an ATX heading. Positions are UTF-8 byte offsets in Source.
type HeadingInput struct {
	Source     string
	Caret      int
	Level      int
	Shape      HeadingShape
	Pending    bool
	PrefixFrom int
	PrefixTo   int
	InsertAt   int
	SetextFrom int
	SetextTo   int
}

// HeadingPlan is one source transaction. Edits are ordered from right to
// left so every range addresses the same source snapshot.
type HeadingPlan struct {
	Edits []Edit
	Caret int
}

type Edit struct {
	Start, End int
	Text       string
}

// PlanHeading returns the source edits for changing a paragraph's heading
// level. The caller supplies parser-derived source positions; it owns applying
// the transaction and updating history.
func PlanHeading(in HeadingInput) (HeadingPlan, bool) {
	if in.Level < 0 || in.Level > 6 || in.Caret < 0 || in.Caret > len(in.Source) {
		return HeadingPlan{}, false
	}
	prefix := ""
	if in.Level > 0 {
		prefix = stringsOf('#', in.Level) + " "
	}
	plan := HeadingPlan{Caret: in.Caret}
	switch {
	case in.Shape == SetextHeading:
		if !validEditRange(in.Source, in.SetextFrom, in.SetextTo) || in.SetextTo <= in.SetextFrom || in.PrefixFrom < 0 || in.PrefixFrom > len(in.Source) {
			return HeadingPlan{}, false
		}
		plan.Edits = append(plan.Edits, Edit{Start: in.SetextFrom, End: in.SetextTo})
		if prefix != "" {
			plan.Edits = append(plan.Edits, Edit{Start: in.PrefixFrom, End: in.PrefixFrom, Text: prefix})
		}
		plan.Caret += len(prefix)
	case in.Shape == ATXHeading && !in.Pending:
		if !validEditRange(in.Source, in.PrefixFrom, in.PrefixTo) {
			return HeadingPlan{}, false
		}
		plan.Edits = append(plan.Edits, Edit{Start: in.PrefixFrom, End: in.PrefixTo, Text: prefix})
		plan.Caret += len(prefix) - (in.PrefixTo - in.PrefixFrom)
	default:
		if in.InsertAt < 0 || in.InsertAt > len(in.Source) {
			return HeadingPlan{}, false
		}
		if prefix != "" {
			plan.Edits = append(plan.Edits, Edit{Start: in.InsertAt, End: in.InsertAt, Text: prefix})
			plan.Caret += len(prefix)
		}
	}
	return plan, true
}

func validEditRange(source string, start, end int) bool {
	return start >= 0 && end >= start && end <= len(source)
}

func stringsOf(char byte, count int) string {
	if count <= 0 {
		return ""
	}
	buf := make([]byte, count)
	for i := range buf {
		buf[i] = char
	}
	return string(buf)
}
