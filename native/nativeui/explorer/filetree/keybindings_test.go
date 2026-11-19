package filetree

import (
	"reflect"
	"testing"

	"github.com/egoist/mygo/ui"
	commands "muse/nativeui/commands"
)

func TestDesktopTreeBindingsUseTypedActionsAndArguments(t *testing.T) {
	cases := []struct {
		name string
		mods ui.Modifiers
		key  ui.Key
		want commands.ActionID
		args TreeBindingArgs
	}{
		{name: "down", key: ui.KeyDown, want: commands.ActionNavigate, args: TreeBindingArgs{Delta: 1}},
		{name: "shift range", mods: ui.Shift, key: ui.KeyDown, want: commands.ActionSelectRange, args: TreeBindingArgs{Delta: 1}},
		{name: "left collapse", key: ui.KeyLeft, want: commands.ActionToggleExpanded, args: TreeBindingArgs{Direction: -1}},
		{name: "enter", key: ui.KeyEnter, want: commands.ActionOpen},
		{name: "copy", mods: ui.Cmd, key: ui.KeyC, want: commands.ActionCopy},
		{name: "delete", key: ui.KeyDelete, want: commands.ActionTrash},
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
		want commands.ActionID
		args TreeBindingArgs
	}{
		{name: "j", key: ui.KeyJ, want: commands.ActionNavigate, args: TreeBindingArgs{Delta: 1}},
		{name: "shift j", mods: ui.Shift, key: ui.KeyJ, want: commands.ActionSelectRange, args: TreeBindingArgs{Delta: 1}},
		{name: "h", key: ui.KeyH, want: commands.ActionToggleExpanded, args: TreeBindingArgs{Direction: -1}},
		{name: "last row", mods: ui.Shift, key: ui.KeyG, want: commands.ActionNavigate, args: TreeBindingArgs{Index: -1, HasIndex: true}},
		{name: "copy relative path", mods: ui.Shift, key: ui.KeyY, want: commands.ActionCopyPath, args: TreeBindingArgs{RelativePath: true}},
		{name: "tab selection", key: ui.KeyTab, want: commands.ActionToggleSelection},
		{name: "invert", mods: ui.Ctrl | ui.Shift, key: ui.KeyI, want: commands.ActionInvertSelection},
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

func TestTreeBindingValuesCarryOnlySetArguments(t *testing.T) {
	got := (TreeBindingArgs{Delta: -1, Direction: 1, Index: 4, HasIndex: true, RelativePath: true, TargetPanel: commands.PanelIDFiles}).Values()
	want := map[string]any{
		"delta": -1, "direction": 1, "index": 4, "relative": true, "panel": commands.PanelIDFiles,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Values() = %#v, want %#v", got, want)
	}
}

func TestTreeKeySequencesUseWorkbenchBindingContract(t *testing.T) {
	vim := (VimTreeBindings{}).Sequences()
	if len(vim) != 1 || vim[0].ActionID != commands.ActionNavigate || len(vim[0].Sequence) != 2 || vim[0].Sequence[0].Key != "G" || vim[0].Sequence[1].Key != "G" || vim[0].Values["index"] != 0 {
		t.Fatalf("Vim tree sequence = %+v", vim)
	}
}
