package vault

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"muse/search"
	vaultfiles "muse/vault/files"
	vaultindex "muse/vault/index"
	vaultlinks "muse/vault/links"
	vaultwatch "muse/vault/watch"
)

var (
	ErrSessionClosed   = errors.New("application/vault: session is closed")
	ErrSessionNotReady = errors.New("application/vault: session is not ready")
)

const (
	searchLimit       = 500
	refreshTimeout    = 30 * time.Second
	resultBufferSize  = 1
	queryBufferSize   = 1
	refreshBufferSize = 1
)

type queryRequest struct {
	token   uint64
	query   string
	literal bool
	path    string
	source  string
	ctx     context.Context
	cancel  context.CancelFunc
}

// Session is a vault-scoped application coordinator. It owns startup,
// refresh, query cancellation, and resource lifetimes while delegating parsing,
// catalog policy, link resolution, search semantics, and filesystem access to
// their owning modules and injected ports.
//
// Session methods are safe for concurrent use. Snapshot returns detached
// slices. Results is a capacity-one latest-state notification stream; when a
// consumer falls behind, the older pending notification is replaced. A
// consumer should call Snapshot after each notification and discard query
// results whose token is no longer current.
type Session struct {
	root string
	deps SessionDependencies

	ctx    context.Context
	cancel context.CancelFunc
	opMu   sync.RWMutex

	mu                           sync.RWMutex
	snapshot                     Snapshot
	index                        IndexPort
	catalog                      CatalogPort
	watcher                      WatcherPort
	links                        *LinkGraph
	linksPath                    string
	search                       *search.Index
	quickOpen                    *QuickOpen
	wikiSuggestions              []vaultlinks.Suggestion
	wikiSuggestionsRevision      uint64
	wikiSuggestionsFilesRevision uint64
	wikiSuggestionsReady         bool
	closed                       bool
	queryCancel                  context.CancelFunc
	queryMu                      sync.Mutex
	queryJobs                    chan queryRequest
	refreshJobs                  chan struct{}
	refreshMu                    sync.Mutex
	pending                      map[string]vaultwatch.Event
	rescan                       bool

	searchGate chan struct{}
	workers    sync.WaitGroup

	resultMu     sync.Mutex
	results      chan Result
	resultsClose bool

	closeOnce sync.Once
	closeDone chan struct{}
	closeErr  error
}

