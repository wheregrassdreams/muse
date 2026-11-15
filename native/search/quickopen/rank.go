package quickopen

import (
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"muse/document/types"
)

// Candidate is the immutable catalogue projection needed for fuzzy file
// selection. PathText is presentation-ready, usually relative to the vault.
type Candidate struct {
	Path, PathText, Title string
	Metadata              types.Metadata
}

// Match is a ranked result. Highlight positions are rune offsets into
// Title and PathText so they remain valid for CJK and other multibyte text.
type Match struct {
	Path, Title               string
	Score                     int
	TitleMatches, PathMatches []int
}

// Rank applies the editor's subsequence ranking to a caller-owned
// catalogue. It does not cache or retain items; application code owns catalogue
// snapshots and query caching.
func Rank(query string, items []Candidate) []Match {
	query = strings.TrimSpace(query)
	if query == "" {
		out := make([]Match, 0, len(items))
		for _, item := range items {
			out = append(out, Match{Path: item.Path, Title: item.Title})
		}
		sort.SliceStable(out, func(i, j int) bool { return out[i].Path < out[j].Path })
		return out
	}
	needle := []rune(query)
	for i := range needle {
		needle[i] = unicode.ToLower(needle[i])
	}

	type scored struct {
		match Match
		score int
	}
	results := make([]scored, 0, len(items))
	for _, item := range items {
		pathScore, pathPositions, pathFound := quickOpenFuzzyMatch(item.PathText, needle, true)
		best, found := pathScore, pathFound
		titleScore, titlePositions, titleFound := quickOpenFuzzyMatch(item.Title, needle, true)
		if titleFound && (!found || titleScore < best) {
			best, found = titleScore, true
		}
		for _, alias := range item.Metadata.Aliases {
			if aliasScore, _, ok := quickOpenFuzzyMatch(alias, needle, false); ok && (!found || aliasScore < best) {
				best, found = aliasScore, true
			}
		}
		if found {
			results = append(results, scored{
				match: Match{Path: item.Path, Title: item.Title, Score: best, TitleMatches: titlePositions, PathMatches: pathPositions},
				score: best,
			})
		}
	}
	sort.SliceStable(results, func(i, j int) bool {
		if results[i].score != results[j].score {
			return results[i].score < results[j].score
		}
		return results[i].match.Path < results[j].match.Path
	})
	out := make([]Match, len(results))
	for i := range results {
		out[i] = results[i].match
	}
	return out
}

func quickOpenFuzzyMatch(haystack string, needle []rune, highlight bool) (int, []int, bool) {
	var positions []int
	if highlight {
		positions = make([]int, 0, len(needle))
	}
	score, at, byteAt := 0, 0, 0
	lastMatched := rune(0)
	for i, wanted := range needle {
		found, foundByte, foundRune, previous := -1, 0, rune(0), lastMatched
		scanIndex := at
		for byteAt < len(haystack) {
			r, size := utf8.DecodeRuneInString(haystack[byteAt:])
			if unicode.ToLower(r) == wanted {
				found, foundByte, foundRune = scanIndex, byteAt+size, r
				break
			}
			previous = r
			scanIndex++
			byteAt += size
		}
		if found < 0 {
			return 0, nil, false
		}
		if highlight {
			positions = append(positions, found)
		}
		if found == at && i > 0 {
			score -= 2
		}
		if found == 0 || previous == '/' {
			score -= 3
		}
		score += found - at
		at = found + 1
		byteAt = foundByte
		lastMatched = foundRune
	}
	return score, positions, true
}
