package document

import (
	"context"
	"strings"
	"testing"
)

type testPDFRenderer struct {
	html, baseURL string
}

func (p *testPDFRenderer) RenderPDF(_ context.Context, html, baseURL string) ([]byte, error) {
	p.html, p.baseURL = html, baseURL
	return []byte("pdf bytes"), nil
}

func TestPrepareExportMarkdownAndHTML(t *testing.T) {
	markdown, err := PrepareExport(context.Background(), ExportRequest{Format: ExportMarkdown, Source: "# Note\n"}, nil)
	if err != nil || string(markdown) != "# Note\n" {
		t.Fatalf("Markdown export = %q, %v", markdown, err)
	}

	html, err := PrepareExport(context.Background(), ExportRequest{
		Format: ExportHTML, Title: `A <note>`, Source: "# Hello", BaseURL: "file:///tmp/", Language: "zh-CN",
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`lang="zh-CN"`, `<title>A &lt;note&gt;</title>`, `<base href="file:///tmp/">`, "<h1>Hello</h1>"} {
		if !strings.Contains(string(html), want) {
			t.Errorf("HTML export missing %q", want)
		}
	}
}

func TestPrepareExportPDFUsesPort(t *testing.T) {
	renderer := &testPDFRenderer{}
	data, err := PrepareExport(context.Background(), ExportRequest{Format: ExportPDF, Title: "Note", Source: "# Hello", BaseURL: "file:///notes/"}, renderer)
	if err != nil || string(data) != "pdf bytes" {
		t.Fatalf("PDF export = %q, %v", data, err)
	}
	if renderer.baseURL != "file:///notes/" || !strings.Contains(renderer.html, "<h1>Hello</h1>") {
		t.Fatalf("renderer input = html %q, base %q", renderer.html, renderer.baseURL)
	}
}

func TestPrepareExportRequiresPDFPort(t *testing.T) {
	if _, err := PrepareExport(context.Background(), ExportRequest{Format: ExportPDF}, nil); err == nil {
		t.Fatal("expected missing PDF renderer error")
	}
}

func TestExportBaseName(t *testing.T) {
	if got := ExportBaseName("Draft.md", "/notes/Actual.md"); got != "Actual" {
		t.Fatalf("ExportBaseName = %q", got)
	}
	if got := ExportBaseName("", ""); got != "Untitled" {
		t.Fatalf("empty ExportBaseName = %q", got)
	}
}
