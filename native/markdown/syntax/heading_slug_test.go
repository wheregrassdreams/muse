package syntax

import "testing"

func TestHeadingSlug(t *testing.T) {
	for _, tc := range []struct{ input, want string }{
		{"  Hello, World!  ", "hello-world"},
		{"API_v2 — 中文 标题", "api_v2-—-中文-标题"},
		{"a---b", "a---b"},
	} {
		if got := HeadingSlug(tc.input); got != tc.want {
			t.Errorf("HeadingSlug(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}
