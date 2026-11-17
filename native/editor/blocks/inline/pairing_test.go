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

func TestPlanAutoPairAtDerivesUnicodeAndInlineCodeContext(t *testing.T) {
	if got, ok := PlanAutoPairAt("x", 1, '"', false); ok || got != 0 {
		t.Fatalf("quote after word = %q, %v; want no pair", got, ok)
	}
	if got, ok := PlanAutoPairAt("👩‍💻 ", len("👩‍💻 "), '(', false); !ok || got != ')' {
		t.Fatalf("pair after multibyte source = %q, %v", got, ok)
	}
	if got, ok := PlanAutoPairAt("`code", len("`code"), '[', false); ok || got != 0 {
		t.Fatalf("link opener inside inline code = %q, %v; want no pair", got, ok)
	}
	if _, ok := PlanAutoPairAt("é", len("é"), '(', false); !ok {
		t.Fatal("valid UTF-8 boundary was rejected")
	}
	if _, ok := PlanAutoPairAt("é", 1, '(', false); ok {
		t.Fatal("mid-rune caret was accepted")
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

func TestShouldSkipCloserAtReadsSourceContext(t *testing.T) {
	if !ShouldSkipCloserAt("(x)", len("(x"), ')') {
		t.Fatal("open pair did not skip the existing closer")
	}
	if ShouldSkipCloserAt("(x)", len("(x)"), ')') {
		t.Fatal("caret after closer incorrectly skipped")
	}
}
