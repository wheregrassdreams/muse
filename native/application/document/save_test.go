package document

import (
	"os"
	"path/filepath"
	"testing"

	"muse/document"
	"muse/events"
	"muse/platform/filesystem/documentstore"
)

func TestSaverPersistsAndPublishesOnlySuccessfulSave(t *testing.T) {
	path := filepath.Join(t.TempDir(), "note.md")
	if err := os.WriteFile(path, []byte("before"), 0o600); err != nil {
		t.Fatal(err)
	}
	adapter := documentstore.New(filepath.Join(t.TempDir(), "recovery"))
	dispatcher := events.New[document.DocumentSaved]()
	var eventsSeen []document.DocumentSaved
	dispatcher.Subscribe(func(event document.DocumentSaved) { eventsSeen = append(eventsSeen, event) })
	saver := NewSaver(adapter, adapter, dispatcher)
	initial := document.RevisionOf("before")
	saved, err := saver.Save("draft", path, "after", initial)
	if err != nil {
		t.Fatal(err)
	}
	if saved.Source != "after" || saved.Path != path || saved.Revision != document.RevisionOf("after") || len(eventsSeen) != 1 {
		t.Fatalf("saved note/event = %+v/%+v", saved, eventsSeen)
	}
	if eventsSeen[0].Path != path || eventsSeen[0].Revision != saved.Revision {
		t.Fatalf("saved event = %+v", eventsSeen[0])
	}
	if _, err := saver.Save("draft", path, "stale", initial); err == nil {
		t.Fatal("stale revision unexpectedly saved")
	}
	if len(eventsSeen) != 1 {
		t.Fatalf("failed save published event: %+v", eventsSeen)
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "after" {
		t.Fatalf("disk source = %q, %v", data, err)
	}
}
