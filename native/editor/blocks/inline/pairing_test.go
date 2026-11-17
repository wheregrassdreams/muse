package inline

import "testing"

func TestPairForUsesCodeOnlyQuoteRules(t *testing.T) {
	for _, tc := range []struct {
		open rune
		code bool
		want rune
		ok   bool
	}{
		{'(', false, ')', true},
		{'（', false, '）', true},
		{'\'', false, 0, false},
		{'`', false, 0, false},
		{'\'', true, '\'', true},
		{'`', true, '`', true},
	} {
		got, ok := PairFor(tc.open, tc.code)
		if got != tc.want || ok != tc.ok {
			t.Errorf("PairFor(%q, %v) = %q, %v; want %q, %v", tc.open, tc.code, got, ok, tc.want, tc.ok)
		}
	}
}

func TestPlanAutoPairRespectsFollowingAndPreviousText(t *testing.T) {
	for _, tc := range []struct {
		name       string
		open, next rune
		afterWord  bool
		inCode     bool
		want       rune
		ok         bool
	}{
		{"open before whitespace", '(', ' ', false, false, ')', true},
		{"open before a word", '(', 'x', false, false, 0, false},
		{"quote after word is not paired", '"', 0, true, false, 0, false},
		{"quote at empty position is paired", '"', 0, false, false, '"', true},
		{"backtick only in code", '`', 0, false, true, '`', true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := PlanAutoPair(tc.open, tc.next, tc.afterWord, tc.inCode)
			if got != tc.want || ok != tc.ok {
				t.Fatalf("PlanAutoPair() = %q, %v; want %q, %v", got, ok, tc.want, tc.ok)
			}
		})
	}
}

func TestShouldSkipCloserOnlyWhenItsPairIsOpen(t *testing.T) {
	for _, tc := range []struct {
		before string
		closer rune
		want   bool
	}{
		{"(", ')', true},
		{"()", ')', false},
		{"“", '”', true},
		{"hello", '”', false},
		{"`code", '`', true},
		{"``code``", '`', false},
		{"", ')', false},
		{">", '>', false},
	} {
		if got := ShouldSkipCloser(tc.before, tc.closer); got != tc.want {
			t.Errorf("ShouldSkipCloser(%q, %q) = %v; want %v", tc.before, tc.closer, got, tc.want)
		}
	}
}
