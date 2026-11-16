package vault

import (
	"path/filepath"
	"testing"

	"muse/document"
	"muse/search"
)

func TestIsCurrentSearchResult(t *testing.T) {
	result := Result{Kind: QueryCompleted, Token: 7, Query: "tag:work", Literal: false}

	tests := []struct {
		name   string
		search SearchSnapshot
		closed bool
		kind   ResultKind
		want   bool
	}{
		{name: "current", search: SearchSnapshot{Token: 7, Query: "tag:work"}, kind: QueryCompleted, want: true},
		{name: "old token", search: SearchSnapshot{Token: 8, Query: "tag:work"}, kind: QueryCompleted},
		{name: "old query", search: SearchSnapshot{Token: 7, Query: "tag:task"}, kind: QueryCompleted},
		{name: "old mode", search: SearchSnapshot{Token: 7, Query: "tag:work", Literal: true}, kind: QueryCompleted},
		{name: "not completion", search: SearchSnapshot{Token: 7, Query: "tag:work"}, kind: QueryStarted},
		{name: "closed", search: SearchSnapshot{Token: 7, Query: "tag:work"}, kind: QueryCompleted, closed: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			session := &Session{snapshot: Snapshot{Search: test.search}, closed: test.closed}
			candidate := result
			candidate.Kind = test.kind
			if got := session.IsCurrentSearchResult(candidate); got != test.want {
				t.Fatalf("IsCurrentSearchResult() = %t, want %t", got, test.want)
			}
		})
	}
}

func TestValidateSearchNavigationShortCircuitsMatchingActiveRevision(t *testing.T) {
	root := t.TempDir()
	session := &Session{root: root}
	path := filepath.Join(root, "note.md")
	status, err := session.ValidateSearchNavigation(nil, search.Hit{Path: "note.md", Revision: "revision"}, path, document.Revision("revision"))
	if err != nil || status != SearchHitCurrent {
		t.Fatalf("ValidateSearchNavigation() = %v, %v; want current without repository I/O", status, err)
	}
}

func TestValidateSearchNavigationRejectsPathsOutsideVault(t *testing.T) {
	session := &Session{root: t.TempDir()}
	status, err := session.ValidateSearchNavigation(nil, search.Hit{Path: "../outside.md"}, "", "")
	if err != ErrInvalidSearchHitPath || status != 0 {
		t.Fatalf("ValidateSearchNavigation() = %v, %v; want invalid hit path", status, err)
	}
}
