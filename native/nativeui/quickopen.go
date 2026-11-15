package nativeui

import (
	"muse/locales"
	"path/filepath"
	"sort"
	"strings"
	"unicode"

	"github.com/egoist/mygo/ui"
)

// Quick open is the fuzzy file-path picker available from the command
// palette. Type to filter by subsequence match on the path, use ↑↓ or Ctrl+N/P
// to move, Enter to open, Esc to dismiss; clicking a row opens it too.

type quickOpenCandidate struct {
	fileRow
	titleMatches []int
	pathMatches  []int
}

func (a *App) quickOpenCandidates() []quickOpenCandidate {
	files, titles, aliases := a.quickOpenCatalog()
	q := strings.ToLower(strings.TrimSpace(a.quickOpenValue))
	root := filepath.Clean(a.root)
	cacheable := a.vault != nil && a.vault.files != nil && a.quickOpenCatalogReady
	if cacheable && a.quickOpenCandidatesReady &&
		a.quickOpenCandidateQuery == q &&
		a.quickOpenCandidateVault == a.vault &&
		a.quickOpenCandidateRoot == root &&
		a.quickOpenCandidateIndexRevision == a.quickOpenIndexRevision &&
		a.quickOpenCandidateFilesRevision == a.quickOpenFilesRevision {
		return a.quickOpenCandidateRows
	}
	if q == "" {
		out := make([]quickOpenCandidate, 0, len(files))
		for _, row := range files {
			out = append(out, quickOpenCandidate{fileRow: row})
		}
		sort.Slice(out, func(i, j int) bool { return out[i].path < out[j].path })
		return a.storeQuickOpenCandidates(q, out, cacheable)
	}
	type scored struct {
		candidate quickOpenCandidate
		score     int
	}
	matches := make([]scored, 0, 16)
	for _, r := range files {
		rel, _ := filepath.Rel(a.root, r.path)
		pathText := filepath.ToSlash(rel)
		pathScore, pathMatches, found := fuzzyMatch(pathText, q)
		best := pathScore
		title := titles[r.path]
		var titleMatches []int
		if title != "" {
			if s, positions, ok := fuzzyMatch(title, q); ok {
				titleMatches = positions
				if !found || s < best {
					best, found = s, true
				}
			}
		}
		for _, alias := range aliases[r.path] {
			if s, _, ok := fuzzyMatch(alias, q); ok && (!found || s < best) {
				best, found = s, true
			}
		}
		if found {
			matches = append(matches, scored{quickOpenCandidate{fileRow: r, titleMatches: titleMatches, pathMatches: pathMatches}, best})
		}
	}
	sort.Slice(matches, func(i, j int) bool {
		if matches[i].score != matches[j].score {
			return matches[i].score < matches[j].score
		}
		return matches[i].candidate.path < matches[j].candidate.path
	})
	out := make([]quickOpenCandidate, len(matches))
	for i, match := range matches {
		out[i] = match.candidate
	}
	return a.storeQuickOpenCandidates(q, out, cacheable)
}

func (a *App) storeQuickOpenCandidates(query string, rows []quickOpenCandidate, cacheable bool) []quickOpenCandidate {
	if !cacheable {
		return rows
	}
	a.quickOpenCandidateQuery = query
	a.quickOpenCandidateVault = a.vault
	a.quickOpenCandidateRoot = filepath.Clean(a.root)
	a.quickOpenCandidateIndexRevision = a.quickOpenIndexRevision
	a.quickOpenCandidateFilesRevision = a.quickOpenFilesRevision
	a.quickOpenCandidateRows = rows
	a.quickOpenCandidatesReady = true
	return rows
}

func (a *App) clearQuickOpenCandidates() {
	a.quickOpenCandidateRows = nil
	a.quickOpenCandidateQuery = ""
	a.quickOpenCandidateVault = nil
	a.quickOpenCandidateRoot = ""
	a.quickOpenCandidateIndexRevision = 0
	a.quickOpenCandidateFilesRevision = 0
	a.quickOpenCandidatesReady = false
}

func (a *App) clearQuickOpenCatalog() {
	a.clearQuickOpenCandidates()
	a.quickOpenCatalogVault = nil
	a.quickOpenCatalogRoot = ""
	a.quickOpenIndexRevision = 0
	a.quickOpenFilesRevision = 0
	a.quickOpenCatalogReady = false
	a.quickOpenRows = nil
	a.quickOpenTitles = nil
	a.quickOpenAliases = nil
}

func (a *App) clearQuickOpenCatalogIfVaultChanged() {
	if !a.quickOpenCatalogReady {
		return
	}
	if a.vault == nil || a.vault.files == nil || a.quickOpenCatalogVault != a.vault || a.quickOpenCatalogRoot != filepath.Clean(a.vault.root) {
		a.clearQuickOpenCatalog()
	}
}

