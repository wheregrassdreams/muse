// Package app assembles application use cases and platform adapters. Desktop
// entry points may inject the returned ports into a presentation, while other
// entry points can use the same application runtime without creating a window.
package app

import (
	"errors"
	"path/filepath"
	"sync"

	"muse/application/agent"
	documentapp "muse/application/document"
	applicationvault "muse/application/vault"
	workspaceapp "muse/application/workspace"
	"muse/config"
	"muse/document"
	"muse/events"
	"muse/markdown/images"
	"muse/platform/cachepath"
	"muse/platform/filesystem/agentworkspace"
	"muse/platform/filesystem/documentstore"
	"muse/platform/filesystem/exportstore"
	"muse/platform/filesystem/filemetadata"
	"muse/platform/filesystem/imagecache"
	"muse/platform/filesystem/naviconstore"
	"muse/platform/filesystem/recentstore"
	"muse/platform/filesystem/vaultfiles"
	"muse/platform/filesystem/vaultfilessource"
	"muse/platform/filesystem/vaultindexsource"
	"muse/platform/filesystem/vaultsearchstore"
	filesystemwatcher "muse/platform/filesystem/watcher"
	"muse/platform/filesystem/workspacestore"
	"muse/platform/process/vaultsearchindexer"
	vaultfilecatalog "muse/vault/files"
	vaultindex "muse/vault/index"
	vaultwatch "muse/vault/watch"
	"muse/workspace"
)

// Runtime is the process-scoped set of feature ports selected by the
// composition root. Its event dispatchers are owned here and closed with it.
type Runtime struct {
	Workspace             workspace.Repository
	WorkspaceEvents       *events.Dispatcher[workspace.WorkspaceRestored]
	Documents             document.Repository
	DocumentExportWriter  documentapp.ExportWriter
	DocumentMetadata      documentapp.MetadataReader
	Recovery              document.RecoveryRepository
	History               document.HistoryRepository
	DocumentEvents        *events.Dispatcher[document.DocumentSaved]
	VaultSearchReconciler applicationvault.SearchReconciler
	NewAgentWorkspace     func(root string, current agent.CurrentDocument) (agent.WorkspacePort, error)
	NewVaultIndex         applicationvault.IndexFactory
	NewVaultCatalog       applicationvault.CatalogFactory
	FileOperations        *applicationvault.FileOperations
	RecentItems           *workspaceapp.RecentItems
	NavIcons              applicationvault.NavigationIconRepository
	ImageCacheStore       images.CacheStore
	NewWatcherBackend     func() (vaultwatch.Backend, error)
	vaultSearchWorker     *vaultsearchindexer.Reconciler

	closeOnce sync.Once
}

// NewRuntime constructs the process-wide application capabilities. The
// destructive trash operation is supplied by the selected desktop platform;
// the rest of the feature ports are platform adapters owned by this package.
func NewRuntime(trashItem func(string) error) (*Runtime, error) {
	cacheRoot, err := cachepath.Root()
	if err != nil {
		return nil, err
	}
	searchStore, err := vaultsearchstore.New(cacheRoot)
	if err != nil {
		return nil, err
	}
	navIcons, err := naviconstore.New(cacheRoot)
	if err != nil {
		return nil, err
	}
	images, err := imagecache.New(cacheRoot)
	if err != nil {
		return nil, err
	}
	documents := documentstore.New(filepath.Join(filepath.Dir(config.PreferencesPath()), "recovery"))
	documentExportWriter := exportstore.New()
	documentMetadata := filemetadata.New()
	workspaceEvents := events.New[workspace.WorkspaceRestored]()
	documentEvents := events.New[document.DocumentSaved]()
	newVaultIndex := func(root string) (applicationvault.IndexPort, error) {
		source, err := vaultindexsource.New(root)
		if err != nil {
			return nil, err
		}
		return vaultindex.New(source), nil
	}
	searchWorker := vaultsearchindexer.New(searchStore, newVaultIndex)
	return &Runtime{
		Workspace:             workspacestore.New(workspacestore.DefaultPath()),
		WorkspaceEvents:       workspaceEvents,
		Documents:             documents,
		DocumentExportWriter:  documentExportWriter,
		DocumentMetadata:      documentMetadata,
		Recovery:              documents,
		History:               documents,
		DocumentEvents:        documentEvents,
		VaultSearchReconciler: searchWorker,
		NewAgentWorkspace: func(root string, current agent.CurrentDocument) (agent.WorkspacePort, error) {
			return agentworkspace.New(root, agentworkspace.CurrentDocument(current))
		},
		NewVaultIndex: newVaultIndex,
		NewVaultCatalog: func(root string) (applicationvault.CatalogPort, error) {
			source, err := vaultfilessource.New(root)
			if err != nil {
				return nil, err
			}
			return vaultfilecatalog.New(source), nil
		},
		FileOperations:  applicationvault.NewFileOperations(vaultfiles.New(trashItem)),
		RecentItems:     workspaceapp.NewRecentItems(recentstore.New(recentstore.DefaultPath())),
		NavIcons:        navIcons,
		ImageCacheStore: images,
		NewWatcherBackend: func() (vaultwatch.Backend, error) {
			return filesystemwatcher.New()
		},
		vaultSearchWorker: searchWorker,
	}, nil
}

// RebuildVaultSearchIndex performs the child-process indexing operation.
func (r *Runtime) RebuildVaultSearchIndex(root string) error {
	if r == nil || r.vaultSearchWorker == nil {
		return errors.New("app: vault search indexer is unavailable")
	}
	return r.vaultSearchWorker.Rebuild(root)
}

// Close releases process-scoped event subscriptions and is safe to call more
// than once, including after a partially used runtime.
func (r *Runtime) Close() {
	if r == nil {
		return
	}
	r.closeOnce.Do(func() {
		if r.WorkspaceEvents != nil {
			r.WorkspaceEvents.Close()
		}
		if r.DocumentEvents != nil {
			r.DocumentEvents.Close()
		}
	})
}
