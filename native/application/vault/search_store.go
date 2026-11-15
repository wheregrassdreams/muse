package vault

import "muse/search"

// SearchIndexStore opens the persistent query index for one normalized vault
// root. The filesystem implementation and its cache directory are selected
// by the composition root.
type SearchIndexStore interface {
	Open(root string) (*search.Index, error)
}

// NavigationIconRepository stores per-vault presentation choices. Vault
// identity and item path semantics are supplied by the caller; the concrete
// adapter owns cache layout and persistence.
type NavigationIconRepository interface {
	LoadVault(vaultRoot string) (map[string]string, error)
	SaveVault(vaultRoot string, icons map[string]string) error
}
