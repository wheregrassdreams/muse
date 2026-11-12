package mdedit

import "sort"

// SelectionRanges returns the active selection as ordered, half-open UTF-8
// source ranges. A visual block is represented by one range per display row.
func (e *Editor) SelectionRanges() []Range {
	if e.tableSelection != nil {
		a, z := e.selection()
		if a < z {
			return []Range{{Start: a, End: z}}
		}
		return nil
	}
	if e.block != nil {
		return e.blockRanges()
	}
	a, z := e.selection()
	if a == z {
		return nil
	}
	return []Range{{Start: a, End: z}}
}

func normalizeRanges(in []Range) []Range {
	r := append([]Range(nil), in...)
	sort.Slice(r, func(i, j int) bool {
		if r[i].Start == r[j].Start {
			return r[i].End < r[j].End
		}
		return r[i].Start < r[j].Start
	})
	out := r[:0]
	for _, v := range r {
		if v.End < v.Start {
			v.Start, v.End = v.End, v.Start
		}
		if v.Start == v.End {
			continue
		}
		if len(out) > 0 && v.Start < out[len(out)-1].End {
			if v.End > out[len(out)-1].End {
				out[len(out)-1].End = v.End
			}
			continue
		}
		out = append(out, v)
	}
	return out
}
