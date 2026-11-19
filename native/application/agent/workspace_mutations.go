package agent

import (
	"errors"
	"io/fs"

	documentsave "muse/application/document"
	vaultfileapp "muse/application/vault/files"
	"muse/document"
	"muse/events"
)

type WorkspaceMutationKind uint8

const (
	WorkspaceDocumentMoved WorkspaceMutationKind = iota + 1
	WorkspaceDocumentsTrashed
	WorkspaceFolderCreated
)

// WorkspaceMutation is a completed product mutation. UI subscribers use it
// only to refresh projections and hosted document identity.
type WorkspaceMutation struct {
	Kind    WorkspaceMutationKind
	Moves   []vaultfileapp.FileMove
	Trashed []string
	Path    string
}

type WorkspaceMutationService struct {
	repository document.Repository
	saver      *documentsave.Saver
	files      func(root string) *vaultfileapp.FileUseCases
	events     *events.Dispatcher[WorkspaceMutation]
}

func NewWorkspaceMutationService(
	repository document.Repository,
	recovery document.RecoveryRepository,
	documentEvents *events.Dispatcher[document.DocumentSaved],
	files func(root string) *vaultfileapp.FileUseCases,
	events *events.Dispatcher[WorkspaceMutation],
) *WorkspaceMutationService {
	return &WorkspaceMutationService{
		repository: repository,
		saver:      documentsave.NewSaver(repository, recovery, documentEvents),
		files:      files, events: events,
	}
}

func (s *WorkspaceMutationService) DestinationRevision(path string) (string, error) {
	if s == nil || s.repository == nil {
		return "", errors.New("agent workspace: document repository is unavailable")
	}
	note, err := s.repository.Read(path)
	if errors.Is(err, fs.ErrNotExist) {
		return "", nil
	}
	return string(note.Revision), err
}

func (s *WorkspaceMutationService) SaveDocument(path, source, expected string) error {
	if s == nil || s.saver == nil {
		return errors.New("agent workspace: document save service is unavailable")
	}
	_, err := s.saver.Save("", path, source, document.Revision(expected))
	return err
}

func (s *WorkspaceMutationService) MoveDocument(root, source, target string) (string, error) {
	files, err := s.fileUseCases(root)
	if err != nil {
		return "", err
	}
	mutation, err := files.MoveFileTo(source, target)
	if err != nil {
		return "", err
	}
	if len(mutation.Moves) == 0 {
		return "", vaultfileapp.ErrFileOperationsUnavailable
	}
	s.publish(WorkspaceMutation{Kind: WorkspaceDocumentMoved, Moves: append([]vaultfileapp.FileMove(nil), mutation.Moves...)})
	return mutation.Moves[0].Target, nil
}

func (s *WorkspaceMutationService) TrashDocuments(root string, paths []string) error {
	files, err := s.fileUseCases(root)
	if err != nil {
		return err
	}
	mutation, err := files.TrashFiles(paths)
	if len(mutation.Trashed) > 0 {
		s.publish(WorkspaceMutation{Kind: WorkspaceDocumentsTrashed, Trashed: append([]string(nil), mutation.Trashed...)})
	}
	return err
}

func (s *WorkspaceMutationService) CreateFolder(root, path string) error {
	files, err := s.fileUseCases(root)
	if err != nil {
		return err
	}
	if _, err := files.CreateFolderAt(path); err != nil {
		return err
	}
	s.publish(WorkspaceMutation{Kind: WorkspaceFolderCreated, Path: path})
	return nil
}

func (s *WorkspaceMutationService) fileUseCases(root string) (*vaultfileapp.FileUseCases, error) {
	if s == nil || s.files == nil {
		return nil, vaultfileapp.ErrFileOperationsUnavailable
	}
	files := s.files(root)
	if files == nil {
		return nil, vaultfileapp.ErrFileOperationsUnavailable
	}
	return files, nil
}

func (s *WorkspaceMutationService) publish(mutation WorkspaceMutation) {
	if s != nil && s.events != nil {
		s.events.Publish(mutation)
	}
}
