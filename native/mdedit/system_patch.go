package mdedit

// ReplaceSystemAll updates source managed by the application without adding a
// separate Undo item. When possible, the patch is folded into the preceding
// user edit so Undo still treats the metadata change as part of that edit.
func (e *Editor) ReplaceSystemAll(src string) {
	e.paintMu.Lock()
	defer e.paintMu.Unlock()
	if src == e.src {
		return
	}
	from := 0
	for from < len(e.src) && from < len(src) && e.src[from] == src[from] {
		from++
	}
	toOld, toNew := len(e.src), len(src)
	for toOld > from && toNew > from && e.src[toOld-1] == src[toNew-1] {
		toOld--
		toNew--
	}
	for from > 0 && from < len(e.src) && !runeStart(e.src[from]) {
		from--
	}
	for toOld < len(e.src) && !runeStart(e.src[toOld]) {
		toOld++
		toNew++
	}
	e.replaceRange(from, toOld, src[from:toNew])

	// replaceRange records a normal atomic edit and updates parser/caret state.
	// Fold that edit into the immediately preceding history step; when no such
	// step exists, keep the source update out of user-visible history.
	n := len(e.hist.undo)
	if n == 0 {
		return
	}
	last := e.hist.undo[n-1]
	e.hist.undo = e.hist.undo[:n-1]
	if n == 1 {
		return
	}
	prev := &e.hist.undo[n-2]
	prev.changes = append(prev.changes, last.changes...)
	prev.after, prev.phAfter, prev.at = last.after, last.phAfter, last.at
	prev.blockAfter = last.blockAfter
	prev.breaksAfter = last.breaksAfter
	prev.closedForCoalescing = true
}
