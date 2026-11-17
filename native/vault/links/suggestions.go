package links

import (
	"net/url"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

// Suggestion is the immutable display projection for one wiki-link target.
// Target always uses a vault-relative slash-separated path. Markdown note
// targets omit their extension; image targets retain theirs.
type Suggestion struct {
	Target  string
	Title   string
	Aliases []string
}

// SuggestionCache owns the derived wiki completion projection for one
// consumer scope. The owner key is normally the vault session pointer; a
// replacement session therefore cannot reuse a previous vault's projection
// even when its root and revision counters happen to match.
type SuggestionCache[K comparable] struct {
	mu sync.RWMutex

	owner         K
	root          string
	indexRevision uint64
	filesRevision uint64
	items         []Suggestion
	valid         bool
}

// Get returns the cached projection only when its vault session and both
// source revisions still match. Returned values are detached from the cache.
func (c *SuggestionCache[K]) Get(owner K, root string, indexRevision, filesRevision uint64) ([]Suggestion, bool) {
	if c == nil {
		return nil, false
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	if !c.valid || c.owner != owner || c.root != root || c.indexRevision != indexRevision || c.filesRevision != filesRevision {
		return nil, false
	}
	return cloneSuggestions(c.items), true
}

// Store replaces the cached projection with a detached copy of one immutable
// source snapshot.
func (c *SuggestionCache[K]) Store(owner K, root string, indexRevision, filesRevision uint64, items []Suggestion) {
	if c == nil {
		return
	}
	c.mu.Lock()
	c.owner, c.root = owner, root
	c.indexRevision, c.filesRevision = indexRevision, filesRevision
	c.items = cloneSuggestions(items)
	c.valid = true
	c.mu.Unlock()
}

// Clear drops the vault-scoped projection and releases its retained strings.
func (c *SuggestionCache[K]) Clear() {
	if c == nil {
		return
	}
	c.mu.Lock()
	var zero K
	c.owner, c.root = zero, ""
	c.indexRevision, c.filesRevision = 0, 0
	c.items = nil
	c.valid = false
	c.mu.Unlock()
}

// WikiSuggestions combines indexed note metadata and the vault's image
// catalog into stable, path-disambiguated completion targets. It performs no
// filesystem access; callers supply detached vault-relative snapshots.
func WikiSuggestions(documents []Document, imagePaths []string) []Suggestion {
	out := make([]Suggestion, 0, len(documents)+len(imagePaths))
	for _, doc := range documents {
		path := cleanPath(doc.Path)
		if !isMarkdown(path) {
			continue
		}
		target := strings.TrimSuffix(path, filepath.Ext(filepath.FromSlash(path)))
		out = append(out, Suggestion{
			Target:  target,
			Title:   doc.Title,
			Aliases: append([]string(nil), doc.Aliases...),
		})
	}
	for _, imagePath := range imagePaths {
		path := cleanPath(imagePath)
		if path == "" {
			continue
		}
		out = append(out, Suggestion{
			Target: path,
			Title:  filepath.Base(filepath.FromSlash(path)),
		})
	}
	sort.Slice(out, func(a, b int) bool { return out[a].Target < out[b].Target })
	return out
}

func cloneSuggestions(items []Suggestion) []Suggestion {
	if items == nil {
		return nil
	}
	out := make([]Suggestion, len(items))
	for i, item := range items {
		out[i] = item
		out[i].Aliases = append([]string(nil), item.Aliases...)
	}
	return out
}

// ResolveImageTarget resolves a wiki-link image target against one immutable
// vault image catalog. The returned path is vault-relative and slash
// separated. Resolution prefers the current note's directory, then the
// vault root, and finally a deterministic basename match for bare names.
//
// This is deliberately a catalog lookup rather than a filesystem resolver:
// the catalog is the authority for whether a target is a vault image, while
// Markdown's image source resolver remains responsible for local-file access,
// symlink confinement, remote URLs, and decoding.
func ResolveImageTarget(root, currentPath, target string, imagePaths []string) (string, bool) {
	if strings.TrimSpace(root) == "" {
		return "", false
	}
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return "", false
	}
	target = strings.TrimSpace(strings.Trim(target, "<>"))
	if target == "" {
		return "", false
	}
	if at := strings.IndexAny(target, "#^"); at >= 0 {
		target = target[:at]
	}
	lowerTarget := strings.ToLower(target)
	if target == "" || strings.HasPrefix(lowerTarget, "http://") || strings.HasPrefix(lowerTarget, "https://") || strings.HasPrefix(lowerTarget, "data:") {
		return "", false
	}
	if strings.HasPrefix(lowerTarget, "file://") {
		target = target[len("file://"):]
	}
	if decoded, decodeErr := url.PathUnescape(target); decodeErr == nil {
		target = decoded
	}
	target = strings.ReplaceAll(target, "\\", "/")

	available := make(map[string]struct{}, len(imagePaths))
	for _, imagePath := range imagePaths {
		path := cleanPath(imagePath)
		if path != "" {
			available[path] = struct{}{}
		}
	}
	contains := func(path string) bool {
		path = cleanPath(path)
		if path == "" || strings.HasPrefix(path, "../") || path == ".." {
			return false
		}
		_, ok := available[path]
		return ok
	}

	local := filepath.FromSlash(target)
	if filepath.IsAbs(local) {
		rel, relErr := filepath.Rel(rootAbs, filepath.Clean(local))
		if relErr != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return "", false
		}
		path := cleanPath(rel)
		if contains(path) {
			return path, true
		}
		return "", false
	}

	current := vaultRelativePath(rootAbs, currentPath)
	base := filepath.ToSlash(filepath.Dir(filepath.FromSlash(current)))
	targetPath := filepath.ToSlash(local)
	var candidates []string
	if base != "." && base != "" {
		candidates = append(candidates, cleanPath(filepath.ToSlash(filepath.Join(filepath.FromSlash(base), filepath.FromSlash(targetPath)))))
	}
	candidates = append(candidates, cleanPath(targetPath))
	for _, candidate := range candidates {
		if contains(candidate) {
			return candidate, true
		}
	}

	// A catalog basename fallback mirrors the vault's attachment lookup, but
	// is intentionally restricted to unqualified names so an explicit path
	// never resolves to a same-named image in another directory.
	if strings.ContainsAny(targetPath, "/\\") {
		return "", false
	}
	wanted := strings.ToLower(filepath.Base(filepath.FromSlash(targetPath)))
	best := ""
	for candidate := range available {
		if !strings.EqualFold(filepath.Base(filepath.FromSlash(candidate)), wanted) {
			continue
		}
		if best == "" || len(candidate) < len(best) || len(candidate) == len(best) && candidate < best {
			best = candidate
		}
	}
	return best, best != ""
}

func vaultRelativePath(root, current string) string {
	if current == "" {
		return ""
	}
	if filepath.IsAbs(current) {
		rel, err := filepath.Rel(root, filepath.Clean(current))
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return ""
		}
		return cleanPath(rel)
	}
	return cleanPath(current)
}
