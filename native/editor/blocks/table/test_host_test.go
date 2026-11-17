package table

import "muse/editor"

type testHost struct {
	view        DocumentView
	selection   editor.Selection
	transaction editor.Transaction
}

func (h *testHost) TableView(lineStart int) (DocumentView, bool) {
	return h.view, h.view.LineStart == lineStart
}

func (h *testHost) TextSelection() editor.Selection { return h.selection }

func (h *testHost) DocumentSource() string { return h.view.Source }

func (h *testHost) Apply(transaction editor.Transaction) error {
	if err := transaction.Validate(len(h.view.Source)); err != nil {
		return err
	}
	h.transaction = transaction
	return nil
}
