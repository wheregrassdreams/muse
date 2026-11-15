package workspace

// RecentEntry is a previously opened vault folder or document.
type RecentEntry struct {
	Path  string `json:"path"`
	IsDir bool   `json:"isDir,omitempty"`
}

// RecentRepository persists recent paths and checks whether they still exist.
type RecentRepository interface {
	Load() ([]RecentEntry, error)
	Save([]RecentEntry) error
	Exists(path string) bool
}

const (
	MaxRecentEntries = 10
	MaxVisibleRecent = 8
)

// RecentItems owns deduplication, ordering, and retention policy for the
// recently opened list. Filesystem serialization and path checks are ports.
type RecentItems struct {
	repository RecentRepository
	entries    []RecentEntry
}

func NewRecentItems(repository RecentRepository) *RecentItems {
	items := &RecentItems{repository: repository}
	if repository != nil {
		entries, err := repository.Load()
		if err == nil {
			for _, entry := range entries {
				if entry.Path != "" && len(items.entries) < MaxRecentEntries {
					items.entries = append(items.entries, entry)
				}
			}
		}
	}
	return items
}

func (r *RecentItems) Entries() []RecentEntry {
	if r == nil {
		return nil
	}
	return append([]RecentEntry(nil), r.entries...)
}

func (r *RecentItems) Add(path string, isDir bool) error {
	if r == nil || r.repository == nil {
		return ErrUnavailable
	}
	if path == "" {
		return nil
	}
	entries := make([]RecentEntry, 0, MaxRecentEntries)
	entries = append(entries, RecentEntry{Path: path, IsDir: isDir})
	for _, entry := range r.entries {
		if entry.Path != path && len(entries) < MaxRecentEntries {
			entries = append(entries, entry)
		}
	}
	r.entries = entries
	return r.repository.Save(r.Entries())
}

func (r *RecentItems) Live(limit int) []RecentEntry {
	if r == nil || r.repository == nil || limit <= 0 {
		return nil
	}
	entries := make([]RecentEntry, 0, min(limit, len(r.entries)))
	for _, entry := range r.entries {
		if r.repository.Exists(entry.Path) {
			entries = append(entries, entry)
			if len(entries) == limit {
				break
			}
		}
	}
	return entries
}
