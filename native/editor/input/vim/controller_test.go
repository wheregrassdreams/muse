package vim

import (
	"testing"
	"time"
)

type commandHost struct{ commands []Command }

func (h *commandHost) DispatchVimCommand(command Command) { h.commands = append(h.commands, command) }

func TestSequenceCommands(t *testing.T) {
	c := New()
	c.SetKeymap("vim")
	now := time.Now()

	if got, ok := c.FeedSequenceText("2", now); !ok || got.Kind != CommandCountPrefix || c.Sequence().Count != 2 {
		t.Fatalf("count prefix = (%+v, %v), state=%+v", got, ok, c.View())
	}
	_, _ = c.ConsumeCounts()
	if got, ok := c.FeedSequenceText("g", now); !ok || got.Kind != CommandPendingPrefix {
		t.Fatalf("g prefix = (%+v, %v)", got, ok)
	}
	if got, ok := c.FeedSequenceText("j", now); !ok || got.Kind != CommandVertical || got.Direction != 1 {
		t.Fatalf("gj command = (%+v, %v)", got, ok)
	}
	if c.Pending() != "" {
		t.Fatalf("pending prefix after gj = %q", c.Pending())
	}

	_, _ = c.FeedSequenceText("z", now)
	got, ok := c.FeedSequenceText("C", now.Add(time.Millisecond))
	if !ok || got.Kind != CommandFold || got.Fold != FoldCloseRecursive {
		t.Fatalf("zC command = (%+v, %v)", got, ok)
	}
}

func TestExpiredPrefixFallsThroughAndStateViewIsDetached(t *testing.T) {
	c := New()
	c.SetKeymap("vim")
	now := time.Now()
	_, _ = c.FeedSequenceText("z", now)
	if got, handled := c.FeedSequenceText("x", now.Add(time.Second)); handled || got.Kind != CommandNone {
		t.Fatalf("expired prefix = (%+v, %v)", got, handled)
	}

	c.SetBlockRegister([]string{"one", "two"})
	view := c.View()
	view.RegisterBlock[0] = "changed"
	if got := c.View().RegisterBlock[0]; got != "one" {
		t.Fatalf("mutating View changed controller state: %q", got)
	}
}

func TestHandleTextDispatchesTypedCommandThroughHost(t *testing.T) {
	c := New()
	c.SetKeymap("vim")
	host := &commandHost{}
	now := time.Now()
	if kind, handled := c.HandleText("z", now, host); !handled || kind != CommandPendingPrefix || len(host.commands) != 0 {
		t.Fatalf("prefix should be consumed without a host command: %+v", host.commands)
	}
	if kind, handled := c.HandleText("M", now.Add(time.Millisecond), host); !handled || kind != CommandFoldAll || len(host.commands) != 1 {
		t.Fatalf("fold-all command was not dispatched: %+v", host.commands)
	}
	if got := host.commands[0]; got.Kind != CommandFoldAll || got.Fold != FoldCloseAll {
		t.Fatalf("dispatched command = %+v", got)
	}
}