func (a *App) quickOpenCatalog() ([]fileRow, map[string]string, map[string][]string) {
	if a.vault == nil || a.vault.files == nil {
		if a.quickOpenCatalogReady || a.quickOpenCandidatesReady || a.quickOpenCatalogVault != nil || a.quickOpenCandidateVault != nil {
			a.clearQuickOpenCatalog()
		}
		rows := a.fileRows()
		files := make([]fileRow, 0, len(rows))
		for _, row := range rows {
			if !row.dir {
				files = append(files, row)
			}
		}
		return files, nil, nil
	}
	root := filepath.Clean(a.vault.root)
	var indexRevision uint64
	if a.vault.index != nil {
		indexRevision = a.vault.index.Revision()
	}
	filesRevision := a.vault.files.Revision()
	if a.quickOpenCatalogReady && (a.quickOpenCatalogVault != a.vault || a.quickOpenCatalogRoot != root) {
		// A new workspace must not leave the prior runtime or its indexed
		// metadata reachable from the app's Quick Open caches.
		a.clearQuickOpenCatalog()
	}
	if a.quickOpenCatalogReady && a.quickOpenCatalogVault == a.vault && a.quickOpenCatalogRoot == root && a.quickOpenIndexRevision == indexRevision && a.quickOpenFilesRevision == filesRevision {
		return a.quickOpenRows, a.quickOpenTitles, a.quickOpenAliases
	}
	if a.vault.files != nil {
		// Quick open is a vault-wide picker: collapsed tree branches must not
		// hide files from it. Consume the shared path catalog instead of walking
		// the UI's expanded tree projection on every query.
		paths := a.vault.files.Paths()
		files := make([]fileRow, 0, len(paths))
		for _, path := range paths {
			files = append(files, fileRow{path: filepath.Join(a.root, filepath.FromSlash(path))})
		}
		titles := make(map[string]string)
		aliases := make(map[string][]string)
		if a.vault.index != nil {
			for _, doc := range a.vault.index.SearchSnapshot() {
				full := filepath.Join(a.root, filepath.FromSlash(doc.Path))
				titles[full] = doc.Title
				aliases[full] = doc.Aliases
			}
		}
		a.quickOpenCatalogRoot = root
		a.quickOpenCatalogVault = a.vault
		a.quickOpenIndexRevision = indexRevision
		a.quickOpenFilesRevision = filesRevision
		a.quickOpenRows = files
		a.quickOpenTitles = titles
		a.quickOpenAliases = aliases
		a.quickOpenCatalogReady = true
		return files, titles, aliases
	}
	return nil, nil, nil
}

func (a *App) quickOpenTitle(path string) string {
	if title := a.quickOpenTitles[path]; title != "" {
		return title
	}
	if a.vault != nil && a.vault.index != nil {
		rel, err := filepath.Rel(a.root, path)
		if err == nil {
			if doc, ok := a.vault.index.Document(filepath.ToSlash(rel)); ok && doc.Title != "" {
				return doc.Title
			}
		}
	}
	return filepath.Base(path)
}

// fuzzyScore reports whether needle is a subsequence of hay (ok), with a
// rank where lower is better (consecutive and word-start hits first). The
// rank may be negative — a fully consecutive match earns real negatives —
// so callers must use ok, not the sign.
func fuzzyScore(hay, needle string) (int, bool) {
	score, _, ok := fuzzyMatch(hay, needle)
	return score, ok
}

// fuzzyMatch returns the same subsequence rank as fuzzyScore and the rune
// offsets to highlight in the displayed string. Matching by rune keeps the
// offsets valid for Unicode titles and paths.
func fuzzyMatch(hay, needle string) (int, []int, bool) {
	hayRunes := []rune(hay)
	needleRunes := []rune(needle)
	for i := range hayRunes {
		hayRunes[i] = unicode.ToLower(hayRunes[i])
	}
	for i := range needleRunes {
		needleRunes[i] = unicode.ToLower(needleRunes[i])
	}
	positions := make([]int, 0, len(needleRunes))
	score, at := 0, 0
	for i, needleRune := range needleRunes {
		found := -1
		for j := at; j < len(hayRunes); j++ {
			if hayRunes[j] == needleRune {
				found = j
				break
			}
		}
		if found < 0 {
			return 0, nil, false
		}
		positions = append(positions, found)
		if found == at && i > 0 {
			score -= 2 // consecutive
		}
		if found == 0 || hayRunes[found-1] == '/' {
			score -= 3 // after a path separator
		}
		score += found - at
		at = found + 1
	}
	return score, positions, true
}

func quickOpenHighlightedText(c *ui.Context, text string, matches []int, color, highlight ui.Color, size float32) ui.Element {
	runes := []rune(text)
	if len(matches) == 0 {
		return ui.RichText(c, ui.Span{Text: text}).FontSize(size).TextColor(color).SingleLine().Ellipsis("…")
	}
	spans := make([]ui.Span, 0, len(matches)*2+1)
	start := 0
	for i := 0; i < len(matches); {
		first := matches[i]
		if first > start {
			spans = append(spans, ui.Span{Text: string(runes[start:first])})
		}
		end := first + 1
		i++
		for i < len(matches) && matches[i] == end {
			end++
			i++
		}
		spans = append(spans, ui.Span{Text: string(runes[first:end]), Color: highlight, Background: ui.RGBA(highlight.R, highlight.G, highlight.B, 0.18), Radius: 2})
		start = end
	}
	if start < len(runes) {
		spans = append(spans, ui.Span{Text: string(runes[start:])})
	}
	return ui.RichText(c, spans...).FontSize(size).TextColor(color).SingleLine().Ellipsis("…")
}

