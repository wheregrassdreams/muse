package nativeui

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	documentapp "muse/application/document"
	applicationvault "muse/application/vault"
	"muse/document"
	doctypes "muse/document/types"
	editorsource "muse/editor/source"
	"muse/locales"
	"muse/nativeui/workbench/features/backlinks"
	"muse/nativeui/workbench/features/dialogs/document"
	searchui "muse/nativeui/workbench/features/search"
	"muse/nativeui/workbench/hosting"
	vaultsearch "muse/search"
)

type vaultNavigationResult struct {
	hit vaultsearch.Hit
	err error
}

func (a *App) activeVaultRoot() string {
	if a.vault != nil && a.vaultSnapshot.Root != "" {
		return a.vaultSnapshot.Root
	}
	return a.root
}

func (a *App) vaultTick(c *ui.Context) {
	root := a.root
	if root == "" && a.documentPath() != "" {
		root = filepath.Dir(a.documentPath())
	}
	if a.noteEditor() == nil || root == "" || a.newVaultIndex == nil || a.newVaultCatalog == nil {
		a.clearWikiSuggestionCache()
		if a.vault != nil {
			old := a.vault
			a.vault = nil
			a.vaultSnapshot = applicationvault.Snapshot{}
			a.directoryCatalog.CloseSession(old)
			go old.Close()
		}
		return
	}
	if a.vault == nil || filepath.Clean(a.vaultSnapshot.Root) != filepath.Clean(root) {
		a.clearWikiSuggestionCache()
		if a.vault != nil {
			old := a.vault
			a.vault = nil
			a.directoryCatalog.CloseSession(old)
			go old.Close()
		}
		newWatcher := a.newWatcher
		session, err := applicationvault.NewSession(root, applicationvault.SessionDependencies{
			NewIndex: a.newVaultIndex, NewCatalog: a.newVaultCatalog,
			NewWatcher: newWatcher, SearchReconciler: a.vaultSearchReconciler,
		})
		if err != nil {
			a.notice(locales.VaultIndexWatchError(err.Error()))
			return
		}
		a.vault = session
		a.vaultSnapshot = session.Snapshot()
		a.vaultResultNavigation = make(chan vaultNavigationResult, 1)
		a.invalidate()
	}
	if a.vault == nil {
		return
	}
	c.After(100 * time.Millisecond)
	for n := 0; n < 32; n++ {
		select {
		case result, ok := <-a.vault.Results():
			if !ok {
				return
			}
			a.handleVaultResult(a.vault, result)
		default:
			n = 32
		}
	}
	if a.vaultResultNavigation != nil {
		select {
		case result := <-a.vaultResultNavigation:
			if result.err != nil {
				a.notice(locales.WorkspaceSearchResultUpdated(result.err.Error()))
			} else {
				a.acceptSearchHit(result.hit)
			}
		default:
		}
	}
	a.linkIndexTick(c)
}

