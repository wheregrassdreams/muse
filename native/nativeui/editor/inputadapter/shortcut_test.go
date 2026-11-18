package inputadapter

import (
	"testing"

	"muse/editor/input"
)

func TestShortcutMapsPlatformBindings(t *testing.T) {
	tests := []struct {
		name  string
		key   input.Key
		shift bool
		alt   bool
		isMac bool
		want  ShortcutCommand
	}{
		{name: "command undo", key: input.KeyZ, want: ShortcutUndo},
		{name: "shift command z redo", key: input.KeyZ, shift: true, want: ShortcutRedo},
		{name: "windows ctrl y redo", key: input.KeyY, want: ShortcutRedo},
		{name: "mac command y is not redo", key: input.KeyY, isMac: true, want: ShortcutNone},
		{name: "alt task", key: input.KeyX, alt: true, want: ShortcutToggleTask},
		{name: "shift command enter inserts row", key: input.KeyEnter, want: ShortcutInsertTableRow},
		{name: "unsupported key", key: input.KeyF, want: ShortcutNone},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Shortcut(tt.key, tt.shift, tt.alt, tt.isMac); got != tt.want {
				t.Fatalf("Shortcut() = %v, want %v", got, tt.want)
			}
		})
	}
}
