// Package session adapts application/document persistence operations to the
// active editor session and enforces editor owner-thread edits.
package session

import (
	documentapp "muse/application/document"
	"muse/document"
)

// MainThread schedules a callback on the native UI owner's thread.
type MainThread func(func())

// EditingPort keeps the application workflow independent from native UI
// scheduling while ensuring system edits always reach the editor owner thread.
type EditingPort struct {
	documentapp.EditingPort
	OnMain MainThread
}

func (p EditingPort) Snapshot() documentapp.EditingSnapshot {
	if p.EditingPort == nil {
		return documentapp.EditingSnapshot{}
	}
	var snapshot documentapp.EditingSnapshot
	apply := func() { snapshot = p.EditingPort.Snapshot() }
	if p.OnMain != nil {
		p.OnMain(apply)
	} else {
		apply()
	}
	return snapshot
}

func (p EditingPort) Relocate(path string, revision document.Revision, source string) error {
	if p.EditingPort == nil {
		return documentapp.ErrWorkflowUnavailable
	}
	var err error
	apply := func() { err = p.EditingPort.Relocate(path, revision, source) }
	if p.OnMain != nil {
		p.OnMain(apply)
	} else {
		apply()
	}
	return err
}

func (p EditingPort) Rebase(path string, revision document.Revision, source string) bool {
	if p.EditingPort == nil {
		return false
	}
	var rebased bool
	apply := func() { rebased = p.EditingPort.Rebase(path, revision, source) }
	if p.OnMain != nil {
		p.OnMain(apply)
	} else {
		apply()
	}
	return rebased
}

func (p EditingPort) ReplaceSystem(source string) error {
	if p.EditingPort == nil {
		return documentapp.ErrWorkflowUnavailable
	}
	var err error
	apply := func() { err = p.EditingPort.ReplaceSystem(source) }
	if p.OnMain != nil {
		p.OnMain(apply)
	} else {
		apply()
	}
	return err
}
