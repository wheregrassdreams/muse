package filetree

import (
	"github.com/egoist/mygo/ui"
	commands "muse/nativeui/workbench/framework/interactions/commands"
	keybindings "muse/nativeui/workbench/framework/interactions/keybindings"
)

// TreeBinding is a panel-owned keyboard intent. Args keeps action data typed;
// the host translates it to its application action context at the boundary.
type TreeBinding struct {
	Action  commands.ActionID
	Args    TreeBindingArgs
	Handled bool
}

type TreeBindingArgs struct {
	Delta        int
	Direction    int
	Index        int
	HasIndex     bool
	RelativePath bool
	TargetPanel  commands.PanelID
	TargetID     string
}

// Values converts a tree-local binding's typed arguments to the shared
// command context used at the workbench dispatch boundary.
func (args TreeBindingArgs) Values() map[string]any {
	values := make(map[string]any, 4)
	if args.Delta != 0 {
		values["delta"] = args.Delta
	}
	if args.Direction != 0 {
		values["direction"] = args.Direction
	}
	if args.HasIndex {
		values["index"] = args.Index
	}
	if args.RelativePath {
		values["relative"] = true
	}
	if args.TargetPanel != "" {
		values["panel"] = args.TargetPanel
	}
	return values
}

// KeyCommandForAction translates tree-local shared command IDs into the
// explorer's typed state transition. Commands with host-side effects are not
// handled here and return false.
func KeyCommandForAction(action commands.ActionID, args TreeBindingArgs, preferredIDs []string) (KeyCommand, bool) {
	command := KeyCommand{
		Delta: args.Delta, Direction: args.Direction, Index: args.Index,
		HasIndex: args.HasIndex, TargetID: args.TargetID,
		PreferredIDs: append([]string(nil), preferredIDs...),
	}
	switch action {
	case commands.ActionNavigate:
		command.Operation = KeyNavigate
	case commands.ActionSelectRange:
		command.Operation = KeySelectRange
	case commands.ActionToggleExpanded:
		command.Operation = KeyToggleExpanded
	case commands.ActionCloseNode:
		command.Operation = KeyCloseNode
	case commands.ActionCollapseAll:
		command.Operation = KeyCollapseAll
	case commands.ActionSelectAll:
		command.Operation = KeySelectAll
	case commands.ActionToggleSelection:
		command.Operation = KeyToggleSelection
	case commands.ActionInvertSelection:
		command.Operation = KeyInvertSelection
	default:
		return KeyCommand{}, false
	}
	return command, true
}

// StandardTreeBindings maps desktop file-browser shortcuts to semantic actions.
type StandardTreeBindings struct{}

func (StandardTreeBindings) Resolve(mods ui.Modifiers, key ui.Key) TreeBinding {
	if mods == ui.Cmd {
		switch key {
		case ui.KeyA:
			return handled(commands.ActionSelectAll)
		case ui.KeyC:
			return handled(commands.ActionCopy)
		case ui.KeyX:
			return handled(commands.ActionCut)
		case ui.KeyV:
			return handled(commands.ActionPaste)
		}
	}
	if mods == 0 {
		switch key {
		case ui.KeyEscape:
			return handled(commands.ActionFocusEditor)
		case ui.KeyF2:
			return handled(commands.ActionRename)
		case ui.KeyDelete, ui.KeyBackspace:
			return handled(commands.ActionTrash)
		case ui.KeyDown:
			return withDelta(commands.ActionNavigate, 1)
		case ui.KeyUp:
			return withDelta(commands.ActionNavigate, -1)
		case ui.KeyLeft:
			return withDirection(commands.ActionToggleExpanded, -1)
		case ui.KeyRight:
			return withDirection(commands.ActionToggleExpanded, 1)
		case ui.KeyEnter:
			return handled(commands.ActionOpen)
		}
	}
	if mods == ui.Shift {
		switch key {
		case ui.KeyDown:
			return withDelta(commands.ActionSelectRange, 1)
		case ui.KeyUp:
			return withDelta(commands.ActionSelectRange, -1)
		}
	}
	return TreeBinding{}
}

// VimTreeBindings maps Vim navigation and file actions to the same semantic
// action IDs as desktop bindings. Sequence state remains in workbench/keybindings.
type VimTreeBindings struct{}

func (VimTreeBindings) Sequences() []keybindings.KeyBinding {
	return []keybindings.KeyBinding{{
		Sequence: []keybindings.KeyStroke{{Key: "G"}, {Key: "G"}},
		ActionID: commands.ActionNavigate,
		Values:   map[string]any{"index": 0},
	}}
}

func (VimTreeBindings) Resolve(mods ui.Modifiers, key ui.Key) TreeBinding {
	if mods == ui.Ctrl|ui.Shift && key == ui.KeyI {
		return handled(commands.ActionInvertSelection)
	}
	if mods == ui.Shift {
		switch key {
		case ui.KeySlash:
			return handled(commands.ActionShowHelp)
		case ui.KeyG:
			return withIndex(commands.ActionNavigate, -1)
		case ui.KeyA:
			return handled(commands.ActionCreateFolder)
		case ui.KeyJ:
			return withDelta(commands.ActionSelectRange, 1)
		case ui.KeyK:
			return withDelta(commands.ActionSelectRange, -1)
		case ui.KeyC:
			return handled(commands.ActionCloseNode)
		case ui.KeyY:
			return TreeBinding{Action: commands.ActionCopyPath, Args: TreeBindingArgs{RelativePath: true}, Handled: true}
		}
	}
	if mods != 0 {
		return TreeBinding{}
	}
	switch key {
	case ui.KeyJ:
		return withDelta(commands.ActionNavigate, 1)
	case ui.KeyK:
		return withDelta(commands.ActionNavigate, -1)
	case ui.KeyH:
		return withDirection(commands.ActionToggleExpanded, -1)
	case ui.KeyL:
		return withDirection(commands.ActionToggleExpanded, 1)
	case ui.KeyR:
		return handled(commands.ActionRename)
	case ui.KeyY:
		return handled(commands.ActionCopy)
	case ui.KeyX:
		return handled(commands.ActionCut)
	case ui.KeyP:
		return handled(commands.ActionPaste)
	case ui.KeyD:
		return handled(commands.ActionTrash)
	case ui.KeyA:
		return handled(commands.ActionCreateFile)
	case ui.KeyZ:
		return handled(commands.ActionCollapseAll)
	case ui.KeyTab:
		return handled(commands.ActionToggleSelection)
	}
	return TreeBinding{}
}

func handled(action commands.ActionID) TreeBinding {
	return TreeBinding{Action: action, Handled: true}
}

func withDelta(action commands.ActionID, delta int) TreeBinding {
	return TreeBinding{Action: action, Args: TreeBindingArgs{Delta: delta}, Handled: true}
}

func withDirection(action commands.ActionID, direction int) TreeBinding {
	return TreeBinding{Action: action, Args: TreeBindingArgs{Direction: direction}, Handled: true}
}

func withIndex(action commands.ActionID, index int) TreeBinding {
	return TreeBinding{Action: action, Args: TreeBindingArgs{Index: index, HasIndex: true}, Handled: true}
}
