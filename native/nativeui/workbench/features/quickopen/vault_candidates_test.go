package quickopen

import (
	"testing"

	applicationvault "muse/application/vault"
)

type testVaultQuickOpen struct {
	snapshot applicationvault.Snapshot
	matches  []applicationvault.QuickOpenMatch
	queries  []string
}

func (v *testVaultQuickOpen) QuickOpen(query string) []applicationvault.QuickOpenMatch {
	v.queries = append(v.queries, query)
	return v.matches
}

func (v *testVaultQuickOpen) Snapshot() applicationvault.Snapshot { return v.snapshot }

func TestVaultCandidatesAdaptsReadySessionAndRevision(t *testing.T) {
	vault := &testVaultQuickOpen{
		snapshot: applicationvault.Snapshot{Ready: true, FilesRevision: 3, IndexRevision: 7},
		matches: []applicationvault.QuickOpenMatch{{
			Path: "/vault/note.md", PathText: "note.md", Title: "Note",
			TitleMatches: []int{0}, PathMatches: []int{0, 1},
		}},
	}
	source := NewVaultCandidates(vault)
	if got := source.Revision(); got != (Revision{Catalog: 3, Index: 7}) {
		t.Fatalf("revision = %+v", got)
	}
	rows := source.Rows("no")
	if rows.Len() != 1 {
		t.Fatalf("row count = %d", rows.Len())
	}
	row := rows.At(0)
	if row.Path != "/vault/note.md" || row.Title != "Note" || row.PathText != "note.md" {
		t.Fatalf("row projection = %+v", row)
	}
	row.TitleMatches[0] = 99
	if vault.matches[0].TitleMatches[0] != 0 {
		t.Fatal("candidate source exposed mutable application match slices")
	}
	if len(vault.queries) != 1 || vault.queries[0] != "no" {
		t.Fatalf("queries = %v", vault.queries)
	}
}

func TestVaultCandidatesSwitchesSessionsAndIgnoresUnreadyVault(t *testing.T) {
	first := &testVaultQuickOpen{
		snapshot: applicationvault.Snapshot{Ready: false, FilesRevision: 4, IndexRevision: 9},
		matches:  []applicationvault.QuickOpenMatch{{Path: "/old.md"}},
	}
	source := NewVaultCandidates(first)
	if got := source.Revision(); got != (Revision{Catalog: 4, Index: 9}) {
		t.Fatalf("unready revision = %+v", got)
	}
	if got := source.Rows("old"); got.Len() != 0 {
		t.Fatalf("unready vault returned %d rows", got.Len())
	}

	second := &testVaultQuickOpen{
		snapshot: applicationvault.Snapshot{Ready: true, FilesRevision: 11, IndexRevision: 12},
		matches:  []applicationvault.QuickOpenMatch{{Path: "/new.md", Title: "New"}},
	}
	source.SetVault(second)
	if got := source.Revision(); got != (Revision{Catalog: 11, Index: 12}) {
		t.Fatalf("switched revision = %+v", got)
	}
	if rows := source.Rows(""); rows.Len() != 1 || rows.At(0).Path != "/new.md" {
		t.Fatalf("switched rows = %+v", rows)
	}
	source.SetVault(nil)
	if got := source.Revision(); got != (Revision{}) || source.Rows("").Len() != 0 {
		t.Fatalf("cleared source retained values: revision=%+v", got)
	}
}