// NewSession creates a session immediately and performs vault startup in the
// background. Startup failures are reported in Snapshot.Error and a
// SessionReady result; independently working capabilities remain available.
// NewSession rejects an empty root and missing index/catalog factories before
// starting any work.
func NewSession(root string, deps SessionDependencies) (*Session, error) {
	if strings.TrimSpace(root) == "" {
		return nil, errors.New("application/vault: root is required")
	}
	if deps.NewIndex == nil {
		return nil, errors.New("application/vault: index factory is required")
	}
	if deps.NewCatalog == nil {
		return nil, errors.New("application/vault: catalog factory is required")
	}
	absolute, err := filepath.Abs(filepath.Clean(root))
	if err != nil {
		return nil, fmt.Errorf("application/vault: resolve root: %w", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	s := &Session{
		root: filepath.Clean(absolute), deps: deps, ctx: ctx, cancel: cancel,
		snapshot:    Snapshot{Root: filepath.Clean(absolute)},
		queryJobs:   make(chan queryRequest, queryBufferSize),
		refreshJobs: make(chan struct{}, refreshBufferSize), pending: make(map[string]vaultwatch.Event),
		searchGate: make(chan struct{}, 1),
		results:    make(chan Result, resultBufferSize), closeDone: make(chan struct{}),
	}
	s.workers.Add(3)
	go s.queryWorker()
	go s.refreshWorker()
	go s.startup()
	return s, nil
}

// Snapshot returns an immutable, detached view of the latest session state.
func (s *Session) Snapshot() Snapshot {
	if s == nil {
		return Snapshot{}
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return cloneSnapshot(s.snapshot)
}

// Results reports state transitions using a latest-only, non-blocking channel.
// The channel closes after Close has stopped and joined all session workers.
func (s *Session) Results() <-chan Result {
	if s == nil {
		closed := make(chan Result)
		close(closed)
		return closed
	}
	return s.results
}

// Query starts a latest-wins search. The active editor source is parsed as a
// query overlay and never persisted into the durable search index. A blank
// query clears results immediately. The returned token identifies this query.
func (s *Session) Query(query string, literal bool, activePath, activeSource string) (uint64, error) {
	if s == nil {
		return 0, ErrSessionClosed
	}
	if !s.beginOperation() {
		return 0, ErrSessionClosed
	}
	defer s.endOperation()
	s.queryMu.Lock()
	defer s.queryMu.Unlock()
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return 0, ErrSessionClosed
	}
	s.snapshot.Search.Token++
	token := s.snapshot.Search.Token
	s.snapshot.Search.Query = query
	s.snapshot.Search.Literal = literal
	s.snapshot.Search.Hits = nil
	s.snapshot.Search.Error = ""
	s.snapshot.Search.Elapsed = 0
	s.snapshot.Search.Busy = strings.TrimSpace(query) != ""
	if s.queryCancel != nil {
		s.queryCancel()
		s.queryCancel = nil
	}
	index, searchIndex := s.index, s.search
	if strings.TrimSpace(query) == "" {
		s.snapshot.Search.Busy = false
		s.queryCancel = nil
		s.mu.Unlock()
		s.publish(Result{Kind: QueryCompleted, Token: token, Query: query, Literal: literal})
		return token, nil
	}
	if searchIndex == nil || index == nil {
		s.snapshot.Search.Busy = false
		s.snapshot.Search.Error = ErrSessionNotReady.Error()
		s.mu.Unlock()
		s.publish(Result{Kind: QueryCompleted, Token: token, Query: query, Literal: literal, Err: ErrSessionNotReady})
		return token, ErrSessionNotReady
	}
	ctx, cancel := context.WithCancel(s.ctx)
	s.queryCancel = cancel
	s.mu.Unlock()

	request := queryRequest{token: token, query: query, literal: literal, path: activePath, source: activeSource, ctx: ctx, cancel: cancel}
	select {
	case <-s.ctx.Done():
		cancel()
		return token, ErrSessionClosed
	default:
	}
	select {
	case s.queryJobs <- request:
	default:
		select {
		case <-s.queryJobs:
		default:
		}
		select {
		case s.queryJobs <- request:
		default:
			cancel()
		}
	}
	s.publish(Result{Kind: QueryStarted, Token: token, Query: query, Literal: literal, Snapshot: s.Snapshot()})
	return token, nil
}

// Refresh queues watcher-style invalidations. An empty path requests a full
// rebuild. Duplicate paths coalesce, and overflow supersedes incremental work.
func (s *Session) Refresh(paths ...string) error {
	if s == nil {
		return ErrSessionClosed
	}
	if !s.beginOperation() {
		return ErrSessionClosed
	}
	defer s.endOperation()
	s.mu.RLock()
	closed, ready := s.closed, s.snapshot.Ready
	s.mu.RUnlock()
	if closed {
		return ErrSessionClosed
	}
	if !ready {
		return ErrSessionNotReady
	}
	s.refreshMu.Lock()
	if len(paths) == 0 {
		s.rescan = true
		clear(s.pending)
	} else if !s.rescan {
		for _, value := range paths {
			if strings.TrimSpace(value) == "" {
				s.rescan = true
				clear(s.pending)
				break
			}
			path := filepath.Clean(value)
			if !filepath.IsAbs(path) {
				path = filepath.Join(s.root, path)
			}
			path, err := filepath.Abs(path)
			if err != nil {
				continue
			}
			s.pending[path] = vaultwatch.Event{Path: path, Kind: vaultwatch.Changed}
		}
	}
	s.refreshMu.Unlock()
	select {
	case s.refreshJobs <- struct{}{}:
	default:
	}
	return nil
}

// ApplyActiveDocument updates the in-memory vault and link snapshots from an
// editor source. It does not write the document or the persistent search
// index; active text is supplied to Query as an ephemeral overlay.
func (s *Session) ApplyActiveDocument(path, source string) error {
	if s == nil {
		return ErrSessionClosed
	}
	if !s.beginOperation() {
		return ErrSessionClosed
	}
	defer s.endOperation()
	s.mu.RLock()
	closed, ready, index, graph := s.closed, s.snapshot.Ready, s.index, s.links
	s.mu.RUnlock()
	if closed {
		return ErrSessionClosed
	}
	if !ready || index == nil {
		return ErrSessionNotReady
	}
	doc, err := index.ParseSnapshot(path, source)
	if err != nil {
		return err
	}
	index.ApplySnapshot(doc)
	if graph != nil {
		if err := graph.Apply(doc.Path); err != nil {
			return err
		}
	}
	s.mu.Lock()
	s.snapshot.IndexRevision = index.Revision()
	s.mu.Unlock()
	copy := cloneIndexDocument(doc)
	s.publish(Result{Kind: ActiveDocumentApplied, Changed: []string{doc.Path}, Document: &copy, Snapshot: s.Snapshot()})
	return nil
}

// OpenLinks selects the link graph projection for one vault-relative source.
// The graph query is in-memory and returns detached result slices.
func (s *Session) OpenLinks(path string) error {
	if s == nil {
		return ErrSessionClosed
	}
	if !s.beginOperation() {
		return ErrSessionClosed
	}
	defer s.endOperation()
	s.mu.RLock()
	closed, graph := s.closed, s.links
	s.mu.RUnlock()
	if closed {
		return ErrSessionClosed
	}
	if graph == nil {
		return ErrSessionNotReady
	}
	incoming, outgoing := graph.Backlinks(path), graph.Outgoing(path)
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return ErrSessionClosed
	}
	s.snapshot.LinksOpen = true
	s.snapshot.LinksBusy = false
	s.snapshot.Incoming = cloneLinks(incoming)
	s.snapshot.Outgoing = cloneLinks(outgoing)
	s.linksPath = path
	s.mu.Unlock()
	s.publish(Result{Kind: LinksRefreshed, Snapshot: s.Snapshot()})
	return nil
}

// CloseLinks clears the selected link projection without closing the graph.
func (s *Session) CloseLinks() error {
	if s == nil {
		return ErrSessionClosed
	}
	if !s.beginOperation() {
		return ErrSessionClosed
	}
	defer s.endOperation()
	s.mu.Lock()
	s.snapshot.LinksOpen = false
	s.snapshot.LinksBusy = false
	s.snapshot.Incoming = nil
	s.snapshot.Outgoing = nil
	s.linksPath = ""
	s.mu.Unlock()
	s.publish(Result{Kind: LinksRefreshed, Snapshot: s.Snapshot()})
	return nil
}

// Document returns a detached index projection for one vault-relative path.
func (s *Session) Document(path string) (vaultindex.Document, bool) {
	if s == nil {
		return vaultindex.Document{}, false
	}
	if !s.beginOperation() {
		return vaultindex.Document{}, false
	}
	defer s.endOperation()
	s.mu.RLock()
	index := s.index
	closed := s.closed
	s.mu.RUnlock()
	if closed || index == nil {
		return vaultindex.Document{}, false
	}
	doc, ok := index.Document(path)
	if !ok {
		return vaultindex.Document{}, false
	}
	return cloneIndexDocument(doc), true
}

// NoteContent returns the title and source snapshot for a vault-relative note.
func (s *Session) NoteContent(path string) (title, source string, ok bool) {
	if s == nil {
		return "", "", false
	}
	if !s.beginOperation() {
		return "", "", false
	}
	defer s.endOperation()
	s.mu.RLock()
	index, closed := s.index, s.closed
	s.mu.RUnlock()
	if closed || index == nil {
		return "", "", false
	}
	return index.NoteContent(path)
}

// Paths returns a stable copy of the vault path catalog.
func (s *Session) Paths() []string {
	if s == nil {
		return nil
	}
	if !s.beginOperation() {
		return nil
	}
	defer s.endOperation()
	s.mu.RLock()
	catalog, closed := s.catalog, s.closed
	s.mu.RUnlock()
	if closed || catalog == nil {
		return nil
	}
	return append([]string(nil), catalog.Paths()...)
}

// ImagePaths returns the catalog's supported image paths.
func (s *Session) ImagePaths() []string {
	if s == nil {
		return nil
	}
	if !s.beginOperation() {
		return nil
	}
	defer s.endOperation()
	s.mu.RLock()
	catalog, closed := s.catalog, s.closed
	s.mu.RUnlock()
	if closed || catalog == nil {
		return nil
	}
	return append([]string(nil), catalog.ImagePaths()...)
}

// WikiSuggestions returns a revision-scoped suggestion projection from the
// vault catalog and indexed note metadata. The cache and its invalidation key
// belong to this vault session, so UI adapters do not retain duplicate data.
func (s *Session) WikiSuggestions() []vaultlinks.Suggestion {
	if s == nil {
		return nil
	}
	snapshot := s.Snapshot()
	if !snapshot.Ready {
		return nil
	}
	s.mu.RLock()
	if s.wikiSuggestionsReady && s.wikiSuggestionsRevision == snapshot.IndexRevision && s.wikiSuggestionsFilesRevision == snapshot.FilesRevision {
		items := append([]vaultlinks.Suggestion(nil), s.wikiSuggestions...)
		s.mu.RUnlock()
		return items
	}
	s.mu.RUnlock()

	paths := s.Paths()
	documents := make([]vaultlinks.Document, 0, len(paths))
	for _, path := range paths {
		// Catalog paths are slash-separated and vault-relative. Do not feed
		// them to filepath.Rel with the absolute vault root: that rejects all
		// relative inputs and silently produces an empty suggestion list.
		rel := filepath.Clean(filepath.FromSlash(path))
		if filepath.IsAbs(rel) || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			continue
		}
		rel = filepath.ToSlash(rel)
		doc, ok := s.Document(rel)
		if !ok {
			continue
		}
		documents = append(documents, vaultlinks.Document{Path: rel, Title: doc.Title, Aliases: doc.Metadata().Aliases})
	}
	items := vaultlinks.WikiSuggestions(documents, s.ImagePaths())
	s.mu.Lock()
	s.wikiSuggestions = append(s.wikiSuggestions[:0], items...)
	s.wikiSuggestionsRevision = snapshot.IndexRevision
	s.wikiSuggestionsFilesRevision = snapshot.FilesRevision
	s.wikiSuggestionsReady = true
	s.mu.Unlock()
	return append([]vaultlinks.Suggestion(nil), items...)
}

