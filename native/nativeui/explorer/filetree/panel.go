package filetree

import (
	"path/filepath"
	"time"

	"github.com/egoist/mygo/transfer"
	"github.com/egoist/mygo/ui"
	"muse/locales"
)

// PanelInput contains view facts and narrow toolkit/application capabilities.
// It never exposes the application object to the Explorer feature.
type PanelInput[T any] struct {
	Model                ModelInput
	Drag                 DragPort[T]
	DragProjection       DragProjection
	Focus                *ui.Handle
	FocusRequested       bool
	NativeHandoffPending bool
	KeyInput             func(ui.InputEvent) bool
	Dispatch             func(Action[T])
}

// PanelResult returns only toolkit results needed by the shell composition
// layer to finish focus and native drag handoff.
type PanelResult struct {
	NativeOffer    *transfer.Offer
	PopoverAnchor  ui.Element
	Focused        bool
	FocusRequested bool
}

// RenderPanel owns the file-tree widget lifecycle, transient visual state,
// delayed preview cleanup, and presentation-level drag transitions. The host
// supplies the catalog-projected model and handles typed effects.
func RenderPanel[T any](c *ui.Context, handle *Handle, input PanelInput[T]) PanelResult {
	if handle == nil || handle.state == nil {
		return PanelResult{}
	}
	state := handle.state
	dragAtFrameStart := state.DragPreview()
	wasDragging := dragAtFrameStart.Active
	dragAtFrameStart.Active = false
	state.SetDragPreview(dragAtFrameStart)
	state.SetTrashTargetActive(false)
	if !wasDragging && input.Model.RootLabel != "" {
		state.SetDragOperationDisplay(DragOperationDisplay{Kind: DragOperationMoveToFolder, TargetLabel: filepath.Base(input.Model.RootLabel)})
	}
	state.SetFocused(input.Focus != nil && input.Focus.Focused(c))
	input.Model.DragActive = wasDragging
	model := BuildViewModel(handle, input.Model)
	childDropHandled := false
	dispatch := func(action Action[T]) {
		switch action.Kind {
		case DropEntries, DropNative, ImportFiles:
			childDropHandled = true
		}
		if input.Dispatch != nil {
			input.Dispatch(action)
		}
	}
	area := ui.Box(c).FillWidth().Grow(1).MinHeight(0)
	var rootDrop *T
	var rootNativeDrop *transfer.Drop
	var rootFiles []string
	rootOver := false
	if len(model.Entries) > 0 && model.RootCanDrop {
		if payload, ok := ui.Drop[T](area); ok {
			rootDrop = &payload
		}
		if _, over := ui.DragOver[T](area); over {
			rootOver = true
		}
		if model.NativeDrop.Operations != transfer.None && len(model.NativeDrop.Formats) > 0 {
			opts := transfer.DropOptions{Formats: model.NativeDrop.Formats, Operations: model.NativeDrop.Operations}
			if drop, ok := ui.DropData(area, opts); ok {
				rootNativeDrop = &drop
			}
			if _, over := ui.DataDragOver(area, opts); over {
				rootOver = true
			}
		}
		if paths := area.DroppedFiles(); len(paths) > 0 {
			rootFiles = append([]string(nil), paths...)
		}
	}
	var result RenderResult
	area.Children(func() {
		result = Render(c, handle, model, input.Drag, dispatch)
		RenderDragProjection(c, handle, input.DragProjection)
	})
	if rootOver && state.DropHover().TargetID == "" {
		state.SetDropTargetRange(model.RootLabel, 0, len(model.Entries))
		state.SetDragOperationDisplay(DragOperationDisplay{Kind: DragOperationMoveToFolder, TargetLabel: filepath.Base(model.RootLabel)})
	}
	if !childDropHandled && state.DropHover().TargetID == "" {
		switch {
		case rootDrop != nil:
			dispatch(Action[T]{Kind: DropEntries, ItemIDs: dragIDs(input.Drag, *rootDrop), Payload: *rootDrop})
		case rootNativeDrop != nil:
			dispatch(Action[T]{Kind: DropNative, Drop: *rootNativeDrop})
		case len(rootFiles) > 0:
			dispatch(Action[T]{Kind: ImportFiles, Paths: rootFiles})
		}
	}

	result.List.Focusable()
	if input.Focus != nil {
		result.List.Bind(input.Focus)
	}
	if input.KeyInput != nil {
		result.List.HandleInput(func(ev ui.InputEvent) bool { return input.KeyInput(ev) })
	}
	result.List.Label(locales.FilesTreeLabel())
	focusRequested := input.FocusRequested
	if focusRequested && input.Focus != nil {
		input.Focus.Focus(c)
		if input.Focus.Focused(c) {
			focusRequested = false
		}
	}
	focused := input.Focus != nil && input.Focus.Focused(c)
	if focused {
		state.SetFocused(true)
	}

	drag := state.DragPreview()
	if !drag.StartedAt.IsZero() && !state.PointerReleased() {
		if x, y, over := c.Root().PointerPosition(); over {
			drag.Pointer = ui.Point{X: x, Y: y}
			state.SetDragPreview(drag)
		} else if !input.NativeHandoffPending {
			dispatchAction(input.Dispatch, Action[T]{Kind: BeginNativeHandoff, ItemIDs: state.SelectedIDs()})
		}
	}
	previewID := state.PreviewID()
	if previewID != "" && result.HoveredID == previewID {
		state.CancelPreviewLeave()
	} else if previewID != "" && !state.PreviewPopupHovered() {
		if state.PreviewLeaveAt().IsZero() {
			state.BeginPreviewLeave(c.Now(), 220*time.Millisecond)
		}
		if state.PreviewLeaveExpired(c.Now()) {
			state.ClearPreview()
		} else if deadline := state.PreviewLeaveAt(); !deadline.IsZero() {
			c.After(deadline.Sub(c.Now()))
		}
	}
	if !state.DragPreview().Active && !input.NativeHandoffPending && state.PointerReleased() {
		endedDrag := !state.DragPreview().StartedAt.IsZero() || state.PointerReleased()
		state.SetDragPreview(DragPreviewState{})
		state.SetPointerReleased(false)
		state.ClearPointerPress()
		if endedDrag {
			dispatchAction(input.Dispatch, Action[T]{Kind: EndDrag})
		}
	}
	state.SetTrashTargetActive(state.DragPreview().Active || wasDragging)
	return PanelResult{NativeOffer: result.NativeOffer, PopoverAnchor: result.PopoverAnchor, Focused: focused, FocusRequested: focusRequested}
}