func (a *App) handleVaultResult(session *applicationvault.Session, result applicationvault.Result) {
	if session == nil || a.vault != session {
		return
	}
	a.vaultSnapshot = result.Snapshot
	switch result.Kind {
	case applicationvault.SessionReady:
		// setWorkspace can be called before vaultTick creates the session. In
		// that interval children(root) caches an empty result; invalidate it as
		// soon as the initial catalog is ready so the first tree render sees it.
		a.clearTreeCache()
		if result.Err != nil {
			a.notice(locales.VaultIndexWatchError(result.Err.Error()))
		}
		if a.noteEditor() != nil {
			a.configureActiveNote(0)
		}
		if a.pendingLinks {
			a.pendingLinks = false
			a.openLinks()
		}
		if a.pendingWiki != "" {
			target := a.pendingWiki
			a.pendingWiki = ""
			a.openWiki(target)
		}
		if a.documentPath() != "" {
			_ = session.WatchFile(a.documentPath())
		}
	case applicationvault.FilesRefreshed:
		a.clearTreeCache()
		if result.Err != nil && !errors.Is(result.Err, os.ErrNotExist) {
			a.notice(locales.WorkspaceIndexRefreshFailed(result.Err.Error()))
		}
		if a.workspaceSearch.IsOpen() {
			a.vault.ForceSearch(time.Now(), 0)
		}
		if result.Full || result.TouchesPath(a.documentPath()) {
			a.reconcileExternalCurrentDocument(session)
		}
		if a.noteEditor() != nil {
			a.configureActiveNote(0)
		}
	case applicationvault.ActiveDocumentApplied:
		if a.noteEditor() != nil {
			a.configureActiveNote(0)
		}
	case applicationvault.QueryCompleted:
		if session.IsCurrentSearchResult(result) && result.Query == a.workspaceSearch.Query() && result.Literal == (a.workspaceSearch.Mode() == searchui.Literal) {
			a.vaultSnapshot = result.Snapshot
			if len(result.Snapshot.Search.Hits) > 0 {
				a.workspaceSearch.Apply(searchui.Action{Kind: searchui.MoveSelection, Index: min(a.workspaceSearch.Selection(), len(result.Snapshot.Search.Hits)-1)})
			}
			if pending := a.vaultPendingNavigation; pending != nil {
				if hit, ok := result.Snapshot.Search.SearchHitAtPath(pending.Path); ok {
					a.vaultPendingNavigation = nil
					a.acceptSearchHit(hit)
				}
			}
		}
	case applicationvault.LinksRefreshed:
		a.vaultSnapshot = result.Snapshot
	}
	a.invalidate()
}

func (a *App) reconcileExternalCurrentDocument(session *applicationvault.Session) {
	if a.documentPath() == "" || !doctypes.IsMarkdownPath(a.documentPath()) {
		return
	}
	root := a.root
	if session != nil && session.Snapshot().Root != "" {
		root = session.Snapshot().Root
	}
	if root == "" {
		return
	}
	rel, err := filepath.Rel(root, a.documentPath())
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return
	}
	// The vault index is an asynchronous catalog, not the source of truth for
	// the active file's existence or bytes. A refresh can temporarily omit a
	// document while rescanning; verify against the repository before reporting
	// a deletion or reconciling a revision.
	if a.documentRepository == nil {
		return
	}
	note, readErr := a.documentRepository.Read(a.documentPath())
	if errors.Is(readErr, os.ErrNotExist) {
		a.handleExternalDocumentMissing()
		return
	}
	if readErr != nil {
		a.notice(locales.WorkspaceIndexRefreshFailed(readErr.Error()))
		return
	}
	a.reconcileExternalNote(note)
}

func (a *App) handleExternalDocumentMissing() {
	if a.noteEditor() == nil || a.documentPath() == "" {
		return
	}
	path := a.documentPath()
	alreadyReported := a.noteEditor().PersistenceState().ConflictPath == path
	_ = a.documentWorkflows().MarkExternalMissing()
	if alreadyReported {
		return
	}
	if a.documents != nil {
		a.documents.Apply(documentdialogs.Action{Kind: documentdialogs.OpenConflict})
	}
	a.notice(locales.VaultCurrentFileDeleted())
}

func (a *App) reconcileExternalNote(fresh document.Note) {
	if a.noteEditor() == nil || fresh.Path == "" {
		return
	}
	caret := a.noteEditor().NavigationCommands().Caret()
	outcome, err := a.documentWorkflows().ReconcileExternal(fresh)
	if err != nil {
		if outcome == documentapp.ExternalChangeConflict {
			a.notice(locales.WorkspaceRecoverySaveFailed(err.Error()))
		} else {
			a.notice(locales.WorkspaceIndexRefreshFailed(err.Error()))
		}
	}
	switch outcome {
	case documentapp.ExternalChangeLoaded:
		a.noteEditor().NavigationCommands().Select(min(caret, len(a.documentSource())), min(caret, len(a.documentSource())))
		a.updateTitle()
		if a.documentDirty() {
			a.notice(locales.VaultExternalUpdatedAt())
		} else {
			a.notice(locales.DocumentExternalLoaded())
		}
	case documentapp.ExternalChangeMergedSaving:
		a.notice(locales.VaultExternalMergedSaving())
	case documentapp.ExternalChangeConflict:
		if a.documents != nil {
			a.documents.Apply(documentdialogs.Action{Kind: documentdialogs.OpenConflict})
		}
	case documentapp.ExternalChangeNoop:
	}
}

