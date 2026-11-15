package agent

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"

	"muse/markdown"
)

// CurrentDocument supplies an immutable snapshot of the active editor note.
// Presentation code owns the callback; platform adapters only invoke it.
type CurrentDocument func() (path, source string)

// WorkspacePort is a narrow capability for assistant workspace tools. The
// application service owns path policy and result limits; platform code owns
// filesystem access and UI code supplies the active document snapshot.
type WorkspacePort interface {
	Root() string
	CurrentDocument() (path, source string)
	ReadFile(path string) ([]byte, error)
	ReadDir(path string) ([]os.DirEntry, error)
	ResolveWithinRoot(path string) (string, error)
}

// WorkspaceReadTools implements bounded, read-only assistant workspace actions.
type WorkspaceReadTools struct{ Workspace WorkspacePort }

type ToolFileEntry struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

func (s WorkspaceReadTools) ListDocuments(ctx context.Context, in ToolInput) (any, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	dir, err := s.resolve(in.Path)
	if err != nil {
		return nil, err
	}
	entries, err := s.Workspace.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	out := make([]ToolFileEntry, 0, min(200, len(entries)))
	truncated := false
	for _, entry := range entries {
		if hidden(entry.Name()) {
			continue
		}
		if len(out) == 200 {
			truncated = true
			break
		}
		kind := "file"
		if entry.IsDir() {
			kind = "dir"
		}
		out = append(out, ToolFileEntry{Name: entry.Name(), Type: kind})
	}
	return map[string]any{"path": dir, "entries": out, "truncated": truncated}, nil
}

func (s WorkspaceReadTools) ReadDocument(ctx context.Context, in ToolInput) (any, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	path, source, err := s.target(in.Path)
	if err != nil {
		return nil, err
	}
	lines := strings.Split(source, "\n")
	start := in.Offset
	if start < 1 {
		start = 1
	}
	if start > len(lines) {
		return nil, fmt.Errorf("line offset %d is outside document with %d lines", start, len(lines))
	}
	limit := in.Limit
	if limit <= 0 || limit > 800 {
		limit = 800
	}
	end := min(start+limit-1, len(lines))
	truncated := end < len(lines)
	var b strings.Builder
	last := start - 1
	for i := start; i <= end; i++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		line := fmt.Sprintf("%5d| %s\n", i, lines[i-1])
		if b.Len()+len(line) > 20000 {
			truncated = true
			break
		}
		b.WriteString(line)
		last = i
	}
	return map[string]any{"path": path, "total_lines": len(lines), "start_line": start, "end_line": last, "truncated": truncated, "content": b.String()}, nil
}

func (s WorkspaceReadTools) SearchDocuments(ctx context.Context, in ToolInput) (any, error) {
	query := strings.TrimSpace(in.Query)
	if query == "" {
		return nil, errors.New("search query is empty")
	}
	path, source, err := s.target(in.Path)
	if err != nil {
		return nil, err
	}
	match := func(line string) bool { return strings.Contains(strings.ToLower(line), strings.ToLower(query)) }
	if in.IsRegex {
		re, err := regexp.Compile(query)
		if err != nil {
			return nil, fmt.Errorf("invalid regex: %w", err)
		}
		match = re.MatchString
	}
	type hit struct {
		Line int    `json:"line"`
		Text string `json:"text"`
	}
	hits := make([]hit, 0, 50)
	truncated := false
	for i, line := range strings.Split(source, "\n") {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if !match(line) {
			continue
		}
		if len(hits) == 50 {
			truncated = true
			break
		}
		text := strings.TrimSpace(line)
		if len(text) > 200 {
			text = text[:200] + "…"
		}
		hits = append(hits, hit{Line: i + 1, Text: text})
	}
	return map[string]any{"path": path, "matches": len(hits), "truncated": truncated, "hits": hits}, nil
}

func (s WorkspaceReadTools) ReadOutline(ctx context.Context, in ToolInput) (any, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	path, source, err := s.target(in.Path)
	if err != nil {
		return nil, err
	}
	var b strings.Builder
	count := 0
	for _, block := range markdown.Parse(source).Blocks {
		if block.Kind != markdown.Heading {
			continue
		}
		if count == 100 {
			b.WriteString("…\n")
			break
		}
		fmt.Fprintf(&b, "H%d %s\n", block.Level, strings.TrimSpace(plainInline(block.Inlines)))
		count++
	}
	if count == 0 {
		return map[string]any{"path": path, "outline": "No headings"}, nil
	}
	return map[string]any{"path": path, "outline": strings.TrimRight(b.String(), "\n")}, nil
}

