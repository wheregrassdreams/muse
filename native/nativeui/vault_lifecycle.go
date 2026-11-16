package nativeui

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	documentsave "muse/application/document"
	applicationvault "muse/application/vault"
	"muse/document"
	"muse/locales"
	"muse/nativeui/components"
	vaultsearch "muse/search"
	vaultlinks "muse/vault/links"
	vaultwatch "muse/vault/watch"
)

type vaultNavigationResult struct {
	hit vaultsearch.Hit
	err error
}

// Search and link offsets address on-disk bytes; the editor normalizes CRLF.
func editorOffset(raw string, offset int) int {
	offset = max(0, min(offset, len(raw)))
	return offset - strings.Count(raw[:offset], "\r\n")
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
	if a.activeNote == nil || root == "" || a.newVaultIndex == nil || a.newVaultCatalog == nil {
		if a.vault != nil {
			old := a.vault
			a.vault = nil
			a.vaultSnapshot = applicationvault.Snapshot{}
			go old.Close()
		}
		return
	}
	if a.vault == nil || filepath.Clean(a.vaultSnapshot.Root) != filepath.Clean(root) {
		if a.vault != nil {
			old := a.vault
			a.vault = nil
			go old.Close()
		}
		newWatcher := func(root string) (applicationvault.WatcherPort, error) {
			if a.watcherBackendFactory == nil {
				return nil, errors.New("filesystem watcher adapter is not configured")
			}
			backend, err := a.watcherBackendFactory()
			if err != nil {
				return nil, err
			}
			return vaultwatchNew(root, backend)
		}
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
		a.vaultSearchSubmitted = "\x00"
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

func vaultwatchNew(root string, backend vaultwatch.Backend) (applicationvault.WatcherPort, error) {
	return vaultwatch.New(root, backend)
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
		if a.activeNote != nil {
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
		if a.workspaceSearch {
			a.vaultSearchDue = time.Now()
		}
		if result.Full || vaultResultTouches(result, a.documentPath()) {
			a.reconcileExternalCurrentDocument(session)
		}
		if a.activeNote != nil {
			a.configureActiveNote(0)
		}
	case applicationvault.ActiveDocumentApplied:
		if a.activeNote != nil {
			a.configureActiveNote(0)
		}
	case applicationvault.QueryCompleted:
		if result.Token == a.vaultSnapshot.Search.Token && result.Query == a.workspaceQuery && result.Literal == a.vaultSearchLiteral {
			a.vaultSnapshot = result.Snapshot
			if len(result.Snapshot.Search.Hits) > 0 {
				a.workspaceSearchSel = min(a.workspaceSearchSel, len(result.Snapshot.Search.Hits)-1)
				a.workspaceReveal = true
			}
			if pending := a.vaultPendingNavigation; pending != nil {
				for _, hit := range result.Snapshot.Search.Hits {
					if hit.Path == pending.Path {
						a.vaultPendingNavigation = nil
						a.acceptSearchHit(hit)
						break
					}
				}
			}
		}
	case applicationvault.LinksRefreshed:
		a.vaultSnapshot = result.Snapshot
	}
	a.invalidate()
}

func (a *App) reconcileExternalCurrentDocument(session *applicationvault.Session) {
	if session == nil || a.documentPath() == "" || filepath.Ext(a.documentPath()) != ".md" {
		return
	}
	root := session.Snapshot().Root
	rel, err := filepath.Rel(root, a.documentPath())
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return
	}
	state := a.activeNote.PersistenceState()
	doc, ok := session.Document(filepath.ToSlash(rel))
	if !ok {
		a.activeNote.SetConflict(a.documentPath())
		if a.documents != nil {
			a.documents.ConflictOpen = true
		}
		a.notice(locales.VaultCurrentFileDeleted())
		return
	}
	if doc.Revision == state.Revision {
		return
	}
	fresh := document.NewNote(a.documentPath(), doc.Source, doc.Revision)
	if a.documentDirty() || state.Saving {
		if fresh.Source == a.diskSource() {
			return
		}
		merged, ok := "", false
		if !state.Saving {
			merged, ok = documentsave.MergeDisjointEdits(state.BaseSource, a.diskSource(), fresh.Source)
		}
		if ok {
			a.activeNote.Rebase(a.documentPath(), fresh.Revision, fresh.Source)
			_ = a.activeNote.ReplaceUser(strings.ReplaceAll(merged, "\r\n", "\n"))
			if a.documentDirty() {
				a.notice(locales.VaultExternalMergedSaving())
			} else {
				a.activeNote.ClearAutosave()
				a.notice(locales.DocumentExternalLoaded())
			}
			return
		}
		a.activeNote.SetConflict(a.documentPath())
		if a.documents != nil {
			a.documents.ConflictOpen = true
		}
		if _, err := a.activeNote.PreserveDraft(a.documentPath(), a.diskSource()); err != nil {
			a.notice(locales.WorkspaceRecoverySaveFailed(err.Error()))
		}
		return
	}
	caret := a.activeNote.NavigationCommands().Caret()
	a.loadDoc(fresh.Source)
	a.activeNote.NavigationCommands().Select(min(caret, len(a.documentSource())), min(caret, len(a.documentSource())))
	a.updateManagedTimestamp()
	a.updateTitle()
	if a.documentDirty() {
		a.notice(locales.VaultExternalUpdatedAt())
	} else {
		a.notice(locales.DocumentExternalLoaded())
	}
}

func vaultResultTouches(result applicationvault.Result, absolutePath string) bool {
	if absolutePath == "" {
		return false
	}
	for _, path := range result.Changed {
		if filepath.Clean(path) == filepath.Clean(absolutePath) {
			return true
		}
	}
	return false
}

func (a *App) queueLinkIndexSnapshot() {
	if a.vault == nil || a.documentPath() == "" || codeLanguage(a.documentPath()) != "" {
		return
	}
	a.linkIndexGeneration++
	a.linkIndexDue = time.Now().Add(120 * time.Millisecond)
}

func (a *App) linkIndexTick(c *ui.Context) {
	if a.vault == nil || a.linkIndexDue.IsZero() {
		return
	}
	if remaining := time.Until(a.linkIndexDue); remaining > 0 {
		c.After(remaining)
		return
	}
	a.linkIndexDue = time.Time{}
	session, path, source, generation := a.vault, a.documentPath(), a.diskSource(), a.linkIndexGeneration
	go func() {
		if err := session.ApplyActiveDocument(path, source); err != nil {
			return
		}
		a.onMain(func() {
			if a.vault != session || a.documentPath() != path || a.diskSource() != source || a.linkIndexGeneration != generation {
				return
			}
			a.vaultSnapshot = session.Snapshot()
			if a.activeNote != nil {
				a.configureActiveNote(0)
			}
			a.invalidate()
		})
	}()
}

func (a *App) syncWorkspaceSearchQuery() {
	if a.vault == nil || !a.vaultSnapshot.Ready {
		return
	}
	if a.vaultSearchSubmitted == a.workspaceQuery && a.vaultSearchSubmittedLiteral == a.vaultSearchLiteral {
		return
	}
	if a.vaultSearchDue.IsZero() {
		a.workspaceSearchSel = 0
		a.workspaceScroll = ui.ScrollState{}
		a.workspaceSearchResults = ui.ListState{}
		a.workspaceReveal = true
		a.vaultSearchDue = time.Now().Add(workspaceSearchCoalesce(a.vaultSnapshot.Search.Cost, a.vaultSnapshot.Search.Busy))
	}
	if time.Until(a.vaultSearchDue) > 0 {
		return
	}
	a.vaultSearchSubmitted, a.vaultSearchSubmittedLiteral = a.workspaceQuery, a.vaultSearchLiteral
	a.vaultSearchDue = time.Time{}
	_, err := a.vault.Query(a.workspaceQuery, a.vaultSearchLiteral, a.documentPath(), a.diskSource())
	if err != nil && !errors.Is(err, applicationvault.ErrSessionNotReady) {
		a.notice(locales.WorkspaceSearchFailed(err.Error()))
	}
	a.vaultSnapshot = a.vault.Snapshot()
}

func (a *App) navigateIndexed(path string, offset int) {
	root := a.activeVaultRoot()
	if root == "" {
		return
	}
	full := filepath.Join(root, filepath.FromSlash(path))
	if filepath.Clean(full) == filepath.Clean(a.documentPath()) {
		a.activeNote.NavigationCommands().ScrollToOffset(editorOffset(a.diskSource(), offset))
		return
	}
	a.indexedPath, a.indexedOffset = full, offset
	a.requestOpen(full)
}

func (a *App) navigateSearchHit(hit vaultsearch.Hit) {
	if a.vault == nil || a.activeNote == nil || !a.vaultSnapshot.SearchAvailable {
		return
	}
	root := a.vaultSnapshot.Root
	full := filepath.Join(root, filepath.FromSlash(hit.Path))
	query, literal := a.workspaceQuery, a.vaultSearchLiteral
	currentPath, currentSource := a.documentPath(), a.diskSource()
	session := a.vault
	if filepath.Clean(full) == filepath.Clean(currentPath) && document.RevisionOf(currentSource) == document.Revision(hit.Revision) {
		a.acceptSearchHit(hit)
		return
	}
	go func() {
		note, err := a.documentRepository.Read(full)
		if err == nil && string(note.Revision) == hit.Revision {
			a.postVaultNavigation(session, vaultNavigationResult{hit: hit})
			return
		}
		if errors.Is(err, os.ErrNotExist) {
			a.postVaultNavigation(session, vaultNavigationResult{err: err})
			return
		}
		if refreshErr := session.Refresh(full); refreshErr != nil {
			a.postVaultNavigation(session, vaultNavigationResult{err: refreshErr})
			return
		}
		pending := hit
		a.onMain(func() {
			if a.vault != session {
				return
			}
			a.vaultPendingNavigation = &pending
			a.workspaceQuery = query
			a.vaultSearchLiteral = literal
			a.vaultSearchSubmitted = "\x00"
			a.vaultSearchDue = time.Now().Add(120 * time.Millisecond)
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
		a.activeNote.NavigationCommands().NavigateToRange(editorOffset(a.diskSource(), hit.Start), editorOffset(a.diskSource(), hit.End))
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
	a.vaultSnapshot = session.Snapshot()
	a.invalidate()
}

func (a *App) closeLinks() {
	if a.vault != nil {
		_ = a.vault.CloseLinks()
		a.vaultSnapshot = a.vault.Snapshot()
	}
}

func (a *App) linksDialog(c *ui.Context) {
	if a.vault == nil || !a.vaultSnapshot.LinksOpen {
		return
	}
	snapshot := a.vaultSnapshot
	linksOpen := snapshot.LinksOpen
	ui.Modal(c, &linksOpen, func() {
		ui.Column(c).Width(540).MaxHeight(600).Padding(20).Gap(10).Background(c.Theme().Background).Radius(12).Children(func() {
			ui.Text(c, locales.VaultBacklinksTitle()).FontWeight(700)
			if !snapshot.Ready {
				ui.Text(c, locales.VaultIndexBuildingShort())
				return
			}
			rel, _ := filepath.Rel(snapshot.Root, a.documentPath())
			_ = rel
			components.QuietScroll(c, &a.vaultLinksScroll, &a.vaultLinksIndicator).Height(360).FillWidth().Children(func() {
				ui.Column(c).FillWidth().Gap(6).Children(func() {
					ui.Text(c, locales.VaultLinksToHere())
					if snapshot.LinksBusy {
						ui.Text(c, locales.VaultFindingBacklinks())
					}
					incoming := groupedBacklinks(snapshot.Incoming)
					if len(incoming) == 0 {
						ui.Text(c, locales.VaultNoBacklinks())
					}
					for _, link := range incoming {
						l := link
						label := l.Source
						if l.Count > 1 {
							label += locales.VaultBacklinkCountSuffix(l.Count)
						}
						if ui.Button(c, label).Label(locales.VaultBacklinkSourceLabel(label)).Clicked() {
							a.closeLinks()
							a.navigateIndexed(l.Source, l.Start)
						}
					}
					ui.Text(c, locales.VaultOutgoingLinks())
					for _, link := range snapshot.Outgoing {
						l := link
						if ui.Button(c, l.Target).Label(locales.VaultOpenLinkLabel(l.Target)).Clicked() {
							a.closeLinks()
							a.indexedLink(l.Target)
						}
					}
				})
			})
			if ui.Button(c, locales.CommonClose()).Clicked() {
				a.closeLinks()
			}
		})
	})
	if !linksOpen {
		a.closeLinks()
	}
}

type backlinkSource struct {
	Source string
	Start  int
	Count  int
}

func groupedBacklinks(links []vaultlinks.Link) []backlinkSource {
	groups := make([]backlinkSource, 0, len(links))
	positions := make(map[string]int, len(links))
	for _, link := range links {
		if at, ok := positions[link.Source]; ok {
			groups[at].Count++
			continue
		}
		positions[link.Source] = len(groups)
		groups = append(groups, backlinkSource{Source: link.Source, Start: link.Start, Count: 1})
	}
	return groups
}

func (a *App) closeVaultSessionNow() {
	if a == nil || a.vault == nil {
		return
	}
	session := a.vault
	a.vault = nil
	a.vaultSnapshot = applicationvault.Snapshot{}
	_ = session.Close()
}