func (a *App) queueLinkIndexSnapshot() {
	if a.vault == nil || a.documentPath() == "" || !doctypes.IsMarkdownPath(a.documentPath()) {
		a.activeLinkIndex.Clear()
		return
	}
	a.activeLinkIndex.Schedule(a.documentPath(), a.diskSource(), time.Now())
}

func (a *App) linkIndexTick(c *ui.Context) {
	if a.vault == nil {
		return
	}
	ticket, remaining, ready := a.activeLinkIndex.Take(time.Now())
	if remaining > 0 {
		c.After(remaining)
		return
	}
	if !ready {
		return
	}
	session, path, source := a.vault, ticket.Path, ticket.Source
	go func() {
		if err := session.ApplyActiveDocument(path, source); err != nil {
			return
		}
		a.onMain(func() {
			if a.vault != session || a.documentPath() != path || a.diskSource() != source || !a.activeLinkIndex.IsCurrent(ticket) {
				return
			}
			a.vaultSnapshot = session.Snapshot()
			if a.noteEditor() != nil {
				a.configureActiveNote(0)
			}
			a.invalidate()
		})
	}()
}

func (a *App) syncWorkspaceSearchQuery() time.Duration {
	if a.vault == nil || !a.vaultSnapshot.Ready {
		return 0
	}
	query := a.workspaceSearch.Query()
	delay, err := a.vault.ScheduleSearch(query, a.workspaceSearch.Mode() == searchui.Literal, a.documentPath(), a.diskSource(), time.Now())
	if err != nil && !errors.Is(err, applicationvault.ErrSessionNotReady) {
		a.notice(locales.WorkspaceSearchFailed(err.Error()))
	}
	a.vaultSnapshot = a.vault.Snapshot()
	return delay
}

func (a *App) navigateIndexed(path string, offset int) {
	root := a.activeVaultRoot()
	if root == "" {
		return
	}
	full := filepath.Join(root, filepath.FromSlash(path))
	if filepath.Clean(full) == filepath.Clean(a.documentPath()) {
		a.noteEditor().NavigationCommands().ScrollToOffset(editorsource.OffsetInNormalizedSource(a.diskSource(), offset))
		return
	}
	a.indexedPath, a.indexedOffset = full, offset
	a.requestOpen(full)
}

func (a *App) navigateSearchHit(hit vaultsearch.Hit) {
	if a.vault == nil || a.noteEditor() == nil || !a.vaultSnapshot.SearchAvailable {
		return
	}
	query, literal := a.workspaceSearch.Query(), a.workspaceSearch.Mode() == searchui.Literal
	currentPath, currentSource := a.documentPath(), a.diskSource()
	session := a.vault
	go func() {
		status, err := session.ValidateSearchNavigation(a.documentRepository, hit, currentPath, document.RevisionOf(currentSource))
		if err == nil && status == applicationvault.SearchHitCurrent {
			a.postVaultNavigation(session, vaultNavigationResult{hit: hit})
			return
		}
		if err != nil && errors.Is(err, os.ErrNotExist) {
			a.postVaultNavigation(session, vaultNavigationResult{err: err})
			return
		}
		if err != nil {
			a.postVaultNavigation(session, vaultNavigationResult{err: err})
			return
		}
		pending := hit
		a.onMain(func() {
			if a.vault != session {
				return
			}
			a.vaultPendingNavigation = &pending
			a.workspaceSearch.Apply(searchui.Action{Kind: searchui.QueryChanged, Query: query})
			if literal {
				a.workspaceSearch.Apply(searchui.Action{Kind: searchui.ModeChanged, Mode: searchui.Literal})
			} else {
				a.workspaceSearch.Apply(searchui.Action{Kind: searchui.ModeChanged, Mode: searchui.FullText})
			}
			session.ForceSearch(time.Now(), 120*time.Millisecond)
			a.invalidate()
		})
	}()
}

