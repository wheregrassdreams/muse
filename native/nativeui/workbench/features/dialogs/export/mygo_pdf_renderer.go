package exportdialogs

import (
	"context"
	"fmt"
	"time"

	"github.com/egoist/mygo"
)

// MyGoPDFRenderer renders an application-prepared HTML document in a hidden
// platform window and returns its printed PDF bytes.
type MyGoPDFRenderer struct{}

func (MyGoPDFRenderer) RenderPDF(ctx context.Context, document, baseURL string) ([]byte, error) {
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
