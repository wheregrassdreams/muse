package nativeui

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/i18n"
	documentapp "muse/application/document"
	"muse/locales"
	"muse/nativeui/workbench/features/dialogs/document"
)

// The menu choice belongs to the document dialog presentation. The adapter
// translates it to the application use case's export format at the boundary.
type noteExportFormat = documentdialogs.ExportFormat

const (
	noteExportMarkdown = documentdialogs.ExportMarkdown
	noteExportHTML     = documentdialogs.ExportHTML
	noteExportPDF      = documentdialogs.ExportPDF
)

func applicationExportFormat(f noteExportFormat) documentapp.ExportFormat {
	switch f {
	case documentdialogs.ExportHTML:
		return documentapp.ExportHTML
	case documentdialogs.ExportPDF:
		return documentapp.ExportPDF
	default:
		return documentapp.ExportMarkdown
	}
}

func exportFormatLabel(f noteExportFormat) string {
	switch f {
	case documentdialogs.ExportHTML:
		return locales.DocumentExportHTML()
	case documentdialogs.ExportPDF:
		return locales.DocumentExportPDF()
	default:
		return locales.DocumentExportMarkdown()
	}
}

func (a *App) exportDocument(format noteExportFormat) {
	if a.busy || a.imageDocumentOpen() {
		return
	}
	_, name, sourcePath, source, _ := a.snapshotDoc()
	baseName := documentapp.ExportBaseName(name, sourcePath)
	if baseName == locales.DocumentUntitled() {
		baseName = "Untitled"
	}
	fileName := baseName + "." + applicationExportFormat(format).Extension()
	label := exportFormatLabel(format)

	a.startBusy()
	go func() {
		path, err := saveDialog(mygo.SaveDialogOptions{
			Parent:      a.win,
			Title:       label,
			DefaultPath: fileName,
			Filters:     []mygo.FileFilter{{Name: label, Extensions: []string{applicationExportFormat(format).Extension()}}},
		})
		if err != nil || path == "" {
			a.finish(nil)
			return
		}
		if filepath.Ext(path) == "" {
			path += "." + applicationExportFormat(format).Extension()
		}

		baseURL := documentapp.FileBaseURL(sourcePath)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		data, err := documentapp.PrepareExport(ctx, documentapp.ExportRequest{
			Format: applicationExportFormat(format), Title: name, Source: source, BaseURL: baseURL, Language: i18n.Locale(),
		}, mygoPDFRenderer{})
		if err == nil {
			if a.documentExportWriter == nil {
				err = errors.New("document export writer is unavailable")
			} else {
				err = a.documentExportWriter.WriteExport(ctx, path, data)
			}
		}
		a.finish(func() {
			if err != nil {
				a.notice(locales.CommonSaveError(err.Error()))
				return
			}
			a.notice(locales.DocumentExported())
		})
	}()
}

type mygoPDFRenderer struct{}

func (mygoPDFRenderer) RenderPDF(ctx context.Context, document, baseURL string) ([]byte, error) {
	return renderNotePDF(ctx, document, baseURL)
}

// renderNotePDF uses MyGo's platform WebView print engine. The hidden page is
// separate from the native editor window, so the PDF contains only the note.
func renderNotePDF(ctx context.Context, document, baseURL string) ([]byte, error) {
	win := mygo.NewWindow(mygo.WindowOptions{
		Title:  "Muse export",
		URL:    "about:blank",
		Width:  900,
		Height: 1200,
		Hidden: true,
	})
	defer win.Destroy()
	page := win.Page()
	if page == nil {
		return nil, fmt.Errorf("PDF export page is unavailable")
	}
	ready := make(chan struct{})
	off := page.OnDOMReady(func() { close(ready) })
	defer off()
	page.LoadHTML(document, baseURL)
	select {
	case <-ready:
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(30 * time.Second):
		return nil, context.DeadlineExceeded
	}
	resourcesCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	_, _ = page.EvalContext(resourcesCtx, `await Promise.race([
		Promise.all(Array.from(document.images, image => image.decode().catch(() => {}))),
		new Promise(resolve => setTimeout(resolve, 8000))
	]); return true`)
	return page.PrintToPDF(mygo.PDFOptions{PageSize: mygo.PageA4})
}