// ResolveImageTarget resolves a Markdown wiki image target using this vault's
// catalog and the source note's directory context.
func (s *Session) ResolveImageTarget(currentPath, target string) (string, bool) {
	if s == nil {
		return "", false
	}
	snapshot := s.Snapshot()
	if !snapshot.Ready || snapshot.Root == "" {
		return "", false
	}
	current := ""
	if currentPath != "" {
		rel, err := filepath.Rel(snapshot.Root, currentPath)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return "", false
		}
		current = filepath.ToSlash(rel)
	}
	rel, ok := vaultlinks.ResolveImageTarget(snapshot.Root, current, target, s.ImagePaths())
	if !ok || !s.HasImage(rel) {
		return "", false
	}
	return filepath.Join(snapshot.Root, filepath.FromSlash(rel)), true
}

func (s *Session) ClearWikiSuggestions() {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.wikiSuggestions = nil
	s.wikiSuggestionsReady = false
	s.mu.Unlock()
}

// Children returns a detached list of immediate entries under a
// vault-relative directory. The empty path addresses the vault root.
func (s *Session) Children(directory string) []vaultfiles.Entry {
	if s == nil || !s.beginOperation() {
		return nil
	}
	defer s.endOperation()
	s.mu.RLock()
	catalog, closed := s.catalog, s.closed
	s.mu.RUnlock()
	if closed || catalog == nil {
		return nil
	}
	return append([]vaultfiles.Entry(nil), catalog.Children(directory)...)
}

