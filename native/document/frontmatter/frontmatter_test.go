package frontmatter

import (
	"strings"
	"testing"
)

func TestLosslessPropertyEdit(t *testing.T) {
	src := "---\r\n# note\r\ntitle: Old # retain\r\ntags: [one, two]\r\ncustom:\r\n  nested: true\r\n---\r\n# Body\r\n"
	next, err := Set(src, "title", "New: title")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(next, `title: "New: title" # retain`) || !strings.Contains(next, "custom:\r\n  nested: true") || !strings.HasSuffix(next, "# Body\r\n") {
		t.Fatalf("unexpected rewrite %q", next)
	}
	if Title(next, "file.md") != "Body" {
		t.Fatal("title should come from the leading H1")
	}
	if Title("body", "file.md") != "file" {
		t.Fatal("filename fallback")
	}
}

func TestCanonicalTitleUsesOnlyFirstNonblankATXHeading(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{"---\ntitle: Legacy\n---\n\n# Heading ##\ntext", "Heading"},
		{"text\n\n# Later", "file"},
		{"---\ntitle: Legacy\n---\n    # Code\n# Later", "file"},
		{"---\ntitle: Legacy\n---\n```md\n# Code\n```\n# Later", "file"},
		{"  # Valid\n", "Valid"},
		{"# Keep attached###\n", "Keep attached###"},
		{"#### Heading\n", "file"},
	} {
		if got := Title(tc.src, "file.md"); got != tc.want {
			t.Errorf("Title(%q) = %q, want %q", tc.src, got, tc.want)
		}
	}
}

func TestEnsureCoreKeepsEmptyCoreProperties(t *testing.T) {
	for _, src := range []string{"# Body\n", "---\ncustom: keep\n---\n# Body\n"} {
		got, err := EnsureCore(src)
		if err != nil {
			t.Fatal(err)
		}
		for _, field := range []string{"type: \"note\"", "tags: []", "aliases: []", "created_at: \"\"", "updated_at: \"\""} {
			if !strings.Contains(got, field) {
				t.Errorf("missing %q in %q", field, got)
			}
		}
		again, err := EnsureCore(got)
		if err != nil || again != got {
			t.Errorf("EnsureCore should be idempotent: %v", err)
		}
	}
}
func TestPropertyTypesAndErrors(t *testing.T) {
	src := "---\ntags:\n  - one\n  - two\npublished: true\ncount: 12\n---\nbody"
	s, err := Set(src, "tags", "three, 四")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(s, `["three", "四"]`) {
		t.Fatal(s)
	}
	if _, err = Set(s, "published", "nope"); err == nil {
		t.Fatal("accepted invalid boolean")
	}
	s, err = Delete(s, "count")
	if err != nil || strings.Contains(s, "count:") || !strings.HasSuffix(s, "body") {
		t.Fatal(s, err)
	}
	if _, err = Set("---\na: 1\na: 2\n---\n", "a", "3"); err == nil {
		t.Fatal("accepted duplicate keys")
	}
	s, err = Set("body", "title", "hello")
	if err != nil || Title(s, "") != "未命名" {
		t.Fatal(s, err)
	}
}

func TestComplexFieldsStaySafeAndCommentsSurviveDelete(t *testing.T) {
	src := "---\nfirst: yes\n# belongs to second\nsecond: 42\nobject:\n  nested: true\nmultiline: [\n  one,\n  two\n]\n---\nbody"
	d := Parse(src)
	if d.Err != nil {
		t.Fatal(d.Err)
	}
	for _, f := range d.Fields {
		if (f.Key == "object" || f.Key == "multiline") && f.Editable {
			t.Fatalf("complex field %s editable", f.Key)
		}
	}
	s, err := Delete(src, "first")
	if err != nil || !strings.Contains(s, "# belongs to second\nsecond: 42") {
		t.Fatal(s, err)
	}
	s, err = Set("body", "tags", "hello, world")
	if err != nil || !strings.Contains(s, `tags: ["hello", "world"]`) {
		t.Fatal(s, err)
	}
}

func TestTypedNewPropertiesAndUnchangedBody(t *testing.T) {
	src := "# body\n\nuntouched 🌿\n"
	for _, x := range []struct{ k, v, kind string }{{"published", "true", "bool"}, {"rating", "4.5", "number"}, {"deadline", "2026-11-10", "date"}, {"tags", "one, 二", "list"}} {
		next, err := SetTyped(src, x.k, x.v, x.kind)
		if err != nil {
			t.Fatal(err)
		}
		d := Parse(next)
		if d.Err != nil || len(d.Fields) != 1 || d.Fields[0].Kind != x.kind || !strings.HasSuffix(next, src) {
			t.Fatalf("typed projection: %+v / %q", d, next)
		}
	}
}

func TestTypedDatesAcceptCommonUnambiguousFormats(t *testing.T) {
	for _, value := range []string{"2026-11-13", "2026-11-13T08:54:30Z", "Nov 13, 2026 8:54:30 AM", "2026/11/13 08:54"} {
		if _, err := SetTyped("", "updated_at", value, "date"); err != nil {
			t.Errorf("rejected %q: %v", value, err)
		}
	}
	if _, err := SetTyped("", "updated_at", "03/04/2026", "date"); err == nil {
		t.Fatal("ambiguous month/day value was accepted")
	}
}

func TestMetadataProjectsCoreNoteFields(t *testing.T) {
	doc := Parse("---\ntype: research\ncreated_at: 2026-11-01\nupdated_at: 2026-11-02\naliases: [Plan, Roadmap]\ntags:\n  - work\n  - project\n---\nbody")
	metadata := doc.Metadata()
	if metadata.Type != "research" || metadata.CreatedAt != "2026-11-01" || metadata.UpdatedAt != "2026-11-02" || len(metadata.Aliases) != 2 || metadata.Aliases[1] != "Roadmap" || len(metadata.Tags) != 2 || metadata.Tags[0] != "work" {
		t.Fatalf("Metadata() = %#v", metadata)
	}
}
