package editor

import (
	"muse/locales"
	"path/filepath"
	"sort"
	"strings"

	"github.com/egoist/mygo/ui"
	"muse/editor/internal/parser"
)

const wikiSuggestionLimit = 8

// WikiSuggestion keeps a canonical inserted target separate from the title
// and aliases used to find and identify it in the popup.
type WikiSuggestion struct {
	Target  string
	Title   string
	Aliases []string
}

func wikiLucideIcon(shapes string) *ui.SVG {
	return ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round">` + shapes + `</svg>`))
}

var (
	wikiNoteIcon  = wikiLucideIcon(`<path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"/><path d="M14 2v6h6"/><path d="M8 13h8M8 17h6"/>`)
	wikiImageIcon = wikiLucideIcon(`<rect x="3" y="3" width="18" height="18" rx="2"/><circle cx="8.5" cy="8.5" r="1.5"/><path d="m21 15-5-5L5 21"/>`)
)

type wikiSuggestionState struct {
	open       bool
	query      string
	start, end int // source range replaced by an accepted target
	items      []string
	details    map[string]WikiSuggestion
	selected   int
	bounds     ui.Rect
	dismissed  bool
	dismissKey string
}

// updateWikiSuggestions follows the incomplete [[ expression at the caret.
// The app supplies vault-relative targets; the editor owns the interaction.
func (e *Editor) updateWikiSuggestions() {
	old := e.wikiPopup
	close := func() {
		e.wikiPopup.open = false
		e.wikiPopup.items = nil
	}
	if e.opts.WikiLinkSuggestions == nil && e.opts.WikiLinkSuggestionDetails == nil || e.opts.Code != "" || e.anchor != e.caret || e.compose != "" || e.phantom != nil || e.block != nil || e.tableSelection != nil {
		close()
		return
	}
	e.layoutIfNeeded()
	u := e.unitAt(e.caret)
	if u != nil && (u.kind == kCode || u.kind == kMath || u.kind == kHTML || u.kind == kRaw) {
		close()
		return
	}
	lineStart := lineStartAt(e.src, e.caret)
	prefix := e.src[lineStart:e.caret]
	open := strings.LastIndex(prefix, "[[")
	if open < 0 || open > 0 && prefix[open-1] == '\\' || strings.Contains(prefix[open+2:], "]]") {
		close()
		return
	}
	// An unclosed backtick span is code; wiki suggestions there are noise.
	if oddBacktickRun(prefix[:open]) {
		close()
		return
	}
	query := prefix[open+2:]
	// `[[` typed by the user is normally already followed by the pair's two
	// closing brackets. Keep them in place and replace only the partial target.
	if strings.ContainsAny(query, "\r\n") {
		close()
		return
	}
	if old.dismissed && old.dismissKey == query && old.start == lineStart+open {
		close()
		e.wikiPopup = old
		e.wikiPopup.open = false
		return
	}
	var suggestions []WikiSuggestion
	if e.opts.WikiLinkSuggestionDetails != nil {
		suggestions = e.opts.WikiLinkSuggestionDetails(query)
	} else {
		for _, target := range e.opts.WikiLinkSuggestions(query) {
			suggestions = append(suggestions, WikiSuggestion{Target: target})
		}
	}
	type match struct {
		target string
		score  int
		order  int
	}
	matches := make([]match, 0, len(suggestions))
	seen := make(map[string]struct{}, len(suggestions))
	details := make(map[string]WikiSuggestion, len(suggestions))
	for order, suggestion := range suggestions {
		target := strings.TrimSpace(suggestion.Target)
		if target == "" {
			continue
		}
		if _, ok := seen[target]; ok {
			continue
		}
		suggestion.Target = target
		details[target] = suggestion
		seen[target] = struct{}{}
		score, matched := wikiSuggestionScore(target, query)
		for _, candidate := range append([]string{suggestion.Title}, suggestion.Aliases...) {
			if candidate == "" {
				continue
			}
			if candidateScore, ok := wikiSuggestionScore(candidate, query); ok && (!matched || candidateScore < score) {
				score, matched = candidateScore, true
			}
		}
		if matched {
			matches = append(matches, match{target: target, score: score, order: order})
		}
	}
	sort.SliceStable(matches, func(i, j int) bool {
		if matches[i].score != matches[j].score {
			return matches[i].score < matches[j].score
		}
		return matches[i].order < matches[j].order
	})
	unique := make([]string, 0, min(len(matches), wikiSuggestionLimit))
	uniqueDetails := make(map[string]WikiSuggestion, min(len(matches), wikiSuggestionLimit))
	for i := 0; i < len(matches) && i < wikiSuggestionLimit; i++ {
		unique = append(unique, matches[i].target)
		uniqueDetails[matches[i].target] = details[matches[i].target]
	}
	if len(unique) == 0 {
		close()
		return
	}
	selected := 0
	if old.query == query && old.start == lineStart+open && old.selected >= 0 && old.selected < len(unique) {
		selected = old.selected
	}
	e.wikiPopup = wikiSuggestionState{
		open: true, query: query,
		start: lineStart + open + 2, end: e.caret,
		items: unique, details: uniqueDetails, selected: selected,
	}
	if old.query == query && old.dismissed && old.dismissKey == query {
		e.wikiPopup.dismissed = true
		e.wikiPopup.dismissKey = query
	}
}

