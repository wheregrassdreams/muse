package tableui

import (
	"strings"
	"testing"

	tablemodel "muse/editor/blocks/table"
)

func TestClipboardHTMLPreservesWhetherSelectedFirstRowWasSourceHeader(t *testing.T) {
	register := tablemodel.NewRegister(tablemodel.Cells, [][]string{{"value"}}, nil, nil, false, true, false)
	body := ClipboardHTML(*register)
	if !strings.Contains(body, "<tbody>") || !strings.Contains(body, "<td>value</td>") {
		t.Fatalf("body-row selection HTML = %q", body)
	}
	register = tablemodel.NewRegister(tablemodel.Cells, [][]string{{"heading"}}, nil, nil, false, true, true)
	header := ClipboardHTML(*register)
	if !strings.Contains(header, "<thead>") || !strings.Contains(header, "<th>heading</th>") {
		t.Fatalf("source-header selection HTML = %q", header)
	}
}
