// Package vaultsearchindexer keeps full Tantivy reconciliation in a short-lived
// Muse process so its native allocator pages are returned to the OS afterward.
package vaultsearchindexer

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"time"

	applicationvault "muse/application/vault"
	"muse/search"
)

const WorkerArgument = "--muse-internal-vault-search-indexer"

// Reconciler implements the application-owned search bootstrap port. Incremental
// updates remain in the long-lived Session; only a full corpus reconciliation
// crosses the process boundary.
type Reconciler struct {
	store     applicationvault.SearchIndexStore
	newIndex  applicationvault.IndexFactory
	runWorker func(context.Context, string) error
}

func New(store applicationvault.SearchIndexStore, newIndex applicationvault.IndexFactory) *Reconciler {
	r := &Reconciler{store: store, newIndex: newIndex}
	r.runWorker = r.spawnWorker
	return r
}

func (r *Reconciler) OpenAndReconcile(ctx context.Context, root string, corpus search.Corpus) (*search.Index, error) {
	if r == nil || r.store == nil {
		return nil, errors.New("vaultsearchindexer: search store is required")
	}
	if corpus == nil {
		return nil, errors.New("vaultsearchindexer: search corpus is required")
	}
	current, err := r.store.Open(root)
	if err != nil {
		return nil, err
	}
	return r.reconcile(ctx, root, current, corpus)
}

func (r *Reconciler) Reconcile(ctx context.Context, root string, current *search.Index, corpus search.Corpus) (*search.Index, error) {
	if current == nil {
		return r.OpenAndReconcile(ctx, root, corpus)
	}
	if r == nil || r.store == nil {
		return current, errors.New("vaultsearchindexer: search store is required")
	}
	if corpus == nil {
		return current, errors.New("vaultsearchindexer: search corpus is required")
	}
	return r.reconcile(ctx, root, current, corpus)
}

func (r *Reconciler) reconcile(ctx context.Context, root string, current *search.Index, corpus search.Corpus) (*search.Index, error) {
	needed, err := current.NeedsReconcile(corpus.SearchDocuments())
	if err != nil {
		return current, err
	}
	if !needed {
		return current, nil
	}
	if err := current.Close(); err != nil {
		return nil, fmt.Errorf("close search index before rebuild: %w", err)
	}
	if r.runWorker == nil {
		return nil, errors.New("vaultsearchindexer: worker runner is required")
	}
	if err := r.runWorker(ctx, root); err != nil {
		workerErr := fmt.Errorf("rebuild search index in worker: %w", err)
		previous, openErr := r.store.Open(root)
		if openErr != nil {
			return nil, errors.Join(workerErr, fmt.Errorf("reopen prior search index: %w", openErr))
		}
		return previous, workerErr
	}
	next, err := r.store.Open(root)
	if err != nil {
		return nil, fmt.Errorf("reopen rebuilt search index: %w", err)
	}
	return next, nil
}

func (r *Reconciler) spawnWorker(ctx context.Context, root string) error {
	executable, err := os.Executable()
	if err != nil {
		return fmt.Errorf("locate Muse executable for search indexing: %w", err)
	}
	cmd := exec.CommandContext(ctx, executable, WorkerArgument, root)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("%w: %s", err, output)
	}
	return nil
}

// Rebuild is the child-process operation: build a fresh vault corpus, reconcile
// its durable search index, and release the native handle before process exit.
func (r *Reconciler) Rebuild(root string) error {
	if r == nil || r.store == nil || r.newIndex == nil {
		return errors.New("vaultsearchindexer: store and index factory are required")
	}
	index, err := r.newIndex(root)
	if err != nil {
		return fmt.Errorf("create vault index: %w", err)
	}
	if err := index.Rebuild(); err != nil {
		return fmt.Errorf("read vault for search index: %w", err)
	}
	searchIndex, err := r.store.Open(root)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	reconcileErr := searchIndex.ReconcileCorpus(ctx, index)
	closeErr := searchIndex.Close()
	return errors.Join(reconcileErr, closeErr)
}
