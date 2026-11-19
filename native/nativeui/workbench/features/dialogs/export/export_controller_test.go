package exportdialogs

import (
	"testing"

	documentdialogs "muse/nativeui/dialogs/document"
)

func TestExportControllerDispatchesSelectedFormat(t *testing.T) {
	var got ExportAction
	called := false
	controller := NewExportController(func(action ExportAction) {
		got = action
		called = true
	})

	controller.Select(documentdialogs.ExportPDF)

	if !called || got.Format != documentdialogs.ExportPDF {
		t.Fatalf("dispatched action = %+v, called = %v", got, called)
	}
}

func TestExportControllerWithoutHandlerIsSafe(t *testing.T) {
	var controller *ExportController
	controller.AddMenu(nil)
	controller.Select(documentdialogs.ExportMarkdown)

	NewExportController(nil).Select(documentdialogs.ExportHTML)
}
