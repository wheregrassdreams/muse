package exportdialogs

import (
	"context"
	"errors"
	"testing"
)

func TestExportHostRoutesSelectedFormatThroughPorts(t *testing.T) {
	var gotFormat ExportFormat
	var gotPath string
	var notices []string
	started, finished := false, false
	host := NewExportHost(ExportPorts{
		Document: func() ExportDocument {
			return ExportDocument{Title: "Note", SourcePath: "/notes/note.md", Source: "body"}
		},
		Spec: func(format ExportFormat, doc ExportDocument) ExportSpec {
			gotFormat = format
			return ExportSpec{Label: "PDF", Extension: "pdf", DefaultName: "note.pdf"}
		},
		ChoosePath: func(_ context.Context, spec ExportSpec) (string, error) {
			if spec.DefaultName != "note.pdf" {
				t.Fatalf("default name = %q", spec.DefaultName)
			}
			return "/tmp/export", nil
		},
		Prepare: func(_ context.Context, request ExportRequest) ([]byte, error) {
			if request.Title != "Note" || request.Source != "body" {
				t.Fatalf("request = %#v", request)
			}
			return []byte("pdf"), nil
		},
		Write: func(_ context.Context, path string, data []byte) error {
			gotPath = path
			if string(data) != "pdf" {
				t.Fatalf("data = %q", data)
			}
			return nil
		},
		StartBusy:    func() { started = true },
		Finish:       func(callback func()) { finished = true; callback() },
		Go:           func(work func()) { work() },
		Notify:       func(message string) { notices = append(notices, message) },
		SavedMessage: func() string { return "saved" },
	})
	host.Select(ExportPDF)
	if !started || !finished || gotFormat != ExportPDF || gotPath != "/tmp/export.pdf" {
		t.Fatalf("started=%v finished=%v format=%v path=%q", started, finished, gotFormat, gotPath)
	}
	if len(notices) != 1 || notices[0] != "saved" {
		t.Fatalf("notices = %#v", notices)
	}
}

func TestExportHostIgnoresPickerCancellationAndReportsApplicationErrors(t *testing.T) {
	var notices []string
	base := ExportPorts{
		Document:         func() ExportDocument { return ExportDocument{} },
		Spec:             func(ExportFormat, ExportDocument) ExportSpec { return ExportSpec{Extension: "md"} },
		Prepare:          func(context.Context, ExportRequest) ([]byte, error) { return nil, errors.New("prepare failed") },
		Write:            func(context.Context, string, []byte) error { return nil },
		Finish:           func(callback func()) { callback() },
		Go:               func(work func()) { work() },
		Notify:           func(message string) { notices = append(notices, message) },
		SaveErrorMessage: func(err error) string { return "save: " + err.Error() },
	}
	base.ChoosePath = func(context.Context, ExportSpec) (string, error) { return "", nil }
	NewExportHost(base).Select(ExportMarkdown)
	if len(notices) != 0 {
		t.Fatalf("cancellation notices = %#v", notices)
	}
	base.ChoosePath = func(context.Context, ExportSpec) (string, error) { return "/tmp/note.md", nil }
	NewExportHost(base).Select(ExportMarkdown)
	if len(notices) != 1 || notices[0] != "save: prepare failed" {
		t.Fatalf("failure notices = %#v", notices)
	}
}
