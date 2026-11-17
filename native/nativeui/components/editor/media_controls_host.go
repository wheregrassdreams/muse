package editor

import (
	"github.com/egoist/mygo/ui"
	media "muse/nativeui/components/editor/blocks/media"
)

// imageHit and the adapter around it remain at the editor host because they
// join a media control target to the editor's leaf and source transaction.
type imageHit struct {
	obj  *object
	box  ui.Rect
	leaf *leaf
}

type imageResizeDrag struct {
	hit     imageHit
	gesture media.ImageResizeGesture
}

type editorImageResizeHost struct{ editor *Editor }

func (h editorImageResizeHost) Source() string { return h.editor.Source() }

func (h editorImageResizeHost) CommitImageResize(from, to int, replacement string) {
	e := h.editor
	e.replace(from, to, replacement)
	e.commit(e.Session.Selection().Anchor, e.Caret())
	e.invalidate()
}

func (e *Editor) applyImagePreview(obj *object, from, to int) {
	d := e.imageDrag
	if d == nil || d.hit.obj == nil || d.hit.obj.imageSource.From != from || d.hit.obj.imageSource.To != to || obj == nil {
		return
	}
	if obj.mediaImage != nil {
		obj.mediaImage.ApplyPreviewWidth(d.gesture.PreviewWidth())
		obj.bmp, obj.assetFrameVersion = obj.mediaImage.Bitmap, obj.mediaImage.FrameVersion
		obj.w, obj.h, obj.descent = obj.mediaImage.Width, obj.mediaImage.Height, obj.mediaImage.Descent
	}
}

func (e *Editor) resizeImageSource(hit imageHit, width float32) {
	obj := hit.obj
	if obj == nil {
		return
	}
	var intrinsicWidth, intrinsicHeight float32
	obj.refreshAssetBitmap()
	if obj.bmp != nil {
		w, h := obj.bmp.Size()
		intrinsicWidth, intrinsicHeight = float32(w), float32(h)
	} else if obj.asset != nil {
		if size, ok := obj.asset.IntrinsicSize(); ok {
			intrinsicWidth, intrinsicHeight = float32(size.X), float32(size.Y)
		}
	}
	media.ResizeImageSource(editorImageResizeHost{editor: e}, obj.imageSource, width, intrinsicWidth, intrinsicHeight)
}

func (e *Editor) imageAt(x, y float32) (imageHit, bool) {
	for i := len(e.imageHits) - 1; i >= 0; i-- {
		h := e.imageHits[i]
		if h.box.Contains(x, y) {
			return h, true
		}
	}
	return imageHit{}, false
}

func (e *Editor) imageAlignCenter(sourceFrom int) bool {
	return e.imageAlignments.Centered(sourceFrom)
}

func (e *Editor) toggleImageAlignment(hit imageHit) {
	if hit.obj == nil || !hit.obj.block || hit.obj.imageSource.From < 0 {
		return
	}
	e.imageAlignments.Toggle(hit.obj.imageSource.From)
	e.views = make(map[viewKey]*leafView)
	for _, l := range e.leaves {
		l.view = nil
	}
	e.leavesLaid = false
	e.invalidate()
}
