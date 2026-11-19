package noteeditor

import (
	documentapp "muse/application/document"
	"muse/document"
)

type workflowEditingPort struct{ Commands }

// WorkflowEditingPort adapts the pane's feature-owned command capability to
// the application document workflow without exposing the native surface.
func WorkflowEditingPort(commands Commands) documentapp.EditingPort {
	if commands == nil {
		return nil
	}
	return workflowEditingPort{Commands: commands}
}

func (p workflowEditingPort) Snapshot() documentapp.EditingSnapshot {
	snapshot, state := p.Commands.Snapshot(), p.Commands.PersistenceState()
	return documentapp.EditingSnapshot{
		Path: snapshot.Path, Source: snapshot.Source, DiskSource: snapshot.DiskSource,
		BaseSource: state.BaseSource, Revision: snapshot.Revision, Dirty: snapshot.Dirty,
		Saving: state.Saving, InFlight: state.InFlight, HasInFlight: state.HasInFlight,
	}
}

func (p workflowEditingPort) Relocate(path string, revision document.Revision, source string) error {
	return p.Commands.Relocate(document.NewNote(path, source, revision))
}
