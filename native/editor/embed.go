package mdedit

import (
	"path/filepath"
	"strings"
)

// isImageName reports whether a `![[…]]` target names a picture the editor
// can draw, so that `![[图.png]]` embeds it and `![[笔记]]` stays a label.
func isImageName(target string) bool {
	switch strings.ToLower(filepath.Ext(target)) {
	case ".png", ".jpg", ".jpeg", ".gif", ".webp", ".svg", ".bmp", ".avif", ".heic", ".heif":
		return true
	}
	return false
}
