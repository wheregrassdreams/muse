package mdedit

import (
	"sort"
	"strings"

	"github.com/egoist/mygo/ui"
)

// The editor is the client of the system's input methods. They read the
// source around the caret and compose there, in UTF-16 units of the source
// with the composition in it: a virtual insertion at the caret, shown
// underlined, which goes into the source only once committed, as one step
// of undo with what it replaced.

var _ ui.TextInputClient = (*Editor)(nil)

// u16 indexes the source by UTF-16 units every so many bytes, for the
// conversions input methods ask for.
//
// An edit moves the index rather than building it again: everything before
// the edit keeps its offsets, everything after it moves by the bytes and
// the units of the text. Reading the whole source per keystroke was the one
// whole-document operation left in the typing path.
type u16Index struct {
	rev   uint64
	bytes []int
	units []int
	// The entries of an edit, and the index being built, kept between
	// edits: none of them may be the array the index itself reads.
	midB, midU []int
	nb, nu     []int
}

const u16Every = 2048

func (e *Editor) u16() *u16Index {
	ix := &e.u16ix
	if ix.rev == e.rev && ix.bytes != nil {
		return ix
	}
	e.rebuildU16()
	return ix
}

// rebuildU16 reads the whole source into the index.
func (e *Editor) rebuildU16() {
	ix := &e.u16ix
	ix.rev = e.rev
	ix.bytes = ix.bytes[:0]
	ix.units = ix.units[:0]
	u := 0
	next := 0
	for i, r := range e.src {
		if i >= next {
			ix.bytes = append(ix.bytes, i)
			ix.units = append(ix.units, u)
			next = i + u16Every
		}
		if r >= 0x10000 {
			u += 2
		} else {
			u++
		}
	}
	ix.bytes = append(ix.bytes, len(e.src))
	ix.units = append(ix.units, u)
}

// noteEdit moves the index with an edit of the source: from and to bound
// what it replaced, old and new the text of it. src is the source as it is
// after the edit.
func (e *Editor) noteEdit(from, to int, old, new, src string) {
	ix := &e.u16ix
	if ix.bytes == nil || ix.rev+1 != e.rev || from < 0 || to < from || to > len(src) {
		// No index, or one built for another source: the next reader reads
		// the source again.
		ix.bytes = nil
		return
	}
	ix.rev = e.rev
	deltaB, deltaU := len(new)-(to-from), ui.UTF16Len(new)-ui.UTF16Len(old)
	lo := searchBytes(ix.bytes, from)
	hi := searchBytes(ix.bytes, to)
	// The entries of the replaced text, from the one before the edit, so
	// that reading a unit stays a walk over a few bytes: the tail moved,
	// and the gap the edit left may be wider than one entry.
	end := to + deltaB
	last, u := 0, 0
	if lo > 0 {
		last = ix.bytes[lo-1]
		u = ix.units[lo-1] + ui.UTF16Len(src[ix.bytes[lo-1]:from])
	}
	ix.midB, ix.midU = ix.midB[:0], ix.midU[:0]
	for at := from; at < end; {
		if last < 0 || at-last >= u16Every {
			ix.midB = append(ix.midB, at)
			ix.midU = append(ix.midU, u)
			last = at
		}
		n := min(at+u16Every, end)
		u += ui.UTF16Len(src[at:n])
		at = n
	}
	// The tail keeps its spacing, moved by the edit.
	for k := hi; k < len(ix.bytes); k++ {
		ix.bytes[k] += deltaB
		ix.units[k] += deltaU
	}
	if lo == 0 {
		// The edit took the first entry: the index starts at the start of
		// the source, whatever the edit did there.
		ix.nb = append(ix.nb[:0], 0)
		ix.nu = append(ix.nu[:0], 0)
	} else {
		ix.nb = append(ix.nb[:0], ix.bytes[:lo]...)
		ix.nu = append(ix.nu[:0], ix.units[:lo]...)
	}
	ix.nb = append(ix.nb, ix.midB...)
	ix.nb = append(ix.nb, ix.bytes[hi:]...)
	ix.nu = append(ix.nu, ix.midU...)
	ix.nu = append(ix.nu, ix.units[hi:]...)
	ix.bytes, ix.nb = ix.nb, ix.bytes[:0]
	ix.units, ix.nu = ix.nu, ix.units[:0]
}

// searchBytes returns the index of the first entry at or after at.
func searchBytes(b []int, at int) int { return sort.SearchInts(b, at) }

// unitsOf returns the UTF-16 offset of byte b of the source.
func (e *Editor) unitsOf(b int) int {
	b = max(0, min(b, len(e.src)))
	ix := e.u16()
	k := 0
	for k+1 < len(ix.bytes) && ix.bytes[k+1] <= b {
		k++
	}
	return ix.units[k] + ui.UTF16Len(e.src[ix.bytes[k]:b])
}

// byteOf returns the byte of the source at UTF-16 offset u, keeping
// surrogate pairs and runes whole.
func (e *Editor) byteOf(u int) int {
	if u <= 0 {
		return 0
	}
	ix := e.u16()
	k := 0
	for k+1 < len(ix.units) && ix.units[k+1] <= u {
		k++
	}
	return ix.bytes[k] + utf16ByteOffset(e.src[ix.bytes[k]:], u-ix.units[k])
}

// composeUnits returns the composition's length in UTF-16 units.
func (e *Editor) composeUnits() int { return ui.UTF16Len(e.compose) }

// virtualByte returns the source byte at UTF-16 offset u of the source
// with the composition, and how far into the composition it is.
func (e *Editor) virtualByte(u int) (int, int) {
	cu := e.unitsOf(e.caret)
	n := e.composeUnits()
	if e.compose == "" || u <= cu {
		return e.byteOf(u), 0
	}
	if u <= cu+n {
		return e.caret, u - cu
	}
	return e.byteOf(u - n), 0
}

