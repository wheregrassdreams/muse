package agent

import (
	"context"
	"sync"
)

// ProviderCredentials contains configuration needed to query one provider.
// The application may pass credentials to the provider adapter but does not
// persist or inspect them.
type ProviderCredentials struct {
	BaseURL string
	APIKey  string
}

// ProviderModel is the provider-neutral model metadata used by settings UI.
type ProviderModel struct {
	ID            string
	Name          string
	Reasoning     bool
	Input         []string
	ContextWindow int
	MaxTokens     int
}

type ProviderModelList struct {
	StatusCode int
	Models     []ProviderModel
}

type ProviderCheck struct {
	StatusCode int
}

type ProviderErrorKind uint8

const (
	ProviderErrorInvalidConfiguration ProviderErrorKind = iota + 1
	ProviderErrorTimeout
	ProviderErrorNetwork
	ProviderErrorUnauthorized
	ProviderErrorRequest
	ProviderErrorReadResponse
	ProviderErrorInvalidModelList
)

// ProviderError carries stable failure classification for presentation and
// retry policy, while Detail preserves a useful upstream diagnostic.
type ProviderError struct {
	Kind       ProviderErrorKind
	StatusCode int
	Detail     string
}

func (e *ProviderError) Error() string {
	if e == nil {
		return "assistant provider error"
	}
	if e.Detail != "" {
		return e.Detail
	}
	return "assistant provider request failed"
}

// ProviderManagement is the application-owned capability for checking a
// provider endpoint and discovering its available models.
type ProviderManagement interface {
	TestConnection(context.Context, ProviderCredentials) (ProviderCheck, error)
	ListModels(context.Context, ProviderCredentials) (ProviderModelList, error)
}

// ModelCatalog is the transient state of one provider-model listing request.
// It owns request deduplication and returns detached model snapshots.
type ModelCatalog struct {
	mu         sync.RWMutex
	models     []ProviderModel
	loading    bool
	err        string
	generation uint64
}

type ModelCatalogSnapshot struct {
	Models  []ProviderModel
	Loading bool
	Error   string
}

func (c *ModelCatalog) Begin() (uint64, bool) {
	if c == nil {
		return 0, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.loading {
		return c.generation, false
	}
	c.generation++
	c.loading = true
	c.err = ""
	return c.generation, true
}

func (c *ModelCatalog) Complete(generation uint64, models []ProviderModel, err string) bool {
	if c == nil {
		return false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if generation == 0 || generation != c.generation {
		return false
	}
	c.loading = false
	c.models = cloneProviderModels(models)
	c.err = err
	return true
}

func (c *ModelCatalog) Reset() {
	if c == nil {
		return
	}
	c.mu.Lock()
	c.generation++
	c.models = nil
	c.loading = false
	c.err = ""
	c.mu.Unlock()
}

func (c *ModelCatalog) Snapshot() ModelCatalogSnapshot {
	if c == nil {
		return ModelCatalogSnapshot{}
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	return ModelCatalogSnapshot{Models: cloneProviderModels(c.models), Loading: c.loading, Error: c.err}
}

func cloneProviderModels(models []ProviderModel) []ProviderModel {
	clone := append([]ProviderModel(nil), models...)
	for i := range clone {
		clone[i].Input = append([]string(nil), clone[i].Input...)
	}
	return clone
}
