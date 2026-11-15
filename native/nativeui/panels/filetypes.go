package nativeui

import (
	"path/filepath"
	"strings"
	"unicode/utf8"
)

var imageExtensions = map[string]bool{
	"png": true, "jpg": true, "jpeg": true, "gif": true, "bmp": true,
	"webp": true, "svg": true, "avif": true,
}

func imageFileExtensions() []string {
	return []string{"png", "jpg", "jpeg", "gif", "bmp", "webp", "svg", "avif"}
}

func isImageFile(path string) bool {
	return imageExtensions[strings.ToLower(strings.TrimPrefix(filepath.Ext(path), "."))]
}

func isVisibleWorkspaceFile(path string) bool { return isEditableFile(path) || isImageFile(path) }

// editableExtensions is the allowlist for files Muse opens in the editor.
// Image media has a separate allowlist: the tree opens it in Muse's image
// preview instead of reading it into the text editor.
var editableExtensions = []string{
	"md", "markdown", "mdx", "txt", "text",
	"json", "jsonc", "json5", "yaml", "yml", "toml", "ini", "conf", "cfg",
	"csv", "tsv", "log",
	"go", "rs", "py", "rb", "php", "java", "kt", "kts",
	"c", "h", "cc", "cpp", "cxx", "hpp", "m", "mm", "swift",
	"js", "jsx", "ts", "tsx", "css", "scss", "sass", "less",
	"html", "htm", "xml", "sh", "bash", "zsh", "fish", "ps1", "bat", "cmd",
	"sql", "graphql", "gql", "proto", "vue", "svelte", "astro",
	"tex", "bib", "org", "rst", "adoc", "properties", "env",
	"mod", "sum", "gradle", "mk", "cmake", "plist", "diff", "patch",
}

var editableExtSet = func() map[string]bool {
	m := make(map[string]bool, len(editableExtensions))
	for _, e := range editableExtensions {
		m[e] = true
	}
	return m
}()

var editableNames = map[string]bool{
	"readme": true, "license": true, "licence": true, "changelog": true,
	"authors": true, "notice": true, "makefile": true, "dockerfile": true,
	"procfile": true, "gemfile": true, "rakefile": true,
}

func isEditableFile(path string) bool {
	ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(path), "."))
	if ext != "" {
		return editableExtSet[ext]
	}
	return editableNames[strings.ToLower(filepath.Base(path))]
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
	return append([]string(nil), editableExtensions...)
}
