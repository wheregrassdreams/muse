package inline

import "testing"

func TestLinkTargetAtUsesParserCandidatesAndExcludesEmbeds(t *testing.T) {
	source := "[[page]] ![[image.png]] [site](https://example.test)"
	candidates := []LinkCandidate{
		{Kind: LinkCandidateText, Start: 0, End: len("[[page]] ![[image.png]]")},
		{Kind: LinkCandidateLink, Start: len("[[page]] ![[image.png]] "), End: len(source), Destination: "https://example.test"},
	}
	if got, ok := LinkTargetAt(source, candidates, 3); !ok || got != "wiki:page" {
		t.Fatalf("wiki link target = %q, %v", got, ok)
	}
	if got, ok := LinkTargetAt(source, candidates, 13); ok || got != "" {
		t.Fatalf("wiki embed unexpectedly navigated to %q, %v", got, ok)
	}
	if got, ok := LinkTargetAt(source, candidates, len(source)-2); !ok || got != "https://example.test" {
		t.Fatalf("Markdown link target = %q, %v", got, ok)
	}
}

func TestLinkRangeAtReturnsNavigableSourceRange(t *testing.T) {
	source := "before [[page|label]] after"
	candidates := []LinkCandidate{{Kind: LinkCandidateText, Start: 0, End: len(source)}}
	start, end, ok := LinkRangeAt(source, candidates, 16)
	if !ok || source[start:end] != "[[page|label]]" {
		t.Fatalf("wiki link range = [%d,%d), %v (%q)", start, end, ok, source[start:end])
	}
}