func (a *App) postVaultNavigation(session *applicationvault.Session, result vaultNavigationResult) {
	a.onMain(func() {
		if a.vault != session || a.vaultResultNavigation == nil {
			return
		}
		select {
		case a.vaultResultNavigation <- result:
		default:
			select {
			case <-a.vaultResultNavigation:
			default:
			}
			select {
			case a.vaultResultNavigation <- result:
			default:
			}
		}
		a.invalidate()
	})
}

func (a *App) acceptSearchHit(hit vaultsearch.Hit) {
	full := filepath.Join(a.activeVaultRoot(), filepath.FromSlash(hit.Path))
	if filepath.Clean(full) == filepath.Clean(a.documentPath()) && document.RevisionOf(a.diskSource()) == document.Revision(hit.Revision) {
		a.noteEditor().NavigationCommands().NavigateToRange(
			editorsource.OffsetInNormalizedSource(a.diskSource(), hit.Start),
			editorsource.OffsetInNormalizedSource(a.diskSource(), hit.End),
		)
		return
	}
	a.indexedPath, a.indexedOffset, a.indexedRevision = full, hit.Start, hit.Revision
	a.requestOpen(full)
}

func (a *App) indexedLink(target string) bool {
	if a.vault == nil {
		return false
	}
	root := a.vaultSnapshot.Root
	current, err := filepath.Rel(root, a.documentPath())
	if err != nil {
		return false
	}
	if location, ok := a.vault.ResolveLink(filepath.ToSlash(current), target); ok {
		a.navigateIndexed(location.Path, location.Offset)
		return true
	}
	return false
}

func (a *App) openLinks() {
	a.enableDocumentServices()
	session := a.vault
	if session == nil || !a.vaultSnapshot.Ready {
		a.pendingLinks = true
		return
	}
	if a.windows == nil {
		return
	}
	if a.windows.Contains(a.backlinksHandle) {
		_ = a.windows.Focus(a.backlinksHandle)
		return
	}
	a.backlinksHandle = window.Handle{}
	path := a.documentPath()
	if a.imageDocumentOpen() && a.imageDocumentPath() != "" {
		path = a.imageDocumentPath()
	}
	rel, err := filepath.Rel(a.vaultSnapshot.Root, path)
	if err != nil {
		return
	}
	if err := session.OpenLinks(filepath.ToSlash(rel)); err != nil {
		a.notice(locales.VaultIndexBuildingShort())
		return
	}
	rel = filepath.ToSlash(rel)
	content := backlinks.NewContent(backlinks.Ports{
		Data: func() backlinks.Model {
			if a.vault != session {
				return backlinks.Model{}
			}
			snapshot := session.Snapshot()
			return backlinks.Model{
				Ready:    snapshot.Ready,
				Busy:     snapshot.LinksBusy,
				Incoming: session.BacklinkSources(rel),
				Outgoing: snapshot.Outgoing,
			}
		},
		OpenSource: func(path string, offset int) {
			if a.vault == session {
				a.navigateIndexed(path, offset)
			}
		},
		OpenTarget: func(target string) {
			if a.vault == session {
				a.indexedLink(target)
			}
		},
		OnClose: func() {
			_ = session.CloseLinks()
			if a.vault == session {
				a.vaultSnapshot = session.Snapshot()
			}
			a.backlinksHandle = window.Handle{}
			a.invalidate()
		},
	})
	handle, err := a.windows.Open(content, window.Placement{Host: "main", Layer: window.ModalOverlay})
	if err != nil {
		_ = session.CloseLinks()
		return
	}
	a.vaultSnapshot = session.Snapshot()
	a.backlinksHandle = handle
	a.invalidate()
}

func (a *App) closeLinks() {
	if a.windows != nil && a.windows.Contains(a.backlinksHandle) {
		_ = a.windows.Close(a.backlinksHandle)
		return
	}
	if a.vault != nil {
		_ = a.vault.CloseLinks()
		a.vaultSnapshot = a.vault.Snapshot()
	}
}

func (a *App) closeVaultSessionNow() {
	if a == nil {
		return
	}
	a.clearWikiSuggestionCache()
	if a.vault == nil {
		return
	}
	session := a.vault
	a.vault = nil
	a.vaultSnapshot = applicationvault.Snapshot{}
	a.directoryCatalog.CloseSession(session)
	_ = session.Close()
}
