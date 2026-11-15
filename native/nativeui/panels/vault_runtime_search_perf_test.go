package nativeui

import (
	"context"
	"fmt"
	"muse/vaultsearch"
	"strings"
	"testing"
	"time"
)

// This exercises the same serialization boundary used by watcher Apply and
// interactive searches. A canceled query is deliberately queued behind a
// large real Apply while a newer query is entered.
func TestWorkspaceSearchLatestQueryWhileApplyOwnsSearchLock(t *testing.T) {
	const count = 512
	notes := make(map[string]string, count)
	for n := 0; n < count; n++ {
		notes[fmt.Sprintf("Notes/%04d.md", n)] = "oldqueueprobe"
	}
	a, tt, _ := runtimeFixtureWithNotes(t, notes)
	updates := make([]vaultsearch.Document, 0, count)
	for _, doc := range a.vault.index.Snapshot() {
		if strings.HasPrefix(doc.Path, "Notes/") {
			doc.Source = "latestqueueprobe"
			updates = append(updates, searchDocument(doc))
		}
	}
	if len(updates) != count {
		t.Fatalf("prepared %d update docs, want %d", len(updates), count)
	}

	applyStarted := make(chan struct{})
	applyDone := make(chan error, 1)
	go func() {
		a.vault.searchMu.Lock()
		close(applyStarted)
		err := a.vault.search.Apply(context.Background(), updates, nil)
		a.vault.searchMu.Unlock()
		applyDone <- err
	}()
	<-applyStarted

	a.openWorkspaceSearch()
	a.vault.searchLiteral = true
	a.workspaceQuery = "oldqueueprobe"
	waitDocument(t, tt, func() bool { return a.vault.due.IsZero() && a.vault.searchBusy })
	// The old query has started and is waiting on searchMu. Change the query
	// while Apply is still expected to own the lock.
	if a.vault.queryToken == 0 {
		t.Fatal("old query was not scheduled")
	}
	oldToken := a.vault.queryToken
	a.workspaceQuery = "latestqueueprobe"
	typedAt := time.Now()
	tt.Frame()
	if a.vault.queryToken <= oldToken {
		t.Fatal("latest query did not supersede the queued query")
	}

	select {
	case err := <-applyDone:
		if err != nil {
			t.Fatalf("Apply: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("large Apply did not complete")
	}
	waitDocument(t, tt, func() bool {
		return !a.vault.searchBusy && len(a.vault.hits) == 500 && a.vault.hits[0].Path == "Notes/0000.md"
	})
	t.Logf("latest query visible after %s from input (Apply lock wait included)", time.Since(typedAt).Round(time.Millisecond))
	for _, hit := range a.vault.hits {
		if !strings.HasPrefix(hit.Path, "Notes/") {
			t.Fatalf("stale or unrelated result surfaced: %+v", hit)
		}
	}
}
