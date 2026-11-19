package shell

import (
	"errors"

	appworkspace "muse/application/workspace"
	domain "muse/workspace"
)

type RestoreHost interface {
	OpenRestorePlan(appworkspace.RestorePlan) error
}

type WorkspaceLifecyclePorts struct {
	Session         *appworkspace.Session
	Restore         RestoreHost
	CurrentDocument func() string
	CloseVault      func()
	SetRoot         func(string)
	Persist         func(domain.State) error
}

// WorkspaceLifecycle coordinates vault identity changes with the portable
// workspace session and its pane host. Platform and feature details stay in
// the injected shell callbacks.
type WorkspaceLifecycle struct{ ports WorkspaceLifecyclePorts }

func NewWorkspaceLifecycle(ports WorkspaceLifecyclePorts) *WorkspaceLifecycle {
	return &WorkspaceLifecycle{ports: ports}
}

func (l *WorkspaceLifecycle) SwitchVault(root string) error {
	if l == nil || l.ports.Session == nil || l.ports.Restore == nil || l.ports.SetRoot == nil {
		return errors.New("shell: workspace lifecycle is unavailable")
	}
	active := ""
	if l.ports.CurrentDocument != nil {
		active = l.ports.CurrentDocument()
	}
	plan, err := l.ports.Session.SwitchVault(root, active)
	if err != nil {
		return err
	}
	if plan.ResetWorkspace {
		if l.ports.CloseVault != nil {
			l.ports.CloseVault()
		}
		l.ports.SetRoot(plan.VaultPath)
		if err := l.ports.Restore.OpenRestorePlan(appworkspace.RestorePlan{
			State: plan.State, DefaultTab: plan.DefaultTab,
		}); err != nil {
			return err
		}
	}
	if l.ports.Persist != nil {
		return l.ports.Persist(plan.State)
	}
	return nil
}
