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

	"muse/vaultindex"
	"muse/vaultsearch"
)

const vaultSearchIndexerArg = "--muse-internal-vault-search-indexer"

// runVaultSearchIndexerMode lets the app executable act as a short-lived
// Tantivy indexer. Native index writers can retain allocator arenas after a
// full rebuild; exiting this process returns those pages to the OS while the
// long-lived UI process opens only the committed search index.
func runVaultSearchIndexerMode() bool {
	if len(os.Args) != 3 || os.Args[1] != vaultSearchIndexerArg {
		return false
	}
	if err := rebuildVaultSearchIndex(os.Args[2]); err != nil {
		fmt.Fprintln(os.Stderr, "muse search indexer:", err)
		os.Exit(1)
	}
	return true
}

func rebuildVaultSearchIndex(root string) error {
	idx := vaultindex.New()
	if err := idx.Rebuild(root); err != nil {
		return fmt.Errorf("read vault for search index: %w", err)
	}
	search, err := openVaultSearch(root)
	if err != nil {
		return err
	}
	docs := searchSnapshotDocuments(idx.SearchSnapshot())
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	reconcileErr := search.Reconcile(ctx, docs)
	closeErr := search.Close()
	return errors.Join(reconcileErr, closeErr)
}

func reconcileVaultSearchInWorker(ctx context.Context, root string, search *vaultsearch.Index, docs []vaultsearch.Document) (*vaultsearch.Index, error) {
	// The Go test executable does not route this private app command through
	// nativeui.Run. Keep small test vaults in-process; shipped Muse binaries use
	// the short-lived indexer below.
	if strings.HasSuffix(filepath.Base(os.Args[0]), ".test") {
		return search, search.Reconcile(ctx, docs)
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
		previous, openErr := openVaultSearch(root)
		if openErr != nil {
			return nil, errors.Join(workerErr, openErr)
		}
		return previous, workerErr
	}

	next, err := openVaultSearch(root)
	if err != nil {
		return nil, err
	}
	if err := next.Reconcile(ctx, docs); err != nil {
		_ = next.Close()
		return nil, err
	}
	return next, nil
}
