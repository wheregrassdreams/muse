package interactions

// VaultItems is the immutable-by-convention payload for dragging vault paths
// between panels. Producers and consumers should use Clone when retaining or
// handing off the value so the slice backing array is not shared mutably.
type VaultItems struct {
	Paths []string
}

// NewVaultItems snapshots paths for a drag operation.
func NewVaultItems(paths []string) VaultItems {
	if paths == nil {
		return VaultItems{}
	}
	snapshot := make([]string, len(paths))
	copy(snapshot, paths)
	return VaultItems{Paths: snapshot}
}

// Clone returns an independent snapshot of the payload.
func (items VaultItems) Clone() VaultItems {
	return NewVaultItems(items.Paths)
}
