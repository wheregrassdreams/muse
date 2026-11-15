// Package vault contains workflows that coordinate a vault's index, link
// graph, file catalogue, and watcher lifecycle.
package vault

import (
	"errors"
	"sync"

	"muse/vault/links"
)

// LinkProjection is the narrow index view required to rebuild and refresh the
// vault's derived relationship graph.
type LinkProjection interface {
	LinkSnapshot() []links.Document
	LinkDocument(path string) (links.Document, bool)
}

// LinkGraph coordinates the lifecycle of the derived link graph for one
// vault. The parser/index remains the source of immutable document snapshots;
// this use case applies rebuild, update, deletion, and shutdown as one policy.
type LinkGraph struct {
	mu         sync.RWMutex
	root       string
	projection LinkProjection
	index      *links.Index
	closed     bool
}

func NewLinkGraph(root string, projection LinkProjection) (*LinkGraph, error) {
	if projection == nil {
		return nil, errors.New("application/vault: link projection is required")
	}
	g := &LinkGraph{root: root, projection: projection, index: links.New()}
	if err := g.Rebuild(); err != nil {
		return nil, err
	}
	return g, nil
}

func (g *LinkGraph) Rebuild() error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.closed {
		return errors.New("application/vault: link graph is closed")
	}
	return g.index.Rebuild(g.root, g.projection.LinkSnapshot())
}

// Apply refreshes a single graph entry from the canonical index after a file
// or editor snapshot has been parsed. Missing projections remove stale links.
func (g *LinkGraph) Apply(path string) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.closed {
		return errors.New("application/vault: link graph is closed")
	}
	doc, ok := g.projection.LinkDocument(path)
	if !ok {
		g.index.Remove(path)
		return nil
	}
	return g.index.ApplySnapshot(doc)
}

func (g *LinkGraph) Remove(path string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if !g.closed {
		g.index.Remove(path)
	}
}

func (g *LinkGraph) Resolve(current, target string) (links.Location, bool) {
	g.mu.RLock()
	defer g.mu.RUnlock()
	if g.closed {
		return links.Location{}, false
	}
	return g.index.Resolve(current, target)
}

func (g *LinkGraph) Outgoing(source string) []links.Link {
	g.mu.RLock()
	defer g.mu.RUnlock()
	if g.closed {
		return nil
	}
	return g.index.Outgoing(source)
}

func (g *LinkGraph) Backlinks(target string) []links.Link {
	g.mu.RLock()
	defer g.mu.RUnlock()
	if g.closed {
		return nil
	}
	return g.index.Backlinks(target)
}

func (g *LinkGraph) BacklinkSources(target string) []links.BacklinkSource {
	g.mu.RLock()
	defer g.mu.RUnlock()
	if g.closed {
		return nil
	}
	return g.index.BacklinkSources(target)
}

// Close ends the graph's vault-scoped lifetime. Reopening a vault creates a
// new graph and cannot retain cached paths from the previous root.
func (g *LinkGraph) Close() {
	g.mu.Lock()
	g.closed = true
	g.index = nil
	g.projection = nil
	g.mu.Unlock()
}
