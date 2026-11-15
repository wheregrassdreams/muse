package editor

import (
	"fmt"

	"muse/editor/internal/core"
)

// ApplyTransaction applies source edits expressed against one original source
// snapshot and records them as one undo step. Selection is restored after the
// transaction, so block commands do not need to mutate source or history
// internals directly.
func (e *Editor) ApplyTransaction(tx core.Transaction) error {
	if err := tx.Validate(len(e.src)); err != nil {
		return err
	}
	if tx.Before.Anchor != e.anchor || tx.Before.Head != e.caret {
		return fmt.Errorf("transaction selection is stale")
	}
	if len(tx.Edits) == 0 {
		return nil
	}
	e.begin(stepEdit)
	for i := len(tx.Edits) - 1; i >= 0; i-- {
		edit := tx.Edits[i]
		e.replace(edit.Range.Start, edit.Range.End, edit.Text)
	}
	e.commit(tx.After.Anchor, tx.After.Head)
	return nil
}

// TransactionSelection returns the text selection coordinates used by the
// transaction contract. Block-specific selection stays with its controller.
func (e *Editor) TransactionSelection() core.Selection {
	return core.Selection{Anchor: e.anchor, Head: e.caret}
}