// Entry returns the current catalog kind for one vault-relative path.
func (s *Session) Entry(path string) (vaultfiles.Entry, bool) {
	if s == nil || !s.beginOperation() {
		return vaultfiles.Entry{}, false
	}
	defer s.endOperation()
	s.mu.RLock()
	catalog, closed := s.catalog, s.closed
	s.mu.RUnlock()
	if closed || catalog == nil {
		return vaultfiles.Entry{}, false
	}
	return catalog.Entry(path)
}

// HasImage reports whether the active catalog contains the exact image path.
func (s *Session) HasImage(path string) bool {
	if s == nil || !s.beginOperation() {
		return false
	}
	defer s.endOperation()
	s.mu.RLock()
	catalog, closed := s.catalog, s.closed
	s.mu.RUnlock()
	if closed || catalog == nil {
		return false
	}
	want := filepath.Clean(path)
	for _, candidate := range catalog.ImagePaths() {
		if filepath.Clean(candidate) == want {
			return true
		}
	}
	return false
}

// FindImage resolves a basename through the per-vault catalog.
func (s *Session) FindImage(name string) string {
	if s == nil {
		return ""
	}
	if !s.beginOperation() {
		return ""
	}
	defer s.endOperation()
	s.mu.RLock()
	catalog, closed := s.catalog, s.closed
	s.mu.RUnlock()
	if closed || catalog == nil {
		return ""
	}
	return catalog.FindImage(name)
}

// WatchFile extends watcher coverage for an active file outside the initial
// directory walk, such as a document opened directly from disk.
func (s *Session) WatchFile(path string) error {
	if s == nil {
		return ErrSessionClosed
	}
	if !s.beginOperation() {
		return ErrSessionClosed
	}
	defer s.endOperation()
	s.mu.RLock()
	watcher, closed := s.watcher, s.closed
	s.mu.RUnlock()
	if closed {
		return ErrSessionClosed
	}
	if watcher == nil {
		return ErrSessionNotReady
	}
	return watcher.WatchFile(path)
}

// QuickOpen searches the latest source-agnostic file/title/alias projection.
func (s *Session) QuickOpen(query string) []QuickOpenMatch {
	if s == nil {
		return nil
	}
	if !s.beginOperation() {
		return nil
	}
	defer s.endOperation()
	s.mu.RLock()
	quickOpen, closed := s.quickOpen, s.closed
	s.mu.RUnlock()
	if closed || quickOpen == nil {
		return nil
	}
	matches := quickOpen.Search(query)
	out := make([]QuickOpenMatch, len(matches))
	for i, match := range matches {
		out[i] = match
		out[i].TitleMatches = append([]int(nil), match.TitleMatches...)
		out[i].PathMatches = append([]int(nil), match.PathMatches...)
	}
	return out
}

