package noteeditor

import (
	"muse/document"
	editormodel "muse/editor"
	editorui "muse/nativeui/editor"
	workspaceui "muse/nativeui/workspace"
	"muse/workspace"
)

// NativeEditorOptions is the MyGo renderer configuration needed to construct
// a note pane. The surface feature owns the adaptation from the neutral editor
// session to its backend-neutral command ports.
type NativeEditorOptions struct {
	Editor   editorui.Options
	OnChange func()
	OnClose  func()
}

// NewNativeContent constructs a note document session and adapts the MyGo
// editor to the feature's generic content contract.
func NewNativeContent(
	paneID workspace.PaneID,
	note document.Note,
	cursorOffset int,
	dependencies Dependencies,
	options NativeEditorOptions,
	onAction func(Action),
) (*Content, error) {
	return NewContent(paneID, note, cursorOffset, dependencies, func(session *editormodel.Session, surface SurfaceOptions) workspaceui.Content {
		editorOptions := options.Editor
		editorOptions.OnChange = surface.OnChange
		editorOptions.OnClose = surface.OnClose
		return NewEditorSurface(editorui.NewFromSession(session, editorOptions))
	}, onAction)
}
