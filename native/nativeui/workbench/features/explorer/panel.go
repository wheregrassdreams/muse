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
	result := Render(c, handle, model, input.Drag, func(action Action[T]) {
		if input.Dispatch != nil {
			input.Dispatch(action)
		}
	})
	RenderDragProjection(c, handle, input.DragProjection)

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
	if !state.DragPreview().Active && !input.NativeHandoffPending && (state.PointerReleased() || (!wasDragging && state.DragPreview().StartedAt.IsZero())) {
		endedDrag := !state.DragPreview().StartedAt.IsZero() || state.PointerReleased()
		state.SetDragPreview(DragPreviewState{})
		state.SetPointerReleased(false)
		state.ClearPointerPress()
		if endedDrag {
			dispatchAction(input.Dispatch, Action[T]{Kind: EndDrag})
		}
	}
	state.SetTrashTargetActive(state.DragPreview().Active || wasDragging)
	return PanelResult{NativeOffer: result.NativeOffer, Focused: focused, FocusRequested: focusRequested}
}
