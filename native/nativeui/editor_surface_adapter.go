package nativeui

import (
	editorui "muse/nativeui/components/editor"
	"muse/nativeui/surfaces/noteeditor"
	"muse/nativeui/window"
)

// editorSurfaceAdapter binds the reusable editor widget to the note editor's
// feature-owned command ports. The widget stays independent of noteeditor and
// the surface does not expose MyGo-specific value types.
type editorSurfaceAdapter struct {
	*editorui.Editor
}

var (
	_ window.Content                  = (*editorSurfaceAdapter)(nil)
	_ noteeditor.FocusCommands        = (*editorSurfaceAdapter)(nil)
	_ noteeditor.ModalCommands        = (*editorSurfaceAdapter)(nil)
	_ noteeditor.NavigationCommands   = (*editorSurfaceAdapter)(nil)
	_ noteeditor.SearchCommands       = (*editorSurfaceAdapter)(nil)
	_ noteeditor.ClipboardCommands    = (*editorSurfaceAdapter)(nil)
	_ noteeditor.FormattingCommands   = (*editorSurfaceAdapter)(nil)
	_ noteeditor.PresentationCommands = (*editorSurfaceAdapter)(nil)
)

func (e *editorSurfaceAdapter) SelectionRange() noteeditor.TextRange {
	r := e.Editor.SelectionRange()
	return noteeditor.TextRange{Start: r.Start, End: r.End}
}

func (e *editorSurfaceAdapter) NavigateToOffset(offset int) {
	e.Editor.NavigateToOffset(offset, editorui.PageJumpNearest)
}

func (e *editorSurfaceAdapter) Headings() []noteeditor.Heading {
	items := e.Editor.Headings()
	result := make([]noteeditor.Heading, len(items))
	for i, item := range items {
		result[i] = noteeditor.Heading{Level: item.Level, Text: item.Text, Offset: item.Offset}
	}
	return result
}

func (e *editorSurfaceAdapter) SetHighlights(ranges []noteeditor.TextRange, current int) {
	converted := make([]editorui.Range, len(ranges))
	for i, item := range ranges {
		converted[i] = editorui.Range{Start: item.Start, End: item.End}
	}
	e.Editor.SetHighlights(converted, current)
}

func (e *editorSurfaceAdapter) Configure(settings noteeditor.PresentationSettings) {
	options := e.Editor.Options()
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

func (e *editorSurfaceAdapter) Settings() noteeditor.PresentationSettings {
	options := e.Editor.Options()
	return noteeditor.PresentationSettings{
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
