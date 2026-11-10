package vaultsearch_test

import (
	"context"
	"path/filepath"
	"reflect"
	"testing"

	"muse/vaultsearch"
)

func TestLiteralSourceSpansAndOccurrenceLimit(t *testing.T) {
	i := acceptanceIndex(t, filepath.Join(t.TempDir(), "cache"))
	ctx := context.Background()
	for _, tc := range []struct {
		source, query string
		want          [][2]int
	}{
		{"foo foo", "foo", [][2]int{{0, 3}, {4, 7}}},
		{"aaaa", "aa", [][2]int{{0, 2}, {2, 4}}},
		{"😀İxİ", "i", [][2]int{{4, 6}, {7, 9}}},
		{"中文中", "中", [][2]int{{0, 3}, {6, 9}}},
		{"a\r\nb", "a\r\nb", [][2]int{{0, 4}}},
		{"title:x *", "title:x *", [][2]int{{0, 9}}},
		{"foo", " foo ", [][2]int{{0, 3}}},
		{"ς", "σ", nil},
		{"ß", "ss", nil},
		{"body", " \n ", nil},
	} {
		t.Run(tc.source+tc.query, func(t *testing.T) {
			d := vaultsearch.Document{ID: "one", Path: "one.md", Source: tc.source}
			if err := i.Reconcile(ctx, []vaultsearch.Document{d}); err != nil {
				t.Fatal(err)
			}
			hits, err := i.Literal(ctx, tc.query, 100)
			if err != nil {
				t.Fatal(err)
			}
			var got [][2]int
			for _, h := range hits {
				got = append(got, [2]int{h.Start, h.End})
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
		})
	}
	d := vaultsearch.Document{ID: "one", Path: "z.md", Source: "foo foo foo"}
	if err := i.Reconcile(ctx, []vaultsearch.Document{d}); err != nil {
		t.Fatal(err)
	}
	overlay := vaultsearch.Document{ID: "new", Path: "a.md", Source: "foo foo"}
	hits, err := i.Literal(ctx, "foo", 3, overlay)
	if err != nil || len(hits) != 3 || hits[0].Path != "a.md" || hits[1].Path != "a.md" || hits[2].Path != "z.md" {
		t.Fatalf("overlay must participate in global path/occurrence ordering: %v, %v", hits, err)
	}
	if hits[0].Start != 0 || hits[1].Start != 4 || hits[2].Start != 0 {
		t.Fatal(hits)
	}
	hits, err = i.Literal(ctx, "foo", 100, vaultsearch.Document{ID: "one", Path: "z.md", Source: "removed"})
	if err != nil || len(hits) != 0 {
		t.Fatalf("overlay did not hide disk snapshot: %v, %v", hits, err)
	}
	hits, err = i.Literal(ctx, "foo", 100)
	if err != nil || len(hits) != 3 {
		t.Fatalf("overlay mutated index: %v, %v", hits, err)
	}
}
