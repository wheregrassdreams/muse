package nativeui

import "testing"

func TestDirtyCloseConfirmationIsConsumedByTheNextCloseEvent(t *testing.T) {
	app := &App{}

	if got := app.closeRequest(true); got != closeConfirm {
		t.Fatalf("first dirty close = %v, want confirmation", got)
	}
	if app.closeState != closeConfirmationPending {
		t.Fatalf("confirmation state = %v, want pending", app.closeState)
	}

	// A duplicate close while the native dialog is open must be canceled and
	// must not start another confirmation.
	app.busy = true
	if got := app.closeRequest(true); got != closePrevent {
		t.Fatalf("close during confirmation = %v, want prevented", got)
	}
	app.finishCloseConfirmation(true)

	// This is Window.Close after the user chose Save or Don't Save. The
	// document can still be dirty after Don't Save; the one-shot permit allows
	// this close event and clears itself.
	if got := app.closeRequest(true); got != closeProceed {
		t.Fatalf("approved close = %v, want proceed", got)
	}
	if app.closeState != closeIdle {
		t.Fatalf("approved close permit was not consumed: state=%v", app.closeState)
	}
}

func TestCanceledDirtyCloseKeepsDocumentDirtyAndPromptsAgainOnlyOnNewRequest(t *testing.T) {
	app := &App{}
	if got := app.closeRequest(true); got != closeConfirm {
		t.Fatalf("first dirty close = %v, want confirmation", got)
	}
	app.finishCloseConfirmation(false)

	if got := app.closeRequest(true); got != closeConfirm {
		t.Fatalf("new close after cancel = %v, want a fresh confirmation", got)
	}
}

func TestSavedDocumentClosesWithoutDirtyConfirmation(t *testing.T) {
	app := &App{}
	if got := app.closeRequest(false); got != closeProceed {
		t.Fatalf("clean close = %v, want proceed", got)
	}
	if app.closeState != closeIdle {
		t.Fatalf("clean close created a confirmation permit: state=%v", app.closeState)
	}
}
