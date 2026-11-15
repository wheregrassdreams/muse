// Package cachepath defines Muse's shared root for disposable, rebuildable
// application data. Vault content and user-authored files must never be
// stored here.
package cachepath

import (
	"errors"
	"os"
	"path/filepath"
)

const (
	// RootEnv overrides the complete Muse cache root. Tests should set this to
	// a dedicated temporary directory so no test data reaches a user cache.
	RootEnv = "MUSE_CACHE_DIR"
	appDir  = "muse"
)

// Root returns the shared cache root. It uses an explicit Muse override,
// then XDG_CACHE_HOME, then the XDG default of ~/.cache. The returned path is
// not created; callers create only the category directory they need.
func Root() (string, error) {
	if override := os.Getenv(RootEnv); override != "" {
		return filepath.Abs(filepath.Clean(override))
	}
	base := os.Getenv("XDG_CACHE_HOME")
	if !filepath.IsAbs(base) {
		base = ""
	}
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil || home == "" {
			return "", errors.New("cachepath: cannot determine XDG cache home")
		}
		base = filepath.Join(home, ".cache")
	}
	return filepath.Abs(filepath.Join(base, appDir))
}
