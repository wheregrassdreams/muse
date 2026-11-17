package editor

import (
	"testing"
	"unicode/utf8"
)

func TestMixedDocumentRangeProjectsOnlyTableIntersection(t *testing.T) {
	h := newHarness(t, documentTable)
	h.e.layoutIfNeeded()
	table := documentTestTable(t, h.e)
	end := blockEnd(h.e.Source(), table)
	firstCell := table.tab.rows[0][0]
	insideFirstCell := firstCell.start + len("左")
	lastRow := table.tab.rows[len(table.tab.rows)-1]
	lastCell := lastRow[len(lastRow)-1]
	source := h.e.Source()
	for _, tc := range []struct {
		name          string
		anchor, caret int
		wantRegion    bool
	}{
		{name: "enters from before", anchor: 0, caret: insideFirstCell, wantRegion: true},
		{name: "leaves after", anchor: insideFirstCell, caret: len(h.e.Source()), wantRegion: true},
		{name: "starts at table boundary", anchor: table.lineStart, caret: firstCell.start},
		{name: "ends at table boundary", anchor: lastCell.start, caret: end},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, offset := range []int{tc.anchor, tc.caret} {
				if offset < 0 || offset > len(source) || offset < len(source) && !utf8.RuneStart(source[offset]) {
					t.Fatalf("fixture offset %d is not a UTF-8 source boundary", offset)
				}
			}
			h.e.setSessionSelection(tc.anchor, tc.caret)
			selection := h.e.Session.Selection()
			if selection.Anchor != tc.anchor || selection.Head != tc.caret {
				t.Fatalf("fixture selection rejected: got [%d,%d), want [%d,%d)", selection.Anchor, selection.Head, tc.anchor, tc.caret)
			}
			h.e.tableSelection = nil
			h.e.tableBridge = nil
			got := h.e.tableStructureSelection(table)
			if tc.wantRegion && (got == nil || !got.cellRegion) {
				t.Fatalf("cross-boundary range did not become a cell region: %+v", got)
			}
			if !tc.wantRegion && got != nil {
				a, z := h.e.selection()
				t.Fatalf("ordinary range projected a partial table structure: %+v, range=[%d,%d)", got, a, z)
			}
			if tc.wantRegion {
				parts := h.e.selectionParts()
				found := false
				foundOuterText := false
				for _, part := range parts {
					found = found || part.structure != nil && part.structure.cellRegion
					foundOuterText = foundOuterText || part.structure == nil
				}
				if !found {
					t.Fatalf("mixed range did not split into an external text part and cell region: %+v", parts)
				}
				if tc.name == "enters from before" {
					if !foundOuterText {
						t.Fatalf("external text selection was lost: %+v", parts)
					}
					for _, row := range table.tab.rows {
						for _, cell := range row {
							if a, z := h.e.textSelectionForUnit(&leaf{table: table, start: cell.start, end: cell.end}); a != 0 || z != 0 {
								t.Fatalf("table text selection leaked beneath its structure mode: cell=[%d,%d) text=[%d,%d)", cell.start, cell.end, a, z)
							}
						}
					}
					if a, z := h.e.textSelectionForUnit(h.e.leaves[0]); a == z {
						t.Fatal("external paragraph did not retain its text selection")
					}
					h.shot("mixed-selection-cell-region-and-external-text")
				}
			}
		})
	}
	h.e.setSessionSelection(0, len(h.e.Source()))
	if got := h.e.tableStructureSelection(table); got == nil || got.axis != tableWhole {
		t.Fatalf("range containing complete table lost whole-table semantics: %+v (table [%d,%d))", got, table.lineStart, end)
	}
}
