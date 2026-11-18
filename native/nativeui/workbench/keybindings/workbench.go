package keybindings

import "muse/nativeui/workbench/commands"

// Modifier bits used by platform-neutral bindings. Primary means the host's
// standard command modifier (Cmd on macOS, Ctrl elsewhere); the UI adapter
// maps toolkit modifier values to and from these stable bits.
const (
	ModShift uint32 = 1 << iota
	ModCtrl
	ModAlt
	ModSuper
	ModPrimary
)

// StandardPanelBindings returns the desktop shortcut that focuses the file
// panel. It is a workbench binding, independent from Explorer's local keys.
func StandardPanelBindings() []KeyBinding {
	return []KeyBinding{{
		Sequence: []KeyStroke{{Key: "E", Mods: ModPrimary | ModShift}},
		ActionID: commands.ActionFocusPanel,
		Values:   map[string]any{"panel": commands.PanelIDFiles},
	}}
}

// VimPanelBindings returns the Vim window-navigation sequences. Focus
// movement is shared workbench behavior; Explorer and Editor remain the
// focused-panel endpoints selected by the host.
func VimPanelBindings() []KeyBinding {
	return []KeyBinding{
		{Sequence: []KeyStroke{{Key: "W", Mods: ModCtrl}, {Key: "H"}}, ActionID: commands.ActionFocusDirection, Values: map[string]any{"direction": -1}},
		{Sequence: []KeyStroke{{Key: "W", Mods: ModCtrl}, {Key: "Left"}}, ActionID: commands.ActionFocusDirection, Values: map[string]any{"direction": -1}},
		{Sequence: []KeyStroke{{Key: "W", Mods: ModCtrl}, {Key: "L"}}, ActionID: commands.ActionFocusDirection, Values: map[string]any{"direction": 1}},
		{Sequence: []KeyStroke{{Key: "W", Mods: ModCtrl}, {Key: "Right"}}, ActionID: commands.ActionFocusDirection, Values: map[string]any{"direction": 1}},
		{Sequence: []KeyStroke{{Key: "W", Mods: ModCtrl}, {Key: "W"}}, ActionID: commands.ActionFocusNextPanel},
		{Sequence: []KeyStroke{{Key: "W", Mods: ModCtrl}, {Key: "W", Mods: ModShift}}, ActionID: commands.ActionFocusPreviousPanel},
	}
}
