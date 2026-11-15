package plan

import (
	"testing"

	"muse/document/types"
	"muse/search/fields"
	"muse/search/query"
)

func TestCompiledPlanMatchesMetadataAndRetainsText(t *testing.T) {
	ast, err := query.Parse(`migration tag:work created>=2026-01-01`)
	if err != nil {
		t.Fatal(err)
	}
	compiled, err := Compile(ast, fields.NewDefaults())
	if err != nil {
		t.Fatal(err)
	}
	if compiled.Text != "migration" {
		t.Fatalf("compiled text = %q", compiled.Text)
	}
	match, err := compiled.Match(types.Metadata{Tags: []string{"work"}, CreatedAt: "2026-03-02"})
	if err != nil || !match {
		t.Fatalf("Match() = %v, %v", match, err)
	}
	match, err = compiled.Match(types.Metadata{Tags: []string{"private"}, CreatedAt: "2026-03-02"})
	if err != nil || match {
		t.Fatalf("Match(non-matching metadata) = %v, %v", match, err)
	}
}

func TestCompileRejectsUnknownQueryField(t *testing.T) {
	ast := query.AST{Filters: []query.Filter{{Field: "mystery", Operator: "eq", Value: "x"}}}
	if _, err := Compile(ast, fields.NewDefaults()); err == nil {
		t.Fatal("unknown field compiled")
	}
}