// wikiSuggestionScore prefers path/name prefixes, then an in-order fuzzy
// subsequence. A nonempty query must match; an empty query lists all options.
func wikiSuggestionScore(target, query string) (int, bool) {
	q := strings.ToLower(strings.TrimSpace(query))
	name := strings.ToLower(target)
	if q == "" {
		return 0, true
	}
	if strings.HasPrefix(name, q) {
		return 0, true
	}
	if i := strings.Index(name, q); i >= 0 {
		return 10 + i, true
	}
	at, gaps := 0, 0
	for _, r := range q {
		found := strings.IndexRune(name[at:], r)
		if found < 0 {
			return 0, false
		}
		gaps += found
		at += found + len(string(r))
		if at >= len(name) && r != []rune(q)[len([]rune(q))-1] {
			return 0, false
		}
	}
	return 100 + gaps, true
}

func oddBacktickRun(s string) bool {
	count := 0
	for i := 0; i < len(s); {
		if s[i] != '`' {
			i++
			continue
		}
		j := i + 1
		for j < len(s) && s[j] == '`' {
			j++
		}
		count++
		i = j
	}
	return count%2 == 1
}

func (e *Editor) wikiSuggestionKey(key ui.Key, mods ui.Modifiers) bool {
	if !e.wikiPopup.open {
		return false
	}
	if mods&ui.Ctrl != 0 {
		switch key {
		case ui.KeyN:
			key = ui.KeyDown
		case ui.KeyP:
			key = ui.KeyUp
		}
	}
	switch key {
	case ui.KeyUp:
		e.wikiPopup.selected = (e.wikiPopup.selected + len(e.wikiPopup.items) - 1) % len(e.wikiPopup.items)
		e.invalidate()
		return true
	case ui.KeyDown:
		e.wikiPopup.selected = (e.wikiPopup.selected + 1) % len(e.wikiPopup.items)
		e.invalidate()
		return true
	case ui.KeyEnter, ui.KeyTab:
		e.acceptWikiSuggestion(e.wikiPopup.selected)
		return true
	case ui.KeyEscape:
		e.wikiPopup.dismissed = true
		e.wikiPopup.dismissKey = e.wikiPopup.query
		e.wikiPopup.open = false
		e.invalidate()
		return true
	}
	return false
}

func (e *Editor) wikiSuggestionIndexAt(x, y float32) (int, bool) {
	p := e.wikiPopup
	if !p.open || !p.bounds.Contains(x, y) {
		return 0, false
	}
	const rowH = float32(30)
	i := int((y - p.bounds.Y - 5) / (rowH + 2))
	if i < 0 || i >= len(p.items) || y >= p.bounds.Y+5+float32(i+1)*(rowH+2)-2 {
		return 0, false
	}
	return i, true
}

func (e *Editor) acceptWikiSuggestion(index int) {
	p := e.wikiPopup
	if !p.open || index < 0 || index >= len(p.items) || p.start > p.end || p.end > len(e.src) {
		return
	}
	target := p.items[index]
	insert := target
	if !strings.HasPrefix(e.src[p.end:], "]]") {
		insert += "]]"
	}
	e.begin(stepEdit)
	e.replace(p.start, p.end, insert)
	caret := p.start + len(target)
	e.commit(caret, caret)
	e.breakCoalescing()
	e.wikiPopup = wikiSuggestionState{}
	e.invalidate()
}

func (e *Editor) setWikiHover(target string, x, y float32, bounds ui.Rect) {
	if e.opts.OnWikiLinkHover == nil || target == e.wikiHover && bounds == e.wikiHoverBounds {
		return
	}
	e.wikiHover = target
	e.wikiHoverBounds = bounds
	e.opts.OnWikiLinkHover(target, x, y, bounds)
}

