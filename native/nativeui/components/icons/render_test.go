package icons

import (
	"testing"

	iconlibrary "muse/ui/iconlibrary"
)

func TestEveryBundledIconParses(t *testing.T) {
	for _, icon := range iconlibrary.Catalog() {
		if _, ok := Lookup(icon.ID); !ok {
			t.Errorf("icon %q has no renderable SVG", icon.ID)
		}
	}
}
