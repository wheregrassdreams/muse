package inline

import "strings"

// WikiLink is a parsed source projection of a Muse wiki link or embed.
// Offsets are half-open UTF-8 byte ranges into the original source.
type WikiLink struct {
	From, To           int
	LabelFrom, LabelTo int
	Target             string
	Embed              bool
}

// LinkCandidate is a parser-projected inline that may act as a navigable
// target. A host supplies candidates from its authoritative Markdown parser;
// this package resolves pointer source offsets without owning that parser.
type LinkCandidate struct {
	Kind        LinkCandidateKind
	Start, End  int
	Destination string
}

type LinkCandidateKind uint8

const (
	LinkCandidateText LinkCandidateKind = iota + 1
	LinkCandidateLink
	LinkCandidateImage
)

// LinkTargetAt returns a navigable destination under a source offset. Wiki
// embeds are excluded because their click behavior belongs to media embeds.
func LinkTargetAt(source string, candidates []LinkCandidate, offset int) (string, bool) {
	for _, candidate := range candidates {
		switch candidate.Kind {
		case LinkCandidateLink, LinkCandidateImage:
			if candidate.Start <= offset && offset <= candidate.End {
				return candidate.Destination, candidate.Destination != ""
			}
		case LinkCandidateText:
			for _, link := range ParseWikiLinks(source, candidate.Start, candidate.End) {
				if link.From <= offset && offset <= link.To && !link.Embed {
					return "wiki:" + link.Target, link.Target != ""
				}
			}
		}
	}
	return "", false
}

// LinkRangeAt returns the source range of a navigable link under an offset.
func LinkRangeAt(source string, candidates []LinkCandidate, offset int) (int, int, bool) {
	for _, candidate := range candidates {
		switch candidate.Kind {
		case LinkCandidateLink:
			if candidate.Start <= offset && offset <= candidate.End {
				return candidate.Start, candidate.End, true
			}
		case LinkCandidateText:
			for _, link := range ParseWikiLinks(source, candidate.Start, candidate.End) {
				if link.From <= offset && offset <= link.To && !link.Embed {
					return link.From, link.To, true
				}
			}
		}
	}
	return 0, 0, false
}

// ParseWikiLinks finds Muse wiki-link syntax in a source range. This is a
// Muse inline extension, not a second Markdown parser; callers should pass
// the smallest parser-owned text range that can contain these constructs.
func ParseWikiLinks(source string, from, to int) []WikiLink {
	if from < 0 || to < from || to > len(source) {
		return nil
	}
	var out []WikiLink
	for p := from; p+1 < to; {
		i := strings.Index(source[p:to], "[[")
		if i < 0 {
			return out
		}
		i += p
		if i > from && source[i-1] == '\\' {
			p = i + 2
			continue
		}
		j := strings.Index(source[i+2:to], "]]")
		if j < 0 {
			return out
		}
		j += i + 2
		inner := source[i+2 : j]
		if strings.ContainsAny(inner, "\n\r") || strings.TrimSpace(inner) == "" {
			p = i + 2
			continue
		}
		start, embed := i, i > from && source[i-1] == '!'
		if embed {
			start--
		}
		labelFrom, labelTo, target := i+2, j, inner
		if k := strings.IndexByte(inner, '|'); k >= 0 {
			target = inner[:k]
			labelFrom = i + 3 + k
		}
		out = append(out, WikiLink{
			From: start, To: j + 2, LabelFrom: labelFrom, LabelTo: labelTo,
			Target: strings.TrimSpace(strings.ReplaceAll(target, `\|`, "|")), Embed: embed,
		})
		p = j + 2
	}
	return out
}

// WikiLinkAt resolves the target of a wiki link whose source range contains
// offset. It returns false for an empty target.
func WikiLinkAt(source string, from, to, offset int) (string, bool) {
	for _, link := range ParseWikiLinks(source, from, to) {
		if link.From <= offset && offset <= link.To {
			target := strings.TrimSpace(link.Target)
			return target, target != ""
		}
	}
	return "", false
}
