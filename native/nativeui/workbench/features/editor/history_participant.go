package editor

import (
	tablemodel "github.com/wheregrassdreams/muse-editor/markdown/table"
	editor "github.com/wheregrassdreams/muse-editor/text/session"
	tableui "muse/nativeui/editor/blocks/table"
)

// productionHistorySnapshot contains only feature-owned state that must move
// with a Session-owned source transaction. It deliberately excludes source,
// text selection, revision, and undo stacks.
type productionHistorySnapshot struct {
	phantom *phantom
	breaks  []int
	block   *blockHistoryState
	table   tablemodel.Snapshot
}

func (s *productionHistorySnapshot) Clone() editor.HistorySnapshot {
	if s == nil {
		return nil
	}
	clone := *s
	clone.phantom = clonePhantom(s.phantom)
	clone.breaks = append([]int(nil), s.breaks...)
	clone.block = cloneBlockHistory(s.block)
	clone.table = cloneTableSnapshot(s.table)
	return &clone
}

func (e *Editor) CaptureHistory() editor.HistorySnapshot {
	// Session calls this while applying a transaction under its write lock.
	// Capture only editor-owned table navigation state here; reading Selection
	// or the Vim keymap would re-enter Session and deadlock.
	table := tablemodel.Snapshot{
		Selection:      tableSelectionSnapshot(e.tableSelection),
		DocumentRegion: tableSelectionSnapshot(e.tableDocumentRegion),
		Bridge:         tableBridgeSnapshot(e.tableBridge),
		VirtualEdit:    tableVirtualSnapshot(e.tableVirtualEdit),
	}
	return &productionHistorySnapshot{
		phantom: clonePhantom(e.phantom),
		breaks:  e.typedBreaks.Snapshot(),
		block:   cloneBlockHistory(e.blockHistoryState()),
		table:   table,
	}
}

func (e *Editor) RestoreHistory(snapshot editor.HistorySnapshot) {
	state, ok := snapshot.(*productionHistorySnapshot)
	if !ok || state == nil {
		return
	}
	e.phantom = clonePhantom(state.phantom)
	e.typedBreaks.Restore(state.breaks)
	_, _ = e.tableUI().Dispatch(tableui.Action{Kind: tableui.ResetNavigation})
	e.clearTableVirtualEdit()
	e.tableSelection, e.tableBridge, e.tableDocumentRegion = nil, nil, nil
	e.tableVirtualEdit = tableVirtualFromSnapshot(state.table.VirtualEdit)
	if state.table.Selection != nil {
		e.tableSelection = tableSelectionFromSnapshot(state.table.Selection)
	}
	if state.table.Bridge != nil {
		e.tableBridge = tableBridgeFromSnapshot(state.table.Bridge)
	}
	if state.table.DocumentRegion != nil {
		e.tableDocumentRegion = tableSelectionFromSnapshot(state.table.DocumentRegion)
	}
	e.restoreBlockHistory(cloneBlockHistory(state.block))
	e.parsed = invalidParsedRevision
	e.wantReveal = true
	e.hasGoal = false
	e.invalidate()
}

func clonePhantom(p *phantom) *phantom {
	if p == nil {
		return nil
	}
	clone := *p
	clone.conts = append([]cont(nil), p.conts...)
	return &clone
}

func cloneBlockHistory(s *blockHistoryState) *blockHistoryState {
	if s == nil || s.selection == nil {
		return nil
	}
	clone := *s
	block := *s.selection
	block.ranges = append([]Range(nil), s.selection.ranges...)
	clone.selection = &block
	return &clone
}

func cloneTableSnapshot(s tablemodel.Snapshot) tablemodel.Snapshot {
	if s.VirtualEdit != nil {
		virtual := *s.VirtualEdit
		s.VirtualEdit = &virtual
	}
	if s.Selection != nil {
		selection := *s.Selection
		s.Selection = &selection
	}
	if s.DocumentRegion != nil {
		selection := *s.DocumentRegion
		s.DocumentRegion = &selection
	}
	if s.Bridge != nil {
		bridge := *s.Bridge
		s.Bridge = &bridge
	}
	return s
}
