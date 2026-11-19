package editor

import (
	"sort"
	"strings"

	"github.com/egoist/mygo/ui"
	syntax "github.com/wheregrassdreams/muse-editor/markdown/syntax"
	inlineblock "github.com/wheregrassdreams/muse-editor/markdown/inline"
	"github.com/wheregrassdreams/muse-editor/text/input"
	inline "muse/nativeui/editor/blocks/inline"
	"muse/search/quickopen"
	vaultlinks "muse/vault/links"
)

// updateWikiSuggestions adapts editor source/caret state and vault-provided
// candidates to the detached inline suggestion view model.
func (e *Editor) updateWikiSuggestions() {
	close := func() { e.wikiPopup.Close() }
	if e.opts.WikiLinkSuggestions == nil && e.opts.WikiLinkSuggestionDetails == nil || e.opts.Code != "" ||
		e.Session.Selection().Anchor != e.Caret() || e.compositionText() != "" || e.phantom != nil || e.block != nil || e.tableSelection != nil {
		close()
		return
	}
	e.layoutIfNeeded()
	u := e.unitAt(e.Caret())
	if u != nil && (u.kind == kCode || u.kind == kMath || u.kind == kHTML || u.kind == kRaw) {
		close()
		return
	}
	lineStart := lineStartAt(e.Source(), e.Caret())
	prefix := e.Source()[lineStart:e.Caret()]
	open := strings.LastIndex(prefix, "[[")
	if open < 0 || open > 0 && prefix[open-1] == '\\' || strings.Contains(prefix[open+2:], "]]") {
		close()
		return
	}
	if oddBacktickRun(prefix[:open]) {
		close()
		return
	}
	query := prefix[open+2:]
	if strings.ContainsAny(query, "\r\n") {
		close()
		return
	}
	start, end := lineStart+open+2, e.Caret()
	if e.wikiPopup.DismissedFor(query, start) {
		e.wikiPopup.RefreshDismissedAnchor(query, start, end)
		return
	}

	var suggestions []vaultlinks.Suggestion
	if e.opts.WikiLinkSuggestionDetails != nil {
		suggestions = e.opts.WikiLinkSuggestionDetails(query)
	} else {
		for _, target := range e.opts.WikiLinkSuggestions(query) {
			suggestions = append(suggestions, vaultlinks.Suggestion{Target: target})
		}
	}
	seen := make(map[string]struct{}, len(suggestions))
	order := make(map[string]int, len(suggestions))
	candidates := make([]quickopen.Candidate, 0, len(suggestions))
	details := make(map[string]vaultlinks.Suggestion, len(suggestions))
	for _, suggestion := range suggestions {
		target := strings.TrimSpace(suggestion.Target)
		if target == "" {
			continue
		}
		if _, ok := seen[target]; ok {
			continue
		}
		seen[target] = struct{}{}
		order[target] = len(candidates)
		suggestion.Target = target
		details[target] = suggestion
		candidates = append(candidates, quickopen.Candidate{
			Path: target, PathText: target, Title: suggestion.Title, Aliases: suggestion.Aliases,
		})
	}
	matches := quickopen.Rank(query, candidates)
	// Quick Open uses path order to break score ties, and sorts an empty query
	// alphabetically. Wiki suggestions are already ordered by the vault's
	// catalogue; keep that order for ties so keyboard selection accepts the row
	// the user sees at that index.
	sort.SliceStable(matches, func(i, j int) bool {
		if matches[i].Score != matches[j].Score {
			return matches[i].Score < matches[j].Score
		}
		return order[matches[i].Path] < order[matches[j].Path]
	})
	items := make([]inline.Suggestion, 0, min(len(matches), inline.SuggestionLimit))
	for i := 0; i < len(matches) && i < inline.SuggestionLimit; i++ {
		detail := details[matches[i].Path]
		items = append(items, inline.Suggestion{Target: matches[i].Path, Title: detail.Title})
	}
	e.wikiPopup.Replace(query, start, end, items)
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

func (e *Editor) wikiSuggestionKey(event input.KeyEvent) bool {
	action, handled := e.wikiPopup.HandleKey(event)
	if !handled {
		return false
	}
	e.applyWikiSuggestionAction(action)
	return true
}

func (e *Editor) linkRangeAt(off int) (Range, bool) {
	u := e.unitAt(off)
	if u == nil || u.id < 0 || u.kind != kPara && u.kind != kHeading {
		return Range{}, false
	}
	for _, in := range e.p.Parser().Inlines(u.id) {
		if in.Kind == syntax.IncrementalLink && in.Start <= off && off <= in.End {
			return Range{Start: in.Start, End: in.End}, true
		}
		if in.Kind == syntax.IncrementalText {
			for _, w := range inlineblock.ParseWikiLinks(e.Source(), in.Start, in.End) {
				if !w.Embed && w.From <= off && off <= w.To {
					return Range{Start: w.From, End: w.To}, true
				}
			}
		}
	}
	return Range{}, false
}

// linkAt resolves a click target from the syntax snapshot without exposing
// parser nodes to app/workspace consumers.
func (e *Editor) linkAt(off int) string {
	u := e.unitAt(off)
	if u == nil || u.id < 0 || u.kind != kPara && u.kind != kHeading {
		return ""
	}
	for _, in := range e.p.Parser().Inlines(u.id) {
		switch in.Kind {
		case syntax.IncrementalLink, syntax.IncrementalImage:
			if in.Start <= off && off <= in.End {
				return in.Dest
			}
		case syntax.IncrementalText:
			for _, w := range inlineblock.ParseWikiLinks(e.Source(), in.Start, in.End) {
				if w.From <= off && off <= w.To && !w.Embed {
					return "wiki:" + w.Target
				}
			}
		}
	}
	return ""
}

func (e *Editor) pointInLink(x, y float32, link Range) bool {
	_, ok := e.linkRectAt(x, y, link)
	return ok
}

func (e *Editor) linkRectAt(x, y float32, link Range) (ui.Rect, bool) {
	l := e.leafAtPoint(y)
	if l == nil || l.view == nil || l.view.layout == nil || link.Start >= l.end || link.End <= l.lineStart {
		return ui.Rect{}, false
	}
	v := l.view
	local := ui.Point{X: x - l.x - v.tx, Y: y - l.y - v.ty}
	for _, p := range v.disp.pieces {
		start, end := p.s0+l.lineStart, p.s1+l.lineStart
		isIcon := p.kind == pDeco && start == link.End && strings.Contains(p.text, "↗")
		if !isIcon && (p.kind == pDeco || end <= link.Start || start >= link.End) {
			continue
		}
		for _, r := range v.layout.SelectionRects(ui.TextInputRange{Start: p.d0, End: p.d1}) {
			if r.Contains(local.X, local.Y) {
				r.X += l.x + v.tx
				r.Y += l.y + v.ty
				return r, true
			}
		}
	}
	return ui.Rect{}, false
}

func (e *Editor) applyWikiSuggestionAction(action inline.Action) {
	e.wikiPopup.ApplyAction(action)
	switch action.Kind {
	case inline.ActionSelectionChanged:
		e.invalidate()
	case inline.ActionAccept:
		e.acceptWikiSuggestion(action.Index)
	case inline.ActionDismiss:
		e.invalidate()
	}
}

func (e *Editor) wikiSuggestionIndexAt(x, y float32) (int, bool) {
	return e.wikiPopup.HitTest(x, y)
}

func (e *Editor) acceptWikiSuggestion(index int) {
	frame := e.wikiPopup.Frame()
	items := frame.Items()
	start, end := frame.SourceRange()
	if !frame.Open() || index < 0 || index >= len(items) || start > end || end > len(e.Source()) {
		return
	}
	target := items[index].Target
	insert := target
	if !strings.HasPrefix(e.Source()[end:], "]]") {
		insert += "]]"
	}
	e.begin(stepEdit)
	e.replace(start, end, insert)
	caret := start + len(target)
	e.commit(caret, caret)
	e.breakCoalescing()
	e.wikiPopup.Reset()
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
	for _, in := range e.p.Parser().Inlines(u.id) {
		if in.Kind != syntax.IncrementalText {
			continue
		}
		for _, w := range inlineblock.ParseWikiLinks(e.Source(), in.Start, in.End) {
			if w.From == r.Start && w.To == r.End && !w.Embed {
				return w.Target
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

func (e *Editor) wikiSuggestionsView(c *ui.Context, viewport ui.Rect) {
	frame := e.wikiPopup.Frame()
	items := frame.Items()
	if !frame.Open() || len(items) == 0 || !e.focused {
		return
	}
	caret, ok := e.caretRect(e.Caret())
	if !ok {
		return
	}
	e.wikiPopup.SetBounds(inline.Bounds(viewport, caret, e.scroll.Y, len(items)))
	frame = e.wikiPopup.Frame()
	inline.Render(c, frame, e.applyWikiSuggestionAction)
}
