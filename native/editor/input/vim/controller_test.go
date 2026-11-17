package vim

import (
	"testing"
	"time"
)

type intentHost struct{ intents []Intent }

func (h *intentHost) DispatchVimIntent(intent Intent) { h.intents = append(h.intents, intent) }

func TestSequenceCommands(t *testing.T) {
	c := New()
	c.SetKeymap("vim")
	now := time.Now()

	if got, ok := c.feedSequenceText("2", now); !ok || got.Kind != CommandCountPrefix || c.Sequence().Count != 2 {
		t.Fatalf("count prefix = (%+v, %v), state=%+v", got, ok, c.View())
	}
	_, _ = c.ConsumeCounts()
	if got, ok := c.feedSequenceText("g", now); !ok || got.Kind != CommandPendingPrefix {
		t.Fatalf("g prefix = (%+v, %v)", got, ok)
	}
	if got, ok := c.feedSequenceText("j", now); !ok || got.Kind != CommandVertical || got.Direction != 1 {
		t.Fatalf("gj command = (%+v, %v)", got, ok)
	}
	if c.Pending() != "" {
		t.Fatalf("pending prefix after gj = %q", c.Pending())
	}

	_, _ = c.feedSequenceText("z", now)
	got, ok := c.feedSequenceText("C", now.Add(time.Millisecond))
	if !ok || got.Kind != CommandFold || got.Fold != FoldCloseRecursive {
		t.Fatalf("zC command = (%+v, %v)", got, ok)
	}
}

func TestExpiredPrefixFallsThroughAndStateViewIsDetached(t *testing.T) {
	c := New()
	c.SetKeymap("vim")
	now := time.Now()
	_, _ = c.feedSequenceText("z", now)
	if got, handled := c.feedSequenceText("x", now.Add(time.Second)); handled || got.Kind != CommandNone {
		t.Fatalf("expired prefix = (%+v, %v)", got, handled)
	}

	c.SetBlockRegister([]string{"one", "two"})
	view := c.View()
	view.RegisterBlock[0] = "changed"
	if got := c.View().RegisterBlock[0]; got != "one" {
		t.Fatalf("mutating View changed controller state: %q", got)
	}
}

func TestInterpretTextReturnsIntentsForHostDispatch(t *testing.T) {
	c := New()
	c.SetKeymap("vim")
	host := &intentHost{}
	now := time.Now()
	intent, handled := c.InterpretText("z", now)
	if !handled || intent.Kind != IntentPrefix || len(host.intents) != 0 {
		t.Fatalf("prefix parse should return an undispatched typed intent: intent=%+v dispatched=%+v", intent, host.intents)
	}
	host.DispatchVimIntent(intent)
	intent, handled = c.InterpretText("M", now.Add(time.Millisecond))
	if !handled || intent.Kind != IntentCommand || len(host.intents) != 1 {
		t.Fatalf("fold-all parse should return an undispatched typed intent: intent=%+v dispatched=%+v", intent, host.intents)
	}
	host.DispatchVimIntent(intent)
	if got := host.intents[1]; got.Kind != IntentCommand || got.Command.Kind != CommandFoldAll || got.Command.Fold != FoldCloseAll {
		t.Fatalf("dispatched intent = %+v", got)
	}
}
