package paragraph

import "testing"

func TestBlockShortcutPreservesContainerPrefix(t *testing.T) {
	for _, tc := range []struct {
		text, prefix, want string
		caret              int
	}{
		{"```", "> ", "```\n> \n> ```", 3},
		{"~~~", "  ", "~~~\n  \n  ~~~", 3},
		{"$$", "> ", "$$\n> \n> $$", 5},
	} {
		got, caret, ok := BlockShortcut(tc.text, tc.prefix)
		if !ok || got != tc.want || caret != tc.caret {
			t.Errorf("BlockShortcut(%q, %q) = %q, %d, %v; want %q, %d, true", tc.text, tc.prefix, got, caret, ok, tc.want, tc.caret)
		}
	}
	if _, _, ok := BlockShortcut("#", ""); ok {
		t.Fatal("unexpected block shortcut")
	}
}
