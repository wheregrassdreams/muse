package nativeui

import (
	"strings"
	"unicode/utf8"

	"muse/nativeui/components/doctypes"
)

func imageFileExtensions() []string {
	extensions := doctypes.ImageExtensions()
	for i := range extensions {
		extensions[i] = strings.TrimPrefix(extensions[i], ".")
	}
	return extensions
}

func isImageFile(path string) bool {
	return doctypes.IsImagePath(path)
}

func isVisibleWorkspaceFile(path string) bool { return isEditableFile(path) || isImageFile(path) }

func isEditableFile(path string) bool {
	return doctypes.IsEditablePath(path)
}

// isTextData rejects binaries that happen to use a text extension (or a
// renamed image) by checking for NUL bytes and invalid UTF-8.
func isTextData(data []byte) bool {
	if len(data) == 0 {
		return true
	}
	sample := data
	if len(sample) > 8192 {
		sample = sample[:8192]
	}
	for _, b := range sample {
		if b == 0 {
			return false
		}
	}
	return utf8.Valid(data)
}

// textFilter is shared by Open/Save dialogs.
func textFileFilter() []string {
	return doctypes.EditableExtensions()
}
