package mdedit

import "testing"

func TestLeafArenaChunksReuseStableLeaves(t *testing.T) {
	var a leafArena
	first := a.get()
	first.kind = kPara
	first.conts = append(first.conts, cont{id: 42})
	details := first.ensureDetails()
	details.lines = append(details.lines, line{off: 1, text: "x"})
	details.lang = "source-backed"
	details.fenceEnd = 17
	first.view = &leafView{}
	first.geom = &tableGeom{}
	first.tab = &table{}
	first.table = first
	lineCap := cap(details.lines)
	ptrs := make([]*leaf, 2*leafArenaChunkSize+1)
	ptrs[0] = first
	for i := 1; i < len(ptrs); i++ {
		ptrs[i] = a.get()
	}
	seen := make(map[*leaf]struct{}, len(ptrs))
	for i, want := range ptrs {
		if got := a.leafAt(i); got != want {
			t.Fatalf("leafAt(%d) = %p, want %p", i, got, want)
		}
		if _, exists := seen[want]; exists {
			t.Fatalf("leaf %d reused an address in the same parse", i)
		}
		seen[want] = struct{}{}
	}
	if len(a.chunks) != 3 {
		t.Fatalf("chunk count = %d, want 3", len(a.chunks))
	}
	a.reset()
	if first.view != nil || first.geom != nil || first.tab != nil || first.table != nil {
		t.Fatal("arena reset retained shaped views or table references")
	}
	for i := range ptrs {
		if got := a.get(); got != ptrs[i] {
			t.Fatalf("leaf %d address changed across reset", i)
		}
	}
	if got := a.get(); got == ptrs[0] {
		t.Fatal("leaf address reused within a parse")
	}
	if a.n != len(ptrs)+1 {
		t.Fatalf("used leaves = %d, want %d", a.n, len(ptrs)+1)
	}
	if ptrs[0].kind != 0 || len(ptrs[0].conts) != 0 || len(ptrs[0].codeLines()) != 0 {
		t.Fatalf("reused leaf kept stale state: %+v", ptrs[0])
	}
	if ptrs[0].details != details || cap(ptrs[0].codeLines()) != lineCap {
		t.Fatal("reused leaf did not retain its sidecar and line capacity")
	}
	if details.lang != "" || details.fenceEnd != 0 {
		t.Fatal("arena reset retained sidecar source metadata")
	}
	for i, ln := range details.lines {
		if ln.text != "" || ln.off != 0 {
			t.Fatalf("arena reset retained line %d: %+v", i, ln)
		}
	}
}

func TestLeafArenaReleaseDropsChunksAndReferences(t *testing.T) {
	var a leafArena
	l := a.get()
	l.view = &leafView{}
	l.geom = &tableGeom{}
	l.tab = &table{}
	l.table = l
	l.conts = append(l.conts, cont{id: 1})
	a.release()
	if a.chunks != nil || a.n != 0 {
		t.Fatalf("released arena retained %d chunks and %d leaves", len(a.chunks), a.n)
	}
	if l.view != nil || l.geom != nil || l.tab != nil || l.table != nil || len(l.conts) != 0 {
		t.Fatal("released arena left references on a leaf")
	}
}

func TestSetSourceReleasesDocumentScopedState(t *testing.T) {
	e := New("same source", Options{})
	old := e.leaves[0]
	oldView := &leafView{}
	old.view = oldView
	e.views[viewKey{}] = oldView
	e.visibleLeaves = []*leaf{old}
	e.unitList = []*leaf{old}
	e.unitIndexCache.first = old
	e.railLeaves = []*leaf{old}
	e.railNext = []*leaf{old}
	e.imageHits = []imageHit{{obj: &object{}}}
	e.wordIndexCache = []*displayWordIndex{{}}
	e.tableEdgeIndex = []tableEdgePosition{{leaf: old}}
	e.wikiEmbeds = map[string]*wikiEmbedCache{"old": {editor: New("embedded", Options{})}}
	e.virtualBaseY = []float32{1}
	e.virtualHeightDelta = []float32{1}
	e.u16ix = u16Index{
		bytes: []int{1}, units: []int{1}, midB: []int{1}, midU: []int{1}, nb: []int{1}, nu: []int{1},
	}

	// Reopening identical content is still a new document lifetime: no leaf,
	// view, or cache from the previous Editor state should be carried over.
	e.SetSource("same source")
	if len(e.leaves) == 0 || e.leaves[0].view == oldView {
		t.Fatal("SetSource carried the previous document's leaf view")
	}
	if e.views == nil {
		t.Fatal("SetSource left the view cache uninitialized")
	}
	for _, view := range e.views {
		if view == oldView {
			t.Fatal("SetSource retained the previous document's cached view")
		}
	}
	if e.visibleLeaves != nil || e.unitList != nil || e.unitIndexCache.first != nil || len(e.railLeaves) != 0 || len(e.railNext) != 0 {
		t.Fatal("SetSource retained document leaf indexes")
	}
	if len(e.imageHits) != 0 || len(e.wordIndexCache) != 0 || len(e.tableEdgeIndex) != 0 || len(e.wikiEmbeds) != 0 {
		t.Fatal("SetSource retained document caches")
	}
	if e.virtualBaseY != nil || e.virtualHeightDelta != nil {
		t.Fatal("SetSource retained large-document layout indexes")
	}
	if e.u16ix.bytes != nil || e.u16ix.units != nil || e.u16ix.midB != nil || e.u16ix.midU != nil || e.u16ix.nb != nil || e.u16ix.nu != nil {
		t.Fatal("SetSource retained UTF-16 index buffers")
	}
	if e.arena[e.arenaCur].n == 0 || len(e.arena[e.arenaCur].chunks) != 1 || len(e.arena[1-e.arenaCur].chunks) != 0 {
		t.Fatal("SetSource did not replace the old parse arenas with only the new document's arena")
	}
}

func TestLeafDetailsOnlyAllocatedForColdContent(t *testing.T) {
	var a leafArena
	prose := a.get()
	prose.kind = kPara
	if prose.details != nil {
		t.Fatal("prose leaf allocated cold details")
	}
	raw := rawLeaf(&a, "line\n", 0, len("line\n"), false)
	if raw.details == nil || len(raw.codeLines()) != 1 || raw.codeLines()[0].text != "line" {
		t.Fatal("raw leaf did not retain its lines in cold details")
	}
}
