package dragdrop

import (
	"testing"
)

func TestEncodeVaultItemsRejectsEmptyAndRelativePaths(t *testing.T) {
	if _, err := EncodeVaultItems(nil); err == nil {
		t.Fatal("empty payload should be rejected")
	}
	if _, err := EncodeVaultItem(VaultItem{Path: "notes/a.md"}); err == nil {
		t.Fatal("relative path should be rejected")
	}
}