// openQuickOpen shows the palette, or the system file dialog when no
// workspace is open.
func (a *App) openQuickOpen() {
	if a.root == "" {
		a.openFileDialog()
		return
	}
	a.quickOpen = true
	a.quickOpenValue = ""
	a.quickOpenSel = 0
	a.invalidate()
}

func (a *App) closeQuickOpen() {
	a.quickOpen = false
	a.clearQuickOpenCandidates()
	a.invalidate()
}

// applyQuickOpen opens the selected candidate.
func (a *App) applyQuickOpen() {
	cands := a.quickOpenCandidates()
	if len(cands) == 0 {
		a.closeQuickOpen()
		return
	}
	if a.quickOpenSel >= len(cands) {
		a.quickOpenSel = 0
	}
	path := cands[a.quickOpenSel].path
	a.closeQuickOpen()
	a.selectedPath = path
	a.requestOpen(path)
}

func (a *App) quickOpenDialog(c *ui.Context) {
	if !a.quickOpen {
		return
	}
	if c.Shortcut(0, ui.KeyEscape) {
		a.closeQuickOpen()
		return
	}
	if c.Shortcut(0, ui.KeyUp) {
		if a.quickOpenSel > 0 {
			a.quickOpenSel--
		}
	}
	if c.Shortcut(0, ui.KeyDown) {
		if a.quickOpenSel < len(a.quickOpenCandidates())-1 {
			a.quickOpenSel++
		}
	}
	t := c.Theme()
	ui.Box(c).Fill().Background(ui.RGBA(0, 0, 0, 0.32)).Children(func() {
		ui.Column(c).Width(560).Margin(110, ui.Auto, 0, ui.Auto).Radius(12).
			Background(t.Background).Border(1, t.Border).
			Shadow(0, 12, 32, -6, ui.RGBA(0, 0, 0, 0.28)).
			Padding(10).Gap(8).Children(func() {
			cands := a.quickOpenCandidates()
			ui.TextInput(c, &a.quickOpenValue).Placeholder(locales.QuickOpenPlaceholder()).Label(locales.QuickOpenTitle()).
				AutoFocus().OnSubmit(func() { a.applyQuickOpen() }).
				OnTextKey(func(k ui.TextKey) bool {
					if k.Mods != ui.Ctrl || (k.Key != ui.KeyN && k.Key != ui.KeyP) || len(cands) == 0 {
						return false
					}
					step := 1
					if k.Key == ui.KeyP {
						step = -1
					}
					a.quickOpenSel = max(0, min(a.quickOpenSel+step, len(cands)-1))
					return true
				})
			if len(cands) == 0 {
				ui.Text(c, locales.QuickOpenNoMatch()).FontSize(12).TextColor(t.TextMuted).SingleLine().Ellipsis("…")
				return
			}
			if a.quickOpenSel >= len(cands) {
				a.quickOpenSel = len(cands) - 1
			}
			if a.quickOpenSel < 0 {
				a.quickOpenSel = 0
			}
			n := len(cands)
			if n > 12 {
				n = 12
			}
			for i := 0; i < n; i++ {
				r := cands[i]
				i := i
				row := ui.Row(c).Height(42).Radius(6).Padding(0, 10).Gap(9).
					AlignItems(ui.Center).Cursor(ui.CursorPointer)
				switch {
				case i == a.quickOpenSel:
					if t.Dark {
						row.Background(ui.RGBA(107, 147, 255, 0.30))
					} else {
						row.Background(ui.RGBA(107, 147, 255, 0.20))
					}
				case row.Hovered():
					row.Background(t.SurfaceHover)
				}
				row.Children(func() {
					ui.Icon(c, iconFile).Size(13, 13).TextColor(iconTint(t))
					rel, _ := filepath.Rel(a.root, r.path)
					ui.Column(c).FillWidth().Gap(1).Children(func() {
						quickOpenHighlightedText(c, a.quickOpenTitle(r.path), r.titleMatches, t.Text, t.Accent, 12.5)
						quickOpenHighlightedText(c, filepath.ToSlash(rel), r.pathMatches, t.TextMuted, t.Accent, 10.5)
					})
				})
				row.OnClick(func() {
					a.quickOpenSel = i
					a.applyQuickOpen()
				})
			}
			ui.Row(c).Height(18).Padding(0, 2).Children(func() {
				ui.Text(c, locales.QuickOpenCount(len(cands))).
					FontSize(10).TextColor(t.TextMuted)
			})
		})
	})
}
