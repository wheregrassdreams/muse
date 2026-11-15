package nativeui

import (
	"os"
	"path/filepath"
	"testing"

	applicationvault "muse/application/vault"
)

func TestSessionReadyInvalidatesEmptyTreeCachedBeforeVaultStartup(t *testing.T) {
	root := t.TempDir()
	note := filepath.Join(root, "first.md")
	if err := os.WriteFile(note, []byte("# first"), 0o644); err != nil {
		t.Fatal(err)
	}

	a := newFileTreeTestApp(t, root)
	session := a.vault
	a.vault = nil
	a.vaultSnapshot = applicationvault.Snapshot{}

	// Opening the workspace happens before vaultTick starts its asynchronous
	// catalog build. This first read must not remain cached after SessionReady.
	a.setWorkspace(root)
	if got := a.children(root); got != nil {
		t.Fatalf("children before vault startup = %v, want cached empty result", got)
	}

	a.vault = session
	a.handleVaultResult(session, applicationvault.Result{
		Kind:     applicationvault.SessionReady,
		Snapshot: session.Snapshot(),
	})
	got := a.children(root)
	if len(got) != 1 || got[0] != note {
		t.Fatalf("children after SessionReady = %v, want [%s]", got, note)
	}
}
