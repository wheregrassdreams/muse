package search

import "muse/search/quickopen"

// QuickOpenItem is a caller-owned catalogue projection for fuzzy file picking.
type QuickOpenItem = quickopen.Candidate

// QuickOpenMatch is a ranked candidate with rune-based highlight offsets.
type QuickOpenMatch = quickopen.Match

// RankQuickOpen is the stable search facade for the application's Quick Open
// workflow. Catalog construction and query caching remain with the caller.
func RankQuickOpen(query string, items []QuickOpenItem) []QuickOpenMatch {
	return quickopen.Rank(query, items)
}
