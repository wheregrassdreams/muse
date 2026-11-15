package frontmatter

import (
	"strings"
	"testing"
)

func TestCorePropertiesProjectDefaultsAndManagedReadOnlyFields(t *testing.T) {
	fields := CoreFields(nil)
	if len(fields) != 5 || fields[0].Key != "type" || fields[0].Value != "note" || !fields[0].Editable {
		t.Fatalf("core fields = %+v", fields)
	}
	for _, field := range fields[3:] {
		if field.Editable {
			t.Errorf("managed field %q is editable", field.Key)
		}
	}
	if !IsCoreKey("Created") || CanonicalKey("last modified date") != "updated_at" {
		t.Fatal("core canonicalization did not normalize authored aliases")
	}
	if !OmitFromPropertySummary("title") || !IsSecondaryProperty("last opened date") || IsSecondaryProperty("tags") {
		t.Fatal("property summary and secondary-field projection rules are inconsistent")
	}
}

func TestSetPropertyKeepsTitleAndListSemantics(t *testing.T) {
	const source = "---\ntitle: Legacy\nstatus: draft\n---\nBody\n"
	withTitle, err := SetProperty(source, "title", "", "text")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(withTitle, "title:") || !strings.HasSuffix(withTitle, "Body\n") {
		t.Fatalf("empty title did not remove only its override: %q", withTitle)
	}
	withTags, err := SetProperty(source, "tags", "one, two", "text")
	if err != nil {
		t.Fatal(err)
	}
	fields := Parse(withTags).Fields
	for _, field := range fields {
		if field.Key == "tags" && (field.Kind != "list" || field.Value != "one, two") {
			t.Fatalf("tags field = %+v", field)
		}
	}
}

func TestDeletePropertyProtectsCoreMetadataAndDisplayFormats(t *testing.T) {
	const source = "---\ntype: note\ntags: [one]\nstatus: in_progress\n---\nBody\n"
	if _, err := DeleteProperty(source, "tags"); err == nil {
		t.Fatal("core tags field was deletable")
	}
	if _, err := SetProperty("---\ncreated_at: today\n---\n", "created", "tomorrow", "date"); err == nil {
		t.Fatal("managed timestamp was editable through generic property operation")
	}
	fields := Parse(source).Fields
	for _, field := range fields {
		if field.Key == "status" && DisplayValue(field) == field.Value {
			t.Fatal("status display value was not localized")
		}
	}
}

func TestReplaceYAMLSourceOnlyReplacesFrontmatter(t *testing.T) {
	const source = "---\ntags: [old]\n---\n# Keep body\n"
	next, err := ReplaceYAMLSource(source, "---\ntags: [new]\n---")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(next, "---\ntags: [new]\n---\n") || !strings.HasSuffix(next, "# Keep body\n") {
		t.Fatalf("merged source = %q", next)
	}
	if _, err := ReplaceYAMLSource(source, "---\ntitle: bad\n---\nbody overwrite"); err == nil {
		t.Fatal("body content in source editor was accepted")
	}
}
