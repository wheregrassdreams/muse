package nativeui

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/egoist/mygo/ui"
	"muse/cachepath"
	"muse/notestore"
	"muse/scrollui"
	"muse/vaultindex"
	"muse/vaultsearch"
	"muse/vaultwatch"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type vaultReady struct {
	root   string
	index  *vaultindex.Index
	search *vaultsearch.Index
	watch  *vaultwatch.Watcher
	err    error
}
type vaultRefresh struct {
	path         string
	doc          notestore.Document
	err          error
	hasDoc       bool
	linksChanged bool
}
type linkResult struct {
	path               string
	incoming, outgoing []vaultindex.Link
}
type vaultQuery struct {
	token   uint64
	query   string
	literal bool
	hits    []vaultsearch.Hit
	err     error
}
type vaultNavigation struct {
	hit vaultsearch.Hit
	err error
}
type vaultRuntime struct {
	linkResults        chan linkResult
	incoming, outgoing []vaultindex.Link
	linksBusy          bool
	linkScroll         ui.ScrollState
	linkIndicator      scrollui.Indicator
	closed             chan struct{}
	closeDone          chan struct{}
	lifeCtx            context.Context
	lifeCancel         context.CancelFunc
	stopped            bool
	root               string
	index              *vaultindex.Index
	linkRevision       uint64
	search             *vaultsearch.Index
	searchMu           sync.Mutex
	watch              *vaultwatch.Watcher
	ready              chan vaultReady
	refresh            chan vaultRefresh
	queries            chan vaultQuery
	navigation         chan vaultNavigation
	refreshing         bool
	query              string
	searchMode         bool
	queryToken         uint64
	queryCancel        context.CancelFunc
	searchLiteral      bool
	due                time.Time
	hits               []vaultsearch.Hit
	searchErr          error
	searchBusy         bool
	linksOpen          bool
}

