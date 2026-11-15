package diagrams

import "testing"

func TestParseClassDiagramAndLayout(t *testing.T) {
	g, ok := ParseClass(`classDiagram
class Animal {
  +String name
  +makeSound()
}
class Dog
Animal <|-- Dog
Dog --> Owner : feeds
`)
	if !ok || len(g.Classes) != 3 || len(g.Relations) != 2 {
		t.Fatalf("class diagram = %+v", g)
	}
	if g.Classes[0].Members[1] != "+makeSound()" {
		t.Fatalf("members = %+v", g.Classes[0].Members)
	}
	if g.Relations[0].Kind != "inherit" || g.Relations[1].Label != "feeds" {
		t.Fatalf("relations = %+v", g.Relations)
	}
	layout := g.Layout(g.Width())
	if len(layout) != len(g.Classes) || layout[0].Rect.W < 130 || layout[1].Rect.Y <= layout[0].Rect.Y {
		t.Fatalf("layout = %+v", layout)
	}
}