// ResolveLink resolves a target through the session-owned link graph.
func (s *Session) ResolveLink(current, target string) (vaultlinks.Location, bool) {
	if s == nil {
		return vaultlinks.Location{}, false
	}
	if !s.beginOperation() {
		return vaultlinks.Location{}, false
	}
	defer s.endOperation()
	s.mu.RLock()
	graph, closed := s.links, s.closed
	s.mu.RUnlock()
	if closed || graph == nil {
		return vaultlinks.Location{}, false
	}
	return graph.Resolve(current, target)
}

// Outgoing returns detached links from one vault-relative source path.
func (s *Session) Outgoing(path string) []vaultlinks.Link {
	if s == nil {
		return nil
	}
	if !s.beginOperation() {
		return nil
	}
	defer s.endOperation()
	s.mu.RLock()
	graph, closed := s.links, s.closed
	s.mu.RUnlock()
	if closed || graph == nil {
		return nil
	}
	return cloneLinks(graph.Outgoing(path))
}

// Backlinks returns detached links targeting one vault-relative path.
func (s *Session) Backlinks(path string) []vaultlinks.Link {
	if s == nil {
		return nil
	}
	if !s.beginOperation() {
		return nil
	}
	defer s.endOperation()
	s.mu.RLock()
	graph, closed := s.links, s.closed
	s.mu.RUnlock()
	if closed || graph == nil {
		return nil
	}
	return cloneLinks(graph.Backlinks(path))
}

// BacklinkSources returns detached source-level groups for the properties UI.
func (s *Session) BacklinkSources(path string) []vaultlinks.BacklinkSource {
	if s == nil || !s.beginOperation() {
		return nil
	}
	defer s.endOperation()
	s.mu.RLock()
	graph, closed := s.links, s.closed
	s.mu.RUnlock()
	if closed || graph == nil {
		return nil
	}
	return append([]vaultlinks.BacklinkSource(nil), graph.BacklinkSources(path)...)
}

// BacklinkSourcesForDocument resolves an absolute document path into the
// session's vault-relative link index and returns detached source groups.
// Paths outside the session root produce no backlinks.
func (s *Session) BacklinkSourcesForDocument(path string) []vaultlinks.BacklinkSource {
	if s == nil || path == "" {
		return nil
	}
	s.mu.RLock()
	root := s.root
	s.mu.RUnlock()
	if root == "" {
		return nil
	}
	relative, err := filepath.Rel(root, path)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return nil
	}
	return s.BacklinkSources(filepath.ToSlash(relative))
}

// Close cancels query and watcher work, joins all workers, closes the watcher
// and search index, and then closes Results. It is idempotent.
func (s *Session) Close() error {
	if s == nil {
		return nil
	}
	s.closeOnce.Do(func() {
		s.opMu.Lock()
		s.mu.Lock()
		s.closed = true
		if s.queryCancel != nil {
			s.queryCancel()
			s.queryCancel = nil
		}
		s.mu.Unlock()
		s.cancel()
		s.opMu.Unlock()
		go s.closeResources()
	})
	<-s.closeDone
	return s.closeErr
}

func (s *Session) beginOperation() bool {
	s.opMu.RLock()
	s.mu.RLock()
	closed := s.closed
	s.mu.RUnlock()
	if closed {
		s.opMu.RUnlock()
		return false
	}
	return true
}

func (s *Session) endOperation() { s.opMu.RUnlock() }

func (s *Session) closeResources() {
	s.workers.Wait()
	s.mu.Lock()
	watcher, searchIndex, graph := s.watcher, s.search, s.links
	s.watcher, s.search, s.links = nil, nil, nil
	s.index, s.catalog, s.quickOpen = nil, nil, nil
	s.wikiSuggestions = nil
	s.wikiSuggestionsReady = false
	s.snapshot.Ready = false
	s.snapshot.SearchAvailable = false
	s.snapshot.Search.Busy = false
	s.snapshot.LinksBusy = false
	s.snapshot.LinksOpen = false
	s.snapshot.Incoming = nil
	s.snapshot.Outgoing = nil
	s.linksPath = ""
	snapshot := cloneSnapshot(s.snapshot)
	s.mu.Unlock()
	var errs []error
	if watcher != nil {
		errs = append(errs, watcher.Close())
	}
	if graph != nil {
		graph.Close()
	}
	if searchIndex != nil {
		errs = append(errs, searchIndex.Close())
	}
	s.closeErr = errors.Join(errs...)
	s.publish(Result{Kind: SessionClosed, Err: s.closeErr, Snapshot: snapshot})
	s.resultMu.Lock()
	if !s.resultsClose {
		s.resultsClose = true
		close(s.results)
	}
	s.resultMu.Unlock()
	close(s.closeDone)
}

