package editor

import (
	"fmt"
	editormodel "github.com/wheregrassdreams/muse-editor/text/session"
	"muse/locales"
	"path/filepath"
	"strings"
)

type wikiEmbedCache struct {
	path, source string
	editor       *Editor
	width        float32
}

// wikiNoteObject builds a read-only Markdown projection for an embedded note.
// Its editor owns a separate parser/layout; it never shares source offsets with
// the host and is never attached to an input handler.
func (e *Editor) wikiNoteObject(target string, width float32) *object {
	if e.opts.ResolveWikiEmbed == nil {
		return nil
	}
	path, source, ok := e.opts.ResolveWikiEmbed(e.opts.CurrentPath, target)
	if !ok || strings.TrimSpace(path) == "" {
		return nil
	}
	path = strings.TrimSpace(path)
	if !filepath.IsAbs(path) && e.opts.AssetRoot != "" {
		path = filepath.Join(e.opts.AssetRoot, filepath.FromSlash(path))
	}
	path = canonicalEmbedPath(path)
	stack := append([]string(nil), e.wikiEmbedStack...)
	if e.opts.CurrentPath != "" && !containsEmbedPath(stack, e.opts.CurrentPath) {
		stack = append(stack, canonicalEmbedPath(e.opts.CurrentPath))
	}
	if containsEmbedPath(stack, path) {
		return &object{
			embedBlocked: locales.EditorCircularEmbedBlocked(filepath.Base(path)),
			w:            max(width, 80),
			h:            e.lk.size * 2.4,
			block:        true,
		}
	}
	stack = append(stack, path)
	baseDir := filepath.Dir(path)
	cacheKey := fmt.Sprintf("%s|%s", path, strings.Join(stack, "\x00"))
	if e.wikiEmbeds == nil {
		e.wikiEmbeds = make(map[string]*wikiEmbedCache)
	}
	cache := e.wikiEmbeds[cacheKey]
	if cache == nil {
		opts := Options{
			FontSize:               e.lk.size,
			LineHeight:             e.lk.lh,
			Font:                   e.lk.font,
			MonoFont:               e.lk.mono,
			DisableSmartTypography: e.opts.DisableSmartTypography,
			SoftBreaksAsSpaces:     e.opts.SoftBreaksAsSpaces,
			HideFrontmatter:        e.opts.HideFrontmatter,
			MaxWidth:               width,
			BaseDir:                baseDir,
			AssetRoot:              e.opts.AssetRoot,
			ImageCacheStore:        e.opts.ImageCacheStore,
			ImageLocalFiles:        e.opts.ImageLocalFiles,
			ResolveImage:           e.opts.ResolveImage,
			Invalidate:             e.invalidateEmbeddedLayout,
			ResolveWikiEmbed:       e.opts.ResolveWikiEmbed,
			ResolveWikiLink:        e.opts.ResolveWikiLink,
			CurrentPath:            path,
			LinkRevision:           e.opts.LinkRevision,
			LinkContext:            path,
		}
		editor := New(source, opts)
		editor.wikiEmbedStack = stack
		editor.focused = false
		editor.pointerX, editor.pointerY = -1e6, -1e6
		cache = &wikiEmbedCache{path: path, source: source, editor: editor, width: width}
		e.wikiEmbeds[cacheKey] = cache
	} else if cache.path != path || cache.source != source {
		cache.path, cache.source = path, source
		cache.editor.SetSource(source)
	}
	child := cache.editor
	// Embedded content is laid out on every request at the host's current
	// width and look. Its body source is read-only and has no caret selection.
	child.lk = e.lk
	child.opts.CurrentPath = path
	child.opts.BaseDir = baseDir
	child.opts.AssetRoot = e.opts.AssetRoot
	child.opts.ImageCacheStore = e.opts.ImageCacheStore
	child.opts.ImageLocalFiles = e.opts.ImageLocalFiles
	child.opts.ResolveImage = e.opts.ResolveImage
	child.opts.LinkRevision = e.opts.LinkRevision
	child.opts.LinkContext = path
	pad := e.lk.size * 0.55
	contentWidth := max(width-2*pad, 60)
	child.colX, child.colW, child.elemW = 0, contentWidth, contentWidth
	child.padTop, child.padBottom = 0, 0
	child.viewH = max(child.docH, e.lk.size*4)
	child.scroll.X, child.scroll.Y = 0, 0
	cache.width = width
	// The virtual document has no active caret. Keeping it beyond the source
	// prevents its opening heading or inline syntax from being revealed.
	child.Session.SetSelection(editormodel.Selection{Anchor: len(child.Source()), Head: len(child.Source())})
	child.block, child.blockInput, child.tableSelection = nil, nil, nil
	child.focused = false
	child.leavesLaid = false
	child.layoutDoc(0, contentWidth, false)
	// Embedded notes are measured and painted as one atomic object. Their
	// viewport must cover that complete measured body as well: leaving the
	// previous placeholder height here lets child.paint's layoutIfNeeded
	// virtualize the lower image leaves out of a host object that already
	// reserves the full child height.
	child.viewH = max(child.docH, e.lk.size*4)
	h := max(child.docH, e.lk.size*1.7) + 2*pad
	return &object{embed: child, w: width, h: h, block: true}
}

// invalidateEmbeddedLayout makes changes in a read-only embedded editor part
// of its host's layout key. Images inside an embed load asynchronously and
// change the embed's measured height; repainting only the child can otherwise
// leave the host's cached object at its placeholder height and clip the image.
func (e *Editor) invalidateEmbeddedLayout() {
	e.assetGen.Add(1)
	e.invalidate()
}

func canonicalEmbedPath(path string) string {
	path = filepath.Clean(strings.TrimSpace(path))
	if abs, err := filepath.Abs(path); err == nil {
		path = abs
	}
	return filepath.Clean(path)
}

func containsEmbedPath(paths []string, path string) bool {
	path = canonicalEmbedPath(path)
	for _, existing := range paths {
		if canonicalEmbedPath(existing) == path {
			return true
		}
	}
	return false
}
