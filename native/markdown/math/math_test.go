package math

import "testing"

func TestTeXToUnicodeFallback(t *testing.T) {
	cases := map[string]string{
		`\alpha + \beta`: "α + β",
		`E = mc^2`:       "E = mc²",
		`x_i`:            "xᵢ",
		`\frac{a}{b}`:    "a/b",
		`\sqrt{x}`:       "√(x)",
		`a \le b \ge c`:  "a ≤ b ≥ c",
	}
	for in, want := range cases {
		if got := TeXToUnicode(in); got != want {
			t.Errorf("%q = %q, want %q", in, got, want)
		}
	}
}
