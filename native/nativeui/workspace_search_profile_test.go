package nativeui

import (
	"os"
	"path/filepath"
	"runtime"
	"runtime/pprof"
	"testing"
	"time"
)

// TestProfileWorkspaceSearchRealVault is an opt-in end-to-end latency probe.
// It reads the requested vault but runtimeFixtureAt redirects application
// cache/config state to temporary directories. Set MUSE_PROFILE_VAULT to run.
func TestProfileWorkspaceSearchRealVault(t *testing.T) {
	root := os.Getenv("MUSE_PROFILE_VAULT")
	if root == "" {
		t.Skip("set MUSE_PROFILE_VAULT to a local Markdown vault")
	}
	root, err := filepath.Abs(root)
	if err != nil {
		t.Fatal(err)
	}
	var notePath string
	err = filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if notePath == "" && !entry.IsDir() && filepath.Ext(path) == ".md" {
			notePath = path
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if notePath == "" {
		t.Fatalf("vault %q has no Markdown notes", root)
	}

	started := time.Now()
	a, tt, _ := runtimeFixtureAt(t, root, notePath)
	deadline := time.Now().Add(2 * time.Minute)
	for time.Now().Before(deadline) {
		tt.Frame()
		if a.vault != nil && a.vaultSnapshot.Ready && a.vaultSnapshot.SearchAvailable {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if a.vault == nil || !a.vaultSnapshot.Ready || !a.vaultSnapshot.SearchAvailable {
		t.Fatal("vault index did not become ready within 2 minutes")
	}
	t.Logf("real vault index ready: %s", time.Since(started).Round(time.Millisecond))
	if profilePath := os.Getenv("MUSE_PROFILE_READY_HEAP_PATH"); profilePath != "" {
		writeProfileHeapSnapshot(t, profilePath)
	}

	a.openWorkspaceSearch()
	for _, query := range []string{"Cantor", "size", "集合", "的", "Cantor"} {
		typedAt := time.Now()
		a.workspaceQuery = query
		deadline = time.Now().Add(5 * time.Second)
		for time.Now().Before(deadline) {
			tt.Frame()
			state := a.vault.Snapshot().Search
			if state.Query == query && !state.Busy {
				if state.Error != "" {
					t.Fatalf("search %q failed: %v", query, state.Error)
				}
				t.Logf("query=%q end-to-end=%s search=%s estimate=%s next-coalesce=%s hits=%d", query,
					time.Since(typedAt).Round(time.Millisecond), state.Elapsed.Round(time.Microsecond),
					state.Cost.Round(time.Microsecond), workspaceSearchCoalesce(state.Cost, false), len(state.Hits))
				if query == "的" {
					first, last := a.workspaceSearchResults.Visible()
					t.Logf("500-result list built visible rows=%d (range %d..%d)", max(0, last-first+1), first, last)
				}
				break
			}
			time.Sleep(2 * time.Millisecond)
		}
		state := a.vault.Snapshot().Search
		if state.Query != query || state.Busy {
			t.Fatalf("search %q did not complete", query)
		}
	}

	parseAt := time.Now()
	if _, err := a.vault.Query("size", false, a.path, a.diskSource()); err != nil {
		t.Fatalf("start current-note overlay query: %v", err)
	}
	t.Logf("current-note overlay parse and search scheduling: %s (source=%d bytes)", time.Since(parseAt).Round(time.Microsecond), len(a.diskSource()))
	for _, query := range []string{"size", "的"} {
		for _, limit := range []int{100, 500} {
			searchAt := time.Now()
			if _, err := a.vault.Query(query, false, a.path, a.diskSource()); err != nil {
				t.Fatalf("Query: %v", err)
			}
			deadline := time.Now().Add(5 * time.Second)
			for time.Now().Before(deadline) && a.vault.Snapshot().Search.Busy {
				tt.Frame()
				time.Sleep(2 * time.Millisecond)
			}
			state := a.vault.Snapshot().Search
			hits := state.Hits
			if len(hits) > limit {
				hits = hits[:limit]
			}
			t.Logf("Session.Query(%q, limit=%d): %s hits=%d", query, limit, time.Since(searchAt).Round(time.Microsecond), len(hits))
		}
	}
	if profilePath := os.Getenv("MUSE_PROFILE_HEAP_PATH"); profilePath != "" {
		// Capture while the app, vault index, search projection, and rendered
		// result list are still reachable. `go test -memprofile` runs after test
		// cleanup and therefore misses most of this retained state.
		writeProfileHeapSnapshot(t, profilePath)
	}
	if pause, err := time.ParseDuration(os.Getenv("MUSE_PROFILE_PAUSE")); err == nil && pause > 0 {
		// Keep this isolated process alive briefly so macOS malloc_history can
		// inspect its stack-logged allocations before the test exits.
		t.Logf("holding profile process for %s", pause)
		time.Sleep(pause)
	}
}

func writeProfileHeapSnapshot(t *testing.T, path string) {
	t.Helper()
	runtime.GC()
	profile, err := os.Create(path)
	if err != nil {
		t.Fatalf("create live heap profile: %v", err)
	}
	if err := pprof.WriteHeapProfile(profile); err != nil {
		_ = profile.Close()
		t.Fatalf("write live heap profile: %v", err)
	}
	if err := profile.Close(); err != nil {
		t.Fatalf("close live heap profile: %v", err)
	}
}
