// Package rank contains explicit, composable result-scoring policies.
package rank

import (
	"errors"
	"fmt"
	"strings"
	"sync"

	"muse/document/types"
)

type Scorer interface {
	ID() string
	Score(Context) float64
}

// Context is the bounded data available while ranking candidates. It excludes
// note source so custom policies cannot force the index to hydrate every body.
type Context struct {
	Query, ID, Path, Title string
	Metadata               types.Metadata
}

// OverlayInput contains fields available without consulting the durable index.
// Overlays have no corpus statistics, so their score approximates the indexed
// field boosts using exact, prefix, and substring matches.
type OverlayInput struct {
	Query, Title, Path, Source string
	Aliases, Tags              []string
}

// ScoreOverlay ranks an unsaved document against a free-text query. Its scores
// are intentionally only comparable to other overlays; Tantivy's corpus-based
// scores use a different scale.
func ScoreOverlay(input OverlayInput) (float64, bool) {
	q := strings.ToLower(strings.TrimSpace(input.Query))
	if q == "" {
		return 0, false
	}
	title := strings.ToLower(input.Title)
	path := strings.ToLower(input.Path)
	score := 0.0
	if title == q {
		score += 24
	} else if strings.HasPrefix(title, q) {
		score += 18
	} else if strings.Contains(title, q) {
		score += 12
	}
	if strings.HasPrefix(path, q) {
		score += 10
	} else if strings.Contains(path, q) {
		score += 8
	}
	if containsQuery(input.Aliases, q) || containsQuery(input.Tags, q) {
		score += 7
	}
	body := strings.ToLower(input.Source)
	if strings.Contains(body, q) {
		score += 2
	}
	if score == 0 {
		for _, token := range strings.Fields(q) {
			if strings.Contains(title, token) {
				score += 4
			} else if strings.Contains(body, token) {
				score += 0.5
			}
		}
	}
	return score, score > 0
}

func containsQuery(values []string, query string) bool {
	for _, value := range values {
		if strings.Contains(strings.ToLower(value), query) {
			return true
		}
	}
	return false
}

type Registry struct {
	mu      sync.RWMutex
	scorers map[string]Scorer
	order   []string
}

func NewRegistry() *Registry { return &Registry{scorers: make(map[string]Scorer)} }

func (r *Registry) Register(scorer Scorer) error {
	if scorer == nil || scorer.ID() == "" {
		return errors.New("search rank: scorer ID is required")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.scorers[scorer.ID()]; exists {
		return fmt.Errorf("search rank: duplicate scorer %q", scorer.ID())
	}
	r.scorers[scorer.ID()] = scorer
	r.order = append(r.order, scorer.ID())
	return nil
}

func (r *Registry) Score(context Context) float64 {
	var score float64
	r.mu.RLock()
	scorers := make([]Scorer, 0, len(r.order))
	for _, id := range r.order {
		scorers = append(scorers, r.scorers[id])
	}
	r.mu.RUnlock()
	for _, scorer := range scorers {
		candidate := context
		candidate.Metadata = context.Metadata.Clone()
		score += scorer.Score(candidate)
	}
	return score
}
