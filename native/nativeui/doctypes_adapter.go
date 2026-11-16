package nativeui

import (
	"strings"

	"muse/nativeui/components/doctypes"
)

func imageFileExtensions() []string {
	extensions := doctypes.ImageExtensions()
	for i := range extensions {
		extensions[i] = strings.TrimPrefix(extensions[i], ".")
	}
	return extensions
}

// textFileFilter returns the explicit editable suffixes shared by Open and
// Save dialogs. UI-specific removal of leading dots stays in this adapter.
func textFileFilter() []string {
	return doctypes.EditableExtensions()
}