func (s WorkspaceReadTools) WorkspaceTree(ctx context.Context, in ToolInput) (any, error) {
	path := in.Path
	if strings.TrimSpace(path) == "" {
		if s.Workspace == nil {
			return nil, errors.New("workspace capability is unavailable")
		}
		path = s.Workspace.Root()
	}
	root, err := s.resolve(path)
	if err != nil {
		return nil, err
	}
	depth := in.Depth
	if depth <= 0 || depth > 4 {
		depth = 2
	}
	var b strings.Builder
	count := 0
	truncated := false
	var walk func(string, string, int) error
	walk = func(dir, prefix string, level int) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		entries, err := s.Workspace.ReadDir(dir)
		if err != nil {
			return nil
		}
		sort.SliceStable(entries, func(i, j int) bool {
			di, dj := entries[i].IsDir(), entries[j].IsDir()
			if di != dj {
				return di
			}
			return strings.ToLower(entries[i].Name()) < strings.ToLower(entries[j].Name())
		})
		for _, entry := range entries {
			name := entry.Name()
			if hidden(name) {
				continue
			}
			if count == 200 {
				truncated = true
				return nil
			}
			count++
			if entry.IsDir() {
				fmt.Fprintf(&b, "%s%s/\n", prefix, name)
				if level < depth {
					if err := walk(filepath.Join(dir, name), prefix+"  ", level+1); err != nil {
						return err
					}
				}
			} else {
				fmt.Fprintf(&b, "%s%s\n", prefix, name)
			}
		}
		return nil
	}
	if err := walk(root, "", 1); err != nil {
		return nil, err
	}
	return map[string]any{"root": root, "tree": strings.TrimRight(b.String(), "\n"), "entries": count, "truncated": truncated}, nil
}

func (s WorkspaceReadTools) DocumentStats(ctx context.Context, in ToolInput) (any, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	path, source, err := s.target(in.Path)
	if err != nil {
		return nil, err
	}
	headings := 0
	for _, block := range markdown.Parse(source).Blocks {
		if block.Kind == markdown.Heading {
			headings++
		}
	}
	return map[string]any{"path": path, "chars": utf8.RuneCountInString(source), "lines": strings.Count(source, "\n") + 1, "headings": headings}, nil
}

// WorkspaceMutationPort supplies durable document/file mutations and the
// current-buffer edit capability. UI-only effects are returned as ToolEffect
// values for the presenter to execute after the application call completes.
type WorkspaceMutationPort interface {
	WorkspacePort
	WorkspaceDirectoryPort
	DestinationRevision(path string) (string, error)
	SaveDocument(path, content, expectedRevision string) error
	ApplyCurrentDocument(content string) error
	MoveDocument(root, source, target string) (string, error)
	TrashDocuments(root string, paths []string) error
}

// WorkspaceDirectoryPort holds filesystem mutations needed for directory
// management tools. It stays separate from the read-only path adapter.
type WorkspaceDirectoryPort interface {
	CreateFolder(path string) error
	Exists(path string) (bool, error)
}

type ToolEffectKind string

const (
	OpenDocumentEffect   ToolEffectKind = "open_document"
	InsertDocumentEffect ToolEffectKind = "insert_document"
)

type ToolEffect struct {
	Kind     ToolEffectKind `json:"kind"`
	Path     string         `json:"path,omitempty"`
	Text     string         `json:"text,omitempty"`
	Position string         `json:"position,omitempty"`
}

// WorkspaceWriteTools applies assistant document and file mutations using
// injected capabilities; it never reaches into editor or widget state.
type WorkspaceWriteTools struct{ Workspace WorkspaceMutationPort }

// WorkspaceOperations composes read and write capabilities into the Executor
// port without coupling either tool family to presentation state.
type WorkspaceOperations struct {
	Read  WorkspaceReadTools
	Write WorkspaceWriteTools
}

