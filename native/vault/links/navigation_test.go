package links

import "testing"

func TestParseNavigationTarget(t *testing.T) {
	for _, tc := range []struct {
		name string
		href string
		want NavigationTarget
	}{
		{"empty", "  ", NavigationTarget{Kind: NavigationEmpty}},
		{"wiki", " wiki:Notes/One#Part ", NavigationTarget{Kind: NavigationWiki, Value: "Notes/One#Part"}},
		{"heading", "#Overview", NavigationTarget{Kind: NavigationHeading, Value: "Overview"}},
		{"web", "https://example.com/a?q=1", NavigationTarget{Kind: NavigationExternal, Value: "https://example.com/a?q=1"}},
		{"mail", "mailto:team@example.com", NavigationTarget{Kind: NavigationExternal, Value: "mailto:team@example.com"}},
		{"path fragment", "Notes/One.md#Overview", NavigationTarget{Kind: NavigationPath, Value: "Notes/One.md"}},
		{"path query", "Notes/One.md?download=1", NavigationTarget{Kind: NavigationPath, Value: "Notes/One.md"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := ParseNavigationTarget(tc.href); got != tc.want {
				t.Fatalf("ParseNavigationTarget(%q) = %+v, want %+v", tc.href, got, tc.want)
			}
		})
	}
}
