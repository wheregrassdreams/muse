package editor

import (
	"strings"

	"github.com/egoist/mygo/ui"
	"muse/markdown/diagrams"
	"muse/markdown/images"
	"muse/markdown/syntax"
	media "muse/nativeui/components/editor/blocks/media"
	render "muse/nativeui/components/editor/render"
)

// editorMediaHost supplies only vault resolution and asset acquisition. The
// media package owns image metrics, placeholder sizing, and bitmap refresh.
type editorMediaHost struct{ editor *Editor }

func (h editorMediaHost) ResolveImage(destination string) string {
	e := h.editor
	if e.opts.ResolveImage != nil {
		return e.opts.ResolveImage(destination, e.opts.BaseDir, e.opts.AssetRoot)
	}
	return images.ResolveSource(destination, images.SourceOptions{
		BaseDir: e.opts.BaseDir, Root: e.opts.AssetRoot, LocalFiles: e.opts.ImageLocalFiles,
	})
}

func (h editorMediaHost) LoadImage(path string, dataURL bool) *images.Asset {
	e := h.editor
	source := images.SourceOptions{BaseDir: e.opts.BaseDir, Root: e.opts.AssetRoot, LocalFiles: e.opts.ImageLocalFiles}
	if dataURL {
		return e.dataImageAssets.Load(path, images.LoadOptions{
			Source: images.SourceOptions{LocalFiles: e.opts.ImageLocalFiles}, Store: e.opts.ImageCacheStore,
		}, e.assetLoaded, e.assetLoaded)
	}
	return images.Load(path, images.LoadOptions{Source: source, Store: e.opts.ImageCacheStore}, e.assetLoaded, e.assetLoaded)
}

func (e *Editor) imageObject(destination, alt string, width float32, alone bool) *object {
	image := media.NewImageObject(editorMediaHost{editor: e}, destination, alt, width, e.lk.size, alone)
	return objectFromMediaImage(image)
}

func (e *Editor) sizedImage(destination, alt string, size media.ImageSize, width float32, alone bool) *object {
	image := media.NewImageObject(editorMediaHost{editor: e}, destination, alt, width, e.lk.size, alone)
	if image == nil {
		return nil
	}
	image.ApplySize(size, width)
	return objectFromMediaImage(image)
}

func objectFromMediaImage(image *media.ImageObject) *object {
	if image == nil {
		return nil
	}
	return &object{
		asset: image.Asset, bmp: image.Bitmap, assetFrameVersion: image.FrameVersion,
		imagePending: image.Pending, imagePath: image.Path, imageDest: image.Destination,
		alt: image.Alt, w: image.Width, h: image.Height, descent: image.Descent, block: image.Block,
		mediaImage: image,
	}
}

func (e *Editor) htmlImageObject(html string, width float32) *object {
	src, alt, _, imageWidth, imageHeight, ok := syntax.HTMLOnlyImage(html)
	if !ok {
		return nil
	}
	size := media.ImageSize{Width: imageWidth, Height: imageHeight}
	if i := strings.Index(strings.ToLower(html), "<img"); i >= 0 {
		if j := strings.IndexByte(html[i:], '>'); j >= 0 {
			if _, _, parsed, ok := render.HTMLImageTag(html[i : i+j+1]); ok {
				size = media.ImageSize{Width: parsed.Width, Height: parsed.Height, Zoom: parsed.Zoom}
			}
		}
	}
	return e.sizedImage(src, alt, size, width, true)
}

func (o *object) refreshAssetBitmap() {
	if o == nil {
		return
	}
	if o.mediaImage != nil {
		o.mediaImage.Refresh()
		o.bmp, o.assetFrameVersion = o.mediaImage.Bitmap, o.mediaImage.FrameVersion
		return
	}
	o.bmp, o.assetFrameVersion = media.RefreshImageBitmap(o.asset, o.bmp, o.assetFrameVersion)
}

// assetLoaded invalidates the host projection after media resolution or decode.
func (e *Editor) assetLoaded() {
	e.assetGen.Add(1)
	if e.opts.Invalidate != nil {
		e.opts.Invalidate()
	}
}

func (e *Editor) mathObject(tex string, size float32, display bool) *object {
	formula, ok := media.TypesetInline(tex, size, display)
	if !ok {
		return nil
	}
	return objectFromMath(formula)
}

func (e *Editor) mathBlockObject(tex string, width float32) *object {
	formula, ok := media.TypesetBlock(tex, e.lk.size, width)
	if !ok {
		return nil
	}
	return objectFromMath(formula)
}

func objectFromMath(formula media.MathObject) *object {
	return &object{math: formula.Raster, w: formula.Width, h: formula.Height, descent: formula.Descent, block: formula.Block}
}

func (e *Editor) sketchOptions() diagrams.Options {
	return diagrams.Options{Theme: media.DiagramTheme(e.lk.panel, e.lk.text, e.lk.muted, e.lk.border, e.lk.codeBg, e.lk.accent)}
}

var _ media.ImageHost = editorMediaHost{}
var _ ui.TextInputClient = (*Editor)(nil)
