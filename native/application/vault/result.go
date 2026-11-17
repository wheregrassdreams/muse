package vault

import "path/filepath"

// TouchesPath reports whether a filesystem-refresh result includes the given
// absolute path. Both sides are normalized because watcher and catalog
// adapters may report equivalent paths with different clean forms.
func (r Result) TouchesPath(path string) bool {
	if path == "" {
		return false
	}
	want := filepath.Clean(path)
	for _, changed := range r.Changed {
		if filepath.Clean(changed) == want {
			return true
		}
	}
	return false
}
