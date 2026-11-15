package nativeui

import (
	"context"
	"fmt"
	"html"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/i18n"
	mdedit "muse/editor"
	"muse/locales"
)

type noteExportFormat uint8

const (
	noteExportMarkdown noteExportFormat = iota
	noteExportHTML
	noteExportPDF
)

func (f noteExportFormat) extension() string {
	switch f {
	case noteExportHTML:
		return "html"
	case noteExportPDF:
		return "pdf"
	default:
		return "md"
	}
}

func (f noteExportFormat) label() string {
	switch f {
	case noteExportHTML:
		return locales.DocumentExportHTML()
	case noteExportPDF:
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
	baseName := strings.TrimSuffix(name, filepath.Ext(name))
	if sourcePath != "" {
		baseName = strings.TrimSuffix(filepath.Base(sourcePath), filepath.Ext(sourcePath))
	}
	if baseName == "" || baseName == locales.DocumentUntitled() {
		baseName = "Untitled"
	}
	fileName := baseName + "." + format.extension()
	label := format.label()

	a.startBusy()
	go func() {
		path, err := saveDialog(mygo.SaveDialogOptions{
			Parent:      a.win,
			Title:       label,
			DefaultPath: fileName,
			Filters:     []mygo.FileFilter{{Name: label, Extensions: []string{format.extension()}}},
		})
		if err != nil || path == "" {
			a.finish(nil)
			return
		}
		if filepath.Ext(path) == "" {
			path += "." + format.extension()
		}

		var data []byte
		switch format {
		case noteExportMarkdown:
			data = []byte(source)
		case noteExportHTML, noteExportPDF:
			baseURL := ""
			if sourcePath != "" {
				dir, _ := filepath.Abs(filepath.Dir(sourcePath))
				baseURL = (&url.URL{Scheme: "file", Path: filepath.ToSlash(dir) + "/"}).String()
			}
			document := noteExportHTMLDocument(name, source, baseURL)
			if format == noteExportPDF {
				data, err = renderNotePDF(document, baseURL)
			} else {
				data = []byte(document)
			}
		}
		if err == nil {
			err = os.WriteFile(path, data, 0o644)
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

func noteExportHTMLDocument(title, source, baseURL string) string {
	base := ""
	if baseURL != "" {
		base = `<base href="` + htmlEscape(baseURL) + `">`
	}
	return `<!doctype html>
<html lang="` + htmlEscape(i18n.Locale()) + `">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>` + htmlEscape(title) + `</title>
` + base + `
<style>
:root { color-scheme: light; font-family: system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif; color: #242424; background: #fff; }
body { max-width: 860px; margin: 48px auto; padding: 0 32px; line-height: 1.65; overflow-wrap: anywhere; }
img, video { max-width: 100%; height: auto; }
pre { overflow: auto; padding: 1em; background: #f3f4f6; border-radius: 6px; }
code { font-family: ui-monospace, SFMono-Regular, Menlo, monospace; }
blockquote { margin-left: 0; padding-left: 1em; border-left: 3px solid #bbb; color: #555; }
table { border-collapse: collapse; width: 100%; }
th, td { border: 1px solid #ccc; padding: .45em .7em; text-align: left; vertical-align: top; }
th { background: #f3f4f6; }
hr { border: 0; border-top: 1px solid #ccc; }
@media print { body { max-width: none; margin: 0; padding: 0; } pre, blockquote, table { break-inside: avoid; } }
</style>
</head>
<body>
` + mdedit.ExportHTML(source) + `
</body>
</html>`
}

// renderNotePDF uses MyGo's platform WebView print engine. The hidden page is
// separate from the native editor window, so the PDF contains only the note.
func renderNotePDF(document, baseURL string) ([]byte, error) {
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
	case <-time.After(30 * time.Second):
		return nil, fmt.Errorf("timed out while preparing the PDF")
	}
	resourcesCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, _ = page.EvalContext(resourcesCtx, `await Promise.race([
		Promise.all(Array.from(document.images, image => image.decode().catch(() => {}))),
		new Promise(resolve => setTimeout(resolve, 8000))
	]); return true`)
	return page.PrintToPDF(mygo.PDFOptions{PageSize: mygo.PageA4})
}

func htmlEscape(s string) string {
	return html.EscapeString(s)
}