func (s *Session) startup() {
	defer s.workers.Done()
	var errs []error
	index, err := s.deps.NewIndex(s.root)
	if err != nil {
		errs = append(errs, fmt.Errorf("create index: %w", err))
	}
	catalog, catalogErr := s.deps.NewCatalog(s.root)
	if catalogErr != nil {
		errs = append(errs, fmt.Errorf("create catalog: %w", catalogErr))
	}
	var watcher WatcherPort
	if s.deps.NewWatcher != nil {
		watcher, err = s.deps.NewWatcher(s.root)
		if err != nil {
			errs = append(errs, fmt.Errorf("start watcher: %w", err))
		}
	} else {
		errs = append(errs, errors.New("watcher factory is not configured"))
	}
	if s.ctx.Err() != nil {
		closeUnstarted(watcher)
		return
	}
	indexReady := false
	if index != nil {
		if err := index.Rebuild(); err != nil {
			errs = append(errs, fmt.Errorf("rebuild index: %w", err))
		} else {
			indexReady = true
			graph, graphErr := NewLinkGraph(s.root, index)
			if graphErr != nil {
				errs = append(errs, fmt.Errorf("build link graph: %w", graphErr))
			} else {
				s.mu.Lock()
				s.links = graph
				s.mu.Unlock()
			}
		}
	}
	if catalog != nil {
		if err := catalog.Rebuild(); err != nil {
			errs = append(errs, fmt.Errorf("rebuild catalog: %w", err))
		}
	}
	var searchIndex *search.Index
	if indexReady && s.deps.SearchReconciler != nil {
		searchIndex, err = s.deps.SearchReconciler.OpenAndReconcile(s.ctx, s.root, index)
		if err != nil {
			errs = append(errs, fmt.Errorf("open search index: %w", err))
		}
	} else if s.deps.SearchReconciler == nil {
		errs = append(errs, errors.New("search index store is not configured"))
	} else if index != nil && !indexReady {
		errs = append(errs, errors.New("search index unavailable because vault index failed to rebuild"))
	}
	if s.ctx.Err() != nil {
		closeUnstarted(watcher)
		if searchIndex != nil {
			_ = searchIndex.Close()
		}
		return
	}
	s.mu.Lock()
	s.index, s.catalog, s.watcher, s.search = index, catalog, watcher, searchIndex
	if index != nil && catalog != nil {
		s.quickOpen = NewQuickOpen(s.root, catalog, index)
	}
	s.snapshot.Ready = true
	s.snapshot.IndexRevision = revisionOf(index)
	s.snapshot.FilesRevision = filesRevisionOf(catalog)
	s.snapshot.SearchAvailable = searchIndex != nil
	s.snapshot.Search.Error = ""
	if searchIndex == nil {
		for _, startupErr := range errs {
			if strings.Contains(startupErr.Error(), "search") {
				s.snapshot.Search.Error = startupErr.Error()
				break
			}
		}
	}
	if joined := errors.Join(errs...); joined != nil {
		s.snapshot.Error = joined.Error()
	} else {
		s.snapshot.Error = ""
	}
	s.mu.Unlock()

	if watcher != nil {
		s.workers.Add(1)
		go s.watchEvents(watcher)
	}
	s.publish(Result{Kind: SessionReady, Err: errors.Join(errs...), Snapshot: s.Snapshot()})
}

func (s *Session) queryWorker() {
	defer s.workers.Done()
	for {
		select {
		case <-s.ctx.Done():
			return
		case request := <-s.queryJobs:
			started := time.Now()
			hits, err := s.runQuery(request)
			request.cancel()
			elapsed := time.Since(started)
			s.mu.Lock()
			if !s.closed && request.token == s.snapshot.Search.Token {
				s.queryCancel = nil
				s.snapshot.Search.Hits = cloneHits(hits)
				s.snapshot.Search.Busy = false
				s.snapshot.Search.Elapsed = elapsed
				s.snapshot.Search.Error = errorString(err)
				if elapsed > 0 {
					if s.snapshot.Search.Cost == 0 {
						s.snapshot.Search.Cost = elapsed
					} else {
						s.snapshot.Search.Cost = (s.snapshot.Search.Cost*3 + elapsed) / 4
					}
				}
				snapshot := cloneSnapshot(s.snapshot)
				s.mu.Unlock()
				s.publish(Result{Kind: QueryCompleted, Token: request.token, Query: request.query, Literal: request.literal, Err: err, Elapsed: elapsed, Snapshot: snapshot})
				continue
			}
			s.mu.Unlock()
		}
	}
}

