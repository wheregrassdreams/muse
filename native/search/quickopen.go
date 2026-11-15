package search

import "muse/search/quickopen"

// QuickOpenItem is a source-agnostic candidate projection. The caller builds
// and refreshes it; QuickOpenService owns its immutable copy and query cache.
type QuickOpenItem = quickopen.Candidate

// QuickOpenMatch is a ranked candidate with rune-based highlight offsets.
type QuickOpenMatch = quickopen.Match

// QuickOpenService owns a bounded candidate snapshot and query cache.
type QuickOpenService = quickopen.Service

// NewQuickOpenService constructs the search-owned catalogue/query service.
func NewQuickOpenService(maxCachedQueries int) *QuickOpenService {
	return quickopen.NewService(maxCachedQueries)
}

// RankQuickOpen is the stateless search facade for fuzzy Quick Open matching.
// Applications that need revisioned snapshots and bounded query caching should
// use NewQuickOpenService; catalog construction and refresh remain caller-owned.
func RankQuickOpen(query string, items []QuickOpenItem) []QuickOpenMatch {
	return quickopen.Rank(query, items)
}
