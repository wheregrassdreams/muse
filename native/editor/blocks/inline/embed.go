package inline

import (
	"path/filepath"
	"strings"
)

// IsImageName reports whether a wiki-embed target has an image extension.
// Resolution and loading are owned by the host; this package only classifies
// the source target.
func IsImageName(target string) bool {
	switch strings.ToLower(filepath.Ext(target)) {
	case ".png", ".jpg", ".jpeg", ".gif", ".webp", ".svg", ".bmp", ".avif", ".heic", ".heif":
		return true
	default:
		return false
	}
}
