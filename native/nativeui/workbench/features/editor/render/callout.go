package render

import "github.com/egoist/mygo/ui"

type calloutCard struct {
	id                 int
	x, top, bottom     float32
	right, fontSize    float32
	title              string
	alert              AlertKind
	headerSourceReveal bool
}

// ProjectCallouts groups visible callout lines by parser-owned identity and
// projects each group into one immutable card block. Source parsing and the
// decision to reveal a header remain with the host.
func ProjectCallouts(lines []CalloutLine, viewportTop, viewportBottom float32) []Block {
	byID := make(map[int]int, len(lines))
	cards := make([]calloutCard, 0, len(lines))
	for _, line := range lines {
		index, ok := byID[line.ID]
		if !ok {
			index = len(cards)
			byID[line.ID] = index
			cards = append(cards, calloutCard{
				id: line.ID, x: line.X, top: line.Y - line.FontSize*.45,
				bottom: line.Y + line.Height, right: line.Right, fontSize: line.FontSize,
				title: line.Title, alert: line.Alert,
				headerSourceReveal: line.HeaderSourceRevealed,
			})
			continue
		}
		card := &cards[index]
		card.bottom = line.Y + line.Height
		card.headerSourceReveal = card.headerSourceReveal || line.HeaderSourceRevealed
	}
	blocks := make([]Block, 0, len(cards))
	for _, card := range cards {
		if card.bottom < viewportTop || card.top > viewportBottom {
			continue
		}
		title := card.title
		if card.headerSourceReveal {
			title = ""
		}
		blocks = append(blocks, Block{
			Kind:   CalloutBlock,
			Bounds: ui.Rect{X: card.x, Y: card.top, W: card.right - card.x, H: card.bottom - card.top + card.fontSize*.6},
			Title:  title,
			Alert:  card.alert,
		})
	}
	return blocks
}
