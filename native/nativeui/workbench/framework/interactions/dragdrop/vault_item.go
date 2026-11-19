package dragdrop

// VaultItem is the typed value transferred when a vault tree item is dragged
// into another native surface or workspaceui.
type VaultItem struct {
	Path      string
	Name      string
	Directory bool
	Paths     []string `json:"-"`
}
