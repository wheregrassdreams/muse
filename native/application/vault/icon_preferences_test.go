package vault

import (
	"errors"
	"testing"
)

type memoryIconRepository struct{ vaults map[string]map[string]string }

func (r *memoryIconRepository) LoadVault(root string) (map[string]string, error) {
	items := make(map[string]string)
	for key, value := range r.vaults[root] {
		items[key] = value
	}
	return items, nil
}

func (r *memoryIconRepository) SaveVault(root string, icons map[string]string) error {
	items := make(map[string]string)
	for key, value := range icons {
		items[key] = value
	}
	r.vaults[root] = items
	return nil
}

func TestIconPreferencesNormalizesAndScopesVaultItems(t *testing.T) {
	repo := &memoryIconRepository{vaults: make(map[string]map[string]string)}
	icons, err := NewIconPreferences(repo)
	if err != nil {
		t.Fatal(err)
	}
	if err := icons.Set("vault-a", `Projects\Atlas//Overview.md`, " bookmark "); err != nil {
		t.Fatal(err)
	}
	if got := icons.Get("vault-a", "Projects/Atlas/Overview.md"); got != "bookmark" {
		t.Fatalf("normalized icon = %q", got)
	}
	if got := icons.Get("vault-b", "Projects/Atlas/Overview.md"); got != "" {
		t.Fatalf("another vault inherited icon %q", got)
	}
	if err := icons.Delete("vault-a", "Projects/Atlas/Overview.md"); err != nil {
		t.Fatal(err)
	}
	if got := icons.Get("vault-a", "Projects/Atlas/Overview.md"); got != "" {
		t.Fatalf("deleted icon = %q", got)
	}
}

func TestIconPreferencesRejectsUnsafePathsAndIDs(t *testing.T) {
	icons, err := NewIconPreferences(&memoryIconRepository{vaults: make(map[string]map[string]string)})
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range []string{"", "../outside.md", "/absolute.md", `C:\outside.md`} {
		if err := icons.Set("vault", item, "star"); err == nil {
			t.Errorf("accepted unsafe item path %q", item)
		}
	}
	for _, id := range []string{"", "bad\nicon"} {
		if err := icons.Set("vault", "note.md", id); err == nil {
			t.Errorf("accepted invalid icon ID %q", id)
		}
	}
}

type failingIconRepository struct{}

func (failingIconRepository) LoadVault(string) (map[string]string, error) {
	return nil, errors.New("read failed")
}
func (failingIconRepository) SaveVault(string, map[string]string) error {
	return errors.New("write failed")
}

func TestIconPreferencesPropagatesRepositoryFailures(t *testing.T) {
	icons, err := NewIconPreferences(failingIconRepository{})
	if err != nil {
		t.Fatal(err)
	}
	if err := icons.Set("vault", "note.md", "star"); err == nil {
		t.Fatal("Set ignored repository read failure")
	}
	if got := icons.Get("vault", "note.md"); got != "" {
		t.Fatalf("Get after repository failure = %q", got)
	}
}
