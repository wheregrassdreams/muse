package query

import "testing"

func TestParseFreeTextAndTypedFilters(t *testing.T) {
	got, err := Parse(`architecture notes tag:"deep work" type=reference created>=2026-01-01 -tag:private`)
	if err != nil {
		t.Fatal(err)
	}
	if got.Text != "architecture notes" || len(got.Filters) != 4 {
		t.Fatalf("Parse() = %#v", got)
	}
	want := []Filter{
		{Field: "tag", Operator: "eq", Value: "deep work"},
		{Field: "type", Operator: "eq", Value: "reference"},
		{Field: "created", Operator: ">=", Value: "2026-01-01"},
		{Field: "tag", Operator: "eq", Value: "private", Negated: true},
	}
	for i := range want {
		if got.Filters[i] != want[i] {
			t.Errorf("filter %d = %#v, want %#v", i, got.Filters[i], want[i])
		}
	}
}

func TestParsePreservesQuotedTextAndEscapes(t *testing.T) {
	got, err := Parse(`"two words" path:notes\/draft`)
	if err != nil {
		t.Fatal(err)
	}
	if got.Text != "two words" || len(got.Filters) != 1 || got.Filters[0].Value != "notes/draft" {
		t.Fatalf("Parse() = %#v", got)
	}
}

func TestParseRejectsMalformedFiltersAndQuotes(t *testing.T) {
	for _, input := range []string{`tag:`, `-bare`, `tag:"unfinished`} {
		if _, err := Parse(input); err == nil {
			t.Errorf("Parse(%q) succeeded", input)
		}
	}
}
