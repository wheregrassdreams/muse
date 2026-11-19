package iconpreferences

// Repository stores per-vault presentation choices. Vault identity and item
// path semantics are supplied by the caller; the concrete adapter owns cache
// layout and persistence.
type NavigationIconRepository interface {
	LoadVault(vaultRoot string) (map[string]string, error)
	SaveVault(vaultRoot string, icons map[string]string) error
}
