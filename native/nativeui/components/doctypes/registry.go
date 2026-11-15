// Package doctypes decides how a file opens. A Type maps file extensions to a
// parser and a native view, and Registry resolves a path to its type.
//
// This is the plugin seam: registering a Type is what makes a new kind of
// file openable in Muse. Markdown and plain text are built in, and any
// extension not known to be binary falls back to plain text, so a .txt,
// .log, .csv or .conf opens and is editable the same way Markdown is.
package doctypes

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/egoist/mygo/ui"

	mdoc "muse/markdown"
	markdownui "muse/nativeui/components/markdown"
)

// Type is one kind of openable document.
type Type struct {
	// ID is a stable identifier, such as "markdown".
	ID string
	// Name is the name shown to the user.
	Name string
	// Extensions are the suffixes this type claims, lowercase and with
	// the leading dot.
	Extensions []string
	// Parse turns file bytes into a document.
	Parse func(src []byte) *mdoc.Doc
	// View draws a document natively.
	View func(c *ui.Context, d *mdoc.Doc, o markdownui.Options) ui.Element
	// Editable reports whether the type can be edited and written back.
	Editable bool
}

// Registry resolves extensions and paths to a Type.
type Registry struct {
	byExt map[string]*Type
	text  *Type // the fallback for unknown but non-binary extensions
	list  []*Type
}

// NewRegistry returns a registry with the built-in types registered.
func NewRegistry() *Registry {
	r := &Registry{byExt: map[string]*Type{}}

	r.Register(&Type{
		ID:         "markdown",
		Name:       "Markdown",
		Extensions: []string{".md", ".markdown", ".mdx", ".mdown", ".mkd"},
		Parse:      mdoc.ParseFile,
		View:       markdownui.View,
		Editable:   true,
	})

	r.Register(&Type{
		ID:   "text",
		Name: "Plain text",
		Extensions: []string{
			".txt", ".text", ".log", ".csv", ".tsv", ".ini", ".conf", ".cfg", ".env", ".yaml", ".yml", ".toml", ".json",
			".jsonc", ".json5", ".go", ".rs", ".py", ".rb", ".php", ".java", ".kt", ".kts", ".c", ".h", ".cc", ".cpp", ".cxx", ".hpp", ".m", ".mm", ".swift",
			".js", ".jsx", ".ts", ".tsx", ".css", ".scss", ".sass", ".less", ".html", ".htm", ".xml", ".sh", ".bash", ".zsh", ".fish", ".ps1", ".bat", ".cmd",
			".sql", ".graphql", ".gql", ".proto", ".vue", ".svelte", ".astro", ".tex", ".bib", ".org", ".rst", ".adoc", ".properties", ".mod", ".sum", ".gradle", ".mk", ".cmake", ".plist", ".diff", ".patch",
		},
		Parse:    func(b []byte) *mdoc.Doc { return mdoc.ParsePlainText(string(b)) },
		View:     markdownui.View,
		Editable: true,
	})

	r.text = r.byExt[".txt"]
	return r
}

// Register adds or replaces a type. Later registrations win, which is how a
// plugin overrides a built-in.
func (r *Registry) Register(t *Type) {
	if t == nil || t.ID == "" {
		return
	}
	replaced := false
	for i, e := range r.list {
		if e.ID == t.ID {
			r.list[i] = t
			replaced = true
			break
		}
	}
	if !replaced {
		r.list = append(r.list, t)
	}
	for _, ext := range t.Extensions {
		r.byExt[strings.ToLower(ext)] = t
	}
}

// Types returns the registered types, ordered by name.
func (r *Registry) Types() []*Type {
	out := append([]*Type(nil), r.list...)
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// Lookup resolves a file extension to a type. An unknown extension still
// resolves to plain text unless it is a known binary, so any text-editable
// file opens rather than being refused.
func (r *Registry) Lookup(ext string) (*Type, bool) {
	ext = strings.ToLower(ext)
	if t, ok := r.byExt[ext]; ok {
		return t, true
	}
	if binaryExts[ext] {
		return nil, false
	}
	return r.text, r.text != nil
}

// IsEditablePath reports whether the path has an explicitly registered,
// editable extension. Unlike Lookup, it does not apply the plain-text
// fallback; callers use it for workspace visibility and file dialogs.
func (r *Registry) IsEditablePath(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	if ext == "" {
		return editableNames[strings.ToLower(filepath.Base(path))]
	}
	t, ok := r.byExt[ext]
	return ok && t.Editable
}

// EditableExtensions returns the explicit editable extensions in registration
// order, with duplicates removed.
func (r *Registry) EditableExtensions() []string {
	seen := make(map[string]bool)
	var extensions []string
	for _, t := range r.list {
		if !t.Editable {
			continue
		}
		for _, ext := range t.Extensions {
			ext = strings.ToLower(ext)
			if !seen[ext] {
				seen[ext] = true
				extensions = append(extensions, strings.TrimPrefix(ext, "."))
			}
		}
	}
	return extensions
}

// OpenSource resolves the path's type and parses caller-provided file content.
// Reading the file belongs to the document or platform adapter that owns it.
func (r *Registry) OpenSource(path string, source []byte) (*Type, *mdoc.Doc, error) {
	t, ok := r.Lookup(filepath.Ext(path))
	if !ok {
		return nil, nil, fmt.Errorf("doc: %s is not a text document", filepath.Base(path))
	}
	return t, t.Parse(source), nil
}

// binaryExts are the suffixes that must not be opened as text.
var binaryExts = map[string]bool{
	".png": true, ".jpg": true, ".jpeg": true, ".gif": true, ".webp": true,
	".ico": true, ".bmp": true, ".tiff": true, ".pdf": true, ".zip": true,
	".gz": true, ".tar": true, ".7z": true, ".rar": true, ".exe": true,
	".dll": true, ".dmg": true, ".app": true, ".so": true, ".dylib": true,
	".mp3": true, ".mp4": true, ".mov": true, ".avi": true, ".wav": true,
	".flac": true, ".ttf": true, ".otf": true, ".woff": true, ".woff2": true,
	".db": true, ".sqlite": true, ".class": true, ".o": true, ".a": true,
}

var imageExtensions = []string{".png", ".jpg", ".jpeg", ".gif", ".bmp", ".webp", ".svg", ".avif"}

// IsImagePath reports whether a path names a raster or vector image handled
// by the native image viewer. Image decoding remains owned by markdown/images.
func IsImagePath(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	for _, candidate := range imageExtensions {
		if ext == candidate {
			return true
		}
	}
	return false
}

// ImageExtensions returns the extensions handled by the image viewer.
func ImageExtensions() []string { return append([]string(nil), imageExtensions...) }

var builtins = NewRegistry()

var editableNames = map[string]bool{
	"readme": true, "license": true, "licence": true, "changelog": true,
	"authors": true, "notice": true, "makefile": true, "dockerfile": true,
	"procfile": true, "gemfile": true, "rakefile": true,
}

// IsEditablePath reports whether a path is part of the built-in editable
// file set. Unknown extensions may still be opened through Lookup's text
// fallback, but are not shown as workspace files.
func IsEditablePath(path string) bool { return builtins.IsEditablePath(path) }

// EditableExtensions returns the built-in explicit editable extensions.
func EditableExtensions() []string { return builtins.EditableExtensions() }
