package iconlibrary

import "testing"

func TestCatalogIsLargeSearchableAndLoadsSVGOnDemand(t *testing.T) {
	all := Catalog()
	if len(all) < 2000 {
		t.Fatalf("catalog has %d icons, expected a broad Lucide collection", len(all))
	}
	if zipFiles != nil {
		t.Fatal("loading catalog metadata eagerly opened the SVG archive")
	}
	for _, id := range []string{"book-open", "accessibility", "image", "wifi-sync"} {
		icon, ok := Lookup(id)
		if !ok || icon.ID != id || icon.Name == "" || icon.SVG == "" {
			t.Errorf("Lookup(%q) = %+v, %v", id, icon, ok)
		}
	}
	results := Search("image")
	if len(results) == 0 {
		t.Fatal("tag search did not return image icons")
	}
	found := false
	for _, icon := range results {
		if icon.ID == "image" {
			found = true
		}
	}
	if !found {
		t.Fatalf("tag search omitted image icon: %v", ids(results))
	}
}

func TestCatalogEntriesAreSafeToModify(t *testing.T) {
	first := Catalog()
	first[0].ID = "changed"
	if len(first[0].Tags) > 0 {
		first[0].Tags[0] = "changed"
	}
	got, ok := Lookup("a-arrow-down")
	if !ok || got.ID != "a-arrow-down" || got.Tags[0] == "changed" {
		t.Fatalf("caller mutation changed bundled catalog: %+v", got)
	}
}

func ids(icons []Icon) []string {
	out := make([]string, len(icons))
	for i, icon := range icons {
		out[i] = icon.ID
	}
	return out
}
