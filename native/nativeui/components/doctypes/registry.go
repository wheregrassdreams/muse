// Package doc decides how a file opens. A Type maps file extensions to a
// parser and a native view, and Registry resolves a path to its type.
//
// This is the plugin seam: registering a Type is what makes a new kind of
// file openable in Muse. Markdown and plain text are built in, and any
// extension not known to be binary falls back to plain text, so a .txt,
// .log, .csv or .conf opens and is editable the same way Markdown is.
package doc

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/egoist/mygo/ui"

	"muse/mdoc"
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
	View func(c *ui.Context, d *mdoc.Doc, o mdoc.Options) ui.Element
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
		View:       mdoc.View,
		Editable:   true,
	})

	r.Register(&Type{
		ID:         "text",
		Name:       "Plain text",
		Extensions: []string{".txt", ".text", ".log", ".csv", ".tsv", ".ini", ".conf", ".cfg", ".env", ".yaml", ".yml", ".toml", ".json"},
		Parse:      func(b []byte) *mdoc.Doc { return mdoc.ParsePlainText(string(b)) },
		View:       mdoc.View,
		Editable:   true,
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

// Open reads path, resolves its type, and parses it.
func (r *Registry) Open(path string) (*Type, *mdoc.Doc, error) {
	t, ok := r.Lookup(filepath.Ext(path))
	if !ok {
		return nil, nil, fmt.Errorf("doc: %s is not a text document", filepath.Base(path))
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, err
	}
	return t, t.Parse(b), nil
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
