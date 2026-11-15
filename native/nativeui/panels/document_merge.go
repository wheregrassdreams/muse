package nativeui

import (
	"sort"
	"strings"
)

type lineHunk struct {
	start, end int
	text       string
}

// mergeDisjointDocumentEdits merges non-overlapping line edits from a shared
// base. The bounded Myers diff keeps the common small-edit case linear in the
// note size; unusually large diffs are left for explicit conflict resolution.
func mergeDisjointDocumentEdits(base, local, external string) (string, bool) {
	left, ok := lineDiffHunks(base, local)
	if !ok {
		return "", false
	}
	right, ok := lineDiffHunks(base, external)
	if !ok {
		return "", false
	}
	if len(left) == 0 {
		return external, true
	}
	if len(right) == 0 {
		return local, true
	}
	baseLines := splitLines(base)
	hunks := append(append([]lineHunk(nil), left...), right...)
	sort.SliceStable(hunks, func(i, j int) bool {
		if hunks[i].start != hunks[j].start {
			return hunks[i].start < hunks[j].start
		}
		return hunks[i].end < hunks[j].end
	})
	unique := hunks[:0]
	for _, h := range hunks {
		if len(unique) > 0 {
			prev := unique[len(unique)-1]
			if h.start == prev.start && h.end == prev.end && h.text == prev.text {
				continue
			}
			if h.start == prev.start || h.start < prev.end {
				return "", false
			}
		}
		unique = append(unique, h)
	}
	var out strings.Builder
	cursor := 0
	for _, h := range unique {
		if h.start < cursor || h.end > len(baseLines) {
			return "", false
		}
		out.WriteString(strings.Join(baseLines[cursor:h.start], ""))
		out.WriteString(h.text)
		cursor = h.end
	}
	out.WriteString(strings.Join(baseLines[cursor:], ""))
	return out.String(), true
}

// lineDiffHunks returns the edits that turn base into changed. It declines
// pathological cases to keep watcher-triggered merges from monopolizing UI
// resources.
func lineDiffHunks(base, changed string) ([]lineHunk, bool) {
	a, b := splitLines(base), splitLines(changed)
	if len(a)+len(b) > 12000 {
		return nil, false
	}
	ops, ok := myersLineOps(a, b, 512)
	if !ok {
		return nil, false
	}
	var hunks []lineHunk
	i, j := 0, 0
	for at := 0; at < len(ops); {
		if ops[at] == '=' {
			i++
			j++
			at++
			continue
		}
		start := i
		var replacement strings.Builder
		for at < len(ops) && ops[at] != '=' {
			switch ops[at] {
			case '-':
				i++
			case '+':
				replacement.WriteString(b[j])
				j++
			}
			at++
		}
		hunks = append(hunks, lineHunk{start: start, end: i, text: replacement.String()})
	}
	return hunks, true
}

func splitLines(s string) []string { return strings.SplitAfter(s, "\n") }

// myersLineOps returns a shortest edit script, bounded by maxDistance.
func myersLineOps(a, b []string, maxDistance int) ([]byte, bool) {
	n, m := len(a), len(b)
	maxD := min(n+m, maxDistance)
	width := 2*(n+m) + 3
	offset := n + m + 1
	v := make([]int, width)
	for i := range v {
		v[i] = -1
	}
	v[offset+1] = 0
	trace := make([][]int, 0, maxD+1)
	found := -1
	for d := 0; d <= maxD; d++ {
		for k := -d; k <= d; k += 2 {
			idx := offset + k
			var x int
			if k == -d || k != d && v[idx-1] < v[idx+1] {
				x = v[idx+1]
			} else {
				x = v[idx-1] + 1
			}
			y := x - k
			for x < n && y < m && a[x] == b[y] {
				x++
				y++
			}
			v[idx] = x
			if x >= n && y >= m {
				found = d
				break
			}
		}
		trace = append(trace, append([]int(nil), v[offset-d:offset+d+1]...))
		if found >= 0 {
			break
		}
	}
	if found < 0 {
		return nil, false
	}
	x, y := n, m
	reversed := make([]byte, 0, n+m)
	for d := found; d > 0; d-- {
		prev := trace[d-1]
		prevOffset := d - 1
		k := x - y
		var prevK int
		left, right := -1, -1
		if k-1 >= -prevOffset && k-1 <= prevOffset {
			left = prev[k-1+prevOffset]
		}
		if k+1 >= -prevOffset && k+1 <= prevOffset {
			right = prev[k+1+prevOffset]
		}
		if k == -d || k != d && left < right {
			prevK = k + 1
		} else {
			prevK = k - 1
		}
		prevX := prev[prevK+prevOffset]
		prevY := prevX - prevK
		for x > prevX && y > prevY {
			reversed = append(reversed, '=')
			x--
			y--
		}
		if x == prevX {
			reversed = append(reversed, '+')
			y--
		} else {
			reversed = append(reversed, '-')
			x--
		}
	}
	for x > 0 && y > 0 {
		reversed = append(reversed, '=')
		x--
		y--
	}
	for x > 0 {
		reversed = append(reversed, '-')
		x--
	}
	for y > 0 {
		reversed = append(reversed, '+')
		y--
	}
	for l, r := 0, len(reversed)-1; l < r; l, r = l+1, r-1 {
		reversed[l], reversed[r] = reversed[r], reversed[l]
	}
	return reversed, true
}
