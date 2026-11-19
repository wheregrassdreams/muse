package session

import (
	"testing"

	documentapp "muse/application/document"
)

type replacePort struct {
	documentapp.EditingPort
	got string
}

func (p *replacePort) ReplaceSystem(source string) error {
	p.got = source
	return nil
}

func TestReplaceSystemUsesOwnerThreadScheduler(t *testing.T) {
	editing := &replacePort{}
	called := false
	port := EditingPort{
		EditingPort: editing,
		OnMain: func(fn func()) {
			called = true
			fn()
		},
	}
	if err := port.ReplaceSystem("metadata"); err != nil {
		t.Fatal(err)
	}
	if !called || editing.got != "metadata" {
		t.Fatalf("owner thread called=%t; source=%q", called, editing.got)
	}
}