func (o WorkspaceOperations) ListDocuments(c context.Context, i ToolInput) (any, error) {
	return o.Read.ListDocuments(c, i)
}
func (o WorkspaceOperations) ReadDocument(c context.Context, i ToolInput) (any, error) {
	return o.Read.ReadDocument(c, i)
}
func (o WorkspaceOperations) SearchDocuments(c context.Context, i ToolInput) (any, error) {
	return o.Read.SearchDocuments(c, i)
}
func (o WorkspaceOperations) ReadOutline(c context.Context, i ToolInput) (any, error) {
	return o.Read.ReadOutline(c, i)
}
func (o WorkspaceOperations) WorkspaceTree(c context.Context, i ToolInput) (any, error) {
	return o.Read.WorkspaceTree(c, i)
}
func (o WorkspaceOperations) DocumentStats(c context.Context, i ToolInput) (any, error) {
	return o.Read.DocumentStats(c, i)
}
func (o WorkspaceOperations) OpenDocument(c context.Context, i ToolInput) (any, error) {
	return o.Write.OpenDocument(c, i)
}
func (o WorkspaceOperations) EditDocument(c context.Context, i ToolInput) (any, error) {
	return o.Write.EditDocument(c, i)
}
func (o WorkspaceOperations) InsertIntoDocument(c context.Context, i ToolInput) (any, error) {
	return o.Write.InsertIntoDocument(c, i)
}
func (o WorkspaceOperations) WriteDocument(c context.Context, i ToolInput) (any, error) {
	return o.Write.WriteDocument(c, i)
}
func (o WorkspaceOperations) RenameDocument(c context.Context, i ToolInput) (any, error) {
	return o.Write.RenameDocument(c, i)
}
func (o WorkspaceOperations) DeleteDocument(c context.Context, i ToolInput) (any, error) {
	return o.Write.DeleteDocument(c, i)
}
func (o WorkspaceOperations) CreateFolder(c context.Context, i ToolInput) (any, error) {
	return o.Write.CreateFolder(c, i)
}

var _ Operations = WorkspaceOperations{}

func (s WorkspaceWriteTools) OpenDocument(ctx context.Context, in ToolInput) (any, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	path, err := s.resolve(in.Path)
	if err != nil {
		return nil, err
	}
	if !editablePath(path) {
		return nil, errors.New("file is not an editable text document")
	}
	return ToolEffect{Kind: OpenDocumentEffect, Path: path}, nil
}
func (s WorkspaceWriteTools) InsertIntoDocument(ctx context.Context, in ToolInput) (any, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if strings.TrimSpace(in.Text) == "" {
		return nil, errors.New("insert text is empty")
	}
	if in.Position != "start" && in.Position != "end" {
		return nil, errors.New("insert position must be start or end")
	}
	return ToolEffect{Kind: InsertDocumentEffect, Text: in.Text, Position: in.Position}, nil
}
func (s WorkspaceWriteTools) EditDocument(ctx context.Context, in ToolInput) (any, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if in.OldText == "" {
		return nil, errors.New("old text is empty")
	}
	if strings.TrimSpace(in.Path) == "" || strings.TrimSpace(in.Path) == "current" {
		if s.Workspace == nil {
			return nil, errors.New("workspace capability is unavailable")
		}
		_, source := s.Workspace.CurrentDocument()
		updated, err := replaceUnique(source, in.OldText, in.NewText)
		if err != nil {
			return nil, err
		}
		if err := s.Workspace.ApplyCurrentDocument(updated); err != nil {
			return nil, err
		}
		return map[string]any{"path": "current", "replaced": true, "new_chars": len(updated)}, nil
	}
	path, source, err := s.target(in.Path)
	if err != nil {
		return nil, err
	}
	updated, err := replaceUnique(source, in.OldText, in.NewText)
	if err != nil {
		return nil, err
	}
	if filepath.Clean(path) == filepath.Clean(s.currentPath()) {
		if err := s.Workspace.ApplyCurrentDocument(updated); err != nil {
			return nil, err
		}
	} else {
		if err := s.Workspace.SaveDocument(path, updated, revisionOf(source)); err != nil {
			return nil, err
		}
	}
	return map[string]any{"path": path, "replaced": true, "new_chars": len(updated)}, nil
}
func (s WorkspaceWriteTools) WriteDocument(ctx context.Context, in ToolInput) (any, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if strings.TrimSpace(in.Path) == "" {
		return nil, errors.New("write path is required")
	}
	path, err := s.resolve(in.Path)
	if err != nil {
		return nil, err
	}
	if !supportedWritePath(path) {
		return nil, errors.New("only Markdown and text files are supported")
	}
	created, err := s.Workspace.Exists(path)
	if err != nil {
		return nil, err
	}
	if filepath.Clean(path) == filepath.Clean(s.currentPath()) {
		if err := s.Workspace.ApplyCurrentDocument(in.Content); err != nil {
			return nil, err
		}
	} else {
		rev, err := s.Workspace.DestinationRevision(path)
		if err != nil {
			return nil, err
		}
		if err := s.Workspace.SaveDocument(path, in.Content, rev); err != nil {
			return nil, err
		}
	}
	return map[string]any{"path": path, "created": !created, "new_chars": len(in.Content)}, nil
}
func (s WorkspaceWriteTools) RenameDocument(ctx context.Context, in ToolInput) (any, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	from, err := s.resolve(in.Path)
	if err != nil {
		return nil, err
	}
	to, err := s.resolve(in.NewPath)
	if err != nil {
		return nil, err
	}
	if !editablePath(to) {
		return nil, errors.New("destination is not editable")
	}
	target, err := s.Workspace.MoveDocument(s.Workspace.Root(), from, to)
	if err != nil {
		return nil, err
	}
	return map[string]any{"path": target, "moved": true}, nil
}
func (s WorkspaceWriteTools) DeleteDocument(ctx context.Context, in ToolInput) (any, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	path, err := s.resolve(in.Path)
	if err != nil {
		return nil, err
	}
	if err := s.Workspace.TrashDocuments(s.Workspace.Root(), []string{path}); err != nil {
		return nil, err
	}
	return map[string]any{"path": path, "trashed": true}, nil
}
func (s WorkspaceWriteTools) CreateFolder(ctx context.Context, in ToolInput) (any, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	path, err := s.resolve(in.Path)
	if err != nil {
		return nil, err
	}
	if err := s.Workspace.CreateFolder(path); err != nil {
		return nil, err
	}
	return map[string]any{"path": path, "created": true}, nil
}

