package exportdialogs

import (
	"context"
	"path/filepath"

	documentdialogs "muse/nativeui/dialogs/document"
)

type ExportFormat = documentdialogs.ExportFormat

const (
	ExportMarkdown = documentdialogs.ExportMarkdown
	ExportHTML     = documentdialogs.ExportHTML
	ExportPDF      = documentdialogs.ExportPDF
)

// ExportDocument is the detached editor snapshot needed to prepare an export.
// It contains no editor or App-owned state.
type ExportDocument struct {
	Title, SourcePath, Source string
}

// ExportSpec is the application-owned policy for presenting an export choice
// in a native save dialog.
type ExportSpec struct {
	Label, Extension, DefaultName string
}

// ExportRequest is passed to the application document export use case.
type ExportRequest struct {
	Format                    ExportFormat
	Title, SourcePath, Source string
}

// ExportPorts are the narrow platform and application capabilities required
// to execute an export. Format policy and document conversion remain behind
// the application callbacks; this package owns only dialog orchestration.
type ExportPorts struct {
	Busy              func() bool
	ImageDocumentOpen func() bool
	Document          func() ExportDocument
	Spec              func(ExportFormat, ExportDocument) ExportSpec
	ChoosePath        func(context.Context, ExportSpec) (string, error)
	Prepare           func(context.Context, ExportRequest) ([]byte, error)
	Write             func(context.Context, string, []byte) error
	StartBusy         func()
	Finish            func(func())
	Go                func(func())
	Notify            func(string)
	SavedMessage      func() string
	SaveErrorMessage  func(error) string
}

// ExportHost routes the presentation choice through injected native and
// application capabilities. It never retains a document snapshot.
type ExportHost struct{ ports ExportPorts }

func NewExportHost(ports ExportPorts) *ExportHost { return &ExportHost{ports: ports} }

func (h *ExportHost) Select(format ExportFormat) {
	if h == nil || h.ports.Document == nil || h.ports.Spec == nil || h.ports.ChoosePath == nil ||
		h.ports.Prepare == nil || h.ports.Write == nil || h.ports.Finish == nil {
		return
	}
	if h.ports.Busy != nil && h.ports.Busy() || h.ports.ImageDocumentOpen != nil && h.ports.ImageDocumentOpen() {
		return
	}
	document := h.ports.Document()
	spec := h.ports.Spec(format, document)
	if h.ports.StartBusy != nil {
		h.ports.StartBusy()
	}
	work := func() {
		ctx := context.Background()
		path, err := h.ports.ChoosePath(ctx, spec)
		pickerFailed := err != nil
		if err == nil && path != "" {
			if filepath.Ext(path) == "" && spec.Extension != "" {
				path += "." + spec.Extension
			}
			var data []byte
			data, err = h.ports.Prepare(ctx, ExportRequest{
				Format: format, Title: document.Title, SourcePath: document.SourcePath,
				Source: document.Source,
			})
			if err == nil {
				err = h.ports.Write(ctx, path, data)
			}
		}
		h.ports.Finish(func() {
			if err != nil {
				if !pickerFailed && h.ports.Notify != nil && h.ports.SaveErrorMessage != nil {
					h.ports.Notify(h.ports.SaveErrorMessage(err))
				}
				return
			}
			// An empty path is a user cancellation, not an export failure.
			if path == "" {
				return
			}
			if h.ports.Notify != nil && h.ports.SavedMessage != nil {
				h.ports.Notify(h.ports.SavedMessage())
			}
		})
	}
	if h.ports.Go != nil {
		h.ports.Go(work)
	} else {
		go work()
	}
}
