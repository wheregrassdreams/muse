// Package fields owns query-field semantics over document metadata.
package fields

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	notedate "muse/document/date"
	"muse/document/types"
	"muse/search/query"
)

type Matcher func(types.Metadata, string, string) (bool, error)

type Spec struct {
	Name      string
	Operators []string
	Match     Matcher
}

type Registry struct {
	mu     sync.RWMutex
	fields map[string]Spec
}

func NewRegistry() *Registry { return &Registry{fields: make(map[string]Spec)} }

func (r *Registry) Register(spec Spec) error {
	name := strings.ToLower(strings.TrimSpace(spec.Name))
	if name == "" || spec.Match == nil {
		return errors.New("search fields: field name and matcher are required")
	}
	spec.Name = name
	seen := make(map[string]struct{}, len(spec.Operators))
	operators := make([]string, 0, len(spec.Operators))
	for _, operator := range spec.Operators {
		if _, ok := seen[operator]; !ok {
			seen[operator] = struct{}{}
			operators = append(operators, operator)
		}
	}
	spec.Operators = operators
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.fields[name]; exists {
		return fmt.Errorf("search fields: duplicate field %q", name)
	}
	r.fields[name] = spec
	return nil
}

func (r *Registry) Match(metadata types.Metadata, filter query.Filter) (bool, error) {
	r.mu.RLock()
	spec, ok := r.fields[strings.ToLower(filter.Field)]
	r.mu.RUnlock()
	if !ok {
		return false, fmt.Errorf("search fields: unknown field %q", filter.Field)
	}
	if !contains(spec.Operators, filter.Operator) {
		return false, fmt.Errorf("search fields: operator %q is not supported for %q", filter.Operator, spec.Name)
	}
	matched, err := spec.Match(metadata.Clone(), filter.Operator, filter.Value)
	if err == nil && filter.Negated {
		matched = !matched
	}
	return matched, err
}

func (r *Registry) Supports(field, operator string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	spec, ok := r.fields[strings.ToLower(field)]
	return ok && contains(spec.Operators, operator)
}

func NewDefaults() *Registry {
	r := NewRegistry()
	_ = r.Register(Spec{Name: "type", Operators: []string{"eq", "ne"}, Match: func(m types.Metadata, op, value string) (bool, error) {
		match := strings.EqualFold(string(m.Type), value)
		if op == "ne" {
			match = !match
		}
		return match, nil
	}})
	_ = r.Register(Spec{Name: "tag", Operators: []string{"eq", "contains"}, Match: func(m types.Metadata, op, value string) (bool, error) {
		for _, tag := range m.Tags {
			if (op == "eq" && strings.EqualFold(tag, value)) || (op == "contains" && strings.Contains(strings.ToLower(tag), strings.ToLower(value))) {
				return true, nil
			}
		}
		return false, nil
	}})
	for _, field := range []struct {
		name  string
		value func(types.Metadata) string
	}{
		{"created", func(m types.Metadata) string { return m.CreatedAt }},
		{"updated", func(m types.Metadata) string { return m.UpdatedAt }},
	} {
		f := field
		_ = r.Register(Spec{Name: f.name, Operators: []string{"eq", ">", ">=", "<", "<="}, Match: func(m types.Metadata, op, value string) (bool, error) {
			actual, _, err := notedate.Parse(f.value(m))
			if err != nil {
				return false, nil
			}
			wanted, _, err := notedate.Parse(value)
			if err != nil {
				return false, fmt.Errorf("search fields: invalid %s date %q: %w", f.name, value, err)
			}
			if actual.Location() != time.UTC {
				actual = actual.UTC()
			}
			wanted = wanted.UTC()
			cmp := actual.Compare(wanted)
			switch op {
			case "eq":
				return cmp == 0, nil
			case ">":
				return cmp > 0, nil
			case ">=":
				return cmp >= 0, nil
			case "<":
				return cmp < 0, nil
			case "<=":
				return cmp <= 0, nil
			}
			return false, nil
		}})
	}
	return r
}

func contains(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}
