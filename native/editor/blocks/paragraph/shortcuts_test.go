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

func TestPlanBlockShortcutEligibilityAndReplacement(t *testing.T) {
	tests := []struct {
		name string
		in   BlockShortcutInput
		want BlockShortcutPlan
		ok   bool
	}{
		{
			name: "typed fence drops trailing spaces",
			in:   BlockShortcutInput{Source: "``   \nnext", Typed: "`", LineStart: 0, ContentStart: 0, Caret: 2, LineEnd: 5, TextUnit: true},
			want: BlockShortcutPlan{Start: 0, End: 5, Text: "```\n\n```", Caret: 3}, ok: true,
		},
		{
			name: "container marker requires item context",
			in:   BlockShortcutInput{Source: "- ``", Typed: "`", LineStart: 0, ContentStart: 2, Caret: 4, LineEnd: 4, ContainerPrefix: "- ", TextUnit: true, OpeningListItem: true},
			want: BlockShortcutPlan{Start: 2, End: 4, Text: "```\n- \n- ```", Caret: 5}, ok: true,
		},
		{
			name: "nonblank suffix blocks shortcut",
			in:   BlockShortcutInput{Source: "``` text", Typed: "`", LineStart: 0, ContentStart: 0, Caret: 3, LineEnd: 8, TextUnit: true},
		},
		{
			name: "non-text unit blocks shortcut",
			in:   BlockShortcutInput{Source: "``", Typed: "`", LineStart: 0, ContentStart: 0, Caret: 2, LineEnd: 2},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, ok := PlanBlockShortcut(test.in)
			if got != test.want || ok != test.ok {
				t.Fatalf("PlanBlockShortcut() = %+v, %v; want %+v, %v", got, ok, test.want, test.ok)
			}
		})
	}
}
