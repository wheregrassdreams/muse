package diagrams

import "testing"

func TestParseSequenceDiagramAndLayout(t *testing.T) {
	g, ok := ParseSequence(`sequenceDiagram
participant A as Alice
participant B as Bob
A->>B: Hello
B-->>A: Hi
Note over A,B: pair
`)
	if !ok || len(g.Participants) != 2 || len(g.Events) != 3 {
		t.Fatalf("sequence = %+v", g)
	}
	if g.Participants[0].Label != "Alice" || g.Events[0].Text != "Hello" || !g.Events[1].Dashed || g.Events[2].Kind != SequenceNote {
		t.Fatalf("events = %+v", g.Events)
	}
	if got := g.Layout(400); len(got) != 2 || got[0].Width <= 0 || got[1].X <= got[0].X {
		t.Fatalf("layout = %+v", got)
	}
}
