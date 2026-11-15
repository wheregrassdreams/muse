// Package document contains user workflows that cross document persistence
// and recovery boundaries.
package document

import (
	"muse/document"
	"muse/events"
)

type Saver struct {
	repository document.Repository
	recovery   document.RecoveryRepository
	dispatcher *events.Dispatcher[document.DocumentSaved]
}

func NewSaver(repository document.Repository, recovery document.RecoveryRepository, dispatcher *events.Dispatcher[document.DocumentSaved]) *Saver {
	return &Saver{repository: repository, recovery: recovery, dispatcher: dispatcher}
}

// Save preserves a recovery copy before replacing the durable source. A failed
// revision check leaves the draft available and publishes no saved event.
func (s *Saver) Save(key, path, source string, expected document.Revision) (document.Note, error) {
	if key != "" {
		if err := s.recovery.WriteDraft(key, path, source); err != nil {
			return document.Note{}, err
		}
	}
	revision, err := s.repository.Save(path, source, expected)
	if err != nil {
		return document.Note{}, err
	}
	if key != "" {
		_ = s.recovery.DeleteDraft(key)
	}
	note := document.NewNote(path, source, revision)
	s.dispatcher.Publish(document.DocumentSaved{Path: note.Path, Revision: note.Revision})
	return note, nil
}
