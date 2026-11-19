package commandpalette

import (
	"testing"

	"muse/locales"
)

func TestCatalogAddsAssistantOnlyWhenAvailable(t *testing.T) {
	withoutAssistant := Catalog(false)
	withAssistant := Catalog(true)
	if len(withAssistant) != len(withoutAssistant)+1 {
		t.Fatalf("catalog sizes without/with assistant = %d/%d", len(withoutAssistant), len(withAssistant))
	}
	if withAssistant[len(withAssistant)-1] != (Command{ID: PanelAssistant, Name: locales.AiAssistantPanel()}) {
		t.Fatalf("assistant command = %+v", withAssistant[len(withAssistant)-1])
	}
	for _, command := range withoutAssistant {
		if command.ID == PanelAssistant {
			t.Fatal("assistant command was included while unavailable")
		}
	}
}
