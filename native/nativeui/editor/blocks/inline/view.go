package inlineui

import (
	"path/filepath"
	"strings"

	"github.com/egoist/mygo/ui"
	"muse/locales"
)

// LinkTargetState reports whether a wiki target resolves in the current vault
// snapshot. Unknown means the index has not supplied a result yet.
type LinkTargetState uint8

const (
	LinkTargetUnknown LinkTargetState = iota
	LinkTargetValid
	LinkTargetInvalid
)

// Render paints the popup from immutable view values. Selection and acceptance
// are reported as typed actions to the owning editor host.
func Render(c *ui.Context, state Frame, dispatch func(Action)) {
	if !state.open || len(state.items) == 0 || state.bounds.W <= 0 || state.bounds.H <= 0 {
		return
	}
	x, y, width := state.bounds.X, state.bounds.Y, state.bounds.W
	const rowH = float32(30)
	t := c.Theme()
	ui.Overlay(c, func() {
		panel := ui.Column(c).Absolute().Left(x).Top(y).Width(width).Gap(2).
			Padding(5).Radius(8).Background(t.Background).Border(1, t.Border).
			Shadow(0, 10, 28, -5, ui.RGBA(0, 0, 0, 0.24)).Label(locales.EditorWikiLinkSuggestions())
		panel.Children(func() {
			for i, item := range state.items {
				i, item := i, item
				row := ui.ButtonBase(c).Height(rowH).Radius(5).Padding(0, 9).AlignItems(ui.Center).Justify(ui.Start).Cursor(ui.CursorPointer).Label(locales.EditorLinkSuggestionLabel(item.Target))
				if i == state.selected {
					row.Background(t.Accent.Alpha(0.16))
				} else if row.Hovered() {
					row.Background(t.SurfaceHover)
				}
				row.Children(func() {
					icon, color := SuggestionIcon(item.Target, t.Accent, t.TextMuted)
					ui.Row(c).Width(18).Height(rowH).Shrink(0).AlignItems(ui.Center).Justify(ui.Center).Children(func() {
						ui.Icon(c, icon).Size(14, 14).TextColor(color)
					})
					label := item.Title
					if label == "" {
						label = filepath.Base(item.Target)
					}
					ui.Column(c).FillWidth().Gap(1).Children(func() {
						ui.Text(c, label).FontSize(13).TextColor(t.Text).SingleLine().Ellipsis("…")
						if item.Title != "" {
							ui.Text(c, item.Target).FontSize(11).TextColor(t.TextMuted).SingleLine().Ellipsis("…")
						}
					})
				})
				row.HandleInput(func(ev ui.InputEvent) bool {
					if ev.Kind == ui.InputPointerDown && ev.Button == 0 {
						dispatch(Action{Kind: ActionAccept, Index: i})
						return true
					}
					return false
				})
				if row.Hovered() && state.selected != i {
					dispatch(Action{Kind: ActionSelectionChanged, Index: i})
				}
			}
		})
		if panel.Clicked() {
			dispatch(Action{Kind: ActionAccept, Index: state.selected})
		}
	})
}

func wikiLucideIcon(shapes string) *ui.SVG {
	return ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round">` + shapes + `</svg>`))
}

var (
	wikiNoteIcon  = wikiLucideIcon(`<path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"/><path d="M14 2v6h6"/><path d="M8 13h8M8 17h6"/>`)
	wikiImageIcon = wikiLucideIcon(`<rect x="3" y="3" width="18" height="18" rx="2"/><circle cx="8.5" cy="8.5" r="1.5"/><path d="m21 15-5-5L5 21"/>`)
)

// SuggestionIcon selects a file-type glyph and its accent.
func SuggestionIcon(target string, accent, muted ui.Color) (*ui.SVG, ui.Color) {
	switch strings.ToLower(filepath.Ext(target)) {
	case ".png", ".jpg", ".jpeg", ".gif", ".webp", ".bmp", ".svg", ".avif", ".heic", ".heif":
		return wikiImageIcon, accent
	default:
		return wikiNoteIcon, muted
	}
}
