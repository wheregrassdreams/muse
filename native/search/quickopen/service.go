package quickopen

import (
	"strconv"
	"strings"
	"sync"
)

const (
	maxCachedQueryBytes  = 256
	hardMaxCachedQueries = 32
	maxCachedResults     = 1024
	maxQueryCacheBytes   = 1 << 20
	cacheEntryWeight     = 64
	matchCacheWeight     = 88 // string/slice headers, path/title, and score
)

type cachedQuery struct {
	matches []Match
	weight  int
}

// Service owns one immutable candidate snapshot and a bounded cache of ranked
// query results. Candidate construction remains with the caller; no candidate
// source text or vault/index storage is retained here.
type Service struct {
	mu         sync.RWMutex
	revision   uint64
	generation uint64
	candidates []Candidate
	cacheLimit int
	cacheBytes int
	queryCache map[string]cachedQuery
	cacheOrder []string
}

// NewService creates a Quick Open query service. A nonpositive query limit
// disables result caching while preserving snapshot ownership and ranking.
func NewService(maxCachedQueries int) *Service {
	if maxCachedQueries > hardMaxCachedQueries {
		maxCachedQueries = hardMaxCachedQueries
	}
	return &Service{cacheLimit: maxCachedQueries, queryCache: make(map[string]cachedQuery)}
}

// ReplaceSnapshot atomically replaces the candidate projection and clears all
// query results. Candidate aliases are copied so caller mutations cannot
// change subsequent searches.
func (s *Service) ReplaceSnapshot(revision uint64, candidates []Candidate) {
	if s == nil {
		return
	}
	owned := make([]Candidate, len(candidates))
	copy(owned, candidates)
	for i := range owned {
		owned[i].Aliases = append([]string(nil), candidates[i].Aliases...)
	}
	s.mu.Lock()
	s.revision = revision
	s.generation++
	s.candidates = owned
	s.queryCache = make(map[string]cachedQuery)
	s.cacheOrder = nil
	s.cacheBytes = 0
	s.mu.Unlock()
}

// Revision returns the revision supplied with the active candidate snapshot.
func (s *Service) Revision() uint64 {
	if s == nil {
		return 0
	}
	s.mu.RLock()
	revision := s.revision
	s.mu.RUnlock()
	return revision
}

// Search ranks the active candidate snapshot and caches small results by
// case-insensitive query. Returned slices are detached from the internal cache.
func (s *Service) Search(query string) []Match {
	if s == nil {
		return nil
	}
	key := strings.TrimSpace(query)
	cacheableQuery := len(key) <= maxCachedQueryBytes
	if cacheableQuery {
		key = strings.ToLower(key)
	}
	for {
		s.mu.RLock()
		if cached, ok := s.queryCache[key]; cacheableQuery && ok {
			out := cloneMatches(cached.matches)
			s.mu.RUnlock()
			return out
		}
		generation := s.generation
		candidates := s.candidates
		s.mu.RUnlock()

		matches := Rank(key, candidates)
		s.mu.Lock()
		if s.generation != generation {
			s.mu.Unlock()
			continue
		}
		if cached, ok := s.queryCache[key]; cacheableQuery && ok {
			out := cloneMatches(cached.matches)
			s.mu.Unlock()
			return out
		}
		if weight, eligible := queryCacheWeight(key, matches); cacheableQuery && eligible && s.cacheLimit > 0 {
			s.insertCacheLocked(key, matches, weight)
		}
		s.mu.Unlock()
		return cloneMatches(matches)
	}
}

func (s *Service) insertCacheLocked(query string, matches []Match, weight int) {
	if s.queryCache == nil {
		s.queryCache = make(map[string]cachedQuery)
	}
	for len(s.cacheOrder) >= s.cacheLimit || s.cacheBytes+weight > maxQueryCacheBytes {
		if len(s.cacheOrder) == 0 {
			return
		}
		oldest := s.cacheOrder[0]
		entry := s.queryCache[oldest]
		delete(s.queryCache, oldest)
		s.cacheBytes -= entry.weight
		copy(s.cacheOrder, s.cacheOrder[1:])
		s.cacheOrder = s.cacheOrder[:len(s.cacheOrder)-1]
	}
	s.queryCache[query] = cachedQuery{matches: cloneMatches(matches), weight: weight}
	s.cacheOrder = append(s.cacheOrder, query)
	s.cacheBytes += weight
}

func queryCacheWeight(query string, matches []Match) (int, bool) {
	if len(query) > maxCachedQueryBytes || len(matches) > maxCachedResults {
		return 0, false
	}
	weight := cacheEntryWeight + len(query) + len(matches)*matchCacheWeight
	for _, match := range matches {
		weight += len(match.Path) + len(match.Title)
		weight += (len(match.TitleMatches) + len(match.PathMatches)) * (strconv.IntSize / 8)
		if weight > maxQueryCacheBytes {
			return 0, false
		}
	}
	return weight, weight <= maxQueryCacheBytes
}

func cloneMatches(matches []Match) []Match {
	if matches == nil {
		return nil
	}
	out := make([]Match, len(matches))
	copy(out, matches)
	for i := range out {
		out[i].TitleMatches = append([]int(nil), matches[i].TitleMatches...)
		out[i].PathMatches = append([]int(nil), matches[i].PathMatches...)
	}
	return out
}
