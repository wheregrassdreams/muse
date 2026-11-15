package mdedit

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// TestTheParserSourceIsWrittenInOnePlace: everything the parser reads — the
// document, the masked source, the comments, the front matter — is written
// by parserSource.set and parserSource.edit, and by nothing else.
//
// That is what keeps the fast path from drifting away from the full one: a
// field of the parse added tomorrow has one place to be filled in, and the
// next person to add one cannot quietly update it on one path only. Go has
// no way to say "read only here", so this reads the package's own source.
func TestTheParserSourceIsWrittenInOnePlace(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	write := regexp.MustCompile(`\bp\.(doc|src|mask|comments|fm)\s*=[^=]`)
	whole := regexp.MustCompile(`\*p\s*=\s*parserSource`)
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		lines := strings.Split(string(src), "\n")
		for i, line := range lines {
			if strings.HasPrefix(strings.TrimSpace(line), "//") {
				continue
			}
			writes := write.MatchString(line) || whole.MatchString(line)
			if !writes {
				continue
			}
			if f != "parsed.go" {
				t.Fatalf("%s:%d writes the parser's source, which only parsed.go may do:\n\t%s",
					f, i+1, strings.TrimSpace(line))
			}
		}
	}
}
