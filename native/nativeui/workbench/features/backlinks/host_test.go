package backlinks

import (
	"testing"

	"github.com/egoist/mygo/ui"
	"muse/vault/links"
)

func TestHostOwnsCloseLifecycleAndDoesNotReadPortsAfterClose(t *testing.T) {
	dataCalls, closeCalls := 0, 0
	host := NewHost(Ports{
		Data: func() Model {
			dataCalls++
			return Model{Ready: true}
		},
		OnClose: func() { closeCalls++ },
	})
	tester := ui.NewTester(host.Render, 800, 600)
	tester.Frame()
	if dataCalls == 0 {
		t.Fatal("Host did not read its model port during Render")
	}
	callsBeforeClose := dataCalls
	host.Close()
	host.Close()
	tester.Frame()
	if closeCalls != 1 {
		t.Fatalf("OnClose calls = %d, want once", closeCalls)
	}
	if dataCalls != callsBeforeClose {
		t.Fatalf("Data called after host closed: before %d, after %d", callsBeforeClose, dataCalls)
	}
}

func TestNewContentRemainsAHostConstructor(t *testing.T) {
	var _ *Host = NewContent(Ports{})
}

func TestHostRoutesBacklinkSelectionThroughTypedPorts(t *testing.T) {
	var openedPath, openedTarget string
	var openedOffset int
	host := NewHost(Ports{
		Data: func() Model {
			return Model{
				Ready:    true,
				Incoming: []links.BacklinkSource{{Source: "projects/source.md", Start: 27}},
				Outgoing: []links.Link{{Target: "guide/target"}},
			}
		},
		OpenSource: func(path string, offset int) { openedPath, openedOffset = path, offset },
		OpenTarget: func(target string) { openedTarget = target },
	})
	tester := ui.NewTester(host.Render, 800, 600)
	tester.Frame()
	if err := tester.Click("projects/source.md"); err != nil {
		t.Fatal(err)
	}
	if openedPath != "projects/source.md" || openedOffset != 27 {
		t.Fatalf("OpenSource = %q at %d", openedPath, openedOffset)
	}
	tester.Frame()
	if err := tester.Click("guide/target"); err != nil {
		t.Fatal(err)
	}
	if openedTarget != "guide/target" {
		t.Fatalf("OpenTarget = %q", openedTarget)
	}
}
