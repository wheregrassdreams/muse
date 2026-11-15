// Package plan compiles validated search syntax into reusable metadata filters.
package plan

import (
	"fmt"

	"muse/document/types"
	"muse/search/fields"
	"muse/search/query"
)

type Plan struct {
	Text    string
	filters []query.Filter
	fields  *fields.Registry
}

func Compile(ast query.AST, registry *fields.Registry) (*Plan, error) {
	if registry == nil {
		return nil, fmt.Errorf("search plan: field registry is required")
	}
	for _, filter := range ast.Filters {
		if !registry.Supports(filter.Field, filter.Operator) {
			return nil, fmt.Errorf("search plan: unsupported field/operator %q %q", filter.Field, filter.Operator)
		}
	}
	return &Plan{Text: ast.Text, filters: append([]query.Filter(nil), ast.Filters...), fields: registry}, nil
}

func (p *Plan) Match(metadata types.Metadata) (bool, error) {
	if p == nil {
		return false, fmt.Errorf("search plan: nil plan")
	}
	for _, filter := range p.filters {
		matched, err := p.fields.Match(metadata, filter)
		if err != nil {
			return false, err
		}
		if !matched {
			return false, nil
		}
	}
	return true, nil
}
