// Package rank contains explicit, composable result-scoring policies.
package rank

import (
	"errors"
	"fmt"
	"muse/document/types"
	"sync"
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
