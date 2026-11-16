package nativeui

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestVaultSessionLatestQueryWinsAndCapsResults(t *testing.T) {
	const count = 512
	notes := make(map[string]string, count)
	for n := 0; n < count; n++ {
		notes[fmt.Sprintf("Notes/%04d.md", n)] = "oldqueueprobe latestqueueprobe"
	}
	a, tt, _ := runtimeFixtureWithNotes(t, notes)
	if _, err := a.vault.Query("oldqueueprobe", true, a.documentPath(), a.diskSource()); err != nil {
		t.Fatal(err)
	}
	oldToken := a.vault.Snapshot().Search.Token
	if _, err := a.vault.Query("latestqueueprobe", true, a.documentPath(), a.diskSource()); err != nil {
		t.Fatal(err)
	}
	if a.vault.Snapshot().Search.Token <= oldToken {
		t.Fatal("new query did not supersede the previous token")
	}
	waitDocument(t, tt, func() bool {
		state := a.vault.Snapshot().Search
		return state.Query == "latestqueueprobe" && !state.Busy && len(state.Hits) == 500
	})
	state := a.vault.Snapshot().Search
	if state.Error != "" {
		t.Fatalf("latest query failed: %s", state.Error)
	}
	if state.Hits[0].Path != "Notes/0000.md" {
		t.Fatalf("unexpected top result: %+v", state.Hits[0])
	}
	for _, hit := range state.Hits {
		if !strings.HasPrefix(hit.Path, "Notes/") {
			t.Fatalf("unrelated result surfaced: %+v", hit)
		}
	}
	t.Logf("latest query completed in %s with %d capped results", state.Elapsed.Round(time.Millisecond), len(state.Hits))
}
