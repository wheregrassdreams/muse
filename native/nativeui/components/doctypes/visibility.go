package doctypes

// IsVisibleWorkspacePath reports whether a path belongs in the workspace
// file catalog. Editable text documents and image documents are visible;
// other binary and unknown files are omitted.
func IsVisibleWorkspacePath(path string) bool {
	return IsEditablePath(path) || IsImagePath(path)
}
