package shell

import (
	"errors"
	"testing"
)

func TestApproveCloseResolutions(t *testing.T) {
	t.Run("save closes only after successful save", func(t *testing.T) {
		called := 0
		if !ApproveClose(CloseSave, func() error { called++; return nil }) || called != 1 {
			t.Fatal("successful save did not approve close exactly once")
		}
		if ApproveClose(CloseSave, func() error { return errors.New("disk full") }) {
			t.Fatal("failed save approved close")
		}
	})
	t.Run("discard closes", func(t *testing.T) {
		if !ApproveClose(CloseDiscard, nil) {
			t.Fatal("discard did not approve close")
		}
	})
	t.Run("cancel keeps window open", func(t *testing.T) {
		if ApproveClose(CloseCancel, nil) {
			t.Fatal("cancel approved close")
		}
	})
}

func TestCloseGateConsumesConfirmedCloseOnce(t *testing.T) {
	var gate CloseGate
	if got := gate.Request(true, false); got != CloseConfirm {
		t.Fatalf("first dirty close = %v, want confirmation", got)
	}
	if got := gate.Request(true, false); got != ClosePrevent {
		t.Fatalf("duplicate close during confirmation = %v, want prevented", got)
	}
	gate.Finish(true)
	if got := gate.Request(true, false); got != CloseProceed {
		t.Fatalf("confirmed close = %v, want proceed", got)
	}
	if got := gate.Request(true, false); got != CloseConfirm {
		t.Fatalf("later close = %v, want a fresh confirmation", got)
	}
}

func TestCloseGateCancellationAndBusyState(t *testing.T) {
	var gate CloseGate
	if got := gate.Request(true, true); got != ClosePrevent {
		t.Fatalf("busy close = %v, want prevented", got)
	}
	if got := gate.Request(true, false); got != CloseConfirm {
		t.Fatalf("dirty close = %v, want confirmation", got)
	}
	gate.Finish(false)
	if got := gate.Request(true, false); got != CloseConfirm {
		t.Fatalf("close after canceled dialog = %v, want a fresh confirmation", got)
	}
	gate.Finish(true)
	if got := gate.Request(false, false); got != CloseProceed {
		t.Fatalf("clean close = %v, want proceed", got)
	}
}
