package document

import (
	"testing"
)

func TestNoteRevisionTracksSourceAndMetadataDoesNotAliasInput(t *testing.T) {
	source := "---\ntype: research\ntags: [work]\n---\nfirst"
	note := NewNote("notes/work.md", source, "disk-r1")
	initial := note.Revision
	if note.Metadata.Tags[0] != "work" {
		t.Fatalf("metadata was not parsed from source: %#v", note.Metadata.Tags)
	}
	if note.Metadata.Type != "research" || note.Source != source || note.Revision != initial {
		t.Fatalf("note snapshot changed unexpectedly: %#v", note)
	}
	note.Metadata.Tags[0] = "changed"
	if note.Source != source {
		t.Fatal("mutating projected metadata changed the immutable source")
	}
}

func TestNoteUsesPathAsCurrentIdentity(t *testing.T) {
	note := NewNote("", "draft", "")
	if note.Path != "" || note.Source != "draft" {
		t.Fatalf("untitled note = %#v", note)
	}
}
