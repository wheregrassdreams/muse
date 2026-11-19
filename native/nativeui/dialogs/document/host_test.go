package documentdialogs

import (
	"errors"
	"testing"

	documentapp "muse/application/document"
)

func TestMigrationHostPreviewsAndAppliesViaTypedPorts(t *testing.T) {
	handle := NewHandle()
	source := MigrationSource{Ready: true, Token: "vault-1", Root: "/vault", OpenPath: "/vault/open.md"}
	var appliedOpenPath string
	var refreshed []string
	var notices []string
	invalidations := 0
	host := NewMigrationHost(handle, MigrationPorts{
		Source:    func() MigrationSource { return source },
		IsCurrent: func(candidate MigrationSource) bool { return candidate.Token == source.Token },
		Preview: func(root, openPath string, _ []documentapp.CoreMetadataCandidate) (documentapp.CoreMetadataPlan, error) {
			if root != "/vault" || openPath != "/vault/open.md" {
				t.Fatalf("preview input root=%q openPath=%q", root, openPath)
			}
			return documentapp.CoreMetadataPlan{Root: root, Changes: []documentapp.CoreMetadataChange{{Path: "/vault/other.md"}}}, nil
		},
		Apply: func(_ documentapp.CoreMetadataPlan, openPath string) (documentapp.CoreMetadataApplyResult, error) {
			appliedOpenPath = openPath
			return documentapp.CoreMetadataApplyResult{AppliedPaths: []string{"/vault/other.md"}}, nil
		},
		Refresh: func(candidate MigrationSource, paths []string) {
			if candidate.Token != "vault-1" {
				t.Fatalf("refresh source token = %q", candidate.Token)
			}
			refreshed = append(refreshed, paths...)
		},
		Go:         func(work func()) { work() },
		OnMain:     func(work func()) { work() },
		Notify:     func(message string) { notices = append(notices, message) },
		Invalidate: func() { invalidations++ },
	})
	host.OpenPreview()
	if state := host.Snapshot(); !state.Open || state.Loading || !state.HasPlan {
		t.Fatalf("after preview = %#v", state)
	}
	if invalidations < 2 {
		t.Fatalf("invalidations = %d", invalidations)
	}
	// The current open document can change after preview; apply uses its latest
	// path while the preview remains bound to the same vault token.
	source.OpenPath = "/vault/new-open.md"
	host.HandleAction(Action{Kind: ApplyCoreMigration})
	if appliedOpenPath != source.OpenPath {
		t.Fatalf("applied open path = %q, want %q", appliedOpenPath, source.OpenPath)
	}
	if len(refreshed) != 1 || refreshed[0] != "/vault/other.md" {
		t.Fatalf("refreshed paths = %#v", refreshed)
	}
	if state := host.Snapshot(); state.Running || state.HasPlan {
		t.Fatalf("after apply = %#v", state)
	}
	if len(notices) != 1 {
		t.Fatalf("notices = %#v", notices)
	}
}

func TestMigrationHostDiscardsStalePreview(t *testing.T) {
	handle := NewHandle()
	source := MigrationSource{Ready: true, Token: "old", Root: "/vault"}
	host := NewMigrationHost(handle, MigrationPorts{
		Source:    func() MigrationSource { return source },
		IsCurrent: func(candidate MigrationSource) bool { return candidate.Token == source.Token },
		Preview: func(string, string, []documentapp.CoreMetadataCandidate) (documentapp.CoreMetadataPlan, error) {
			source.Token = "new"
			return documentapp.CoreMetadataPlan{}, nil
		},
		Go:     func(work func()) { work() },
		OnMain: func(work func()) { work() },
	})
	host.OpenPreview()
	if state := host.Snapshot(); state.Open || state.Loading || state.HasPlan {
		t.Fatalf("stale preview retained: %#v", state)
	}
}

func TestMigrationHostReportsPreviewFailure(t *testing.T) {
	var notices []string
	host := NewMigrationHost(nil, MigrationPorts{
		Source: func() MigrationSource { return MigrationSource{Ready: true, Token: "t", Root: "/vault"} },
		Preview: func(string, string, []documentapp.CoreMetadataCandidate) (documentapp.CoreMetadataPlan, error) {
			return documentapp.CoreMetadataPlan{}, errors.New("preview failed")
		},
		Go:     func(work func()) { work() },
		OnMain: func(work func()) { work() },
		Notify: func(message string) { notices = append(notices, message) },
	})
	host.OpenPreview()
	if state := host.Snapshot(); state.Loading || state.HasPlan {
		t.Fatalf("after failed preview = %#v", state)
	}
	if len(notices) != 1 || notices[0] != "preview failed" {
		t.Fatalf("notices = %#v", notices)
	}
}

func TestMigrationHostReportsApplyFailure(t *testing.T) {
	var notices []string
	host := NewMigrationHost(nil, MigrationPorts{
		Source: func() MigrationSource { return MigrationSource{Ready: true, Token: "t", Root: "/vault"} },
		Preview: func(string, string, []documentapp.CoreMetadataCandidate) (documentapp.CoreMetadataPlan, error) {
			return documentapp.CoreMetadataPlan{Root: "/vault", Changes: []documentapp.CoreMetadataChange{{Path: "/vault/note.md"}}}, nil
		},
		Apply: func(documentapp.CoreMetadataPlan, string) (documentapp.CoreMetadataApplyResult, error) {
			return documentapp.CoreMetadataApplyResult{}, errors.New("apply failed")
		},
		Go:     func(work func()) { work() },
		OnMain: func(work func()) { work() },
		Notify: func(message string) { notices = append(notices, message) },
	})
	host.OpenPreview()
	host.HandleAction(Action{Kind: ApplyCoreMigration})
	if state := host.Snapshot(); state.Running || !state.HasPlan {
		t.Fatalf("after failed apply = %#v", state)
	}
	if len(notices) != 1 || notices[0] != "apply failed" {
		t.Fatalf("notices = %#v", notices)
	}
}
