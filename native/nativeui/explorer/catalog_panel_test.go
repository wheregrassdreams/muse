package explorer

import "testing"

func TestRootLabelUsesVaultNameAndPreservesDotRoot(t *testing.T) {
	for _, test := range []struct{ root, want string }{
		{root: "/notes/work", want: "work"},
		{root: ".", want: "."},
	} {
		if got := RootLabel(test.root); got != test.want {
			t.Errorf("RootLabel(%q) = %q, want %q", test.root, got, test.want)
		}
	}
}
