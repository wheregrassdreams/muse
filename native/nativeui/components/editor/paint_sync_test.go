package editor

import (
	"sync"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

// TestAppEditWaitsForPaint keeps the frame painter inside an invalidation
// callback while an app-style ReplaceAll arrives. MyGo paints after View has
// returned; without the editor's paint boundary, the replacement can swap
// the parser while that frame is still walking its old image leaf.
func TestAppEditWaitsForPaint(t *testing.T) {
	const before = "# 旧标题\n\n![图](a.png)\n\n正文\n"
	const after = "# 新标题\n"
	e := New(before, Options{})
	var picture *leaf
	for _, l := range e.leaves {
		if e.pictureBlock(l) {
			picture = l
			break
		}
	}
	if picture == nil || picture.id < 0 {
		t.Fatal("fixture has no parsed image paragraph")
	}

	paintEntered := make(chan struct{})
	resumePaint := make(chan struct{})
	var enteredOnce sync.Once
	forceWidthChange := false
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Column(c).Fill().Children(func() { e.View(c).Grow(1) })
		// Let the draw callback observe a changed width after View has laid
		// out the old document, giving the test a deterministic paint pause.
		if forceWidthChange {
			e.elemW = 0
		}
	}, 800, 600)
	e.opts.Invalidate = func() {
		if forceWidthChange {
			enteredOnce.Do(func() {
				close(paintEntered)
				<-resumePaint
			})
		}
	}
	forceWidthChange = true

	frameDone := make(chan struct{})
	go func() {
		tt.Frame()
		close(frameDone)
	}()
	select {
	case <-paintEntered:
	case <-time.After(2 * time.Second):
		close(resumePaint)
		t.Fatal("paint did not reach the synchronization point")
	}

	editStarted, editDone := make(chan struct{}), make(chan struct{})
	go func() {
		close(editStarted)
		e.ReplaceAll(after)
		close(editDone)
	}()
	<-editStarted
	select {
	case <-editDone:
		close(resumePaint)
		<-frameDone
		t.Fatal("ReplaceAll changed parser state during paint")
	case <-time.After(30 * time.Millisecond):
	}
	close(resumePaint)
	<-frameDone
	<-editDone

	forceWidthChange = false
	tt.Frame()
	if got := e.Source(); got != after {
		t.Fatalf("source after app edit = %q, want %q", got, after)
	}
	for _, l := range e.leaves {
		if l.id >= e.p.doc.Len() {
			t.Fatalf("leaf id %d is outside current parser (%d blocks)", l.id, e.p.doc.Len())
		}
		if l.kind == kPara {
			e.pictureBlock(l)
		}
	}
	if picture.id < e.p.doc.Len() {
		t.Fatalf("old picture leaf id %d should be outside the replacement parser (%d blocks)", picture.id, e.p.doc.Len())
	}
}
