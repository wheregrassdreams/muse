// Package dragpayload defines typed in-process payloads exchanged by native
// UI components.
package dragpayload

// VaultItem is a navigation-tree entry dragged into an editor surface.
type VaultItem struct {
	Path      string
	Name      string
	Directory bool
}