// virtualUnits returns the UTF-16 offset in the source with the
// composition of byte b of the source.
func (e *Editor) virtualUnits(b int) int {
	u := e.unitsOf(b)
	if e.compose != "" && b > e.caret {
		u += e.composeUnits()
	}
	return u
}

func (e *Editor) TextForRange(r ui.TextInputRange) (string, ui.TextInputRange) {
	if r.Start > r.End {
		r.Start, r.End = r.End, r.Start
	}
	cu := e.unitsOf(e.caret)
	n := e.composeUnits()
	full := e.unitsOf(len(e.src)) + n
	r.Start, r.End = max(0, min(r.Start, full)), max(0, min(r.End, full))
	var out strings.Builder
	// The part before the composition, the composition, the part after.
	if r.Start < cu {
		a, z := e.byteOf(r.Start), e.byteOf(min(r.End, cu))
		out.WriteString(e.src[a:z])
	}
	if n > 0 && r.Start < cu+n && r.End > cu {
		a := utf16ByteOffset(e.compose, max(r.Start-cu, 0))
		z := utf16ByteOffset(e.compose, min(r.End-cu, n))
		out.WriteString(e.compose[a:z])
	}
	if r.End > cu+n {
		a, z := e.byteOf(max(r.Start, cu+n)-n), e.byteOf(r.End-n)
		out.WriteString(e.src[a:z])
	}
	s := out.String()
	return s, ui.TextInputRange{Start: r.Start, End: r.Start + ui.UTF16Len(s)}
}

func (e *Editor) Selection() ui.TextInputSelection {
	if e.compose != "" {
		cu := e.unitsOf(e.caret)
		return ui.TextInputSelection{Range: ui.TextInputRange{Start: cu + e.composeSel.Start, End: cu + e.composeSel.End}}
	}
	a, z := e.selection()
	return ui.TextInputSelection{
		Range:    ui.TextInputRange{Start: e.unitsOf(a), End: e.unitsOf(z)},
		Reversed: e.caret == a && a != z,
	}
}

func (e *Editor) MarkedRange() (ui.TextInputRange, bool) {
	if e.compose == "" {
		return ui.TextInputRange{}, false
	}
	cu := e.unitsOf(e.caret)
	return ui.TextInputRange{Start: cu, End: cu + e.composeUnits()}, true
}

func (e *Editor) ReplaceText(r *ui.TextInputRange, s string) {
	if !e.ModalAcceptsText() {
		e.modalText(s)
		return
	}
	defer e.invalidate()
	switch {
	case r == nil && e.compose != "":
		e.compose, e.composing = "", false
		e.typeText(s)
	case r == nil:
		e.typeText(s)
	default:
		a, _ := e.virtualByte(min(r.Start, r.End))
		z, _ := e.virtualByte(max(r.Start, r.End))
		e.compose, e.composing = "", false
		e.dropPhantomIfAway(a)
		e.begin(stepEdit)
		e.replace(a, z, s)
		e.commit(a+len(s), a+len(s))
	}
}

// dropPhantomIfAway takes the phantom away when an edit is not in it.
func (e *Editor) dropPhantomIfAway(at int) {
	if e.phantom != nil && at != e.phantom.at {
		e.dropPhantom()
	}
}

func (e *Editor) SetMarkedText(r *ui.TextInputRange, s string, selected ui.TextInputRange) {
	if !e.ModalAcceptsText() {
		return
	}
	defer e.invalidate()
	if r != nil && e.compose == "" {
		// Composing over a range of the source, as reconversion does: it
		// leaves the source, and comes back with the commit.
		a, _ := e.virtualByte(min(r.Start, r.End))
		z, _ := e.virtualByte(max(r.Start, r.End))
		if a != z {
			e.begin(stepEdit)
			e.replace(a, z, "")
			e.commit(a, a)
		}
	} else if e.compose == "" && e.anchor != e.caret {
		// Composing over the selection: it goes.
		e.deleteSelection()
	}
	if selected.Start > selected.End {
		selected.Start, selected.End = selected.End, selected.Start
	}
	n := ui.UTF16Len(s)
	selected.Start, selected.End = min(selected.Start, n), min(selected.End, n)
	e.compose, e.composeSel, e.composing = s, selected, s != ""
	e.blinkStart = e.blinkStart.Add(0)
	e.wantReveal = true
}

func (e *Editor) UnmarkText() {
	if !e.ModalAcceptsText() {
		e.compose = ""
		e.composing = false
		return
	}
	if e.compose == "" {
		e.composing = false
		return
	}
	s := e.compose
	e.compose, e.composing = "", false
	e.typeText(s)
	e.invalidate()
}

func (e *Editor) BoundsForRange(r ui.TextInputRange) (ui.Rect, ui.TextInputRange, bool) {
	b, extra := e.virtualByte(min(r.Start, r.End))
	var rect ui.Rect
	ok := false
	if extra > 0 || (e.compose != "" && b == e.caret) {
		if cr, cok := e.composeCaretRect(); cok {
			rect, ok = cr, true
		}
	}
	if !ok {
		rect, ok = e.caretRect(b)
	}
	if !ok {
		return ui.Rect{}, ui.TextInputRange{}, false
	}
	actual := ui.TextInputRange{Start: r.Start, End: r.Start}
	if r.End > r.Start {
		rect.W = max(rect.W, e.lk.size*0.6)
		actual.End = r.Start + 1
	}
	return rect, actual, true
}

func (e *Editor) IndexForPoint(p ui.Point) (int, bool) {
	off := e.hitTest(p.X, p.Y)
	return e.virtualUnits(off), true
}
