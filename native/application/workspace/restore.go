// Package workspace contains use cases that restore and persist a session
// across the workspace and application event boundary.
package workspace

import (
	"errors"
	"muse/events"
	"muse/workspace"
)

var ErrUnavailable = errors.New("workspace repository is not configured")

type Restorer struct {
	store      workspace.Repository
	dispatcher *events.Dispatcher[workspace.WorkspaceRestored]
}

func NewRestorer(store workspace.Repository, dispatcher *events.Dispatcher[workspace.WorkspaceRestored]) *Restorer {
	return &Restorer{store: store, dispatcher: dispatcher}
}

func (r *Restorer) Restore() (RestorePlan, error) {
	if r == nil || r.store == nil {
		return RestorePlan{}, ErrUnavailable
	}
	state, err := r.store.Load()
	if err != nil {
		return RestorePlan{}, err
	}
	plan, err := makeRestorePlan(state)
	if err != nil {
		return RestorePlan{}, err
	}
	if r.dispatcher != nil {
		r.dispatcher.Publish(workspace.WorkspaceRestored{State: plan.State})
	}
	return plan, nil
}

func (r *Restorer) Persist(state workspace.State) error {
	if r == nil || r.store == nil {
		return ErrUnavailable
	}
	state, err := stateForPersistence(state)
	if err != nil {
		return err
	}
	return r.store.Save(state)
}
