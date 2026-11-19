package nativeui

import (
	"testing"
	"time"

	vaultlinksapp "muse/application/vault/links"
)

func TestActiveLinkIndexQueueClearsWhenVaultIsUnavailable(t *testing.T) {
	now := time.Now()
	index := &vaultlinksapp.ActiveLinkIndex{}
	if _, scheduled := index.ScheduleDocument("notes/current.md", "[[stale]]", now); !scheduled {
		t.Fatal("fixture did not schedule Markdown source")
	}

	app := &App{activeLinkIndex: index}
	app.queueLinkIndexSnapshot()
	if _, remaining, ready := index.Take(now.Add(time.Hour)); ready || remaining != 0 {
		t.Fatalf("unavailable vault left link indexing pending: ready=%v remaining=%v", ready, remaining)
	}
}
