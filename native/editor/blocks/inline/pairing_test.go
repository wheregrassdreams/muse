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
		got, ok := pairFor(tc.open, tc.code)
		if got != tc.want || ok != tc.ok {
			t.Errorf("PairFor(%q, %v) = %q, %v; want %q, %v", tc.open, tc.code, got, ok, tc.want, tc.ok)
		}
	}
}

func TestWrapPairForIncludesSymmetricFormattingMarkers(t *testing.T) {
	for _, tc := range []struct {
		open rune
		code bool
		want rune
	}{{'(', false, ')'}, {'"', false, '"'}, {'*', false, '*'}, {'_', false, '_'}, {'~', false, '~'}, {'‘', false, '’'}, {'\'', true, '\''}} {
		if got, ok := WrapPairFor(tc.open, tc.code); !ok || got != tc.want {
			t.Errorf("WrapPairFor(%q, %v) = %q, %v; want %q, true", tc.open, tc.code, got, ok, tc.want)
		}
	}
	if _, ok := WrapPairFor('x', false); ok {
		t.Fatal("WrapPairFor accepted a non-delimiter")
	}
	if _, ok := WrapPairFor('\'', false); ok {
		t.Fatal("WrapPairFor accepted apostrophe wrapping outside code")
	}
}

func TestPlanAutoPairRespectsFollowingAndPreviousText(t *testing.T) {
	for _, tc := range []struct {
		name  string
		input AutoPairInput
		want  rune
		ok    bool
	}{
		{"open before whitespace", AutoPairInput{Open: '(', Next: ' '}, ')', true},
		{"open before a word", AutoPairInput{Open: '(', Next: 'x'}, 0, false},
		{"quote after word is not paired", AutoPairInput{Open: '"', PreviousWord: true}, 0, false},
		{"quote at empty position is paired", AutoPairInput{Open: '"'}, '"', true},
		{"backtick only in code", AutoPairInput{Open: '`', InCode: true}, '`', true},
		{"link opener in inline code", AutoPairInput{Open: '[', InInlineCode: true}, 0, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := PlanAutoPair(tc.input)
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
		if got := ShouldSkipCloser(tc.before, tc.closer, tc.closer); got != tc.want {
			t.Errorf("ShouldSkipCloser(%q, %q) = %v; want %v", tc.before, tc.closer, got, tc.want)
		}
	}
	if ShouldSkipCloser("(", 'x', ')') {
		t.Fatal("ShouldSkipCloser accepted a different next rune")
	}
}