func (a *App) vaultTick(c *ui.Context) {
	if a.documents == nil {
		if a.vault != nil {
			stopVault(a.vault)
			a.vault = nil
		}
		return
	}
	root := a.root
	if root == "" && a.path != "" {
		root = filepath.Dir(a.path)
	}
	if root == "" {
		if a.vault != nil {
			stopVault(a.vault)
			a.vault = nil
		}
		return
	}
	if a.vault == nil || a.vault.root != root {
		if a.vault != nil {
			stopVault(a.vault)
		}
		lifeCtx, lifeCancel := context.WithCancel(context.Background())
		v := &vaultRuntime{linkResults: make(chan linkResult, 1), root: root, closed: make(chan struct{}), closeDone: make(chan struct{}), lifeCtx: lifeCtx, lifeCancel: lifeCancel, ready: make(chan vaultReady), refresh: make(chan vaultRefresh, 1), queries: make(chan vaultQuery, 4), navigation: make(chan vaultNavigation, 1)}
		a.vault = v
		go func() {
			idx := vaultindex.New()
			w, watchErr := vaultwatch.New(root)
			indexErr := idx.Rebuild(root)
			var search *vaultsearch.Index
			var searchErr error
			if indexErr == nil {
				search, searchErr = openVaultSearch(root)
				if searchErr == nil {
					searchErr = search.Reconcile(v.lifeCtx, searchDocuments(idx.Snapshot()))
					if searchErr != nil {
						_ = search.Close()
						search = nil
					}
				}
			}
			err := errors.Join(watchErr, indexErr, searchErr)
			select {
			case v.ready <- vaultReady{root: root, index: idx, search: search, watch: w, err: err}:
				a.invalidate()
			case <-v.closed:
				if w != nil {
					_ = w.Close()
				}
				if search != nil {
					_ = search.Close()
				}
			}
		}()
	}
	v := a.vault
	select {
	case ready := <-v.ready:
		if ready.err != nil {
			a.notice("笔记库索引／监听失败：" + ready.err.Error())
		}
		v.index, v.search, v.watch = ready.index, ready.search, ready.watch
		v.linkRevision++
		if a.ed != nil {
			a.ed.SetOptions(a.editorOptions())
		}
		if ready.search == nil && ready.err != nil {
			v.searchErr = ready.err
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
		if v.watch != nil && a.path != "" {
			_ = v.watch.WatchFile(a.path)
		}
	default:
	}
	select {
	case fresh := <-v.refresh:
		v.refreshing = false
		if fresh.linksChanged {
			v.linkRevision++
			if a.ed != nil {
				a.ed.SetOptions(a.editorOptions())
			}
		}
		if a.workspaceSearch {
			v.due = time.Now()
		}
		if fresh.err != nil && !errors.Is(fresh.err, os.ErrNotExist) {
			a.notice("刷新笔记库索引失败：" + fresh.err.Error())
		}
		if fresh.path == a.path {
			r := a.documents
			if fresh.err != nil {
				if errors.Is(fresh.err, os.ErrNotExist) {
					r.conflict = a.path
					r.conflictOpen = true
					a.notice("当前文件已在外部删除，编辑内容仍保留")
				}
			} else if fresh.hasDoc && fresh.doc.Revision != r.revision {
				if a.dirty || r.saving {
					if fresh.doc.Text != a.diskSource() {
						r.conflict = a.path
						r.conflictOpen = true
						if err := r.store.WriteDraft(a.path, a.path, a.diskSource()); err != nil {
							a.notice("恢复草稿保存失败：" + err.Error())
						}
					}
				} else {
					at := a.ed.Caret()
					a.loadDoc(fresh.doc.Text)
					a.ed.Select(min(at, len(a.source)), min(at, len(a.source)))
					a.dirty = false
					a.updateTitle()
					a.notice("已加载外部修改")
				}
			}
		}
	default:
	}
	if v.watch != nil && !v.refreshing {
		paths := map[string]bool{}
		rescan := false
		for n := 0; n < 128; n++ {
			select {
			case ev, ok := <-v.watch.Events():
				if ok {
					paths[ev.Path] = true
					rescan = rescan || ev.Overflow
				}
			default:
				n = 128
			}
		}
		if len(paths) > 0 {
			a.clearTreeCache()
			v.refreshing = true
			current := filepath.Clean(a.path)
			idx := v.index
			search := v.search
			store := a.documents.store
			go func() {
				var fresh vaultRefresh
				upserts := make([]vaultsearch.Document, 0, len(paths))
				deletes := make([]string, 0, len(paths))
				if idx != nil {
					if rescan {
						if err := idx.Rebuild(root); err != nil {
							fresh.err = err
						} else {
							if search != nil {
								upserts = searchDocuments(idx.Snapshot())
							}
							if current != "." && current != "" {
								if doc, ok := idx.Document(current); ok && filepath.Ext(current) == ".md" {
									fresh = vaultRefresh{path: current, doc: notestore.Document{Path: current, Text: doc.Source, Revision: notestore.Revision(doc.Source)}, hasDoc: true}
								} else if _, err := os.Stat(current); os.IsNotExist(err) {
									fresh = vaultRefresh{path: current, err: err}
								}
							}
						}
					} else {
						for path := range paths {
							path = filepath.Clean(path)
							if path == current {
								doc, err := store.Read(path)
								fresh = vaultRefresh{path: path, doc: doc, err: err, hasDoc: err == nil}
								if err == nil && filepath.Ext(path) == ".md" {
									parsed, parseErr := idx.ParseSnapshot(path, doc.Text)
									if parseErr == nil {
										idx.ApplySnapshot(parsed)
										upserts = append(upserts, searchDocument(parsed))
									} else {
										fresh.err = parseErr
									}
								} else if err == nil {
									idx.Remove(path)
									deletes = append(deletes, searchDocumentID(relativeVaultPath(root, path)))
								} else if errors.Is(err, os.ErrNotExist) {
									idx.Remove(path)
									deletes = append(deletes, searchDocumentID(relativeVaultPath(root, path)))
								}
								continue
							}
							if _, err := os.Stat(path); os.IsNotExist(err) {
								idx.Remove(path)
								deletes = append(deletes, searchDocumentID(relativeVaultPath(root, path)))
							} else {
								if err := idx.Update(path); err != nil {
									fresh.err = errors.Join(fresh.err, err)
									continue
								}
								if doc, ok := idx.Document(path); ok {
									upserts = append(upserts, searchDocument(doc))
								} else {
									deletes = append(deletes, searchDocumentID(relativeVaultPath(root, path)))
								}
							}
						}
					}
				}
				if search != nil {
					searchCtx, cancel := context.WithTimeout(v.lifeCtx, 30*time.Second)
					v.searchMu.Lock()
					if rescan {
						fresh.err = errors.Join(fresh.err, search.Reconcile(searchCtx, upserts))
					} else {
						fresh.err = errors.Join(fresh.err, search.Apply(searchCtx, upserts, deletes))
					}
					v.searchMu.Unlock()
					cancel()
				}
				fresh.linksChanged = idx != nil
				select {
				case v.refresh <- fresh:
				case <-v.closed:
				}
				a.invalidate()
			}()
		}
	}
	select {
	case result := <-v.queries:
		if result.query == a.workspaceQuery && result.literal == v.searchLiteral && result.token == v.queryToken {
			v.hits = result.hits
			v.searchErr = result.err
			v.searchBusy = false
			if len(result.hits) > 0 {
				a.workspaceSearchSel = 0
				a.workspaceReveal = true
			}
		}
	default:
	}
	select {
	case result := <-v.navigation:
		if result.err != nil {
			a.notice("搜索结果已更新，请重试：" + result.err.Error())
		} else {
			full := filepath.Join(v.root, filepath.FromSlash(result.hit.Path))
			if filepath.Clean(full) == filepath.Clean(a.path) && notestore.Revision(a.diskSource()) == result.hit.Revision {
				a.ed.ScrollToOffset(editorOffset(a.diskSource(), result.hit.Start))
				break
			}
			a.indexedPath, a.indexedOffset, a.indexedRevision = full, result.hit.Start, result.hit.Revision
			a.requestOpen(full)
		}
	default:
	}
	select {
	case result := <-v.linkResults:
		rel, _ := filepath.Rel(v.root, a.path)
		if filepath.ToSlash(rel) == result.path {
			v.incoming, v.outgoing = result.incoming, result.outgoing
			v.linksBusy = false
		}
	default:
	}

}
func (a *App) workspaceSearchView(c *ui.Context) {
	ui.Column(c).Fill().Padding(10).Gap(8).Children(func() {
		v := a.vault
		if v == nil || v.index == nil || v.search == nil {
			if v != nil && v.searchErr != nil {
				ui.Text(c, "搜索索引不可用："+v.searchErr.Error()).Wrap()
				return
			}
			ui.Text(c, "正在建立笔记库索引…")
			return
		}
		if v.query != a.workspaceQuery || v.searchMode != v.searchLiteral {
			a.workspaceSearchSel = 0
			a.workspaceReveal = true
			a.workspaceScroll = ui.ScrollState{}
			v.query = a.workspaceQuery
			v.searchMode = v.searchLiteral
			if v.queryCancel != nil {
				v.queryCancel()
				v.queryCancel = nil
			}
			v.queryToken++
			v.hits = nil
			v.searchErr = nil
			v.due = time.Now().Add(150 * time.Millisecond)
		}
		if !v.due.IsZero() && !time.Now().Before(v.due) {
			q := v.query
			v.queryToken++
			token := v.queryToken
			literal := v.searchLiteral
			current, buffer := a.path, a.diskSource()
			idx, search := v.index, v.search
			ctx, cancel := context.WithCancel(v.lifeCtx)
			v.queryCancel = cancel
			v.searchBusy = q != ""
			v.due = time.Time{}
			if q == "" {
				v.hits = nil
				v.searchErr = nil
				return
			}
			go func() {
				var overlays []vaultsearch.Document
				if current != "" && filepath.Ext(current) == ".md" {
					if doc, err := idx.ParseSnapshot(current, buffer); err == nil {
						overlays = append(overlays, searchDocument(doc))
					}
				}
				v.searchMu.Lock()
				var hits []vaultsearch.Hit
				var err error
				if literal {
					hits, err = search.Literal(ctx, q, 500, overlays...)
				} else {
					hits, err = search.Search(ctx, q, 500, overlays...)
				}
				v.searchMu.Unlock()
				select {
				case v.queries <- vaultQuery{query: q, literal: literal, hits: hits, token: token, err: err}:
				default:
					select {
					case <-v.queries:
					default:
					}
					select {
					case v.queries <- vaultQuery{query: q, literal: literal, hits: hits, token: token, err: err}:
					default:
					}
				}
				a.invalidate()
			}()
		}
		if v.searchErr != nil {
			ui.Text(c, "搜索失败："+v.searchErr.Error()).Wrap()
		}
		if v.searchBusy {
			ui.Text(c, "正在搜索…").FontSize(11).TextColor(c.Theme().TextMuted)
		}
		if a.workspaceQuery != "" && !v.searchBusy && v.searchErr == nil && len(v.hits) == 0 && v.search != nil {
			ui.Text(c, "没有找到笔记").FontSize(12).TextColor(c.Theme().TextMuted)
		}
		showSearchScroll := a.workspaceIndicator.Visible(c.Now(), a.workspaceScroll.X, a.workspaceScroll.Y)
		if showSearchScroll {
			c.After(a.workspaceIndicator.Remaining(c.Now()))
		}
		quietScroll(c, &a.workspaceScroll, &a.workspaceIndicator).FillWidth().Grow(1).MinHeight(0).Scrollbars(showSearchScroll).Children(func() {
			ui.Column(c).FillWidth().Gap(6).Children(func() {
				for i, hit := range v.hits {
					h := hit
					title := filepath.Base(h.Path)
					if doc, ok := v.index.Document(h.Path); ok && strings.TrimSpace(doc.Title) != "" {
						title = doc.Title
					}
					row := ui.ButtonBase(c).Label("搜索结果：" + h.Path).FillWidth().Height(72).Shrink(0).Padding(8)
					if i == a.workspaceSearchSel {
						row.Background(c.Theme().SurfaceHover)
						if a.workspaceReveal {
							row.ScrollIntoView()
							a.workspaceReveal = false
						}
					}
					row.OnClick(func() {
						a.workspaceSearchSel = i
						a.openSelectedWorkspaceHit()
					}).Children(func() {
						ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(10).Children(func() {
							ui.Icon(c, iconFile).Size(16, 16).TextColor(iconTint(c.Theme()))
							ui.Column(c).FillWidth().AlignItems(ui.Start).Gap(3).Children(func() {
								ui.Text(c, title).FontWeight(600).SingleLine().Ellipsis("…")
								ui.Text(c, h.Path).FontSize(10.5).TextColor(c.Theme().TextMuted).SingleLine().Ellipsis("…")
								ui.Text(c, strings.Join(strings.Fields(h.Snippet), " ")).FontSize(11.5).TextColor(c.Theme().TextMuted).SingleLine().Ellipsis("…")
							})
						})
					})
				}
			})
		})
	})
}
func (a *App) syncWorkspaceSearchQuery() {
	v := a.vault
	if v == nil || v.search == nil || (v.query == a.workspaceQuery && v.searchMode == v.searchLiteral) {
		return
	}
	if v.queryCancel != nil {
		v.queryCancel()
		v.queryCancel = nil
	}
	v.queryToken++
	v.query = a.workspaceQuery
	v.searchMode = v.searchLiteral
	v.hits = nil
	v.searchErr = nil
	v.searchBusy = v.query != ""
	a.workspaceSearchSel = 0
	a.workspaceScroll = ui.ScrollState{}
	a.workspaceReveal = true
	if v.query == "" {
		v.searchBusy = false
		v.due = time.Time{}
	} else {
		v.due = time.Now().Add(150 * time.Millisecond)
	}
}

func (a *App) navigateIndexed(path string, offset int) {
	if a.vault == nil {
		return
	}
	full := filepath.Join(a.vault.root, filepath.FromSlash(path))
	if full == a.path {
		a.ed.ScrollToOffset(editorOffset(a.diskSource(), offset))
		return
	}
	a.indexedPath, a.indexedOffset = full, offset
	a.requestOpen(full)
}

// navigateSearchHit verifies the result revision before using its byte offset.
// A changed snapshot is indexed and queried again before navigation proceeds.
func (a *App) navigateSearchHit(hit vaultsearch.Hit) {
	v := a.vault
	if v == nil || a.documents == nil || v.search == nil {
		return
	}
	full := filepath.Join(v.root, filepath.FromSlash(hit.Path))
	query, literal := a.workspaceQuery, v.searchLiteral
	currentPath, currentSource := a.path, a.diskSource()
	search, idx, store := v.search, v.index, a.documents.store
	go func() {
		ctx, cancel := context.WithTimeout(v.lifeCtx, 30*time.Second)
		defer cancel()
		var snapshot notestore.Document
		if filepath.Clean(full) == filepath.Clean(currentPath) {
			snapshot = notestore.Document{Path: currentPath, Text: currentSource, Revision: notestore.Revision(currentSource)}
		} else {
			var err error
			snapshot, err = store.Read(full)
			if err != nil {
				select {
				case v.navigation <- vaultNavigation{err: err}:
				case <-v.closed:
				}
				return
			}
		}
		if snapshot.Revision == hit.Revision {
			select {
			case v.navigation <- vaultNavigation{hit: hit}:
			case <-v.closed:
			}
			return
		}
		parsed, err := idx.ParseSnapshot(full, snapshot.Text)
		if err != nil {
			select {
			case v.navigation <- vaultNavigation{err: err}:
			case <-v.closed:
			}
			return
		}
		fresh := searchDocument(parsed)
		v.searchMu.Lock()
		idx.ApplySnapshot(parsed)
		err = search.Apply(ctx, []vaultsearch.Document{fresh}, nil)
		var hits []vaultsearch.Hit
		if err == nil {
			var overlays []vaultsearch.Document
			if currentPath != "" && filepath.Ext(currentPath) == ".md" {
				if currentDoc, parseErr := idx.ParseSnapshot(currentPath, currentSource); parseErr == nil {
					overlays = append(overlays, searchDocument(currentDoc))
				}
			}
			if literal {
				hits, err = search.Literal(ctx, query, 500, overlays...)
			} else {
				hits, err = search.Search(ctx, query, 500, overlays...)
			}
		}
		v.searchMu.Unlock()
		if err != nil {
			select {
			case v.navigation <- vaultNavigation{err: err}:
			case <-v.closed:
			}
			return
		}
		for _, current := range hits {
			if current.Path == hit.Path {
				select {
				case v.navigation <- vaultNavigation{hit: current}:
				case <-v.closed:
				}
				return
			}
		}
		select {
		case v.navigation <- vaultNavigation{err: errors.New("该笔记已不再匹配当前搜索")}:
		case <-v.closed:
		}
	}()
}
func (a *App) indexedLink(target string) bool {
	if a.vault == nil || a.vault.index == nil {
		return false
	}
	current, _ := filepath.Rel(a.vault.root, a.path)
	if loc, ok := a.vault.index.Resolve(filepath.ToSlash(current), target); ok {
		a.navigateIndexed(loc.Path, loc.Offset)
		return true
	}
	return false
}
func (a *App) linksDialog(c *ui.Context) {
	if a.vault == nil {
		return
	}
	v := a.vault
	ui.Modal(c, &v.linksOpen, func() {
		ui.Column(c).Width(540).MaxHeight(600).Padding(20).Gap(10).Background(c.Theme().Background).Radius(12).Children(func() {
			ui.Text(c, "关联笔记").FontWeight(700)
			if v.index == nil {
				ui.Text(c, "索引正在建立")
				return
			}
			rel, _ := filepath.Rel(v.root, a.path)
			rel = filepath.ToSlash(rel)
			quietScroll(c, &v.linkScroll, &v.linkIndicator).Height(360).FillWidth().Children(func() {
				ui.Column(c).FillWidth().Gap(6).Children(func() {
					ui.Text(c, "链接到这里")
					if v.linksBusy {
						ui.Text(c, "正在查找关联笔记…")
					}
					incoming := v.incoming
					if len(incoming) == 0 {
						ui.Text(c, "暂无反向链接")
					}
					for _, link := range groupedBacklinks(incoming) {
						l := link
						label := l.Source
						if l.Count > 1 {
							label += fmt.Sprintf("（%d 处）", l.Count)
						}
						if ui.Button(c, label).Label("反向链接：" + label).Clicked() {
							v.linksOpen = false
							a.navigateIndexed(l.Source, l.Start)
						}
					}
					ui.Text(c, "本文链接")
					for _, link := range v.outgoing {
						l := link
						if ui.Button(c, l.Target).Label("打开链接：" + l.Target).Clicked() {
							v.linksOpen = false
							a.indexedLink(l.Target)
						}
					}
				})
			})
			if ui.Button(c, "关闭").Clicked() {
				v.linksOpen = false
			}
		})
	})
}

type backlinkSource struct {
	Source string
	Start  int
	Count  int
}

func groupedBacklinks(links []vaultindex.Link) []backlinkSource {
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

func stopVault(v *vaultRuntime) {
	if v == nil || v.stopped {
		return
	}
	v.stopped = true
	if v.queryCancel != nil {
		v.queryCancel()
	}
	if v.lifeCancel != nil {
		v.lifeCancel()
	}
	close(v.closed)
	if v.watch != nil {
		_ = v.watch.Close()
	}
	if v.search != nil {
		go func() {
			v.searchMu.Lock()
			_ = v.search.Close()
			v.searchMu.Unlock()
			close(v.closeDone)
		}()
	} else if v.closeDone != nil {
		close(v.closeDone)
	}
}

func openVaultSearch(root string) (*vaultsearch.Index, error) {
	canonical, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	if resolved, resolveErr := filepath.EvalSymlinks(canonical); resolveErr == nil {
		canonical = resolved
	}
	canonical = filepath.Clean(canonical)
	base, err := cachepath.Root()
	if err != nil {
		return nil, err
	}
	key := sha256.Sum256([]byte(canonical))
	dir := filepath.Join(base, "vaultsearch", hex.EncodeToString(key[:]))
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	return vaultsearch.Open(dir)
}

func searchDocuments(docs []vaultindex.Document) []vaultsearch.Document {
	out := make([]vaultsearch.Document, 0, len(docs))
	for _, doc := range docs {
		out = append(out, searchDocument(doc))
	}
	return out
}

func searchDocument(doc vaultindex.Document) vaultsearch.Document {
	return vaultsearch.Document{
		ID: searchDocumentID(doc.Path), Path: doc.Path, Title: doc.Title, Source: doc.Source,
		Revision: notestore.Revision(doc.Source), Aliases: append([]string(nil), doc.Aliases...), Tags: append([]string(nil), doc.Tags...),
	}
}

func searchDocumentID(path string) string {
	key := sha256.Sum256([]byte(filepath.ToSlash(filepath.Clean(path))))
	return hex.EncodeToString(key[:])
}

func relativeVaultPath(root, path string) string {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return filepath.ToSlash(path)
	}
	return filepath.ToSlash(rel)
}

func (a *App) openLinks() {
	a.enableDocumentServices()
	v := a.vault
	if v == nil || v.index == nil {
		a.pendingLinks = true
		return
	}
	v.linksOpen = true
	v.linksBusy = true
	v.incoming = nil
	v.outgoing = nil
	rel, _ := filepath.Rel(v.root, a.path)
	rel = filepath.ToSlash(rel)
	idx := v.index
	go func() {
		r := linkResult{rel, idx.Backlinks(rel), idx.Outgoing(rel)}
		select {
		case v.linkResults <- r:
		default:
		}
		a.invalidate()
	}()
}

// Index positions address disk bytes; the editor normalizes CRLF internally.
func editorOffset(raw string, off int) int {
	off = max(0, min(off, len(raw)))
	return off - strings.Count(raw[:off], "\r\n")
}
