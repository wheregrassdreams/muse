// Package session adapts value transactions to the editor's authoritative
// source and history session without depending on a UI component.
package session

import (
	"fmt"

	editormodel "muse/editor"
)

// ApplyTransaction applies edits expressed against one source snapshot as one
// Session-owned history transaction. Edits are applied right-to-left so their
// coordinates remain valid against each intermediate source.
func ApplyTransaction(target *editormodel.Session, tx editormodel.Transaction) error {
	if target == nil {
		return fmt.Errorf("transaction session is nil")
	}
	if tx.Before != target.Selection() {
		return fmt.Errorf("transaction selection is stale")
	}
	batch, err := target.BeginTransaction(editormodel.TransactionOptions{
		Group:          editormodel.HistoryGroupExplicit,
		BeforeMetadata: target.CaptureHistory(),
	})
	if err != nil {
		return err
	}
	for i := len(tx.Edits) - 1; i >= 0; i-- {
		if _, err := target.ApplyEdit(batch, tx.Edits[i]); err != nil {
			_ = target.AbortTransaction(batch)
			return err
		}
	}
	if _, err := target.CommitTransaction(batch, tx.After); err != nil {
		_ = target.AbortTransaction(batch)
		return err
	}
	return nil
}
