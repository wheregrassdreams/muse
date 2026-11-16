package config

import (
	"errors"

	userconfig "muse/config"
)

var ErrRepositoryUnavailable = errors.New("configuration repository is not configured")

// Service applies application-level defaults and normalization around the
// settings persistence capability. The filesystem repository remains an
// outer adapter; presentation code only translates between these values and
// live UI state.
type Service struct {
	repository Repository
}

func NewService(repository Repository) Service {
	return Service{repository: repository}
}

func (s Service) LoadPreferences() (userconfig.Preferences, error) {
	if s.repository == nil {
		return userconfig.DefaultPreferences(), nil
	}
	preferences, err := s.repository.LoadPreferences()
	if err != nil {
		return userconfig.DefaultPreferences(), err
	}
	return userconfig.NormalizePreferences(preferences), nil
}

func (s Service) SavePreferences(preferences userconfig.Preferences) error {
	if s.repository == nil {
		return nil
	}
	return s.repository.SavePreferences(userconfig.NormalizePreferences(preferences))
}

func (s Service) LoadEditorSettings() (userconfig.EditorSettings, error) {
	if s.repository == nil {
		return userconfig.DefaultEditorSettings(), nil
	}
	settings, err := s.repository.LoadEditorSettings()
	if err != nil {
		return userconfig.DefaultEditorSettings(), err
	}
	return userconfig.NormalizeEditorSettings(settings), nil
}

func (s Service) SaveEditorSettings(settings userconfig.EditorSettings) error {
	if s.repository == nil {
		return nil
	}
	return s.repository.SaveEditorSettings(userconfig.NormalizeEditorSettings(settings))
}

func (s Service) LoadAssistantSettings() (userconfig.AssistantSettings, error) {
	if s.repository == nil {
		return userconfig.DefaultAssistantSettings(), nil
	}
	settings, err := s.repository.LoadAssistantSettings()
	if err != nil {
		return userconfig.DefaultAssistantSettings(), err
	}
	return settings, nil
}

func (s Service) SaveAssistantSettings(settings userconfig.AssistantSettings) error {
	if s.repository == nil {
		return ErrRepositoryUnavailable
	}
	return s.repository.SaveAssistantSettings(settings)
}

func (s Service) LoadDocumentPropertyPreferences(notePath string) (userconfig.DocumentPropertyPreferences, error) {
	if s.repository == nil {
		return userconfig.DocumentPropertyPreferences{}, nil
	}
	preferences, err := s.repository.LoadDocumentPropertyPreferences(notePath)
	if err != nil {
		return userconfig.DocumentPropertyPreferences{}, err
	}
	return preferences, nil
}

func (s Service) SaveDocumentPropertyPreferences(notePath string, preferences userconfig.DocumentPropertyPreferences) error {
	if s.repository == nil {
		return nil
	}
	return s.repository.SaveDocumentPropertyPreferences(notePath, preferences)
}