// wikiLinkTargetAt returns a non-embedded wiki target only when the pointer
// is over its visible text. Parser link ranges can include hidden delimiters.
func (e *Editor) wikiLinkTargetAt(x, y float32, off int) string {
	r, ok := e.linkRangeAt(off)
	if !ok || !e.pointInLink(x, y, r) {
		return ""
	}
	u := e.unitAt(off)
	if u == nil || u.id < 0 {
		return ""
	}
	for _, in := range e.p.doc.Inlines(u.id) {
		if in.Kind != parser.Text {
			continue
		}
		for _, w := range wikiLinks(e.src, in.Start, in.End) {
			if w.from == r.Start && w.to == r.End && !w.embed {
				return w.target
			}
		}
	}
	return ""
}

func (e *Editor) wikiLinkHoverAt(x, y float32, off int) (string, ui.Rect) {
	target := e.wikiLinkTargetAt(x, y, off)
	if target == "" {
		return "", ui.Rect{}
	}
	link, ok := e.linkRangeAt(off)
	if !ok {
		return target, ui.Rect{}
	}
	bounds, ok := e.linkRectAt(x, y, link)
	if !ok {
		return target, ui.Rect{}
	}
	return target, bounds
}

// wikiSuggestionsView paints a small, selectable target list near the caret.
func (e *Editor) wikiSuggestionsView(c *ui.Context, viewport ui.Rect) {
	if !e.wikiPopup.open || len(e.wikiPopup.items) == 0 || !e.focused {
		return
	}
	r, ok := e.caretRect(e.caret)
	if !ok {
		return
	}
	const width = float32(340)
	const rowH = float32(30)
	height := rowH*float32(len(e.wikiPopup.items)) + 2*float32(len(e.wikiPopup.items)-1) + 10
	x := max(viewport.X+8, min(viewport.X+r.X, viewport.X+viewport.W-width-8))
	y := viewport.Y + r.Y - e.scroll.Y + r.H + 6
	if y+height > viewport.Y+viewport.H-8 {
		y = viewport.Y + r.Y - e.scroll.Y - height - 6
	}
	y = max(viewport.Y+8, min(y, viewport.Y+viewport.H-height-8))
	e.wikiPopup.bounds = ui.Rect{X: x, Y: y, W: width, H: height}
	t := c.Theme()
	ui.Overlay(c, func() {
		panel := ui.Column(c).Absolute().Left(x).Top(y).Width(width).Gap(2).
			Padding(5).Radius(8).Background(t.Background).Border(1, t.Border).
			Shadow(0, 10, 28, -5, ui.RGBA(0, 0, 0, 0.24)).Label(locales.EditorWikiLinkSuggestions())
		panel.Children(func() {
			for i, item := range e.wikiPopup.items {
				i, item := i, item
				detail := e.wikiPopup.details[item]
				row := ui.ButtonBase(c).Height(rowH).Radius(5).Padding(0, 9).AlignItems(ui.Center).Justify(ui.Start).Cursor(ui.CursorPointer).Label(locales.EditorLinkSuggestionLabel(item))
				if i == e.wikiPopup.selected {
					row.Background(t.Accent.Alpha(0.16))
				} else if row.Hovered() {
					row.Background(t.SurfaceHover)
				}
				row.Children(func() {
					icon, color := wikiSuggestionIcon(item, t.Accent, t.TextMuted)
					ui.Row(c).Width(18).Height(rowH).Shrink(0).AlignItems(ui.Center).Justify(ui.Center).Children(func() {
						ui.Icon(c, icon).Size(14, 14).TextColor(color)
					})
					label := detail.Title
					if label == "" {
						label = filepath.Base(item)
					}
					ui.Column(c).FillWidth().Gap(1).Children(func() {
						ui.Text(c, label).FontSize(13).TextColor(t.Text).SingleLine().Ellipsis("…")
						if detail.Title != "" {
							ui.Text(c, item).FontSize(11).TextColor(t.TextMuted).SingleLine().Ellipsis("…")
						}
					})
				})
				row.HandleInput(func(ev ui.InputEvent) bool {
					if ev.Kind == ui.InputPointerDown && ev.Button == 0 {
						e.acceptWikiSuggestion(i)
						return true
					}
					return false
				})
				if row.Hovered() && e.wikiPopup.selected != i {
					e.wikiPopup.selected = i
					e.invalidate()
				}
			}
		})
		if panel.Clicked() {
			e.acceptWikiSuggestion(e.wikiPopup.selected)
		}
	})
}

func wikiSuggestionIcon(target string, accent, muted ui.Color) (*ui.SVG, ui.Color) {
	switch strings.ToLower(filepath.Ext(target)) {
	case ".png", ".jpg", ".jpeg", ".gif", ".webp", ".bmp", ".svg", ".avif", ".heic", ".heif":
		return wikiImageIcon, accent
	default:
		return wikiNoteIcon, muted
	}
}
