package fields

import (
	"testing"

	"muse/document/types"
	"muse/search/query"
)

func TestDefaultFieldsMatchTypeTagsAndTime(t *testing.T) {
	meta := types.Metadata{Type: "research", Tags: []string{"work"}, CreatedAt: "2026-03-01"}
	for _, filter := range []query.Filter{
		{Field: "type", Operator: "eq", Value: "RESEARCH"},
		{Field: "tag", Operator: "contains", Value: "work"},
		{Field: "created", Operator: ">=", Value: "2026-03-01"},
	} {
		matched, err := NewDefaults().Match(meta, filter)
		if err != nil || !matched {
			t.Errorf("Match(%#v) = %v, %v", filter, matched, err)
		}
	}
}

func TestRegistryRejectsDuplicateAndUnknownFields(t *testing.T) {
	r := NewDefaults()
	if err := r.Register(Spec{Name: "type", Operators: []string{"eq"}, Match: func(types.Metadata, string, string) (bool, error) { return true, nil }}); err == nil {
		t.Fatal("duplicate field registered")
	}
	if _, err := r.Match(types.Metadata{}, query.Filter{Field: "missing", Operator: "eq", Value: "x"}); err == nil {
		t.Fatal("unknown field accepted")
	}
}
