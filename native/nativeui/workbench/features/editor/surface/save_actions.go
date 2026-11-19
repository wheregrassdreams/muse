package noteeditor

import (
	"errors"
	"path/filepath"
	"time"

	documentapp "muse/application/document"
)

// SaveActionPorts bind editor save commands to the active pane and native
// dialog/UI effects. Save policy and revision handling stay in
// application/document.SessionWorkflows.
type SaveActionPorts struct {
	Workflows       func() documentapp.SessionWorkflows
	CurrentPath     func() string
	UntitledName    func() string
	RequestSaveAs   func(defaultPath string)
	RecordSavedPath func(path string)
	SelectPath      func(path string)
	NotifyError     func(error)
	NotifySaved     func()
	Invalidate      func()
}

// SaveActions owns the editor feature's command-to-workflow interaction. It
// does not retain document state; every command resolves the current pane.
type SaveActions struct{ ports SaveActionPorts }

func NewSaveActions(ports SaveActionPorts) *SaveActions {
	return &SaveActions{ports: ports}
}

// SaveCurrent runs the application save workflow without UI effects. Runtime
// lifecycle uses it when a window close has already been confirmed.
func (a *SaveActions) SaveCurrent() error {
	workflow, ok := a.workflow()
	if !ok {
		return documentapp.ErrWorkflowUnavailable
	}
	return workflow.SaveCurrent()
}

// Save handles the user-facing Save command and its Save As fallback.
func (a *SaveActions) Save() {
	if a == nil {
		return
	}
	err := a.SaveCurrent()
	if errors.Is(err, documentapp.ErrSaveAsRequired) {
		a.RequestSaveAs()
		return
	}
	if err != nil {
		a.notifyError(err)
	} else if a.ports.NotifySaved != nil {
		a.ports.NotifySaved()
	}
	a.invalidate()
}

// RequestSaveAs asks the host to display the platform save dialog, seeded
// from the active document's name.
func (a *SaveActions) RequestSaveAs() {
	if a == nil || a.ports.RequestSaveAs == nil {
		return
	}
	path := ""
	if a.ports.CurrentPath != nil {
		path = filepath.Base(a.ports.CurrentPath())
	}
	if path == "." || path == "" {
		name := "Untitled"
		if a.ports.UntitledName != nil {
			name = a.ports.UntitledName()
		}
		path = name + ".md"
	}
	a.ports.RequestSaveAs(path)
}

// CommitSaveAs applies and commits the application-owned plan for the still
// active document. The host records the new active path only after success.
func (a *SaveActions) CommitSaveAs(path string) {
	if a == nil || path == "" {
		return
	}
	workflow, ok := a.workflow()
	if !ok {
		a.notifyError(documentapp.ErrWorkflowUnavailable)
		a.invalidate()
		return
	}
	plan, err := workflow.PrepareSaveAs(path, time.Now())
	if err == nil {
		err = workflow.ApplySaveAsSource(plan)
	}
	if err == nil {
		err = workflow.CommitSaveAs(plan)
	}
	if err != nil {
		a.notifyError(err)
		a.invalidate()
		return
	}
	if a.ports.RecordSavedPath != nil {
		a.ports.RecordSavedPath(path)
	}
	if a.ports.SelectPath != nil {
		a.ports.SelectPath(path)
	}
	if a.ports.NotifySaved != nil {
		a.ports.NotifySaved()
	}
	a.invalidate()
}

func (a *SaveActions) workflow() (documentapp.SessionWorkflows, bool) {
	if a == nil || a.ports.Workflows == nil {
		return documentapp.SessionWorkflows{}, false
	}
	return a.ports.Workflows(), true
}

func (a *SaveActions) notifyError(err error) {
	if err != nil && a.ports.NotifyError != nil {
		a.ports.NotifyError(err)
	}
}

func (a *SaveActions) invalidate() {
	if a.ports.Invalidate != nil {
		a.ports.Invalidate()
	}
}