func (s *Session) runQuery(request queryRequest) ([]search.Hit, error) {
	if err := request.ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.RLock()
	index, searchIndex := s.index, s.search
	s.mu.RUnlock()
	if index == nil || searchIndex == nil {
		return nil, ErrSessionNotReady
	}
	var overlays []search.Document
	if request.path != "" && strings.EqualFold(filepath.Ext(request.path), ".md") {
		doc, err := index.ParseSnapshot(request.path, request.source)
		if err == nil {
			overlays = append(overlays, searchDocument(doc))
		}
	}
	if err := acquire(request.ctx, s.searchGate); err != nil {
		return nil, err
	}
	defer release(s.searchGate)
	s.mu.RLock()
	searchIndex = s.search
	s.mu.RUnlock()
	if searchIndex == nil {
		return nil, ErrSessionNotReady
	}
	if request.literal {
		return searchIndex.Literal(request.ctx, request.query, searchLimit, overlays...)
	}
	return searchIndex.Search(request.ctx, request.query, searchLimit, overlays...)
}

func (s *Session) watchEvents(watcher WatcherPort) {
	defer s.workers.Done()
	events := watcher.Events()
	for {
		select {
		case <-s.ctx.Done():
			return
		case event, ok := <-events:
			if !ok {
				return
			}
			s.enqueueEvent(event)
		}
	}
}

func (s *Session) enqueueEvent(event vaultwatch.Event) {
	s.refreshMu.Lock()
	if event.Overflow || event.Path == "" {
		s.rescan = true
		clear(s.pending)
	} else if !s.rescan {
		path, err := filepath.Abs(filepath.Clean(event.Path))
		if err == nil {
			event.Path = path
			s.pending[path] = event
		}
	}
	s.refreshMu.Unlock()
	select {
	case s.refreshJobs <- struct{}{}:
	default:
	}
}

func (s *Session) refreshWorker() {
	defer s.workers.Done()
	for {
		select {
		case <-s.ctx.Done():
			return
		case <-s.refreshJobs:
			paths, full := s.takePending()
			if len(paths) == 0 && !full {
				continue
			}
			s.refresh(paths, full)
		}
	}
}

func (s *Session) takePending() (map[string]vaultwatch.Event, bool) {
	s.refreshMu.Lock()
	paths, full := s.pending, s.rescan
	s.pending = make(map[string]vaultwatch.Event)
	s.rescan = false
	s.refreshMu.Unlock()
	return paths, full
}

func (s *Session) refresh(events map[string]vaultwatch.Event, full bool) {
	s.mu.RLock()
	if s.closed {
		s.mu.RUnlock()
		return
	}
	index, catalog, graph, searchIndex := s.index, s.catalog, s.links, s.search
	s.mu.RUnlock()
	ctx, cancel := context.WithTimeout(s.ctx, refreshTimeout)
	defer cancel()
	var errs []error
	var upserts []search.Document
	var deletes []string
	changed := make([]string, 0, len(events))
	indexReady := false
	if full {
		if catalog != nil {
			if err := catalog.Rebuild(); err != nil {
				errs = append(errs, fmt.Errorf("rebuild catalog: %w", err))
			}
		}
		if index != nil {
			if err := index.Rebuild(); err != nil {
				errs = append(errs, fmt.Errorf("rebuild index: %w", err))
			} else {
				indexReady = true
				if graph != nil {
					errs = appendIfError(errs, graph.Rebuild())
				}
				upserts = index.SearchDocuments()
			}
		}
	} else {
		ordered := make([]string, 0, len(events))
		for path := range events {
			ordered = append(ordered, path)
		}
		sort.Strings(ordered)
		for _, path := range ordered {
			event := events[path]
			changed = append(changed, path)
			if event.Deleted || event.Kind == vaultwatch.Removed {
				relative := relativePath(s.root, path)
				if catalog != nil {
					catalog.Remove(path)
				}
				if index != nil {
					found := false
					prefix := strings.TrimSuffix(relative, "/") + "/"
					for _, doc := range index.SearchDocuments() {
						if doc.Path == relative || strings.HasPrefix(doc.Path, prefix) {
							deletes = append(deletes, searchDocumentID(doc.Path))
							found = true
						}
					}
					if !found {
						deletes = append(deletes, searchDocumentID(relative))
					}
					index.Remove(path)
				}
				if graph != nil {
					graph.Remove(path)
				}
				continue
			}
			if catalog != nil {
				catalog.Update(path)
			}
			if index == nil {
				continue
			}
			if err := index.Update(path); err != nil {
				errs = append(errs, fmt.Errorf("update %s: %w", path, err))
				continue
			}
			if doc, ok := index.Document(path); ok {
				if graph != nil {
					errs = appendIfError(errs, graph.Apply(doc.Path))
				}
				upserts = append(upserts, searchDocument(doc))
			} else {
				if graph != nil {
					graph.Remove(path)
				}
				deletes = append(deletes, searchDocumentID(relativePath(s.root, path)))
			}
		}
	}
	if (full && indexReady && s.deps.SearchReconciler != nil) || (!full && searchIndex != nil) {
		if err := acquire(ctx, s.searchGate); err != nil {
			errs = append(errs, err)
		} else {
			s.mu.RLock()
			current := s.search
			s.mu.RUnlock()
			if full && indexReady {
				next, err := s.deps.SearchReconciler.Reconcile(ctx, s.root, current, index)
				errs = appendIfError(errs, err)
				s.mu.Lock()
				s.search = next
				s.snapshot.SearchAvailable = next != nil
				s.mu.Unlock()
			} else if current != nil {
				errs = appendIfError(errs, current.Apply(ctx, upserts, deletes))
			}
			release(s.searchGate)
		}
	}
	s.mu.Lock()
	s.snapshot.IndexRevision = revisionOf(index)
	s.snapshot.FilesRevision = filesRevisionOf(catalog)
	if graph != nil && s.linksPath != "" && s.snapshot.LinksOpen {
		s.snapshot.Incoming = cloneLinks(graph.Backlinks(s.linksPath))
		s.snapshot.Outgoing = cloneLinks(graph.Outgoing(s.linksPath))
	}
	if len(errs) > 0 {
		s.snapshot.Error = errors.Join(errs...).Error()
	} else {
		s.snapshot.Error = ""
	}
	snapshot := cloneSnapshot(s.snapshot)
	s.mu.Unlock()
	sortStrings(changed)
	s.publish(Result{Kind: FilesRefreshed, Changed: changed, Full: full, Err: errors.Join(errs...), Snapshot: snapshot})
}

