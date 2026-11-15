package vault

import (
	"context"

	"muse/search"
)

// SearchIndexStore opens the persistent query index for one normalized vault
// root. The filesystem implementation and its cache directory are selected
// by the composition root.
type SearchIndexStore interface {
	Open(root string) (*search.Index, error)
}

// SearchReconciler owns the boundary for opening and refreshing a durable
// search index. Production composition runs full corpus reconciliation in a
// short-lived worker process; application tests can inject an in-process fake.
// Reconcile may replace and close current. On failure it should return a usable
// reopened prior index when possible, together with the error.
type SearchReconciler interface {
	OpenAndReconcile(ctx context.Context, root string, corpus search.Corpus) (*search.Index, error)
	Reconcile(ctx context.Context, root string, current *search.Index, corpus search.Corpus) (*search.Index, error)
}

// NavigationIconRepository stores per-vault presentation choices. Vault
// identity and item path semantics are supplied by the caller; the concrete
// adapter owns cache layout and persistence.
type NavigationIconRepository interface {
	LoadVault(vaultRoot string) (map[string]string, error)
	SaveVault(vaultRoot string, icons map[string]string) error
}
