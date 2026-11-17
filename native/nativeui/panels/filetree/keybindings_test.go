package filetree

import (
	"testing"

	"github.com/egoist/mygo/ui"
	"muse/nativeui/interactions"
)

func TestDesktopTreeBindingsUseTypedActionsAndArguments(t *testing.T) {
	cases := []struct {
		name string
		mods ui.Modifiers
		key  ui.Key
		want interactions.ActionID
		args TreeBindingArgs
	}{
		{name: "down", key: ui.KeyDown, want: interactions.ActionNavigate, args: TreeBindingArgs{Delta: 1}},
		{name: "shift range", mods: ui.Shift, key: ui.KeyDown, want: interactions.ActionSelectRange, args: TreeBindingArgs{Delta: 1}},
		{name: "left collapse", key: ui.KeyLeft, want: interactions.ActionToggleExpanded, args: TreeBindingArgs{Direction: -1}},
		{name: "enter", key: ui.KeyEnter, want: interactions.ActionOpen},
		{name: "copy", mods: ui.Cmd, key: ui.KeyC, want: interactions.ActionCopy},
		{name: "delete", key: ui.KeyDelete, want: interactions.ActionTrash},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := (StandardTreeBindings{}).Resolve(tc.mods, tc.key)
			if !got.Handled || got.Action != tc.want || got.Args != tc.args {
				t.Fatalf("binding = %+v, want action %q args %+v", got, tc.want, tc.args)
			}
		})
	}
	if got := (StandardTreeBindings{}).Resolve(ui.Shift, ui.KeyQ); got.Handled {
		t.Fatalf("unbound shortcut was handled: %+v", got)
	}
}

func TestVimTreeBindingsUseTypedActionsAndArguments(t *testing.T) {
	cases := []struct {
		name string
		mods ui.Modifiers
		key  ui.Key
		want interactions.ActionID
		args TreeBindingArgs
	}{
		{name: "j", key: ui.KeyJ, want: interactions.ActionNavigate, args: TreeBindingArgs{Delta: 1}},
		{name: "shift j", mods: ui.Shift, key: ui.KeyJ, want: interactions.ActionSelectRange, args: TreeBindingArgs{Delta: 1}},
		{name: "h", key: ui.KeyH, want: interactions.ActionToggleExpanded, args: TreeBindingArgs{Direction: -1}},
		{name: "last row", mods: ui.Shift, key: ui.KeyG, want: interactions.ActionNavigate, args: TreeBindingArgs{Index: -1, HasIndex: true}},
		{name: "copy relative path", mods: ui.Shift, key: ui.KeyY, want: interactions.ActionCopyPath, args: TreeBindingArgs{RelativePath: true}},
		{name: "tab selection", key: ui.KeyTab, want: interactions.ActionToggleSelection},
		{name: "invert", mods: ui.Ctrl | ui.Shift, key: ui.KeyI, want: interactions.ActionInvertSelection},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := (VimTreeBindings{}).Resolve(tc.mods, tc.key)
			if !got.Handled || got.Action != tc.want || got.Args != tc.args {
				t.Fatalf("binding = %+v, want action %q args %+v", got, tc.want, tc.args)
			}
		})
	}
	if got := (VimTreeBindings{}).Resolve(ui.Cmd, ui.KeyJ); got.Handled {
		t.Fatalf("modified Vim motion was handled: %+v", got)
	}
}

func TestTreeKeySequencesStayInTypedInteractionBindingContract(t *testing.T) {
	standard := (StandardTreeBindings{}).Sequences()
	if len(standard) != 1 || standard[0].ActionID != interactions.ActionFocusPanel || standard[0].Values["panel"] != interactions.PanelIDFiles {
		t.Fatalf("desktop panel sequence = %+v", standard)
	}
	vim := (VimTreeBindings{}).Sequences()
	if len(vim) != 1 || vim[0].ActionID != interactions.ActionNavigate || len(vim[0].Sequence) != 2 || vim[0].Sequence[0].Key != "G" || vim[0].Sequence[1].Key != "G" || vim[0].Values["index"] != 0 {
		t.Fatalf("Vim tree sequence = %+v", vim)
	}
	panel := (VimTreeBindings{}).PanelSequences()
	if len(panel) != 6 || panel[0].ActionID != interactions.ActionFocusDirection || panel[4].ActionID != interactions.ActionFocusNextPanel || panel[5].ActionID != interactions.ActionFocusPreviousPanel {
		t.Fatalf("Vim panel sequences = %+v", panel)
	}
}