func (s *Session) publish(result Result) {
	if result.Snapshot.Root == "" && s != nil {
		result.Snapshot = s.Snapshot()
	}
	result.Changed = append([]string(nil), result.Changed...)
	if result.Document != nil {
		doc := cloneIndexDocument(*result.Document)
		result.Document = &doc
	}
	result.Snapshot = cloneSnapshot(result.Snapshot)
	s.resultMu.Lock()
	defer s.resultMu.Unlock()
	if s.resultsClose {
		return
	}
	select {
	case s.results <- result:
	default:
		select {
		case <-s.results:
		default:
		}
		select {
		case s.results <- result:
		default:
		}
	}
}

func cloneSnapshot(snapshot Snapshot) Snapshot {
	snapshot.Search.Hits = cloneHits(snapshot.Search.Hits)
	snapshot.Incoming = cloneLinks(snapshot.Incoming)
	snapshot.Outgoing = cloneLinks(snapshot.Outgoing)
	return snapshot
}

func cloneHits(in []search.Hit) []search.Hit { return append([]search.Hit(nil), in...) }

func cloneLinks(in []vaultlinks.Link) []vaultlinks.Link { return append([]vaultlinks.Link(nil), in...) }

func cloneIndexDocument(in vaultindex.Document) vaultindex.Document {
	in.Aliases = append([]string(nil), in.Aliases...)
	in.Tags = append([]string(nil), in.Tags...)
	in.Headings = append([]vaultlinks.Heading(nil), in.Headings...)
	in.Blocks = append([]vaultlinks.BlockID(nil), in.Blocks...)
	in.Links = append([]vaultlinks.Link(nil), in.Links...)
	return in
}

func searchDocument(doc vaultindex.Document) search.Document {
	return search.Document{ID: searchDocumentID(doc.Path), Path: doc.Path, Title: doc.Title, Source: doc.Source, Revision: string(doc.Revision), Metadata: doc.Metadata()}
}

func searchDocumentID(path string) string {
	key := sha256.Sum256([]byte(filepath.ToSlash(filepath.Clean(path))))
	return hex.EncodeToString(key[:])
}

func relativePath(root, path string) string {
	clean := filepath.Clean(path)
	if !filepath.IsAbs(clean) {
		return filepath.ToSlash(clean)
	}
	rel, err := filepath.Rel(root, clean)
	if err != nil {
		return filepath.ToSlash(clean)
	}
	return filepath.ToSlash(rel)
}

func revisionOf(index IndexPort) uint64 {
	if index == nil {
		return 0
	}
	return index.Revision()
}

func filesRevisionOf(catalog CatalogPort) uint64 {
	if catalog == nil {
		return 0
	}
	return catalog.Revision()
}

func errorString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func appendIfError(errs []error, err error) []error {
	if err != nil {
		return append(errs, err)
	}
	return errs
}

func acquire(ctx context.Context, gate chan struct{}) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case gate <- struct{}{}:
		return nil
	}
}

func release(gate chan struct{}) { <-gate }

func closeUnstarted(watcher WatcherPort) {
	if watcher != nil {
		_ = watcher.Close()
	}
}

func sortStrings(values []string) {
	sort.Strings(values)
}
