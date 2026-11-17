package filetree

import (
	"github.com/egoist/mygo/ui"
	"muse/nativeui/interactions"
)

// TreeBinding is a panel-owned keyboard intent. Args keeps action data typed;
// the host translates it to its application action context at the boundary.
type TreeBinding struct {
	Action  interactions.ActionID
	Args    TreeBindingArgs
	Handled bool
}

type TreeBindingArgs struct {
	Delta        int
	Direction    int
	Index        int
	HasIndex     bool
	RelativePath bool
	TargetPanel  interactions.PanelID
}

// StandardTreeBindings maps desktop file-browser shortcuts to semantic actions.
type StandardTreeBindings struct{}

func (StandardTreeBindings) Sequences() []interactions.KeyBinding {
	return []interactions.KeyBinding{{
		Sequence: []interactions.KeyStroke{{Key: "E", Mods: uint32(ui.Cmd | ui.Shift)}},
		ActionID: interactions.ActionFocusPanel,
		Values:   map[string]any{"panel": interactions.PanelIDFiles},
	}}
}

func (StandardTreeBindings) Resolve(mods ui.Modifiers, key ui.Key) TreeBinding {
	if mods == ui.Cmd {
		switch key {
		case ui.KeyA:
			return handled(interactions.ActionSelectAll)
		case ui.KeyC:
			return handled(interactions.ActionCopy)
		case ui.KeyX:
			return handled(interactions.ActionCut)
		case ui.KeyV:
			return handled(interactions.ActionPaste)
		}
	}
	if mods == 0 {
		switch key {
		case ui.KeyEscape:
			return handled(interactions.ActionFocusEditor)
		case ui.KeyF2:
			return handled(interactions.ActionRename)
		case ui.KeyDelete, ui.KeyBackspace:
			return handled(interactions.ActionTrash)
		case ui.KeyDown:
			return withDelta(interactions.ActionNavigate, 1)
		case ui.KeyUp:
			return withDelta(interactions.ActionNavigate, -1)
		case ui.KeyLeft:
			return withDirection(interactions.ActionToggleExpanded, -1)
		case ui.KeyRight:
			return withDirection(interactions.ActionToggleExpanded, 1)
		case ui.KeyEnter:
			return handled(interactions.ActionOpen)
		}
	}
	if mods == ui.Shift {
		switch key {
		case ui.KeyDown:
			return withDelta(interactions.ActionSelectRange, 1)
		case ui.KeyUp:
			return withDelta(interactions.ActionSelectRange, -1)
		}
	}
	return TreeBinding{}
}

// VimTreeBindings maps Vim navigation and file actions to the same semantic
// action IDs as desktop bindings. Sequence state remains in interactions.
type VimTreeBindings struct{}

func (VimTreeBindings) Sequences() []interactions.KeyBinding {
	return []interactions.KeyBinding{{
		Sequence: []interactions.KeyStroke{{Key: "G"}, {Key: "G"}},
		ActionID: interactions.ActionNavigate,
		Values:   map[string]any{"index": 0},
	}}
}

func (VimTreeBindings) PanelSequences() []interactions.KeyBinding {
	return []interactions.KeyBinding{
		{Sequence: []interactions.KeyStroke{{Key: "W", Mods: uint32(ui.Ctrl)}, {Key: "H"}}, ActionID: interactions.ActionFocusDirection, Values: map[string]any{"direction": -1}},
		{Sequence: []interactions.KeyStroke{{Key: "W", Mods: uint32(ui.Ctrl)}, {Key: "Left"}}, ActionID: interactions.ActionFocusDirection, Values: map[string]any{"direction": -1}},
		{Sequence: []interactions.KeyStroke{{Key: "W", Mods: uint32(ui.Ctrl)}, {Key: "L"}}, ActionID: interactions.ActionFocusDirection, Values: map[string]any{"direction": 1}},
		{Sequence: []interactions.KeyStroke{{Key: "W", Mods: uint32(ui.Ctrl)}, {Key: "Right"}}, ActionID: interactions.ActionFocusDirection, Values: map[string]any{"direction": 1}},
		{Sequence: []interactions.KeyStroke{{Key: "W", Mods: uint32(ui.Ctrl)}, {Key: "W"}}, ActionID: interactions.ActionFocusNextPanel},
		{Sequence: []interactions.KeyStroke{{Key: "W", Mods: uint32(ui.Ctrl)}, {Key: "W", Mods: uint32(ui.Shift)}}, ActionID: interactions.ActionFocusPreviousPanel},
	}
}

func (VimTreeBindings) Resolve(mods ui.Modifiers, key ui.Key) TreeBinding {
	if mods == ui.Ctrl|ui.Shift && key == ui.KeyI {
		return handled(interactions.ActionInvertSelection)
	}
	if mods == ui.Shift {
		switch key {
		case ui.KeySlash:
			return handled(interactions.ActionShowHelp)
		case ui.KeyG:
			return withIndex(interactions.ActionNavigate, -1)
		case ui.KeyA:
			return handled(interactions.ActionCreateFolder)
		case ui.KeyJ:
			return withDelta(interactions.ActionSelectRange, 1)
		case ui.KeyK:
			return withDelta(interactions.ActionSelectRange, -1)
		case ui.KeyC:
			return handled(interactions.ActionCloseNode)
		case ui.KeyY:
			return TreeBinding{Action: interactions.ActionCopyPath, Args: TreeBindingArgs{RelativePath: true}, Handled: true}
		}
	}
	if mods != 0 {
		return TreeBinding{}
	}
	switch key {
	case ui.KeyJ:
		return withDelta(interactions.ActionNavigate, 1)
	case ui.KeyK:
		return withDelta(interactions.ActionNavigate, -1)
	case ui.KeyH:
		return withDirection(interactions.ActionToggleExpanded, -1)
	case ui.KeyL:
		return withDirection(interactions.ActionToggleExpanded, 1)
	case ui.KeyR:
		return handled(interactions.ActionRename)
	case ui.KeyY:
		return handled(interactions.ActionCopy)
	case ui.KeyX:
		return handled(interactions.ActionCut)
	case ui.KeyP:
		return handled(interactions.ActionPaste)
	case ui.KeyD:
		return handled(interactions.ActionTrash)
	case ui.KeyA:
		return handled(interactions.ActionCreateFile)
	case ui.KeyZ:
		return handled(interactions.ActionCollapseAll)
	case ui.KeyTab:
		return handled(interactions.ActionToggleSelection)
	}
	return TreeBinding{}
}

func handled(action interactions.ActionID) TreeBinding {
	return TreeBinding{Action: action, Handled: true}
}

func withDelta(action interactions.ActionID, delta int) TreeBinding {
	return TreeBinding{Action: action, Args: TreeBindingArgs{Delta: delta}, Handled: true}
}

func withDirection(action interactions.ActionID, direction int) TreeBinding {
	return TreeBinding{Action: action, Args: TreeBindingArgs{Direction: direction}, Handled: true}
}

func withIndex(action interactions.ActionID, index int) TreeBinding {
	return TreeBinding{Action: action, Args: TreeBindingArgs{Index: index, HasIndex: true}, Handled: true}
}
