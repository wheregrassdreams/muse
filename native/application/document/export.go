package document

import (
	"context"
	"fmt"
	"html"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/wheregrassdreams/muse-editor/markdown/render"
)

type ExportFormat uint8

const (
	ExportMarkdown ExportFormat = iota
	ExportHTML
	ExportPDF
)

type ExportRequest struct {
	Format   ExportFormat
	Title    string
	Source   string
	BaseURL  string
	Language string
}

type PDFRenderer interface {
	RenderPDF(context.Context, string, string) ([]byte, error)
}

// ExportWriter persists an artifact at the path selected by the presentation.
type ExportWriter interface {
	WriteExport(context.Context, string, []byte) error
}

func (f ExportFormat) Extension() string {
	switch f {
	case ExportHTML:
		return "html"
	case ExportPDF:
		return "pdf"
	default:
		return "md"
	}
}

// PrepareExport produces the bytes for one export format. PDF conversion is a
// presentation/runtime capability; the use case supplies it through a port.
func PrepareExport(ctx context.Context, req ExportRequest, pdf PDFRenderer) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	switch req.Format {
	case ExportMarkdown:
		return []byte(req.Source), nil
	case ExportHTML, ExportPDF:
		doc := htmlDocument(req)
		if req.Format == ExportHTML {
			return []byte(doc), nil
		}
		if pdf == nil {
			return nil, fmt.Errorf("document: PDF renderer is unavailable")
		}
		return pdf.RenderPDF(ctx, doc, req.BaseURL)
	default:
		return nil, fmt.Errorf("document: unsupported export format %d", req.Format)
	}
}

func htmlDocument(req ExportRequest) string {
	base := ""
	if req.BaseURL != "" {
		base = `<base href="` + html.EscapeString(req.BaseURL) + `">`
	}
	language := html.EscapeString(req.Language)
	if language == "" {
		language = "en"
	}
	title := html.EscapeString(req.Title)
	return `<!doctype html>
<html lang="` + language + `">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>` + title + `</title>
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
` + render.ExportHTML(req.Source) + `
</body>
</html>`
}

// FileBaseURL returns an escaped file URL for relative note assets.
func FileBaseURL(sourcePath string) string {
	if sourcePath == "" {
		return ""
	}
	dir, err := filepath.Abs(filepath.Dir(sourcePath))
	if err != nil {
		return ""
	}
	return (&url.URL{Scheme: "file", Path: filepath.ToSlash(dir) + "/"}).String()
}

func ExportBaseName(name, sourcePath string) string {
	base := strings.TrimSuffix(name, filepath.Ext(name))
	if sourcePath != "" {
		base = strings.TrimSuffix(filepath.Base(sourcePath), filepath.Ext(sourcePath))
	}
	if base == "" {
		return "Untitled"
	}
	return base
}
