package editor

import (
	"fmt"

	editormodel "muse/editor"
)

// ApplyTransaction applies source edits expressed against one source snapshot
// as one Session-owned history transaction. Edits are replayed from right to
// left so their coordinates remain valid against each intermediate source.
func (e *Editor) ApplyTransaction(tx editormodel.Transaction) error {
	if tx.Before != e.Session.Selection() {
		return fmt.Errorf("transaction selection is stale")
	}
	batch, err := e.Session.BeginTransaction(editormodel.TransactionOptions{
		Group:          editormodel.HistoryGroupExplicit,
		BeforeMetadata: e.Session.CaptureHistory(),
	})
	if err != nil {
		return err
	}
	for i := len(tx.Edits) - 1; i >= 0; i-- {
		if _, err := e.Session.ApplyEdit(batch, tx.Edits[i]); err != nil {
			_ = e.Session.AbortTransaction(batch)
			return err
		}
	}
	if _, err := e.Session.CommitTransaction(batch, tx.After); err != nil {
		_ = e.Session.AbortTransaction(batch)
		return err
	}
	return nil
}

// TransactionSelection returns the Session-owned text selection. Block
// structure remains owned by the component's typed subsystem snapshots.
func (e *Editor) TransactionSelection() editormodel.Selection {
	return e.Session.Selection()
}
