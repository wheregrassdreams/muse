package nativeui

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	applicationvault "muse/application/vault"
	vaultsearch "muse/search"
)

const vaultSearchIndexerArg = "--muse-internal-vault-search-indexer"

// runVaultSearchIndexerMode lets the app executable act as a short-lived
// Tantivy indexer. Native index writers can retain allocator arenas after a
// full rebuild; exiting this process returns those pages to the OS while the
// long-lived UI process opens only the committed search index.
func runVaultSearchIndexerMode(store applicationvault.SearchIndexStore, newIndex applicationvault.IndexFactory) bool {
	if len(os.Args) != 3 || os.Args[1] != vaultSearchIndexerArg {
		return false
	}
	if err := rebuildVaultSearchIndex(os.Args[2], store, newIndex); err != nil {
		fmt.Fprintln(os.Stderr, "muse search indexer:", err)
		os.Exit(1)
	}
	return true
}

func rebuildVaultSearchIndex(root string, store applicationvault.SearchIndexStore, newIndex applicationvault.IndexFactory) error {
	if newIndex == nil {
		return errors.New("vault index factory is not configured")
	}
	idx, err := newIndex(root)
	if err != nil {
		return fmt.Errorf("prepare vault index: %w", err)
	}
	if err := idx.Rebuild(); err != nil {
		return fmt.Errorf("read vault for search index: %w", err)
	}
	if store == nil {
		return errors.New("vault search index store is not configured")
	}
	search, err := store.Open(root)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	reconcileErr := search.ReconcileCorpus(ctx, idx)
	closeErr := search.Close()
	return errors.Join(reconcileErr, closeErr)
}

func reconcileVaultSearchInWorker(ctx context.Context, root string, search *vaultsearch.Index, docs []vaultsearch.Document, store applicationvault.SearchIndexStore) (*vaultsearch.Index, error) {
	// The Go test executable does not route this private app command through
	// nativeui.Run. Keep small test vaults in-process; shipped Muse binaries use
	// the short-lived indexer below.
	if strings.HasSuffix(filepath.Base(os.Args[0]), ".test") {
		return search, search.Reconcile(ctx, docs)
	}
	if store == nil {
		return search, errors.New("vault search index store is not configured")
	}
	needed, err := search.NeedsReconcile(docs)
	if err != nil {
		return search, err
	}
	if !needed {
		return search, search.Reconcile(ctx, docs)
	}
	if err := search.Close(); err != nil {
		return nil, err
	}

	executable, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("locate Muse executable for search indexing: %w", err)
	}
	cmd := exec.CommandContext(ctx, executable, vaultSearchIndexerArg, root)
	if output, err := cmd.CombinedOutput(); err != nil {
		workerErr := fmt.Errorf("rebuild search index in worker: %w: %s", err, output)
		// Reopen the previous durable index so the rest of the workspace remains
		// usable even if the worker was canceled or indexing failed.
		previous, openErr := store.Open(root)
		if openErr != nil {
			return nil, errors.Join(workerErr, openErr)
		}
		return previous, workerErr
	}

	next, err := store.Open(root)
	if err != nil {
		return nil, err
	}
	if err := next.Reconcile(ctx, docs); err != nil {
		_ = next.Close()
		return nil, err
	}
	return next, nil
}
