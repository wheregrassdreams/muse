package links

import "strings"

// NavigationKind identifies how a clicked Markdown link should be handled by
// the host application.
type NavigationKind uint8

const (
	NavigationEmpty NavigationKind = iota
	NavigationWiki
	NavigationHeading
	NavigationExternal
	NavigationPath
)

// NavigationTarget is the host-neutral result of parsing a link destination.
// Value is a wiki target, heading fragment, external URI, or local path with
// its query and fragment removed.
type NavigationTarget struct {
	Kind  NavigationKind
	Value string
}

// ParseNavigationTarget classifies the Markdown link forms Muse supports
// before the platform host resolves paths or launches external protocols.
func ParseNavigationTarget(href string) NavigationTarget {
	href = strings.TrimSpace(href)
	switch {
	case href == "":
		return NavigationTarget{Kind: NavigationEmpty}
	case strings.HasPrefix(href, "wiki:"):
		return NavigationTarget{Kind: NavigationWiki, Value: strings.TrimPrefix(href, "wiki:")}
	case strings.HasPrefix(href, "#"):
		return NavigationTarget{Kind: NavigationHeading, Value: strings.TrimPrefix(href, "#")}
	case strings.Contains(href, "://") || strings.HasPrefix(href, "mailto:"):
		return NavigationTarget{Kind: NavigationExternal, Value: href}
	default:
		if i := strings.IndexAny(href, "#?"); i >= 0 {
			href = href[:i]
		}
		return NavigationTarget{Kind: NavigationPath, Value: href}
	}
}
