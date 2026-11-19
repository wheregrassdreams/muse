package editor

import (
	"path/filepath"
	"strings"

	"github.com/egoist/mygo/transfer"
	"github.com/egoist/mygo/ui"
	media "muse/nativeui/workbench/features/editor/blocks/media"
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

// copyImageObject places an Obsidian-compatible internal embed on the
// Markdown clipboard, keeping the original asset file in place.
func (e *Editor) copyImageObject(path string) {
	path = strings.TrimSpace(path)
	if path == "" {
		return
	}
	target := filepath.Base(path)
	if root := strings.TrimSpace(e.opts.AssetRoot); root != "" {
		if rel, err := filepath.Rel(root, path); err == nil && rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel) {
			target = filepath.ToSlash(rel)
		}
	}
	target = strings.ReplaceAll(target, "|", "\\|")
	target = strings.ReplaceAll(target, "]", "\\]")
	md := "![[" + target + "]]"
	data := transfer.New(transfer.NewItem(transfer.Bytes(transfer.Text, []byte(md)), transfer.Bytes(transfer.Markdown, []byte(md))))
	if e.opts.WriteClipboard != nil {
		e.opts.WriteClipboard(data)
	} else {
		e.services.WriteClipboardData(data)
	}
}

// CopyImageObject copies an image as an internal Markdown embed without
// changing the asset file. Workbench clipboard commands use this entry point.
func (e *Editor) CopyImageObject(path string) {
	e.copyImageObject(path)
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
