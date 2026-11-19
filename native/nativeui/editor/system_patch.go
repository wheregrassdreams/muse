package editor

// ReplaceSystemAll applies application-managed source through the Session's
// system-patch primitive. The Session owns source offsets, selection mapping,
// and history folding; this adapter refreshes the derived editor projection.
func (e *Editor) ReplaceSystemAll(source string) {
	e.paintMu.Lock()
	defer e.paintMu.Unlock()
	if e.editBatch != nil {
		e.commit(e.Caret(), e.Caret())
	}
	patch, ok := e.Session.PlanSystemPatch(source)
	if !ok {
		return
	}
	metadata := e.Session.CaptureHistory()
	if _, err := e.Session.ApplySystemPatch(patch, metadata, metadata); err != nil {
		panic(err)
	}
	e.pending, e.pendingMulti = nil, false
	e.parsed = invalidParsedRevision
	e.sync()
	e.changed()
	e.invalidate()
}
