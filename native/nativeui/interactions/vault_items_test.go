package interactions

import "testing"

func TestVaultItemsSnapshotsInputAndClone(t *testing.T) {
	paths := []string{"/vault/a.md", "/vault/b.md"}
	items := NewVaultItems(paths)
	paths[0] = "/mutated/input.md"
	if items.Paths[0] != "/vault/a.md" {
		t.Fatalf("constructor did not snapshot paths: %v", items.Paths)
	}

	clone := items.Clone()
	clone.Paths[1] = "/mutated/clone.md"
	if items.Paths[1] != "/vault/b.md" {
		t.Fatalf("clone shares mutable storage with source: %v", items.Paths)
	}
}

func TestVaultItemsClonePreservesNilAndEmpty(t *testing.T) {
	var nilItems VaultItems
	if got := nilItems.Clone().Paths; got != nil {
		t.Fatalf("nil payload cloned to %#v", got)
	}
	empty := NewVaultItems([]string{})
	if got := empty.Clone().Paths; got == nil || len(got) != 0 {
		t.Fatalf("empty payload clone = %#v, want non-nil empty slice", got)
	}
}