func (s WorkspaceWriteTools) resolve(value string) (string, error) {
	return (WorkspaceReadTools{Workspace: s.Workspace}).resolve(value)
}
func (s WorkspaceWriteTools) target(value string) (string, string, error) {
	return (WorkspaceReadTools{Workspace: s.Workspace}).target(value)
}
func (s WorkspaceWriteTools) currentPath() string {
	if s.Workspace == nil {
		return ""
	}
	p, _ := s.Workspace.CurrentDocument()
	return p
}
func replaceUnique(source, old, replacement string) (string, error) {
	switch n := strings.Count(source, old); n {
	case 0:
		return "", errors.New("old text not found")
	case 1:
		return strings.Replace(source, old, replacement, 1), nil
	default:
		return "", fmt.Errorf("old text appears %d times", n)
	}
}
func revisionOf(source string) string {
	sum := sha256.Sum256([]byte(source))
	return hex.EncodeToString(sum[:])
}
func editablePath(path string) bool { return supportedWritePath(path) }
func supportedWritePath(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	return ext == ".md" || ext == ".markdown" || ext == ".txt"
}

func (s WorkspaceReadTools) resolve(value string) (string, error) {
	if s.Workspace == nil {
		return "", errors.New("workspace capability is unavailable")
	}
	root := filepath.Clean(s.Workspace.Root())
	if root == "." || root == "" {
		return "", errors.New("workspace is unavailable")
	}
	current, _ := s.Workspace.CurrentDocument()
	p := strings.TrimSpace(value)
	if p == "" || p == "current" {
		if current == "" {
			return "", errors.New("current document is not saved")
		}
		return filepath.Clean(current), nil
	}
	full := p
	if !filepath.IsAbs(full) {
		full = filepath.Join(root, p)
	}
	full = filepath.Clean(full)
	rel, err := filepath.Rel(root, full)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", errors.New("path escapes workspace")
	}
	return s.Workspace.ResolveWithinRoot(full)
}
func (s WorkspaceReadTools) target(value string) (string, string, error) {
	p := strings.TrimSpace(value)
	current, source := "", ""
	if s.Workspace == nil {
		return "", "", errors.New("workspace capability is unavailable")
	}
	current, source = s.Workspace.CurrentDocument()
	if p == "" || p == "current" {
		if current == "" && source == "" {
			return "", "", errors.New("current document is empty")
		}
		if current == "" {
			current = "Untitled"
		}
		return current, source, nil
	}
	path, err := s.resolve(p)
	if err != nil {
		return "", "", err
	}
	if filepath.Clean(path) == filepath.Clean(current) {
		return path, source, nil
	}
	data, err := s.Workspace.ReadFile(path)
	if err != nil {
		return "", "", err
	}
	if len(data) > 2<<20 {
		return "", "", errors.New("file exceeds 2 MiB limit")
	}
	if !utf8.Valid(data) || strings.IndexByte(string(data), 0) >= 0 {
		return "", "", errors.New("file is not text")
	}
	return path, string(data), nil
}
func hidden(name string) bool { return strings.HasPrefix(name, ".") || name == "node_modules" }
func plainInline(in []markdown.Span) string {
	var b strings.Builder
	for _, item := range in {
		b.WriteString(item.Text)
	}
	return b.String()
}
