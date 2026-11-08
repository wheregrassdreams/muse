package mdedit

import (
	"testing"

	"github.com/egoist/mygo/ui/markdown/parser"
)

func TestNumericTagsAreNotReferences(t *testing.T) {
	// GitHub's reference extension would take `#2024` for an issue link;
	// it is left out, so a numeric Obsidian tag stays a tag.
	got := shown(t, "看 #2024 这个\n")
	if len(got) != 1 || got[0] != "看 «a:#2024» 这个" {
		t.Fatalf("shows %q", got)
	}
}

func TestAlertsComeFromTheParser(t *testing.T) {
	// MyGo 0.8.3 parses `> [!NOTE]` into a quote block with its Alert set,
	// so a callout's recognition is not ours to write.
	e := shownEditor(t, "> [!NOTE]\n> 正文\n")
	found := false
	for i := 0; i < e.doc.Len(); i++ {
		if b := e.doc.Block(i); b.Kind == parser.Quote && b.Alert != parser.NoAlert {
			found = true
		}
	}
	if !found {
		t.Fatal("the alert did not parse")
	}
}
