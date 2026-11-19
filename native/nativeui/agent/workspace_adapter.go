package agent

import (
	"context"
	"errors"

	applicationagent "muse/application/agent"
)

// WorkspaceAdapterPorts are the typed capabilities needed to expose the
// assistant's dynamic workspace tools. The adapter owns the composition of
// the read-only workspace port with mutation capabilities; callers provide
// document and vault operations without exposing App or widget state.
type WorkspaceAdapterPorts struct {
	Current              func() (root, path, source string)
	NewWorkspace         func(root string, current applicationagent.CurrentDocument) (applicationagent.WorkspacePort, error)
	CurrentDocumentDirty func() bool
	DestinationRevision  func(path string) (string, error)
	SaveDocument         func(path, content, expectedRevision string) error
	ApplyCurrentDocument func(content string) error
	MoveDocument         func(root, source, target string) (string, error)
	TrashDocuments       func(root string, paths []string) error
	CreateFolder         func(root, path string) error
}

type WorkspaceAdapter struct{ ports WorkspaceAdapterPorts }

func NewWorkspaceAdapter(ports WorkspaceAdapterPorts) *WorkspaceAdapter {
	return &WorkspaceAdapter{ports: ports}
}

func (a *WorkspaceAdapter) Workspace(ctx context.Context) (applicationagent.WorkspaceMutationPort, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if a == nil || a.ports.Current == nil || a.ports.NewWorkspace == nil {
		return nil, applicationagent.ErrUnavailable
	}
	root, path, source := a.ports.Current()
	if root == "" {
		return nil, errors.New("workspace is unavailable")
	}
	base, err := a.ports.NewWorkspace(root, func() (string, string) { return path, source })
	if err != nil {
		return nil, err
	}
	directory, _ := base.(applicationagent.WorkspaceDirectoryPort)
	return workspaceMutationAdapter{WorkspacePort: base, directory: directory, ports: a.ports}, nil
}

type workspaceMutationAdapter struct {
	applicationagent.WorkspacePort
	directory applicationagent.WorkspaceDirectoryPort
	ports     WorkspaceAdapterPorts
}

func (w workspaceMutationAdapter) CurrentDocumentDirty() bool {
	return w.ports.CurrentDocumentDirty != nil && w.ports.CurrentDocumentDirty()
}

func (w workspaceMutationAdapter) DestinationRevision(path string) (string, error) {
	if w.ports.DestinationRevision == nil {
		return "", applicationagent.ErrUnavailable
	}
	return w.ports.DestinationRevision(path)
}

func (w workspaceMutationAdapter) SaveDocument(path, content, expected string) error {
	if w.ports.SaveDocument == nil {
		return applicationagent.ErrUnavailable
	}
	return w.ports.SaveDocument(path, content, expected)
}

func (w workspaceMutationAdapter) ApplyCurrentDocument(content string) error {
	if w.ports.ApplyCurrentDocument == nil {
		return applicationagent.ErrUnavailable
	}
	return w.ports.ApplyCurrentDocument(content)
}

func (w workspaceMutationAdapter) MoveDocument(root, source, target string) (string, error) {
	if w.ports.MoveDocument == nil {
		return "", applicationagent.ErrUnavailable
	}
	return w.ports.MoveDocument(root, source, target)
}

func (w workspaceMutationAdapter) TrashDocuments(root string, paths []string) error {
	if w.ports.TrashDocuments == nil {
		return applicationagent.ErrUnavailable
	}
	return w.ports.TrashDocuments(root, paths)
}

func (w workspaceMutationAdapter) CreateFolder(path string) error {
	if w.ports.CreateFolder == nil {
		return applicationagent.ErrUnavailable
	}
	return w.ports.CreateFolder(w.Root(), path)
}

func (w workspaceMutationAdapter) Exists(path string) (bool, error) {
	if w.directory == nil {
		return false, applicationagent.ErrUnavailable
	}
	return w.directory.Exists(path)
}

var _ applicationagent.WorkspaceCapabilityProvider = (*WorkspaceAdapter)(nil)
var _ applicationagent.WorkspaceMutationPort = workspaceMutationAdapter{}
