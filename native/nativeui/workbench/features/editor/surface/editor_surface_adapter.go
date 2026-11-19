package noteeditor

import (
	layoutindex "github.com/wheregrassdreams/muse-editor/text/layout"
	editorui "muse/nativeui/workbench/features/editor"
	hosting "muse/nativeui/workbench/framework/hosting"
)

// editorSurfaceAdapter binds the reusable editor widget to the note editor's
// feature-owned command ports. The widget stays independent of noteeditor and
// the surface does not expose MyGo-specific value types.
type editorSurfaceAdapter struct {
	*editorui.Editor
}

var (
	_ hosting.Content  = (*editorSurfaceAdapter)(nil)
	_ DocumentOpenCommands = (*editorSurfaceAdapter)(nil)
	_ FocusCommands        = (*editorSurfaceAdapter)(nil)
	_ ModalCommands        = (*editorSurfaceAdapter)(nil)
	_ NavigationCommands   = (*editorSurfaceAdapter)(nil)
	_ SearchCommands       = (*editorSurfaceAdapter)(nil)
	_ ClipboardCommands    = (*editorSurfaceAdapter)(nil)
	_ FormattingCommands   = (*editorSurfaceAdapter)(nil)
	_ PresentationCommands = (*editorSurfaceAdapter)(nil)
)

// NewEditorSurface adapts the reusable editor widget to noteeditor's feature
// command ports without exposing renderer-specific types through App.
func NewEditorSurface(editor *editorui.Editor) hosting.Content {
	return &editorSurfaceAdapter{Editor: editor}
}

func (e *editorSurfaceAdapter) ResetForDocumentOpen() {
	e.Editor.ResetForDocumentOpen()
}

func (e *editorSurfaceAdapter) SelectionRange() TextRange {
	r := e.Editor.SelectionRange()
	return TextRange{Start: r.Start, End: r.End}
}

func (e *editorSurfaceAdapter) NavigateToOffset(offset int) {
	e.Editor.NavigateToOffset(offset, layoutindex.PageJumpNearest)
}

func (e *editorSurfaceAdapter) Headings() []Heading {
	items := e.Editor.Headings()
	result := make([]Heading, len(items))
	for i, item := range items {
		result[i] = Heading{Level: item.Level, Text: item.Text, Offset: item.Offset}
	}
	return result
}

func (e *editorSurfaceAdapter) SetHighlights(ranges []TextRange, current int) {
	converted := make([]editorui.Range, len(ranges))
	for i, item := range ranges {
		converted[i] = editorui.Range{Start: item.Start, End: item.End}
	}
	e.Editor.SetHighlights(converted, current)
}

func (e *editorSurfaceAdapter) Configure(settings PresentationSettings) {
	options := e.Editor.Options()
	options.Header = settings.Header
	options.FontSize = settings.FontSize
	options.LineHeight = settings.LineHeight
	options.DisableSmartTypography = settings.DisableSmartTypography
	options.HeadingMarkerReveal = settings.HeadingMarkerReveal
	options.VimVirtualImageRows = settings.VimVirtualImageRows
	options.CursorFade = settings.CursorFade
	options.SmoothCursor = settings.SmoothCursor
	options.HideMouseCursor = settings.HideMouseCursor
	options.SmoothPageJumps = settings.SmoothPageJumps
	options.SearchMatchPulse = settings.SearchMatchPulse
	options.VimYankFlash = settings.VimYankFlash
	options.TopInset = settings.TopInset
	options.HeaderHeight = settings.HeaderHeight
	options.HideFrontmatter = settings.HideFrontmatter
	options.BaseDir = settings.BaseDir
	options.AssetRoot = settings.AssetRoot
	options.CurrentPath = settings.CurrentPath
	options.Code = settings.Code
	options.LinkRevision = settings.LinkRevision
	options.LinkContext = settings.LinkContext
	e.Editor.SetOptions(options)
}

func (e *editorSurfaceAdapter) Settings() PresentationSettings {
	options := e.Editor.Options()
	return PresentationSettings{
		Header:   options.Header,
		FontSize: options.FontSize, LineHeight: options.LineHeight,
		DisableSmartTypography: options.DisableSmartTypography,
		HeadingMarkerReveal:    options.HeadingMarkerReveal,
		VimVirtualImageRows:    options.VimVirtualImageRows,
		CursorFade:             options.CursorFade, SmoothCursor: options.SmoothCursor,
		HideMouseCursor: options.HideMouseCursor, SmoothPageJumps: options.SmoothPageJumps,
		SearchMatchPulse: options.SearchMatchPulse, VimYankFlash: options.VimYankFlash,
		TopInset: options.TopInset, HeaderHeight: options.HeaderHeight,
		HideFrontmatter: options.HideFrontmatter,
		BaseDir:         options.BaseDir, AssetRoot: options.AssetRoot,
		CurrentPath: options.CurrentPath, Code: options.Code,
		LinkRevision: options.LinkRevision, LinkContext: options.LinkContext,
	}
}

func (e *editorSurfaceAdapter) MemorySnapshot() MemorySnapshot {
	snapshot := e.Editor.MemorySnapshot()
	return MemorySnapshot{
		SourceBytes: snapshot.SourceBytes, Leaves: snapshot.Leaves,
		VirtualLayout: snapshot.VirtualLayout, ImageCandidateLeaves: snapshot.ImageCandidateLeaves,
		ImageAssetViews: snapshot.ImageAssetViews, ImageAssets: snapshot.ImageAssets,
		LoadedImageAssets: snapshot.LoadedImageAssets, ImagePixelBytes: snapshot.ImagePixelBytes,
	}
}
